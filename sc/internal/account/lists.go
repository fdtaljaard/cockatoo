package account

import (
	"context"
	"time"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

const (
	// ListWindow is how far back the lists look unless the user asks for all their mail.
	ListWindow = 30 * 24 * time.Hour
	// NewWindow: unsorted mail younger than this is shown at the top of the page, to deal with in the
	// moment.
	NewWindow = 24 * time.Hour
)

// ListsView is one list of mail (Reply, Attention, Save or Unsorted), with the counts the
// navigation shows and the new mail no rule or choice has sorted yet.
type ListsView struct {
	List     string          `json:"list"`
	All      bool            `json:"all"`             // all mail, of any age; otherwise the last 30 days
	Since    *time.Time      `json:"since,omitempty"` // nil with All
	Counts   map[string]int  `json:"counts"`          // open mail in each list
	Done     int             `json:"done"`            // dealt-with mail in this list, shown only on request
	Messages []store.Message `json:"messages"`        // this list, newest first
	New      []store.Message `json:"new"`             // unsorted mail of the last day, not dealt with, newest first
}

// Lists returns one list. A message is in the list the user last put it in; otherwise in the list
// of the most specific rule that matches it; otherwise in Unsorted (SCOPE.md, section 8a). Mail
// the user has dealt with leaves its list unless showDone is set. accountID 0 is every account.
func (s *Service) Lists(ctx context.Context, list string, all bool, accountID int64, showDone bool) (ListsView, error) {
	if !store.ValidList(list) {
		return ListsView{}, &InvalidError{"There is no list called " + list + "."}
	}
	now := time.Now().UTC()
	var since time.Time
	view := ListsView{List: list, All: all, Counts: map[string]int{store.ListReply: 0, store.ListAttention: 0,
		store.ListSave: 0, store.ListUnsorted: 0, store.ListIgnore: 0}, Messages: []store.Message{}, New: []store.Message{}}
	if !all {
		since = now.Add(-ListWindow)
		view.Since = &since
	}
	msgs, err := s.st.ListMessages(ctx, since)
	if err != nil {
		return ListsView{}, err
	}
	if accountID != 0 {
		mine := msgs[:0]
		for _, m := range msgs {
			if m.AccountID == accountID {
				mine = append(mine, m)
			}
		}
		msgs = mine
	}
	if err := s.AttachHandled(ctx, msgs); err != nil {
		return ListsView{}, err
	}
	if err := s.markRoutines(ctx, msgs); err != nil {
		return ListsView{}, err
	}
	rules, err := s.st.ListRoutines(ctx)
	if err != nil {
		return ListsView{}, err
	}
	rule := store.ListRule(rules)
	marks, err := s.st.SortMarks(ctx)
	if err != nil {
		return ListsView{}, err
	}
	suggestions, err := s.st.Suggestions(ctx)
	if err != nil {
		return ListsView{}, err
	}
	for _, m := range msgs {
		k := store.MessageKey{AccountID: m.AccountID, UIDValidity: m.UIDValidity, UID: m.UID}
		info := store.SortInfo{List: store.ListUnsorted}
		if mark, ok := marks[k]; ok {
			info = store.SortInfo{List: mark.List, By: mark.By}
		} else if r := rule(m); r != nil {
			info = store.SortInfo{List: r.Action, By: store.ByRule, RuleID: r.ID}
		}
		m.Sort = &info
		if info.List == store.ListUnsorted {
			if g, ok := suggestions[k]; ok {
				m.Suggestion = &g
			}
		}
		done := m.Handled != nil
		if !done {
			view.Counts[info.List]++
		}
		if info.List == list {
			if done {
				view.Done++
			}
			if !done || showDone {
				view.Messages = append(view.Messages, m)
			}
		}
		if !done && info.List == store.ListUnsorted && now.Sub(m.ReceivedAt) < NewWindow {
			view.New = append(view.New, m)
		}
	}
	return view, nil
}

// SortMessages puts messages in a list, as the user chose on their rows (by is store.ByYou) or by
// confirming the AI's suggestion (store.ByAI). Nothing changes in the mailbox.
func (s *Service) SortMessages(ctx context.Context, keys []store.MessageKey, list, by string) error {
	if len(keys) == 0 {
		return &InvalidError{"Choose at least one message."}
	}
	if !store.ValidList(list) {
		return &InvalidError{"There is no list called " + list + "."}
	}
	if by != store.ByYou && by != store.ByAI {
		by = store.ByYou
	}
	return s.st.AddSortMarks(ctx, keys, list, by)
}

// Snippet is what the AI may read of a message beyond its headers: the first lines of its text
// (when the user allows it) and the names of the files it carries.
type Snippet struct {
	Text        string
	Attachments []imap.AttachmentInfo
}

// MessageSnippets reads, over one connection, the attachment names of these messages and, with
// withText, the first max characters of their text.
func (s *Service) MessageSnippets(ctx context.Context, id int64, uids []uint32, max int, withText bool) (map[uint32]Snippet, error) {
	a, sealed, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return nil, err
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return nil, err
	}
	box, err := s.dial(ctx, s.server, login)
	if err != nil {
		return nil, explainMail(err)
	}
	defer box.Close()
	files, err := box.Attachments(uids)
	if err != nil {
		return nil, explainMail(err)
	}
	out := make(map[uint32]Snippet, len(uids))
	for _, u := range uids {
		out[u] = Snippet{Attachments: files[u]}
	}
	if withText {
		texts, err := box.Texts(uids, max)
		if err != nil {
			return nil, explainMail(err)
		}
		for u, t := range texts {
			sn := out[u]
			sn.Text = t
			out[u] = sn
		}
	}
	return out, nil
}
