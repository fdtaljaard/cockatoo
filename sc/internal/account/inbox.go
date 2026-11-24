package account

import (
	"context"
	"time"

	"cockatoo/internal/store"
)

// Window is how far back the Inbox list goes: the last 24 hours.
const Window = 24 * time.Hour

// InboxList is the last 24 hours of every account's inbox, newest first, from the copy in
// DuckDB. The copy is kept up to date by the sync (sync.go).
type InboxList struct {
	Since    time.Time       `json:"since"`
	Messages []store.Message `json:"messages"`
}

// StoredInbox returns the last 24 hours from the copy, without asking the mail server.
func (s *Service) StoredInbox(ctx context.Context) (InboxList, error) {
	since := time.Now().UTC().Add(-Window)
	msgs, err := s.st.ListMessages(ctx, since)
	if err != nil {
		return InboxList{}, err
	}
	return InboxList{Since: since, Messages: msgs}, nil
}

// Overview is the shape of one account's whole inbox, from the copy.
func (s *Service) Overview(ctx context.Context, id int64) (store.Overview, error) {
	if _, _, err := s.st.AccountSecret(ctx, id); err != nil {
		return store.Overview{}, err // store.ErrNotFound for an unknown account
	}
	return s.st.InboxOverview(ctx, id)
}
