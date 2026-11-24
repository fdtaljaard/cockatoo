package account

import (
	"context"
	"net/mail"
	"sort"
	"strings"
	"time"

	"cockatoo/internal/files"
	"cockatoo/internal/store"
)

// WaitingWindow is how far back the Inbox looks for mail a routine is waiting on: far enough for
// a monthly invoice that came in while the user was away.
const WaitingWindow = 60 * 24 * time.Hour

// RoutineInput is a routine as the user sets it up.
type RoutineInput struct {
	From     string `json:"from"`    // an address or a domain
	Subject  string `json:"subject"` // optional: text the subject contains
	FolderID int64  `json:"folderId"`
	Path     string `json:"path"`  // inside the save folder; with Dated, where the dated folder goes
	Dated    string `json:"dated"` // "", "month" or "day"
}

// AddRoutine checks and saves a routine that saves attachments.
func (s *Service) AddRoutine(ctx context.Context, in RoutineInput) (store.Routine, error) {
	from := strings.ToLower(strings.TrimSpace(in.From))
	from = strings.TrimPrefix(from, "@")
	switch {
	case from == "":
		return store.Routine{}, &InvalidError{"Say which mail: an address like billing@acme.com, or a domain like acme.com."}
	case strings.Contains(from, "@"):
		if a, err := mail.ParseAddress(from); err != nil || !strings.EqualFold(a.Address, from) {
			return store.Routine{}, &InvalidError{from + " does not look like an email address."}
		}
	case !strings.Contains(from, ".") || strings.ContainsAny(from, " <>,;/\\"):
		return store.Routine{}, &InvalidError{from + " does not look like a domain, such as acme.com."}
	}
	subject := strings.ToLower(strings.Join(strings.Fields(in.Subject), " "))
	if len([]rune(subject)) > 200 {
		return store.Routine{}, &InvalidError{"Keep the subject text short: a word or two that every one of these subjects has."}
	}
	if in.Dated != "" && in.Dated != "month" && in.Dated != "day" {
		return store.Routine{}, &InvalidError{"A dated folder is by month or by day."}
	}
	f, err := s.st.GetSaveFolder(ctx, in.FolderID)
	if err != nil {
		return store.Routine{}, err
	}
	if _, err := files.Resolve(f.Path, in.Path); err != nil {
		return store.Routine{}, err
	}
	return s.st.AddRoutine(ctx, store.Routine{From: from, Subject: subject, Action: store.RoutineSave,
		FolderID: f.ID, Path: strings.Trim(strings.ReplaceAll(in.Path, `\`, "/"), "/"), Dated: in.Dated})
}

// RoutineStatus is a routine as the Routines screen shows it.
type RoutineStatus struct {
	store.Routine
	FolderName string `json:"folderName,omitempty"`
	Missing    bool   `json:"missing"` // its save folder was removed or is no longer on disk
}

// ListRoutines returns the routines, each with its save folder's name.
func (s *Service) ListRoutines(ctx context.Context) ([]RoutineStatus, error) {
	routines, err := s.st.ListRoutines(ctx)
	if err != nil {
		return nil, err
	}
	folders, err := s.saveFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RoutineStatus, len(routines))
	for i, r := range routines {
		f, ok := folders[r.FolderID]
		out[i] = RoutineStatus{Routine: r, FolderName: f.Name, Missing: !ok || f.Missing}
	}
	return out, nil
}

// RemoveRoutine forgets a routine.
func (s *Service) RemoveRoutine(ctx context.Context, id int64) error {
	return s.st.DeleteRoutine(ctx, id)
}

// saveFolders returns the save folders by id, each marked when it is no longer on disk.
func (s *Service) saveFolders(ctx context.Context) (map[int64]SaveFolderStatus, error) {
	list, err := s.ListSaveFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]SaveFolderStatus, len(list))
	for _, f := range list {
		out[f.ID] = f
	}
	return out, nil
}

// routineMatcher returns what applies to a message, with a save folder that is gone marked as
// missing; nil when there are no routines.
func (s *Service) routineMatcher(ctx context.Context) (func(store.Message) *store.RoutineMatch, error) {
	routines, err := s.st.ListRoutines(ctx)
	if err != nil || len(routines) == 0 {
		return nil, err
	}
	folders, err := s.saveFolders(ctx)
	if err != nil {
		return nil, err
	}
	plain := make(map[int64]store.SaveFolder, len(folders))
	for id, f := range folders {
		plain[id] = f.SaveFolder
	}
	match := store.Matcher(routines, plain)
	return func(m store.Message) *store.RoutineMatch {
		r := match(m)
		if r != nil {
			if f, ok := folders[r.FolderID]; !ok || f.Missing {
				r.Missing = true
			}
		}
		return r
	}, nil
}

// attachRoutines marks the messages a routine applies to and that are not dealt with yet, and
// returns the mail of the last WaitingWindow that a routine is still waiting on, newest first.
func (s *Service) attachRoutines(ctx context.Context, shown []store.Message) ([]store.Message, error) {
	match, err := s.routineMatcher(ctx)
	if err != nil || match == nil {
		return []store.Message{}, err
	}
	for i := range shown {
		if shown[i].Handled == nil {
			shown[i].Routine = match(shown[i])
		}
	}
	recent, err := s.st.ListMessages(ctx, time.Now().UTC().Add(-WaitingWindow))
	if err != nil {
		return nil, err
	}
	var waiting []store.Message
	for _, m := range recent {
		if r := match(m); r != nil {
			m.Routine = r
			waiting = append(waiting, m)
		}
	}
	if err := s.AttachHandled(ctx, waiting); err != nil {
		return nil, err
	}
	out := []store.Message{}
	for _, m := range waiting {
		if m.Handled == nil {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ReceivedAt.After(out[j].ReceivedAt) })
	return out, nil
}

// markRoutines marks the messages a routine applies to and that are not dealt with yet.
func (s *Service) markRoutines(ctx context.Context, msgs []store.Message) error {
	match, err := s.routineMatcher(ctx)
	if err != nil || match == nil {
		return err
	}
	for i := range msgs {
		if msgs[i].Handled == nil {
			msgs[i].Routine = match(msgs[i])
		}
	}
	return nil
}

// RoutineMatcher is for the AI triage: what routine applies to a message, or nil. It is nil when
// there are no routines.
func (s *Service) RoutineMatcher(ctx context.Context) (func(store.Message) *store.RoutineMatch, error) {
	return s.routineMatcher(ctx)
}
