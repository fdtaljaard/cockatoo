package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Routine is something the user does every time a certain mail comes in: "save the attachments of
// mail from acme.com to Expenses, in a folder for the month". Cockatoo points such mail out and
// tells the AI about it; it does not act on it by itself.
type Routine struct {
	ID        int64     `json:"id"`
	From      string    `json:"from"`              // a lower-case address ("billing@acme.com") or domain ("acme.com")
	Subject   string    `json:"subject,omitempty"` // lower-case text the subject must contain; "" for any
	Action    string    `json:"action"`            // the list it puts mail in: ListReply, ListAttention or ListSave
	FolderID  int64     `json:"folderId,omitempty"`
	Path      string    `json:"path"`            // inside the save folder ("/"-separated); with Dated, where the dated folder goes
	Dated     string    `json:"dated,omitempty"` // "month" or "day": a subfolder named for the mail's date; "" for none
	CreatedAt time.Time `json:"createdAt"`
}

// RoutineSave is the action of a routine that saves attachments, which is also the Save list: a
// routine is a rule for the Save list that can say where the attachments go.
const RoutineSave = ListSave

// RoutineMatch is how a routine applies to one message: where its attachments go.
type RoutineMatch struct {
	RoutineID  int64  `json:"routineId"`
	Action     string `json:"action"`
	FolderID   int64  `json:"folderId,omitempty"`
	FolderName string `json:"folderName,omitempty"`
	Parent     string `json:"parent"`         // where the dated folder goes, or the target itself
	Name       string `json:"name,omitempty"` // the dated folder for this message ("2026-11"); "" when not dated
	Missing    bool   `json:"missing,omitempty"`
}

// Target is the place inside the save folder the attachments go to.
func (m RoutineMatch) Target() string {
	switch {
	case m.Name == "":
		return m.Parent
	case m.Parent == "":
		return m.Name
	default:
		return m.Parent + "/" + m.Name
	}
}

// Describe says what the routine wants done, in words: "save its attachments to Expenses › 2026-11".
func (m RoutineMatch) Describe() string {
	where := m.FolderName
	if t := m.Target(); t != "" {
		where += " › " + strings.ReplaceAll(t, "/", " › ")
	}
	return "save its attachments to " + where
}

// Matches says whether a routine is for this message: the sender's address or domain, and the
// subject when the routine names one. The sender is what the From header says, which anyone can
// write, so a match is a hint to the user, never a reason to act alone.
func (r Routine) Matches(m Message) bool {
	from := strings.ToLower(strings.TrimSpace(m.FromAddr))
	if strings.Contains(r.From, "@") {
		if from != r.From {
			return false
		}
	} else {
		at := strings.LastIndex(from, "@")
		if at < 0 || from[at+1:] != r.From {
			return false
		}
	}
	return r.Subject == "" || strings.Contains(strings.ToLower(m.Subject), r.Subject)
}

// specific ranks routines: an address beats a domain, and a subject beats none.
func (r Routine) specific() int {
	n := 0
	if strings.Contains(r.From, "@") {
		n += 2
	}
	if r.Subject != "" {
		n++
	}
	return n
}

// Matcher returns what applies to a message: the most specific routine that matches it and was
// made before the message arrived (mail from before a routine existed is not pointed out), or
// nil. folders gives the save folders' names.
func Matcher(routines []Routine, folders map[int64]SaveFolder) func(Message) *RoutineMatch {
	return func(m Message) *RoutineMatch {
		var best *Routine
		for i := range routines {
			r := &routines[i]
			if r.Action != RoutineSave || r.FolderID == 0 || m.ReceivedAt.Before(r.CreatedAt) || !r.Matches(m) {
				continue
			}
			if best == nil || r.specific() > best.specific() || (r.specific() == best.specific() && r.ID > best.ID) {
				best = r
			}
		}
		if best == nil {
			return nil
		}
		f, ok := folders[best.FolderID]
		match := &RoutineMatch{RoutineID: best.ID, Action: best.Action, FolderID: best.FolderID, FolderName: f.Name,
			Parent: best.Path, Name: DatedName(best.Dated, m.ReceivedAt), Missing: !ok}
		return match
	}
}

// DatedName is the name of the folder for a date, in this computer's time zone: "2026-11" for a
// month, "2026-11-24" for a day, "" for neither.
func DatedName(dated string, t time.Time) string {
	switch dated {
	case "month":
		return t.Local().Format("2006-01")
	case "day":
		return t.Local().Format("2006-01-02")
	}
	return ""
}

// ListRoutines returns the routines, oldest first.
func (s *Store) ListRoutines(ctx context.Context) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, match_from, coalesce(match_subject, ''), action, coalesce(folder_id, 0), coalesce(path, ''),
		        coalesce(dated, ''), created_at
		 FROM routine ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list routines: %w", err)
	}
	defer rows.Close()
	out := []Routine{}
	for rows.Next() {
		var r Routine
		if err := rows.Scan(&r.ID, &r.From, &r.Subject, &r.Action, &r.FolderID, &r.Path, &r.Dated, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("list routines: %w", err)
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReplaceRules removes the rules for exactly this mail (address or domain, and subject), whatever
// list they put it in, so a new rule for it is the only one.
func (s *Store) ReplaceRules(ctx context.Context, from, subject string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM routine WHERE match_from = ? AND coalesce(match_subject, '') = ?`, from, subject); err != nil {
		return fmt.Errorf("replace rules: %w", err)
	}
	return nil
}

// ErrDuplicateRoutine means a routine for the same mail and action is already there.
var ErrDuplicateRoutine = errors.New("there is already a routine for this mail")

// AddRoutine saves a routine. Its fields must already be checked and in lower case.
func (s *Store) AddRoutine(ctx context.Context, r Routine) (Routine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM routine WHERE match_from = ? AND coalesce(match_subject, '') = ? AND action = ?`,
		r.From, r.Subject, r.Action).Scan(&n); err != nil {
		return Routine{}, fmt.Errorf("add routine: %w", err)
	}
	if n > 0 {
		return Routine{}, ErrDuplicateRoutine
	}
	r.CreatedAt = time.Now().UTC().Truncate(time.Second)
	if err := s.db.QueryRowContext(ctx,
		`INSERT INTO routine (match_from, match_subject, action, folder_id, path, dated, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		r.From, nullable(r.Subject), r.Action, nullableID(r.FolderID), nullable(r.Path), nullable(r.Dated), r.CreatedAt).Scan(&r.ID); err != nil {
		return Routine{}, fmt.Errorf("add routine: %w", err)
	}
	return r, nil
}

// DeleteRoutine removes a routine. Mail it pointed out is not touched.
func (s *Store) DeleteRoutine(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var gone int64
	err := s.db.QueryRowContext(ctx, `DELETE FROM routine WHERE id = ? RETURNING id`, id).Scan(&gone)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove routine %d: %w", id, err)
	}
	return nil
}

// nullableID stores 0 as NULL.
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
