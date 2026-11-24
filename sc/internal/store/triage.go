package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Candidate is an unread message offered to the AI, with what cockatoo knows about its sender in
// the same mailbox: how much mail they sent, how much of it was read, and whether their mail
// carries an unsubscribe address (a sign of bulk mail). These are facts for the AI; nothing is
// left out because of them.
type Candidate struct {
	Message
	SenderMessages int
	SenderRead     int
	Bulk           bool // any of the sender's mail carries an unsubscribe address
	HasUnsubscribe bool // this message carries one
}

// TriageCandidates returns the unread messages of these accounts received at or after since,
// newest first, at most limit of them, and how many more there were.
func (s *Store) TriageCandidates(ctx context.Context, accountIDs []int64, since time.Time, limit int) ([]Candidate, int, error) {
	if len(accountIDs) == 0 {
		return []Candidate{}, 0, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?, ", len(accountIDs)), ", ")
	args := []any{}
	for _, id := range accountIDs {
		args = append(args, id)
	}
	where := `NOT m.seen AND m.received_at >= ? AND m.account_id IN (` + marks + `)`

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM message m WHERE `+where,
		append([]any{since.UTC()}, args...)...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("unread mail: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`WITH senders AS (
		   SELECT account_id, coalesce(from_addr, '') AS addr, count(*) AS n, count(*) FILTER (WHERE seen) AS r,
		          count(unsub_url) + count(unsub_mailto) > 0 AS bulk
		   FROM message GROUP BY account_id, addr)
		 SELECT m.account_id, m.uid_validity, m.uid, coalesce(m.message_id, ''), coalesce(m.from_name, ''),
		        coalesce(m.from_addr, ''), coalesce(m.subject, ''), m.received_at, m.seen, s.n, s.r, s.bulk,
		        coalesce(m.to_addrs, ''), coalesce(m.cc_addrs, ''), coalesce(m.in_reply_to, ''), coalesce(m.list_id, ''),
		        coalesce(m.precedence, ''), coalesce(m.auto_submitted, ''),
		        m.unsub_url IS NOT NULL OR m.unsub_mailto IS NOT NULL
		 FROM message m JOIN senders s ON s.account_id = m.account_id AND s.addr = coalesce(m.from_addr, '')
		 WHERE `+where+` ORDER BY m.received_at DESC, m.uid DESC LIMIT ?`,
		append(append([]any{since.UTC()}, args...), limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("unread mail: %w", err)
	}
	defer rows.Close()
	out := []Candidate{}
	for rows.Next() {
		var c Candidate
		var validity, uid int64
		var to, cc string
		if err := rows.Scan(&c.AccountID, &validity, &uid, &c.MessageID, &c.FromName, &c.FromAddr, &c.Subject,
			&c.ReceivedAt, &c.Seen, &c.SenderMessages, &c.SenderRead, &c.Bulk,
			&to, &cc, &c.InReplyTo, &c.ListID, &c.Precedence, &c.AutoSubmitted, &c.HasUnsubscribe); err != nil {
			return nil, 0, fmt.Errorf("unread mail: %w", err)
		}
		c.To, c.Cc = splitAddrs(to), splitAddrs(cc)
		c.UIDValidity, c.UID = uint32(validity), uint32(uid)
		c.ReceivedAt = c.ReceivedAt.UTC()
		out = append(out, c)
	}
	return out, total - len(out), rows.Err()
}

// MessageKey names one message in the copy.
type MessageKey struct {
	AccountID   int64  `json:"accountId"`
	UIDValidity uint32 `json:"uidValidity"`
	UID         uint32 `json:"uid"`
}

// MessagesByKey returns the messages still in the copy among these keys.
func (s *Store) MessagesByKey(ctx context.Context, keys []MessageKey) (map[MessageKey]Message, error) {
	out := map[MessageKey]Message{}
	for _, k := range keys {
		m := Message{AccountID: k.AccountID}
		var validity, uid int64
		err := s.db.QueryRowContext(ctx,
			`SELECT uid_validity, uid, coalesce(message_id, ''), coalesce(from_name, ''), coalesce(from_addr, ''),
			        coalesce(subject, ''), received_at, seen
			 FROM message WHERE account_id = ? AND uid_validity = ? AND uid = ?`,
			k.AccountID, int64(k.UIDValidity), int64(k.UID)).
			Scan(&validity, &uid, &m.MessageID, &m.FromName, &m.FromAddr, &m.Subject, &m.ReceivedAt, &m.Seen)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read message: %w", err)
		}
		m.UIDValidity, m.UID = uint32(validity), uint32(uid)
		m.ReceivedAt = m.ReceivedAt.UTC()
		out[k] = m
	}
	return out, nil
}

// AIRequest is one request sent to an AI provider, as the record keeps it.
type AIRequest struct {
	ID            int64     `json:"id"`
	CreatedAt     time.Time `json:"createdAt"`
	Kind          string    `json:"kind"`  // "triage"
	Scope         string    `json:"scope"` // "all", or an account id
	Model         string    `json:"model"`
	InputTokens   int64     `json:"inputTokens"`
	OutputTokens  int64     `json:"outputTokens"`
	Sent          int       `json:"sent"`    // messages sent
	LeftOut       int       `json:"leftOut"` // messages that matched but were over the limit
	Partial       bool      `json:"partial"` // the copy of a mailbox was not complete
	Answer        string    `json:"-"`       // the checked answer, as JSON; empty on error
	Error         string    `json:"error,omitempty"`
	TextSent      bool      `json:"textSent"`      // the start of each message's text was sent
	TextChars     int       `json:"textChars"`     // how much text, in characters (the text itself is not kept)
	LeftAutomated int       `json:"leftAutomated"` // unread mail left out as automated or bulk
	LeftAnswered  int       `json:"leftAnswered"`  // unread mail left out because the user already replied
}

// AIRequestItem is one message that was sent, under the reference the AI saw.
type AIRequestItem struct {
	Ref string
	MessageKey
	FromAddr string
	Subject  string
}

// RecordAIRequest writes a request and the messages it sent. It is never updated.
func (s *Store) RecordAIRequest(ctx context.Context, r AIRequest, items []AIRequestItem) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("record the AI request: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	var id int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO ai_request (created_at, kind, scope, model, input_tokens, output_tokens, sent, left_out, partial, answer, error,
		   text_sent, text_chars, left_automated, left_answered)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		r.CreatedAt.UTC(), r.Kind, r.Scope, nullable(r.Model), r.InputTokens, r.OutputTokens, r.Sent, r.LeftOut, r.Partial,
		nullable(r.Answer), nullable(r.Error), r.TextSent, r.TextChars, r.LeftAutomated, r.LeftAnswered).Scan(&id); err != nil {
		return 0, fmt.Errorf("record the AI request: %w", err)
	}
	for start := 0; start < len(items); start += addBatch {
		part := items[start:min(start+addBatch, len(items))]
		values := make([]string, len(part))
		args := make([]any, 0, len(part)*7)
		for i, it := range part {
			values[i] = "(?, ?, ?, ?, ?, ?, ?)"
			args = append(args, id, it.Ref, it.AccountID, int64(it.UIDValidity), int64(it.UID), it.FromAddr, it.Subject)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO ai_request_item (request_id, ref, account_id, uid_validity, uid, from_addr, subject) VALUES `+
				strings.Join(values, ", "), args...); err != nil {
			return 0, fmt.Errorf("record the AI request's messages: %w", err)
		}
	}
	return id, tx.Commit()
}

// LatestAIRequest returns the most recent request of a kind and scope that has an answer; found
// is false when there is none.
func (s *Store) LatestAIRequest(ctx context.Context, kind, scope string) (r AIRequest, found bool, err error) {
	var model, answer sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT id, created_at, kind, scope, model, input_tokens, output_tokens, sent, left_out, partial, answer,
		        coalesce(text_sent, false), coalesce(text_chars, 0), coalesce(left_automated, 0), coalesce(left_answered, 0)
		 FROM ai_request WHERE kind = ? AND scope = ? AND answer IS NOT NULL ORDER BY id DESC LIMIT 1`, kind, scope).
		Scan(&r.ID, &r.CreatedAt, &r.Kind, &r.Scope, &model, &r.InputTokens, &r.OutputTokens, &r.Sent, &r.LeftOut, &r.Partial, &answer,
			&r.TextSent, &r.TextChars, &r.LeftAutomated, &r.LeftAnswered)
	if errors.Is(err, sql.ErrNoRows) {
		return AIRequest{}, false, nil
	}
	if err != nil {
		return AIRequest{}, false, fmt.Errorf("read the last AI request: %w", err)
	}
	r.CreatedAt = r.CreatedAt.UTC()
	r.Model, r.Answer = model.String, answer.String
	return r, true, nil
}
