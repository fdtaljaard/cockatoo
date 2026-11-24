package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SentMessage is the header of one message in the Sent folder: who the user wrote to, from which
// of their addresses, and what it answered.
type SentMessage struct {
	UIDValidity uint32
	UID         uint32
	MessageID   string
	FromAddr    string
	To, Cc      []string
	InReplyTo   string
	SentAt      time.Time
}

// SentState is how far an account's copy of its Sent folder goes; found is false before the first.
func (s *Store) SentState(ctx context.Context, accountID int64) (validity uint32, complete, found bool, err error) {
	var v int64
	err = s.db.QueryRowContext(ctx, `SELECT uid_validity, complete FROM sent_sync WHERE account_id = ?`, accountID).Scan(&v, &complete)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("sent sync state of account %d: %w", accountID, err)
	}
	return uint32(v), complete, true, nil
}

// BeginSentSync records a listing of the Sent folder, as BeginInboxSync does for the inbox.
func (s *Store) BeginSentSync(ctx context.Context, accountID int64, folder string, uidValidity uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin the sent sync of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op
	if _, err := tx.ExecContext(ctx, `DELETE FROM sent_message WHERE account_id = ? AND uid_validity <> ?`, accountID, int64(uidValidity)); err != nil {
		return fmt.Errorf("begin the sent sync of account %d: %w", accountID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO sent_sync (account_id, folder, uid_validity, complete) VALUES (?, ?, ?, false)
		 ON CONFLICT (account_id) DO UPDATE SET folder = excluded.folder, uid_validity = excluded.uid_validity, complete = false`,
		accountID, folder, int64(uidValidity)); err != nil {
		return fmt.Errorf("begin the sent sync of account %d: %w", accountID, err)
	}
	return tx.Commit()
}

// StoredSentUIDs are the UIDs the Sent copy holds for an account.
func (s *Store) StoredSentUIDs(ctx context.Context, accountID int64) (map[uint32]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid FROM sent_message WHERE account_id = ?`, accountID)
	if err != nil {
		return nil, fmt.Errorf("sent copy of account %d: %w", accountID, err)
	}
	defer rows.Close()
	out := map[uint32]bool{}
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out[uint32(uid)] = true
	}
	return out, rows.Err()
}

// AddSent writes Sent headers into the copy.
func (s *Store) AddSent(ctx context.Context, accountID int64, msgs []SentMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save sent mail of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op
	for start := 0; start < len(msgs); start += addBatch {
		part := msgs[start:min(start+addBatch, len(msgs))]
		values := make([]string, len(part))
		args := make([]any, 0, len(part)*9)
		for i, m := range part {
			values[i] = "(?, ?, ?, ?, ?, ?, ?, ?, ?)"
			args = append(args, accountID, int64(m.UIDValidity), int64(m.UID), nullable(m.MessageID), strings.ToLower(m.FromAddr),
				joinAddrs(m.To), joinAddrs(m.Cc), nullable(m.InReplyTo), m.SentAt.UTC())
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sent_message (account_id, uid_validity, uid, message_id, from_addr, to_addrs, cc_addrs, in_reply_to, sent_at)
			 VALUES `+strings.Join(values, ", ")+` ON CONFLICT (account_id, uid_validity, uid) DO NOTHING`, args...); err != nil {
			return fmt.Errorf("save sent mail of account %d: %w", accountID, err)
		}
	}
	return tx.Commit()
}

// FinishSentSync removes what left the Sent folder and marks the copy complete. Only after a
// complete listing.
func (s *Store) FinishSentSync(ctx context.Context, accountID int64, listed map[uint32]bool) error {
	stored, err := s.StoredSentUIDs(ctx, accountID)
	if err != nil {
		return err
	}
	var gone []any
	for uid := range stored {
		if !listed[uid] {
			gone = append(gone, int64(uid))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("finish the sent sync of account %d: %w", accountID, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op
	for start := 0; start < len(gone); start += 500 {
		part := gone[start:min(start+500, len(gone))]
		marks := strings.TrimSuffix(strings.Repeat("?, ", len(part)), ", ")
		if _, err := tx.ExecContext(ctx, `DELETE FROM sent_message WHERE account_id = ? AND uid IN (`+marks+`)`,
			append([]any{accountID}, part...)...); err != nil {
			return fmt.Errorf("finish the sent sync of account %d: %w", accountID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sent_sync SET complete = true, synced_at = ? WHERE account_id = ?`, time.Now().UTC(), accountID); err != nil {
		return fmt.Errorf("finish the sent sync of account %d: %w", accountID, err)
	}
	return tx.Commit()
}

// SentFacts is what the Sent folder says about the user, for telling mail that needs them.
type SentFacts struct {
	Complete       bool            // the Sent copy is complete; without it, the facts below are partial
	MyAddresses    map[string]bool // the addresses the user sends from (aliases included)
	Correspondents map[string]int  // address -> how many messages the user sent to it
	SentIDs        map[string]bool // Message-IDs of the user's own mail
	Answered       map[string]bool // Message-IDs the user replied to
}

// SentFactsFor reads the facts of one account's Sent copy.
func (s *Store) SentFactsFor(ctx context.Context, accountID int64) (SentFacts, error) {
	f := SentFacts{MyAddresses: map[string]bool{}, Correspondents: map[string]int{}, SentIDs: map[string]bool{}, Answered: map[string]bool{}}
	_, complete, _, err := s.SentState(ctx, accountID)
	if err != nil {
		return f, err
	}
	f.Complete = complete
	rows, err := s.db.QueryContext(ctx,
		`SELECT coalesce(message_id, ''), coalesce(from_addr, ''), coalesce(to_addrs, ''), coalesce(cc_addrs, ''), coalesce(in_reply_to, '')
		 FROM sent_message WHERE account_id = ?`, accountID)
	if err != nil {
		return f, fmt.Errorf("sent facts of account %d: %w", accountID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, from, to, cc, reply string
		if err := rows.Scan(&id, &from, &to, &cc, &reply); err != nil {
			return f, err
		}
		if from != "" {
			f.MyAddresses[from] = true
		}
		for _, a := range append(splitAddrs(to), splitAddrs(cc)...) {
			f.Correspondents[a]++
		}
		if id != "" {
			f.SentIDs[id] = true
		}
		if reply != "" {
			f.Answered[reply] = true
		}
	}
	return f, rows.Err()
}
