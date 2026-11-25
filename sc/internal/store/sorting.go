package store

import (
	"context"
	"fmt"
	"time"
)

// The lists mail is sorted into, by what it asks of the user (SCOPE.md, section 8a). Every open
// message is in exactly one.
const (
	ListReply     = "reply"     // someone is waiting on an answer
	ListAttention = "attention" // read it and act on it; no answer needed
	ListSave      = "save"      // it carries something to keep
	ListUnsorted  = "unsorted"  // not sorted yet
	ListIgnore    = "ignore"    // nothing to do: it never needed the user, and stays out of the other lists
)

// SuggestDone is what the AI suggests for mail that needs nothing: mark it done.
const SuggestDone = "done"

// ValidList says whether a list is one mail can be put in.
func ValidList(l string) bool {
	return l == ListReply || l == ListAttention || l == ListSave || l == ListUnsorted || l == ListIgnore
}

// Who put a message in its list.
const (
	ByYou  = "you"  // the user, on its row
	ByAI   = "ai"   // the user, confirming the AI's suggestion
	ByRule = "rule" // one of the user's rules
)

// SortInfo is the list a message is in and how it got there; filled where it is shown.
type SortInfo struct {
	List   string `json:"list"`
	By     string `json:"by,omitempty"`     // ByYou, ByAI or ByRule; "" when it is unsorted
	RuleID int64  `json:"ruleId,omitempty"` // the rule, when By is ByRule
}

// Suggestion is the AI's idea of where a message belongs, kept so it is asked once.
type Suggestion struct {
	List   string    `json:"list"` // a list other than unsorted, or SuggestDone
	Reason string    `json:"reason"`
	Skill  string    `json:"skill,omitempty"` // the skill it followed, by name; "" for none
	At     time.Time `json:"at"`
}

// SortMark is the latest place the user put a message.
type SortMark struct {
	List string
	By   string
	At   time.Time
}

// AddSortMarks records that the user put these messages in a list. The record is insert-only:
// the latest mark for a message wins.
func (s *Store) AddSortMarks(ctx context.Context, keys []MessageKey, list, by string) error {
	if !ValidList(list) {
		return fmt.Errorf("%q is not a list", list)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sort mail: %w", err)
	}
	defer tx.Rollback()
	at := time.Now().UTC()
	for _, k := range keys {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO sort_mark (account_id, uid_validity, uid, list, placed_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			k.AccountID, int64(k.UIDValidity), int64(k.UID), list, by, at); err != nil {
			return fmt.Errorf("sort mail: %w", err)
		}
	}
	return tx.Commit()
}

// SortMarks returns the latest mark of every message the user has put in a list.
func (s *Store) SortMarks(ctx context.Context) (map[MessageKey]SortMark, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, uid_validity, uid, list, placed_by, created_at FROM sort_mark ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("read sort marks: %w", err)
	}
	defer rows.Close()
	out := map[MessageKey]SortMark{}
	for rows.Next() {
		var k MessageKey
		var validity, uid int64
		var m SortMark
		if err := rows.Scan(&k.AccountID, &validity, &uid, &m.List, &m.By, &m.At); err != nil {
			return nil, fmt.Errorf("read sort marks: %w", err)
		}
		k.UIDValidity, k.UID = uint32(validity), uint32(uid)
		m.At = m.At.UTC()
		out[k] = m
	}
	return out, rows.Err()
}

// AddSuggestions keeps the AI's suggestions from one request.
func (s *Store) AddSuggestions(ctx context.Context, requestID int64, sugg map[MessageKey]Suggestion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("keep suggestions: %w", err)
	}
	defer tx.Rollback()
	for k, g := range sugg {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO suggestion (account_id, uid_validity, uid, list, reason, skill, ai_request_id, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			k.AccountID, int64(k.UIDValidity), int64(k.UID), g.List, g.Reason, nullable(g.Skill), requestID, g.At.UTC()); err != nil {
			return fmt.Errorf("keep suggestions: %w", err)
		}
	}
	return tx.Commit()
}

// Suggestions returns the latest suggestion for every message the AI has looked at.
func (s *Store) Suggestions(ctx context.Context) (map[MessageKey]Suggestion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id, uid_validity, uid, list, reason, coalesce(skill, ''), created_at FROM suggestion ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("read suggestions: %w", err)
	}
	defer rows.Close()
	out := map[MessageKey]Suggestion{}
	for rows.Next() {
		var k MessageKey
		var validity, uid int64
		var g Suggestion
		if err := rows.Scan(&k.AccountID, &validity, &uid, &g.List, &g.Reason, &g.Skill, &g.At); err != nil {
			return nil, fmt.Errorf("read suggestions: %w", err)
		}
		k.UIDValidity, k.UID = uint32(validity), uint32(uid)
		g.At = g.At.UTC()
		out[k] = g
	}
	return out, rows.Err()
}

// ListRule returns the rule that puts a message in a list: the most specific rule that matches
// it, of any age (a rule sorts the mail that was there before it too), or nil.
func ListRule(rules []Routine) func(Message) *Routine {
	return func(m Message) *Routine {
		var best *Routine
		for i := range rules {
			r := &rules[i]
			if !ValidList(r.Action) || r.Action == ListUnsorted || !r.Matches(m) {
				continue
			}
			if best == nil || r.specific() > best.specific() || (r.specific() == best.specific() && r.ID > best.ID) {
				best = r
			}
		}
		return best
	}
}
