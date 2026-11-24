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
	Waiting  []store.Message `json:"waiting"` // mail of the last 60 days a routine is waiting on, newest first
}

// StoredInbox returns the last 24 hours from the copy, without asking the mail server.
func (s *Service) StoredInbox(ctx context.Context) (InboxList, error) {
	since := time.Now().UTC().Add(-Window)
	msgs, err := s.st.ListMessages(ctx, since)
	if err != nil {
		return InboxList{}, err
	}
	if err := s.AttachHandled(ctx, msgs); err != nil {
		return InboxList{}, err
	}
	waiting, err := s.attachRoutines(ctx, msgs)
	if err != nil {
		return InboxList{}, err
	}
	return InboxList{Since: since, Messages: msgs, Waiting: waiting}, nil
}

// SearchResult is what a search of the inbox found.
type SearchResult struct {
	Query    string          `json:"query"`
	Messages []store.Message `json:"messages"`
	More     bool            `json:"more"` // there were more than store.SearchLimit; the newest are shown
}

// Search finds mail in the copy of the whole inbox (not only the last 24 hours) by sender or
// subject, with what the user did about each and the routine for it, as the Inbox list shows them.
func (s *Service) Search(ctx context.Context, query string, accountID int64) (SearchResult, error) {
	msgs, more, err := s.st.SearchMessages(ctx, query, accountID)
	if err != nil {
		return SearchResult{}, err
	}
	if err := s.AttachHandled(ctx, msgs); err != nil {
		return SearchResult{}, err
	}
	if err := s.markRoutines(ctx, msgs); err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Query: query, Messages: msgs, More: more}, nil
}

// Overview is the shape of one account's whole inbox, from the copy.
func (s *Service) Overview(ctx context.Context, id int64) (store.Overview, error) {
	if _, _, err := s.st.AccountSecret(ctx, id); err != nil {
		return store.Overview{}, err // store.ErrNotFound for an unknown account
	}
	return s.st.InboxOverview(ctx, id)
}
