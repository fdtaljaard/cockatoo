package account

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// ErrBusy means a sync or another action is running; actions and syncs take turns so the copy
// and the mailbox never disagree about what an action moved.
var ErrBusy = errors.New("cockatoo is checking your mailbox. Try again when it has finished")

// ConflictError means what the user confirmed no longer matches the mailbox. Nothing was changed.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }

// mover is what an action needs from a mail server. *mail.Mover is the real one.
type mover interface {
	UIDValidity() uint32
	Folder(imap.Destination) (string, error)
	Move(uids []uint32, folder string) error
	Present(uids []uint32) (map[uint32]bool, error)
	Close()
}

func openMover(ctx context.Context, srv imap.Server, login imap.Login) (mover, error) {
	return imap.OpenMover(ctx, srv, login)
}

// moveBatch is how many messages one MOVE command carries.
const moveBatch = 500

// actionTimeout bounds one action. It runs on its own context, not the request's: closing the
// window must not stop an action halfway without its result in the audit.
const actionTimeout = 15 * time.Minute

// ActionRequest is an action the user confirmed: what to do, to which messages, and how many
// messages they were shown.
type ActionRequest struct {
	Kind     string         `json:"kind"` // "trash" or "spam"
	Criteria store.Criteria `json:"criteria"`
	Expected int            `json:"expected"`
}

// Preview is what an action would do, for the user to confirm.
type Preview struct {
	Messages int       `json:"messages"`
	Unread   int       `json:"unread"`
	Oldest   time.Time `json:"oldest"`
	Latest   time.Time `json:"latest"`
	Subjects []string  `json:"subjects"` // the three most recent, to recognise the mail by
}

// ActionOutcome is what an action did.
type ActionOutcome struct {
	ActionID    int64  `json:"actionId"`
	Description string `json:"description"`
	Outcome     string `json:"outcome"` // "done", "partial", "failed" or "unknown" (could not confirm what moved)
	Moved       int    `json:"moved"`
	NotMoved    int    `json:"notMoved"`
	Error       string `json:"error,omitempty"`
}

var destinations = map[string]imap.Destination{"trash": imap.Trash, "spam": imap.Spam}

func checkRequest(kind string, c store.Criteria) error {
	if _, ok := destinations[kind]; !ok {
		return &InvalidError{fmt.Sprintf("%q is not an action cockatoo knows", kind)}
	}
	if kind == "spam" && c.Kind != "sender" {
		return &InvalidError{"Only a sender's mail can be reported as spam."}
	}
	return nil
}

// PreviewAction says what an action on these messages would touch. Nothing is changed.
func (s *Service) PreviewAction(ctx context.Context, id int64, c store.Criteria) (Preview, error) {
	if _, _, err := s.st.AccountSecret(ctx, id); err != nil {
		return Preview{}, err
	}
	msgs, err := s.st.MatchMessages(ctx, id, c)
	if err != nil {
		return Preview{}, &InvalidError{err.Error()}
	}
	p := Preview{Messages: len(msgs), Subjects: []string{}}
	for i, m := range msgs {
		if !m.Seen {
			p.Unread++
		}
		if i == 0 || m.ReceivedAt.After(p.Latest) {
			p.Latest = m.ReceivedAt
		}
		if i == 0 || m.ReceivedAt.Before(p.Oldest) {
			p.Oldest = m.ReceivedAt
		}
		if len(p.Subjects) < 3 {
			p.Subjects = append(p.Subjects, m.Subject)
		}
	}
	return p, nil
}

// ApplyAction moves the messages the user confirmed to Trash or Spam, and audits it: the record
// of what and why is written before anything moves, and what happened after. Only messages the
// server confirms have left the inbox are taken out of the copy.
func (s *Service) ApplyAction(ctx context.Context, id int64, req ActionRequest) (ActionOutcome, error) {
	if err := checkRequest(req.Kind, req.Criteria); err != nil {
		return ActionOutcome{}, err
	}
	// The audit keeps the criteria as they were matched.
	req.Criteria.Address = strings.ToLower(strings.TrimSpace(req.Criteria.Address))
	if err := s.beginAction(); err != nil {
		return ActionOutcome{}, err
	}
	defer s.endAction()

	// From here the action runs to its end even if the request is cancelled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), actionTimeout)
	defer cancel()

	a, sealed, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return ActionOutcome{}, err
	}
	state, found, err := s.st.GetInboxSync(ctx, id)
	if err != nil {
		return ActionOutcome{}, err
	}
	if !found || !state.Complete {
		return ActionOutcome{}, &ConflictError{"Cockatoo has not finished reading this mailbox yet. Try again when it has."}
	}
	items, err := s.st.MatchMessages(ctx, id, req.Criteria)
	if err != nil {
		return ActionOutcome{}, &InvalidError{err.Error()}
	}
	if len(items) == 0 {
		return ActionOutcome{}, &ConflictError{"There is no mail left that matches. Nothing was changed."}
	}
	if len(items) != req.Expected {
		return ActionOutcome{}, &ConflictError{fmt.Sprintf(
			"This now matches %d messages, not the %d you confirmed. Nothing was changed; check again.", len(items), req.Expected)}
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return ActionOutcome{}, err
	}

	desc := describe(req, items, a.Email)
	return s.moveAndAudit(ctx, id, a.Email, login, state.UIDValidity, req.Kind, req.Criteria, desc, items)
}

// moveAndAudit moves messages of one account's inbox to Trash or Spam and audits it: the record
// of what and why is written before anything moves, and what happened after. Only messages the
// server confirms have left the inbox are taken out of the copy. validity is the UID validity the
// messages were read with; a mailbox that has changed since is left alone.
func (s *Service) moveAndAudit(ctx context.Context, id int64, email string, login imap.Login, validity uint32,
	kind string, criteria store.Criteria, desc string, items []store.Message) (ActionOutcome, error) {
	m, err := s.openMover(ctx, s.server, login)
	if err != nil {
		return ActionOutcome{}, explainMail(err)
	}
	defer m.Close()
	if m.UIDValidity() != validity {
		return ActionOutcome{}, &ConflictError{"The mailbox has changed since cockatoo last read it. Nothing was changed; let it check again."}
	}
	folder, err := m.Folder(destinations[kind])
	if err != nil {
		return ActionOutcome{}, err
	}

	actionID, err := s.st.StartAction(ctx, store.ActionStart{
		AccountID: id, AccountEmail: email, Kind: kind, Criteria: criteria,
		Description: desc, Source: "you", Folder: folder,
	}, items)
	if err != nil {
		return ActionOutcome{}, err
	}

	uids := make([]uint32, len(items))
	for i, it := range items {
		uids[i] = it.UID
	}
	var moveErr error
	for start := 0; start < len(uids) && moveErr == nil; start += moveBatch {
		moveErr = m.Move(uids[start:min(start+moveBatch, len(uids))], folder)
	}

	// What actually left the inbox, by asking the server, not by assuming.
	out := ActionOutcome{ActionID: actionID, Description: desc}
	var moved, notMoved []uint32
	present, checkErr := m.Present(uids)
	if checkErr != nil {
		// A move that failed has often taken the connection with it: sign in again and ask once more.
		if again, err := s.openMover(ctx, s.server, login); err == nil {
			present, checkErr = again.Present(uids)
			again.Close()
		}
	}
	if checkErr == nil {
		for _, u := range uids {
			if present[u] {
				notMoved = append(notMoved, u)
			} else {
				moved = append(moved, u)
			}
		}
		if err := s.st.RemoveMessages(ctx, id, validity, moved); err != nil {
			moveErr = errors.Join(moveErr, err)
		}
	} else {
		// Unknown: nothing is taken out of the copy; the next sync sets it right.
		notMoved = uids
		moveErr = errors.Join(moveErr, checkErr)
	}
	out.Moved, out.NotMoved = len(moved), len(notMoved)
	switch {
	case checkErr != nil:
		out.Outcome, out.Moved, out.NotMoved = "unknown", 0, 0
	case out.NotMoved == 0:
		out.Outcome = "done"
	case out.Moved == 0:
		out.Outcome = "failed"
	default:
		out.Outcome = "partial"
	}
	if moveErr != nil {
		out.Error = moveErr.Error()
	}
	if err := s.st.FinishAction(ctx, actionID, store.ActionResult{
		Outcome: out.Outcome, Done: out.Moved, NotDone: notMoved, Error: out.Error,
	}); err != nil {
		return out, err
	}
	return out, nil
}

// describe says what an action does, in words, as the audit keeps it.
func describe(req ActionRequest, items []store.Message, email string) string {
	where := map[string]string{"trash": "Trash", "spam": "Spam"}[req.Kind]
	n := len(items)
	noun := "messages"
	if n == 1 {
		noun = "message"
	}
	switch req.Criteria.Kind {
	case "sender":
		return fmt.Sprintf("Move %d %s from %s to %s (%s)", n, noun, req.Criteria.Address, where, email)
	default:
		return fmt.Sprintf("Move %d %s received in %d to %s (%s)", n, noun, req.Criteria.Year, where, email)
	}
}

// LogUnsubscribe records in the audit that the user opened a sender's unsubscribe address.
// Cockatoo does not fetch or send anything itself: the address must be the one the sender's own
// mail gave, and it is opened by the user's click in their browser or mail program.
func (s *Service) LogUnsubscribe(ctx context.Context, id int64, address, target string) error {
	a, _, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return err
	}
	u, err := s.st.SenderUnsubscribe(ctx, id, address)
	if err != nil {
		return err
	}
	if target == "" || (target != u.URL && target != u.Mailto) {
		return &InvalidError{"That is not an unsubscribe address this sender's mail gave."}
	}
	how := "web page"
	if parsed, err := url.Parse(target); err == nil && strings.EqualFold(parsed.Scheme, "mailto") {
		how = "email"
	}
	ctx = context.WithoutCancel(ctx)
	actionID, err := s.st.StartAction(ctx, store.ActionStart{
		AccountID: id, AccountEmail: a.Email, Kind: "unsubscribe",
		Criteria:    store.Criteria{Kind: "sender", Address: strings.ToLower(address)},
		Description: fmt.Sprintf("Opened the unsubscribe %s of %s (%s)", how, strings.ToLower(address), a.Email),
		Source:      "you", Target: target,
	}, nil)
	if err != nil {
		return err
	}
	return s.st.FinishAction(ctx, actionID, store.ActionResult{Outcome: "opened"})
}

// Audit returns the most recent actions.
func (s *Service) Audit(ctx context.Context) ([]store.AuditEntry, error) {
	return s.st.Audit(ctx, 200)
}

// beginAction takes the turn from the sync; endAction gives it back.
func (s *Service) beginAction() error {
	s.sync.mu.Lock()
	defer s.sync.mu.Unlock()
	if s.sync.running || s.acting || s.stopped {
		return ErrBusy
	}
	s.acting = true
	s.actions.Add(1)
	return nil
}

func (s *Service) endAction() {
	s.sync.mu.Lock()
	s.acting = false
	s.sync.mu.Unlock()
	s.actions.Done()
}
