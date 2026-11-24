package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Handled is what the user has done about a message. Nothing here is a separate state to keep in
// step: it is read from the audit (saves, and marks of done) and from the copy of the Sent folder
// (replies), so the audit stays the one record of what the user did.
type Handled struct {
	SavedTo    []string   `json:"savedTo,omitempty"`    // the folders its attachments were saved in
	SavedFiles int        `json:"savedFiles,omitempty"` // how many files were saved from it
	SavedAt    *time.Time `json:"savedAt,omitempty"`    // the latest save
	DoneAt     *time.Time `json:"doneAt,omitempty"`     // marked done by the user, and not undone since
	Replied    bool       `json:"replied,omitempty"`    // the Sent folder holds a reply to it
}

// Any says whether the user has dealt with the message in one of the ways cockatoo knows.
func (h Handled) Any() bool { return h.SavedFiles > 0 || h.DoneAt != nil || h.Replied }

// The kinds of action that mark a message done and take the mark away. They change nothing in
// the mailbox; they are recorded like every other action.
const (
	KindDone    = "done"
	KindNotDone = "not_done"
	KindSave    = "save_attachments"
)

// HandledFor returns what the user has done about each of these messages; messages they have not
// dealt with are not in the map.
func (s *Store) HandledFor(ctx context.Context, msgs []Message) (map[MessageKey]Handled, error) {
	out := map[MessageKey]Handled{}
	if len(msgs) == 0 {
		return out, nil
	}
	want := make(map[MessageKey]bool, len(msgs))
	accounts := map[int64]bool{}
	for _, m := range msgs {
		want[MessageKey{AccountID: m.AccountID, UIDValidity: m.UIDValidity, UID: m.UID}] = true
		accounts[m.AccountID] = true
	}
	ids := make([]string, 0, len(accounts))
	args := make([]any, 0, len(accounts))
	for id := range accounts {
		ids = append(ids, "?")
		args = append(args, id)
	}
	inAccounts := "a.account_id IN (" + strings.Join(ids, ", ") + ")"

	// Saved: every file a save wrote, by message.
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.account_id, f.uid_validity, f.uid, f.path, a.started_at
		 FROM action a JOIN action_file f ON f.action_id = a.id
		 WHERE a.kind = '`+KindSave+`' AND `+inAccounts+` ORDER BY a.started_at, a.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("read saved attachments: %w", err)
	}
	for rows.Next() {
		var k MessageKey
		var validity, uid int64
		var path string
		var at time.Time
		if err := rows.Scan(&k.AccountID, &validity, &uid, &path, &at); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read saved attachments: %w", err)
		}
		k.UIDValidity, k.UID = uint32(validity), uint32(uid)
		if !want[k] {
			continue
		}
		h := out[k]
		h.SavedFiles++
		at = at.UTC()
		h.SavedAt = &at
		if dir := filepath.Dir(path); !contains(h.SavedTo, dir) {
			h.SavedTo = append(h.SavedTo, dir)
		}
		out[k] = h
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Done: the latest mark wins, so "not done" takes back an earlier "done".
	rows, err = s.db.QueryContext(ctx,
		`SELECT a.account_id, i.uid_validity, i.uid, a.kind, a.started_at
		 FROM action a JOIN action_item i ON i.action_id = a.id
		 WHERE a.kind IN ('`+KindDone+`', '`+KindNotDone+`') AND `+inAccounts+` ORDER BY a.started_at, a.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("read done marks: %w", err)
	}
	for rows.Next() {
		var k MessageKey
		var validity, uid int64
		var kind string
		var at time.Time
		if err := rows.Scan(&k.AccountID, &validity, &uid, &kind, &at); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read done marks: %w", err)
		}
		k.UIDValidity, k.UID = uint32(validity), uint32(uid)
		if !want[k] {
			continue
		}
		h := out[k]
		if kind == KindDone {
			at = at.UTC()
			h.DoneAt = &at
		} else {
			h.DoneAt = nil
		}
		out[k] = h
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Replied: the Sent copy holds a message whose In-Reply-To is this message's Message-ID.
	byID := map[int64]map[string][]MessageKey{}
	for _, m := range msgs {
		if m.MessageID == "" {
			continue
		}
		if byID[m.AccountID] == nil {
			byID[m.AccountID] = map[string][]MessageKey{}
		}
		k := MessageKey{AccountID: m.AccountID, UIDValidity: m.UIDValidity, UID: m.UID}
		byID[m.AccountID][m.MessageID] = append(byID[m.AccountID][m.MessageID], k)
	}
	for account, ids := range byID {
		list := make([]string, 0, len(ids))
		for id := range ids {
			list = append(list, id)
		}
		for start := 0; start < len(list); start += addBatch {
			part := list[start:min(start+addBatch, len(list))]
			marks := make([]string, len(part))
			qargs := make([]any, 0, len(part)+1)
			qargs = append(qargs, account)
			for i, id := range part {
				marks[i] = "?"
				qargs = append(qargs, id)
			}
			rows, err := s.db.QueryContext(ctx,
				`SELECT DISTINCT in_reply_to FROM sent_message WHERE account_id = ? AND in_reply_to IN (`+strings.Join(marks, ", ")+`)`,
				qargs...)
			if err != nil {
				return nil, fmt.Errorf("read replies: %w", err)
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return nil, fmt.Errorf("read replies: %w", err)
				}
				for _, k := range ids[id] {
					h := out[k]
					h.Replied = true
					out[k] = h
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
