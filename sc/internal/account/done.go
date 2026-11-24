package account

import (
	"context"
	"errors"
	"fmt"

	"cockatoo/internal/store"
)

// AttachHandled fills in what the user has done about each message: attachments saved, marked
// done, replied to.
func (s *Service) AttachHandled(ctx context.Context, msgs []store.Message) error {
	handled, err := s.st.HandledFor(ctx, msgs)
	if err != nil {
		return err
	}
	for i := range msgs {
		k := store.MessageKey{AccountID: msgs[i].AccountID, UIDValidity: msgs[i].UIDValidity, UID: msgs[i].UID}
		if h, ok := handled[k]; ok && h.Any() {
			msgs[i].Handled = &h
		}
	}
	return nil
}

// MarkDone marks messages done (or, with done false, takes the mark away). It changes nothing in
// the mailbox: the mark is an action in the audit, like every other, one per account. Messages no
// longer in cockatoo's copy are left out; it returns how many were marked.
func (s *Service) MarkDone(ctx context.Context, keys []store.MessageKey, done bool) (int, error) {
	if len(keys) == 0 {
		return 0, &InvalidError{"Choose at least one message."}
	}
	known, err := s.st.MessagesByKey(ctx, keys)
	if err != nil {
		return 0, err
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
		return 0, &InvalidError{"Those messages are no longer in the inbox."}
	}
	kind, verb := store.KindDone, "Mark %d messages done (%s)"
	if !done {
		kind, verb = store.KindNotDone, "Mark %d messages not done (%s)"
	}
	marked := 0
	for _, id := range order {
		a, _, err := s.st.AccountSecret(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return marked, err
		}
		items := byAccount[id]
		actionID, err := s.st.StartAction(ctx, store.ActionStart{
			AccountID: id, AccountEmail: a.Email, Kind: kind, Criteria: store.Criteria{Kind: "messages"},
			Description: fmt.Sprintf(verb, len(items), a.Email), Source: "you",
		}, items)
		if err != nil {
			return marked, err
		}
		if err := s.st.FinishAction(context.WithoutCancel(ctx), actionID, store.ActionResult{Outcome: "done", Done: len(items)}); err != nil {
			return marked, err
		}
		marked += len(items)
	}
	return marked, nil
}
