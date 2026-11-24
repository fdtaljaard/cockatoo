package account

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// Opening a message shows its conversation: the messages linked to it in the inbox and the Sent
// folder, each read from the mail server with a peek (nothing is marked as read) and cleaned of
// quotes, signatures and pictures. Nothing read is kept.

// ConversationMessage is one message of a conversation.
type ConversationMessage struct {
	Key         *store.MessageKey     `json:"key,omitempty"` // inbox mail only: what Save and Done work on
	Folder      string                `json:"folder"`        // "inbox" or "sent"
	Mine        bool                  `json:"mine"`          // the user sent it
	Opened      bool                  `json:"opened"`        // the message the user opened
	FromName    string                `json:"fromName"`
	FromAddr    string                `json:"fromAddr"`
	At          time.Time             `json:"at"`
	Seen        bool                  `json:"seen"`
	Text        string                `json:"text"` // what the person wrote
	Full        string                `json:"full"` // everything, as text
	Cut         bool                  `json:"cut"`  // the message is longer than what was read
	Attachments []imap.AttachmentInfo `json:"attachments"`
	Handled     *store.Handled        `json:"handled,omitempty"`
	Routine     *store.RoutineMatch   `json:"routine,omitempty"`
	Problem     string                `json:"problem,omitempty"` // why it could not be shown
}

// Conversation is an opened message and the messages around it, oldest first.
type Conversation struct {
	Subject     string                `json:"subject"`
	MessageID   string                `json:"messageId"` // the opened message's, to find it in Gmail
	Account     string                `json:"account"`   // the account's address
	Messages    []ConversationMessage `json:"messages"`
	Older       int                   `json:"older"`           // older messages left out
	SentPartial bool                  `json:"sentPartial"`     // the Sent folder is not fully read: replies may be missing
	Reply       *ReplyPreview         `json:"reply,omitempty"` // who a reply goes to: the newest message from someone else
}

// Conversation opens a message of the inbox copy.
func (s *Service) Conversation(ctx context.Context, key store.MessageKey) (Conversation, error) {
	a, sealed, err := s.st.AccountSecret(ctx, key.AccountID)
	if err != nil {
		return Conversation{}, err
	}
	items, older, err := s.st.Thread(ctx, key.AccountID, key, nil)
	if err != nil {
		return Conversation{}, err
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return Conversation{}, err
	}
	read := s.readThread(ctx, login, items)

	// The References a message names can reach mail that no In-Reply-To links to: one more round.
	known := map[string]bool{}
	var refs []string
	for _, it := range items {
		known[it.MessageID] = true
	}
	for _, r := range read {
		for _, id := range r.read.References {
			if !known[id] {
				known[id] = true
				refs = append(refs, id)
			}
		}
	}
	if len(refs) > 0 {
		more, moreOlder, err := s.st.Thread(ctx, key.AccountID, key, refs)
		if err == nil && len(more) > len(items) {
			var fresh []store.ThreadItem
			have := map[string]bool{}
			for _, it := range items {
				have[readKey(it)] = true
			}
			for _, it := range more {
				if !have[readKey(it)] {
					fresh = append(fresh, it)
				}
			}
			for k, v := range s.readThread(ctx, login, fresh) {
				read[k] = v
			}
			items, older = more, moreOlder
		}
	}

	facts, err := s.st.SentFactsFor(ctx, key.AccountID)
	if err != nil {
		return Conversation{}, err
	}
	mine := func(addr string) bool {
		addr = strings.ToLower(addr)
		return addr == strings.ToLower(a.Email) || facts.MyAddresses[addr]
	}

	out := Conversation{Account: a.Email, Older: older, SentPartial: !facts.Complete, Messages: []ConversationMessage{}}
	var inbox []store.Message
	for _, it := range items {
		m := ConversationMessage{Folder: it.Folder, FromName: it.FromName, FromAddr: it.FromAddr, At: it.At, Seen: it.Seen,
			Attachments: []imap.AttachmentInfo{}}
		m.Mine = it.Folder == "sent" || mine(it.FromAddr)
		if it.Folder == "inbox" {
			k := store.MessageKey{AccountID: key.AccountID, UIDValidity: it.UIDValidity, UID: it.UID}
			m.Key = &k
			m.Opened = k == key
			inbox = append(inbox, store.Message{AccountID: key.AccountID, UIDValidity: it.UIDValidity, UID: it.UID,
				MessageID: it.MessageID, FromAddr: it.FromAddr, Subject: it.Subject, ReceivedAt: it.At})
		}
		r := read[readKey(it)]
		if m.Key != nil && !m.Mine && r.problem == "" {
			rd := r.read
			if rd.FromAddr == "" {
				rd.FromAddr, rd.FromName = it.FromAddr, it.FromName
			}
			to, _ := replyRecipients(rd, mine, false)
			_, cc := replyRecipients(rd, mine, true)
			out.Reply = &ReplyPreview{Key: *m.Key, To: addrStrings(to), Cc: addrStrings(cc)}
		}
		switch {
		case r.problem != "":
			m.Problem = r.problem
		default:
			m.Text, m.Full, m.Cut, m.Attachments = r.read.Text, r.read.Full, r.cut, r.attachments
			if m.FromName == "" && !m.Mine {
				m.FromName = r.read.FromName
			}
		}
		if m.Opened {
			out.Subject, out.MessageID = it.Subject, it.MessageID
		}
		out.Messages = append(out.Messages, m)
	}
	if out.Subject == "" && len(out.Messages) > 0 {
		out.Subject = items[len(items)-1].Subject
	}

	// What the user did about the inbox mail, and the routine for it, as the Inbox shows them.
	if err := s.AttachHandled(ctx, inbox); err != nil {
		return Conversation{}, err
	}
	if err := s.markRoutines(ctx, inbox); err != nil {
		return Conversation{}, err
	}
	byKey := map[store.MessageKey]store.Message{}
	for _, m := range inbox {
		byKey[store.MessageKey{AccountID: m.AccountID, UIDValidity: m.UIDValidity, UID: m.UID}] = m
	}
	for i := range out.Messages {
		if k := out.Messages[i].Key; k != nil {
			out.Messages[i].Handled, out.Messages[i].Routine = byKey[*k].Handled, byKey[*k].Routine
		}
	}
	return out, nil
}

type readResult struct {
	read        imap.Read
	cut         bool
	attachments []imap.AttachmentInfo
	problem     string
}

func readKey(it store.ThreadItem) string {
	return fmt.Sprintf("%s/%d/%d", it.Folder, it.UIDValidity, it.UID)
}

// readThread reads the messages from the inbox and, when the conversation has any, the Sent
// folder, at the same time. A message that cannot be read, or is no longer the message cockatoo
// knows by that UID, gets a problem instead of a text: never another message's text.
func (s *Service) readThread(ctx context.Context, login imap.Login, items []store.ThreadItem) map[string]readResult {
	out := map[string]readResult{}
	var mu sync.Mutex
	set := func(k string, r readResult) {
		mu.Lock()
		out[k] = r
		mu.Unlock()
	}
	byFolder := map[string][]store.ThreadItem{}
	for _, it := range items {
		byFolder[it.Folder] = append(byFolder[it.Folder], it)
	}
	var wg sync.WaitGroup
	for folder, list := range byFolder {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fail := func(problem string) {
				for _, it := range list {
					set(readKey(it), readResult{problem: problem})
				}
			}
			var box mailbox
			var err error
			if folder == "sent" {
				var found bool
				if s.dialSent == nil {
					fail("Your Sent folder could not be opened.")
					return
				}
				box, found, err = s.dialSent(ctx, s.server, login)
				if err == nil && !found {
					fail("Your Sent folder could not be found.")
					return
				}
			} else {
				box, err = s.dial(ctx, s.server, login)
			}
			if err != nil {
				fail("Cockatoo could not read this from your mailbox: " + explainMail(err).Error())
				return
			}
			defer box.Close()
			validity := box.Validity()
			var uids []uint32
			for _, it := range list {
				if it.UIDValidity == validity {
					uids = append(uids, it.UID)
				}
			}
			bodies, err := box.Bodies(uids)
			if err != nil {
				fail("Cockatoo could not read this from your mailbox: " + explainMail(err).Error())
				return
			}
			for _, it := range list {
				b, ok := bodies[it.UID]
				switch {
				case it.UIDValidity != validity:
					set(readKey(it), readResult{problem: "Your mailbox changed since cockatoo last read it. Refresh, then open it again."})
				case !ok:
					set(readKey(it), readResult{problem: "This message is no longer in your " + folderWord(folder) + "."})
				default:
					r := imap.ReadMessage(b.Raw)
					if it.MessageID != "" && r.MessageID != "" && !imap.SameMessageID(it.MessageID, r.MessageID) {
						set(readKey(it), readResult{problem: "Your mailbox changed since cockatoo last read it. Refresh, then open it again."})
						continue
					}
					set(readKey(it), readResult{read: r, cut: b.Size > int64(len(b.Raw)), attachments: b.Attachments})
				}
			}
		}()
	}
	wg.Wait()
	return out
}

func folderWord(folder string) string {
	if folder == "sent" {
		return "Sent folder"
	}
	return "inbox"
}
