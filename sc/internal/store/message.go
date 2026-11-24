package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Message is the header of one inbox message, as kept in the cache.
type Message struct {
	AccountID   int64         `json:"accountId"`
	UIDValidity uint32        `json:"uidValidity"`
	UID         uint32        `json:"uid"`
	MessageID   string        `json:"messageId"`
	FromName    string        `json:"fromName"`
	FromAddr    string        `json:"fromAddr"`
	Subject     string        `json:"subject"`
	ReceivedAt  time.Time     `json:"receivedAt"` // UTC
	Seen        bool          `json:"seen"`
	Handled     *Handled      `json:"handled,omitempty"` // what the user did about it; filled where it is shown
	Routine     *RoutineMatch `json:"routine,omitempty"` // the routine for it, while it is not dealt with; filled where it is shown
	Unsubscribe Unsubscribe   `json:"-"`
	// What tells mail that needs a reply from mail to skim; kept, not shown as they are.
	To            []string `json:"-"`
	Cc            []string `json:"-"`
	InReplyTo     string   `json:"-"`
	ListID        string   `json:"-"`
	Precedence    string   `json:"-"`
	AutoSubmitted string   `json:"-"`
}

// Unsubscribe is how a sender's mail says it can be unsubscribed from. URL is https only and
// Mailto mailto only; both come from the sender and are shown, never fetched by cockatoo.
type Unsubscribe struct {
	URL      string `json:"url,omitempty"`
	Mailto   string `json:"mailto,omitempty"`
	OneClick bool   `json:"oneClick"`
}

// InboxSync is how far an account's copy of its inbox goes.
type InboxSync struct {
	UIDValidity uint32     `json:"-"`
	ServerTotal int        `json:"serverTotal"` // messages in the inbox at the last listing
	Complete    bool       `json:"complete"`    // every listed message has been read into the copy
	SyncedAt    *time.Time `json:"syncedAt"`    // when the last complete sync finished
}

// GetInboxSync returns an account's sync state; found is false before its first sync.
func (s *Store) GetInboxSync(ctx context.Context, accountID int64) (st InboxSync, found bool, err error) {
	var validity int64
	var synced sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT uid_validity, server_total, complete, synced_at FROM inbox_sync WHERE account_id = ?`, accountID).
		Scan(&validity, &st.ServerTotal, &st.Complete, &synced)
	if errors.Is(err, sql.ErrNoRows) {
		return InboxSync{}, false, nil
	}
	if err != nil {
		return InboxSync{}, false, fmt.Errorf("read the sync state of account %d: %w", accountID, err)
	}
	st.UIDValidity = uint32(validity)
	if synced.Valid {
		t := synced.Time.UTC()
		st.SyncedAt = &t
	}
	return st, true, nil
}

// BeginInboxSync records a new listing of an account's inbox and marks its copy incomplete until
// FinishInboxSync. When the server's UID validity has changed, every UID the copy holds means
// something else now, so the account's messages are removed and read again.
func (s *Store) BeginInboxSync(ctx context.Context, accountID int64, uidValidity uint32, serverTotal int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the sync of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM message WHERE account_id = ? AND uid_validity <> ?`, accountID, int64(uidValidity)); err != nil {
		return fmt.Errorf("begin the sync of account %d: %w", accountID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO inbox_sync (account_id, uid_validity, server_total, complete, started_at) VALUES (?, ?, ?, false, ?)
		 ON CONFLICT (account_id) DO UPDATE SET uid_validity = excluded.uid_validity,
		   server_total = excluded.server_total, complete = false, started_at = excluded.started_at`,
		accountID, int64(uidValidity), serverTotal, time.Now().UTC()); err != nil {
		return fmt.Errorf("begin the sync of account %d: %w", accountID, err)
	}
	return tx.Commit()
}

// StoredSeen returns the UIDs the copy holds for an account, with their read state.
func (s *Store) StoredSeen(ctx context.Context, accountID int64) (map[uint32]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid, seen FROM message WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, fmt.Errorf("read the stored messages of account %d: %w", accountID, err)
	}
	defer rows.Close()
	out := map[uint32]bool{}
	for rows.Next() {
		var uid int64
		var seen bool
		if err := rows.Scan(&uid, &seen); err != nil {
			return nil, fmt.Errorf("read the stored messages of account %d: %w", accountID, err)
		}
		out[uint32(uid)] = seen
	}
	return out, rows.Err()
}

// addBatch is how many rows one INSERT carries: enough to be quick, few enough parameters.
const addBatch = 200

// AddMessages writes headers into the copy, replacing a row with the same UID. Each call commits
// on its own, so an interrupted sync keeps what it has read.
func (s *Store) AddMessages(ctx context.Context, accountID int64, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save messages of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	for start := 0; start < len(msgs); start += addBatch {
		part := msgs[start:min(start+addBatch, len(msgs))]
		values := make([]string, len(part))
		args := make([]any, 0, len(part)*10)
		for i, m := range part {
			values[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
			args = append(args, accountID, int64(m.UIDValidity), int64(m.UID), m.MessageID, m.FromName,
				strings.ToLower(m.FromAddr), joinAddrs(m.To), joinAddrs(m.Cc), nullable(m.InReplyTo), m.Subject,
				m.ReceivedAt.UTC(), m.Seen, nullable(m.Unsubscribe.URL), nullable(m.Unsubscribe.Mailto), m.Unsubscribe.OneClick,
				nullable(m.ListID), nullable(m.Precedence), nullable(m.AutoSubmitted), now)
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO message (account_id, uid_validity, uid, message_id, from_name, from_addr, to_addrs, cc_addrs,
			   in_reply_to, subject, received_at, seen, unsub_url, unsub_mailto, unsub_one_click, list_id, precedence,
			   auto_submitted, refreshed_at)
			 VALUES `+strings.Join(values, ", ")+`
			 ON CONFLICT (account_id, uid_validity, uid) DO UPDATE SET
			   message_id = excluded.message_id, from_name = excluded.from_name, from_addr = excluded.from_addr,
			   to_addrs = excluded.to_addrs, cc_addrs = excluded.cc_addrs, in_reply_to = excluded.in_reply_to,
			   subject = excluded.subject, received_at = excluded.received_at, seen = excluded.seen,
			   unsub_url = excluded.unsub_url, unsub_mailto = excluded.unsub_mailto,
			   unsub_one_click = excluded.unsub_one_click, list_id = excluded.list_id, precedence = excluded.precedence,
			   auto_submitted = excluded.auto_submitted, refreshed_at = excluded.refreshed_at`, args...)
		if err != nil {
			return fmt.Errorf("save messages of account %d: %w", accountID, err)
		}
	}
	return tx.Commit()
}

// FinishInboxSync brings the copy in line with a complete listing of the inbox: read states that
// changed are updated, messages no longer in the inbox (deleted, archived, moved) are removed,
// and the copy is marked complete. Call it only after every listed message has been read; a
// partial listing must never remove anything.
func (s *Store) FinishInboxSync(ctx context.Context, accountID int64, listed map[uint32]bool) error {
	stored, err := s.StoredSeen(ctx, accountID)
	if err != nil {
		return err
	}
	var gone []any
	var nowSeen, nowUnseen []any
	for uid, seen := range stored {
		serverSeen, there := listed[uid]
		switch {
		case !there:
			gone = append(gone, int64(uid))
		case serverSeen && !seen:
			nowSeen = append(nowSeen, int64(uid))
		case !serverSeen && seen:
			nowUnseen = append(nowUnseen, int64(uid))
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("finish the sync of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	for _, step := range []struct {
		sql  string
		uids []any
	}{
		{`DELETE FROM message WHERE account_id = ? AND uid IN (%s)`, gone},
		{`UPDATE message SET seen = true WHERE account_id = ? AND uid IN (%s)`, nowSeen},
		{`UPDATE message SET seen = false WHERE account_id = ? AND uid IN (%s)`, nowUnseen},
	} {
		for start := 0; start < len(step.uids); start += 500 {
			part := step.uids[start:min(start+500, len(step.uids))]
			marks := strings.TrimSuffix(strings.Repeat("?, ", len(part)), ", ")
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(step.sql, marks), append([]any{accountID}, part...)...); err != nil {
				return fmt.Errorf("finish the sync of account %d: %w", accountID, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE inbox_sync SET complete = true, synced_at = ? WHERE account_id = ?`, time.Now().UTC(), accountID); err != nil {
		return fmt.Errorf("finish the sync of account %d: %w", accountID, err)
	}
	return tx.Commit()
}

// ListMessages returns the cached messages of every account received at or after since, newest
// first.
func (s *Store) ListMessages(ctx context.Context, since time.Time) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, uid_validity, uid, coalesce(message_id, ''), coalesce(from_name, ''), coalesce(from_addr, ''),
		        coalesce(subject, ''), received_at, seen
		 FROM message WHERE received_at >= ? ORDER BY received_at DESC, account_id, uid DESC`, since.UTC())
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		var validity, uid int64
		if err := rows.Scan(&m.AccountID, &validity, &uid, &m.MessageID, &m.FromName, &m.FromAddr,
			&m.Subject, &m.ReceivedAt, &m.Seen); err != nil {
			return nil, fmt.Errorf("list messages: %w", err)
		}
		m.UIDValidity, m.UID = uint32(validity), uint32(uid)
		m.ReceivedAt = m.ReceivedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// nullable stores an empty string as NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RemoveMessages takes messages out of the copy: the ones an action moved out of the inbox.
func (s *Store) RemoveMessages(ctx context.Context, accountID int64, uidValidity uint32, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for start := 0; start < len(uids); start += 500 {
		part := uids[start:min(start+500, len(uids))]
		args := []any{accountID, int64(uidValidity)}
		for _, u := range part {
			args = append(args, int64(u))
		}
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(part)), ", ")
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM message WHERE account_id = ? AND uid_validity = ? AND uid IN (`+marks+`)`, args...); err != nil {
			return fmt.Errorf("remove messages of account %d: %w", accountID, err)
		}
	}
	return nil
}

// joinAddrs stores a list of addresses as one lower-case, comma-separated value.
func joinAddrs(addrs []string) any {
	if len(addrs) == 0 {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" && !strings.Contains(a, ",") {
			out = append(out, a)
		}
	}
	return strings.Join(out, ",")
}

// splitAddrs reads a value stored by joinAddrs.
func splitAddrs(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
