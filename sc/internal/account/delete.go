package account

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cockatoo/internal/store"
)

// DeleteResult is what deleting chosen messages did.
type DeleteResult struct {
	Moved    int    `json:"moved"`    // now in Trash
	NotMoved int    `json:"notMoved"` // still in the inbox
	Outcome  string `json:"outcome"`  // "done", "partial", "failed" or "unknown"
	Error    string `json:"error,omitempty"`
}

// waitForSync is how long a delete waits for a running sync to finish before it gives up.
const waitForSync = 45 * time.Second

// DeleteMessages moves the chosen inbox messages to Gmail's Trash (kept there 30 days), per
// account, audited like every other move. Messages no longer in the copy are left out. A sync
// that is running is waited for, briefly, instead of refused.
func (s *Service) DeleteMessages(ctx context.Context, keys []store.MessageKey) (DeleteResult, error) {
	if len(keys) == 0 {
		return DeleteResult{}, &InvalidError{"Choose at least one message."}
	}
	known, err := s.st.MessagesByKey(ctx, keys)
	if err != nil {
		return DeleteResult{}, err
	}
	byAccount := map[int64][]store.Message{}
	var order []int64
	for _, k := range keys {
		m, ok := known[k]
		if !ok {
			continue
		}
		if byAccount[k.AccountID] == nil {
			order = append(order, k.AccountID)
		}
		byAccount[k.AccountID] = append(byAccount[k.AccountID], m)
	}
	if len(order) == 0 {
		return DeleteResult{}, &ConflictError{"Those messages are no longer in the inbox. Nothing was changed."}
	}
	if err := s.beginActionWaiting(ctx, waitForSync); err != nil {
		return DeleteResult{}, err
	}
	defer s.endAction()
	// From here the move runs to its end even if the request is cancelled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), actionTimeout)
	defer cancel()

	var out DeleteResult
	var errs []error
	for _, id := range order {
		items := byAccount[id]
		a, sealed, err := s.st.AccountSecret(ctx, id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		login, err := s.loginFor(ctx, a, sealed)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		validity := items[0].UIDValidity
		for _, m := range items {
			if m.UIDValidity != validity {
				return out, &ConflictError{"The mailbox has changed since cockatoo last read it. Nothing was changed; let it check again."}
			}
		}
		res, err := s.moveAndAudit(ctx, id, a.Email, login, validity, "trash", store.Criteria{Kind: "messages"},
			describeDelete(items, a.Email), items)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out.Moved += res.Moved
		out.NotMoved += res.NotMoved
		if res.Error != "" {
			errs = append(errs, errors.New(res.Error))
		}
		if res.Outcome == "unknown" {
			out.Outcome = "unknown"
		}
	}
	switch {
	case out.Outcome == "unknown":
	case out.Moved > 0 && out.NotMoved == 0 && len(errs) == 0:
		out.Outcome = "done"
	case out.Moved > 0:
		out.Outcome = "partial"
	default:
		out.Outcome = "failed"
	}
	if len(errs) > 0 {
		out.Error = errors.Join(errs...).Error()
		if out.Moved == 0 && out.Outcome == "failed" {
			return out, errs[0]
		}
	}
	return out, nil
}

// describeDelete says, for the audit, which messages went to Trash.
func describeDelete(items []store.Message, email string) string {
	if len(items) == 1 {
		m := items[0]
		return fmt.Sprintf("Move the message %q from %s to Trash (%s)", m.Subject, m.FromAddr, email)
	}
	var subjects []string
	for _, m := range items[:min(3, len(items))] {
		subjects = append(subjects, fmt.Sprintf("%q", m.Subject))
	}
	more := ""
	if len(items) > 3 {
		more = ", …"
	}
	return fmt.Sprintf("Move %d chosen messages to Trash: %s%s (%s)", len(items), strings.Join(subjects, ", "), more, email)
}

// beginActionWaiting is beginAction, waiting up to wait for a running sync to finish.
func (s *Service) beginActionWaiting(ctx context.Context, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		err := s.beginAction()
		if !errors.Is(err, ErrBusy) || time.Now().After(deadline) {
			return err
		}
		s.sync.mu.Lock()
		stopped := s.stopped
		s.sync.mu.Unlock()
		if stopped {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
