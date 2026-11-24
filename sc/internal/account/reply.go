package account

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// Replying from a conversation: plain text, through Gmail's own server (which keeps a copy in
// Sent), in the same thread, with the message answered quoted below. Only the user's click sends;
// every send is audited (who to, about what), the text itself is not kept by cockatoo.

// ReplyRequest is a reply as the user wrote it.
type ReplyRequest struct {
	Message store.MessageKey `json:"message"` // the inbox message answered
	Text    string           `json:"text"`
	All     bool             `json:"all"` // reply to everyone it was sent to, not only the sender
}

// ReplyResult is a reply that was sent.
type ReplyResult struct {
	MessageID string    `json:"messageId"`
	To        []string  `json:"to"`
	Cc        []string  `json:"cc"`
	At        time.Time `json:"at"`
}

// ReplyPreview is who a reply to a message goes to: To for a reply, and the Cc a reply to
// everyone adds.
type ReplyPreview struct {
	Key store.MessageKey `json:"key"`
	To  []string         `json:"to"`
	Cc  []string         `json:"cc"` // with "Reply all" only
}

// maxReply bounds what the user can write in one reply.
const maxReply = 100_000

// Reply sends a reply to an inbox message.
func (s *Service) Reply(ctx context.Context, req ReplyRequest) (ReplyResult, error) {
	text := strings.TrimSpace(req.Text)
	switch {
	case text == "":
		return ReplyResult{}, &InvalidError{"Write something to send."}
	case len([]rune(text)) > maxReply:
		return ReplyResult{}, &InvalidError{"That is too long for one reply."}
	}
	a, sealed, err := s.st.AccountSecret(ctx, req.Message.AccountID)
	if err != nil {
		return ReplyResult{}, err
	}
	known, err := s.st.MessagesByKey(ctx, []store.MessageKey{req.Message})
	if err != nil {
		return ReplyResult{}, err
	}
	orig, ok := known[req.Message]
	if !ok {
		return ReplyResult{}, &ConflictError{"That message is no longer in your inbox. Nothing was sent."}
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return ReplyResult{}, err
	}
	mine, err := s.myAddresses(ctx, req.Message.AccountID, a.Email)
	if err != nil {
		return ReplyResult{}, err
	}

	// The message answered, read again from the server: its headers decide the thread and who
	// the reply goes to, and it must still be the message the user saw.
	read, err := s.readOne(ctx, login, orig)
	if err != nil {
		return ReplyResult{}, err
	}
	to, cc := replyRecipients(read, mine, req.All)
	if len(to) == 0 {
		return ReplyResult{}, &ConflictError{"Cockatoo could not tell who to reply to. Nothing was sent."}
	}
	quoteFrom := read.FromAddr
	if read.FromName != "" {
		quoteFrom = read.FromName + " <" + read.FromAddr + ">"
	}
	subject := orig.Subject
	if read.Subject != "" {
		subject = read.Subject
	}
	now := time.Now()
	msg, id, err := imap.BuildReply(imap.Reply{
		From: &gomail.Address{Name: a.Name, Address: a.Email}, To: to, Cc: cc,
		Subject: imap.ReplySubject(subject), InReplyTo: read.MessageID, References: read.References,
		Text: text, Quote: read.Full, QuoteFrom: quoteFrom, QuoteDate: read.Date,
	}, now)
	if err != nil {
		return ReplyResult{}, err
	}

	out := ReplyResult{MessageID: id, To: addrStrings(to), Cc: addrStrings(cc), At: now.UTC()}
	rcpt := append(addrOnly(to), addrOnly(cc)...)
	// From here the send runs to its end, and is recorded, even if the request is cancelled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	actionID, err := s.st.StartAction(ctx, store.ActionStart{
		AccountID: req.Message.AccountID, AccountEmail: a.Email, Kind: "reply", Criteria: store.Criteria{Kind: "messages"},
		Description: fmt.Sprintf("Reply to %s about %q (%s)", strings.Join(out.To, ", "), orig.Subject, a.Email),
		Source:      "you", Target: strings.Join(rcpt, ", "),
	}, []store.Message{orig})
	if err != nil {
		return ReplyResult{}, err
	}
	if s.send == nil {
		return ReplyResult{}, &ConflictError{"Sending is not set up. Nothing was sent."}
	}
	sendErr := s.send(ctx, s.smtp, login, a.Email, rcpt, msg)
	result := store.ActionResult{Outcome: "done", Done: 1}
	if sendErr != nil {
		result = store.ActionResult{Outcome: "failed", Error: sendErr.Error()}
	}
	if err := s.st.FinishAction(ctx, actionID, result); err != nil && sendErr == nil {
		return out, err
	}
	if sendErr != nil {
		return ReplyResult{}, explainMail(sendErr)
	}
	// Gmail keeps the reply in Sent; the next sync brings it into the conversation.
	if s.syncAfterSend != nil {
		s.syncAfterSend()
	}
	return out, nil
}

// readOne reads one inbox message from the server and checks it is still the one in the copy.
func (s *Service) readOne(ctx context.Context, login imap.Login, m store.Message) (imap.Read, error) {
	box, err := s.dial(ctx, s.server, login)
	if err != nil {
		return imap.Read{}, explainMail(err)
	}
	defer box.Close()
	changed := &ConflictError{"Your mailbox changed since cockatoo last read it. Refresh, then try again. Nothing was sent."}
	if box.Validity() != m.UIDValidity {
		return imap.Read{}, changed
	}
	bodies, err := box.Bodies([]uint32{m.UID})
	if err != nil {
		return imap.Read{}, explainMail(err)
	}
	b, ok := bodies[m.UID]
	if !ok {
		return imap.Read{}, &ConflictError{"That message is no longer in your inbox. Nothing was sent."}
	}
	read := imap.ReadMessage(b.Raw)
	if m.MessageID != "" && read.MessageID != "" && !imap.SameMessageID(m.MessageID, read.MessageID) {
		return imap.Read{}, changed
	}
	if read.FromAddr == "" {
		read.FromAddr, read.FromName = m.FromAddr, m.FromName
	}
	return read, nil
}

// myAddresses says whether an address is one of the user's own: the account's, or one they send
// from (an alias, from the Sent folder).
func (s *Service) myAddresses(ctx context.Context, id int64, email string) (func(string) bool, error) {
	facts, err := s.st.SentFactsFor(ctx, id)
	if err != nil {
		return nil, err
	}
	return func(addr string) bool {
		addr = strings.ToLower(strings.TrimSpace(addr))
		return addr == strings.ToLower(email) || facts.MyAddresses[addr]
	}, nil
}

// replyRecipients: the sender (or where the sender asks replies to go), and, for a reply to all,
// everyone else it was sent to except the user.
func replyRecipients(r imap.Read, mine func(string) bool, all bool) (to, cc []*gomail.Address) {
	seen := map[string]bool{}
	add := func(list *[]*gomail.Address, a *gomail.Address) {
		addr := strings.ToLower(a.Address)
		if addr == "" || seen[addr] || mine(addr) {
			return
		}
		seen[addr] = true
		*list = append(*list, &gomail.Address{Name: a.Name, Address: addr})
	}
	if len(r.ReplyTo) > 0 {
		for _, a := range r.ReplyTo {
			add(&to, a)
		}
	} else {
		add(&to, &gomail.Address{Name: r.FromName, Address: r.FromAddr})
	}
	if all {
		for _, a := range append(append([]*gomail.Address{}, r.To...), r.Cc...) {
			add(&cc, a)
		}
	}
	return to, cc
}

func addrStrings(list []*gomail.Address) []string {
	out := []string{}
	for _, a := range list {
		if a.Name != "" {
			out = append(out, a.Name+" <"+a.Address+">")
		} else {
			out = append(out, a.Address)
		}
	}
	return out
}

func addrOnly(list []*gomail.Address) []string {
	out := []string{}
	for _, a := range list {
		out = append(out, a.Address)
	}
	return out
}
