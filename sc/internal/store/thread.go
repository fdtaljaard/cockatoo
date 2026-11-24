package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ThreadItem is one message of a conversation, as cockatoo's copies know it: in the inbox, or in
// the Sent folder (the user's own).
type ThreadItem struct {
	Folder      string // "inbox" or "sent"
	UIDValidity uint32
	UID         uint32
	MessageID   string
	InReplyTo   string
	FromName    string
	FromAddr    string
	Subject     string // empty for sent mail: its copy keeps no subject
	At          time.Time
	Seen        bool
}

// ThreadLimit is the most messages a conversation shows; the newest are kept.
const ThreadLimit = 30

// Thread finds the conversation a message belongs to, in one account: the messages linked to it
// by Message-ID and In-Reply-To, in either direction, in the inbox copy and the Sent copy. extra
// are more Message-IDs to start from (the References a message names). Oldest first; more is how
// many older ones were left out.
func (s *Store) Thread(ctx context.Context, accountID int64, start MessageKey, extra []string) (items []ThreadItem, more int, err error) {
	byID := map[string]ThreadItem{}
	var noID []ThreadItem
	seen := map[string]bool{}
	frontier := []string{}
	for _, id := range extra {
		if id = strings.Trim(id, "<> "); id != "" && !seen[id] {
			seen[id] = true
			frontier = append(frontier, id)
		}
	}

	first, err := s.threadQuery(ctx, accountID, `account_id = ? AND uid_validity = ? AND uid = ?`,
		[]any{accountID, int64(start.UIDValidity), int64(start.UID)}, true)
	if err != nil {
		return nil, 0, err
	}
	if len(first) == 0 {
		return nil, 0, ErrNotFound
	}
	add := func(it ThreadItem) {
		if it.MessageID == "" {
			noID = append(noID, it)
			return
		}
		if old, ok := byID[it.MessageID]; ok && old.Folder == "inbox" {
			return // the inbox copy wins over the Sent copy of the same message
		}
		byID[it.MessageID] = it
		for _, id := range []string{it.MessageID, it.InReplyTo} {
			if id != "" && !seen[id] {
				seen[id] = true
				frontier = append(frontier, id)
			}
		}
	}
	add(first[0])

	for round := 0; round < 8 && len(frontier) > 0 && len(byID) < 4*ThreadLimit; round++ {
		ids := frontier
		frontier = nil
		for startAt := 0; startAt < len(ids); startAt += addBatch {
			part := ids[startAt:min(startAt+addBatch, len(ids))]
			marks := strings.TrimSuffix(strings.Repeat("?, ", len(part)), ", ")
			args := []any{accountID}
			for _, id := range part {
				args = append(args, id)
			}
			args = append(args, args[1:]...)
			cond := `account_id = ? AND (message_id IN (` + marks + `) OR in_reply_to IN (` + marks + `))`
			for _, inbox := range []bool{true, false} {
				found, err := s.threadQuery(ctx, accountID, cond, args, inbox)
				if err != nil {
					return nil, 0, err
				}
				for _, it := range found {
					add(it)
				}
			}
		}
	}

	for _, it := range byID {
		items = append(items, it)
	}
	items = append(items, noID...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	if len(items) > ThreadLimit {
		more = len(items) - ThreadLimit
		items = items[more:]
	}
	return items, more, nil
}

func (s *Store) threadQuery(ctx context.Context, accountID int64, cond string, args []any, inbox bool) ([]ThreadItem, error) {
	q := `SELECT 'inbox', uid_validity, uid, coalesce(message_id, ''), coalesce(in_reply_to, ''), coalesce(from_name, ''),
	             coalesce(from_addr, ''), coalesce(subject, ''), received_at, seen
	      FROM message WHERE ` + cond
	if !inbox {
		q = `SELECT 'sent', uid_validity, uid, coalesce(message_id, ''), coalesce(in_reply_to, ''), '',
		            coalesce(from_addr, ''), '', sent_at, true
		     FROM sent_message WHERE ` + cond
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("read the conversation: %w", err)
	}
	defer rows.Close()
	var out []ThreadItem
	for rows.Next() {
		var it ThreadItem
		var validity, uid int64
		if err := rows.Scan(&it.Folder, &validity, &uid, &it.MessageID, &it.InReplyTo, &it.FromName, &it.FromAddr,
			&it.Subject, &it.At, &it.Seen); err != nil {
			return nil, fmt.Errorf("read the conversation: %w", err)
		}
		it.UIDValidity, it.UID = uint32(validity), uint32(uid)
		it.MessageID, it.InReplyTo = strings.Trim(it.MessageID, "<> "), strings.Trim(it.InReplyTo, "<> ")
		it.At = it.At.UTC()
		out = append(out, it)
	}
	return out, rows.Err()
}
