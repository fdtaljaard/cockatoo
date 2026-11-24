package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Criteria says which messages an action is for. It is kept in the audit as it was given, and it
// is the one definition both the overview's rows and an action's match use, so the count the
// user sees on a row is the count the action works on.
type Criteria struct {
	Kind    string `json:"kind"`              // "sender" or "year"
	Address string `json:"address,omitempty"` // for "sender": the lower-case address
	Year    int    `json:"year,omitempty"`    // for "year": the year the mail arrived (UTC)
}

// where is the SQL condition for the criteria, on the message table.
func (c Criteria) where() (string, []any, error) {
	switch c.Kind {
	case "sender":
		if strings.TrimSpace(c.Address) == "" {
			return "", nil, errors.New("mail without a sender address cannot be acted on as one sender")
		}
		return `coalesce(from_addr, '') = ?`, []any{strings.ToLower(strings.TrimSpace(c.Address))}, nil
	case "year":
		if c.Year < 1970 || c.Year > 9999 {
			return "", nil, fmt.Errorf("%d is not a year", c.Year)
		}
		return `year(received_at) = ?`, []any{c.Year}, nil
	default:
		return "", nil, fmt.Errorf("unknown kind of criteria %q", c.Kind)
	}
}

// MatchMessages returns the messages in an account's copy that the criteria select, newest
// first.
func (s *Store) MatchMessages(ctx context.Context, accountID int64, c Criteria) ([]Message, error) {
	cond, args, err := c.where()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT uid_validity, uid, coalesce(message_id, ''), coalesce(from_name, ''), coalesce(from_addr, ''),
		        coalesce(subject, ''), received_at, seen
		 FROM message WHERE account_id = ? AND `+cond+` ORDER BY received_at DESC, uid DESC`,
		append([]any{accountID}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("match messages: %w", err)
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		m := Message{AccountID: accountID}
		var validity, uid int64
		if err := rows.Scan(&validity, &uid, &m.MessageID, &m.FromName, &m.FromAddr, &m.Subject, &m.ReceivedAt, &m.Seen); err != nil {
			return nil, fmt.Errorf("match messages: %w", err)
		}
		m.UIDValidity, m.UID = uint32(validity), uint32(uid)
		m.ReceivedAt = m.ReceivedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}

// ActionStart is what the audit records before an action changes anything.
type ActionStart struct {
	AccountID    int64
	AccountEmail string
	Kind         string // "trash", "spam" or "unsubscribe"
	Criteria     Criteria
	Description  string // in words, as the user confirmed it
	Source       string // "you" today; later "ai" for what an AI session proposed and the user confirmed
	Folder       string // where the messages were moved to
	Target       string // for "unsubscribe": the address opened
}

// StartAction writes the audit record of an action and the messages it is for, before anything
// is changed. It returns the action's id.
func (s *Store) StartAction(ctx context.Context, a ActionStart, items []Message) (int64, error) {
	criteria, err := json.Marshal(a.Criteria)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("record the action: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO action (started_at, account_id, account_email, kind, criteria, description, source, folder, target, matched)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		time.Now().UTC(), a.AccountID, a.AccountEmail, a.Kind, string(criteria), a.Description, a.Source,
		nullable(a.Folder), nullable(a.Target), len(items)).Scan(&id); err != nil {
		return 0, fmt.Errorf("record the action: %w", err)
	}
	for start := 0; start < len(items); start += addBatch {
		part := items[start:min(start+addBatch, len(items))]
		values := make([]string, len(part))
		args := make([]any, 0, len(part)*8)
		for i, m := range part {
			values[i] = "(?, ?, ?, ?, ?, ?, ?, ?)"
			args = append(args, id, int64(m.UIDValidity), int64(m.UID), m.MessageID, m.FromName, m.FromAddr, m.Subject, m.ReceivedAt.UTC())
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO action_item (action_id, uid_validity, uid, message_id, from_name, from_addr, subject, received_at)
			 VALUES `+strings.Join(values, ", "), args...); err != nil {
			return 0, fmt.Errorf("record the action's messages: %w", err)
		}
	}
	return id, tx.Commit()
}

// ActionResult is what the audit records after an action.
type ActionResult struct {
	Outcome string   // "done", "partial", "failed", "unknown" (could not confirm what moved) or "opened"
	Done    int      // messages moved
	NotDone []uint32 // UIDs that are still in the inbox
	Error   string
}

// FinishAction writes what an action did. Call it with a context that outlives the request, so
// a closed window does not leave an action without its result.
func (s *Store) FinishAction(ctx context.Context, id int64, r ActionResult) error {
	notDone, err := json.Marshal(append([]uint32{}, r.NotDone...))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO action_result (action_id, finished_at, outcome, done, not_done, error) VALUES (?, ?, ?, ?, ?, ?)`,
		id, time.Now().UTC(), r.Outcome, r.Done, string(notDone), nullable(r.Error)); err != nil {
		return fmt.Errorf("record the result of action %d: %w", id, err)
	}
	return nil
}

// AuditEntry is one action as the audit holds it.
type AuditEntry struct {
	ID           int64      `json:"id"`
	StartedAt    time.Time  `json:"startedAt"`
	AccountEmail string     `json:"accountEmail"`
	Kind         string     `json:"kind"`
	Criteria     Criteria   `json:"criteria"`
	Description  string     `json:"description"`
	Source       string     `json:"source"`
	Folder       string     `json:"folder,omitempty"`
	Target       string     `json:"target,omitempty"`
	Matched      int        `json:"matched"`
	Outcome      string     `json:"outcome"` // "running" when no result is recorded
	Done         int        `json:"done"`
	Error        string     `json:"error,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
}

// Audit returns the most recent actions, newest first.
func (s *Store) Audit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.started_at, a.account_email, a.kind, a.criteria, a.description, a.source,
		        coalesce(a.folder, ''), coalesce(a.target, ''), a.matched,
		        coalesce(r.outcome, 'running'), coalesce(r.done, 0), coalesce(r.error, ''), r.finished_at
		 FROM action a LEFT JOIN action_result r ON r.action_id = a.id
		 ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("read the audit: %w", err)
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var criteria string
		var finished *time.Time
		if err := rows.Scan(&e.ID, &e.StartedAt, &e.AccountEmail, &e.Kind, &criteria, &e.Description, &e.Source,
			&e.Folder, &e.Target, &e.Matched, &e.Outcome, &e.Done, &e.Error, &finished); err != nil {
			return nil, fmt.Errorf("read the audit: %w", err)
		}
		_ = json.Unmarshal([]byte(criteria), &e.Criteria)
		e.StartedAt = e.StartedAt.UTC()
		if finished != nil {
			t := finished.UTC()
			e.FinishedAt = &t
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
