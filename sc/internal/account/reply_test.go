package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// A reply goes to the sender (and, to all, everyone else but the user), in the thread, with the
// answered message quoted; it is audited; a message that changed in the mailbox sends nothing.
func TestReply(t *testing.T) {
	ctx := context.Background()
	s, id := newService(t)
	now := time.Now().UTC()
	if err := s.st.AddMessages(ctx, id, []store.Message{
		{UIDValidity: 3, UID: 1, MessageID: "a@x", FromName: "Ann", FromAddr: "ann@example.com", Subject: "Lunch", ReceivedAt: now},
		{UIDValidity: 3, UID: 2, MessageID: "b@x", FromAddr: "bob@example.com", Subject: "Other", ReceivedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	box := newFake(0)
	box.raw = map[uint32][]byte{
		1: []byte("From: Ann <ann@example.com>\r\nTo: Me <ME@example.com>, Bob <bob@example.com>\r\nCc: carol@example.com\r\n" +
			"Subject: Lunch\r\nMessage-ID: <a@x>\r\nReferences: <z@x>\r\nContent-Type: text/plain\r\n\r\nFriday?\r\n"),
		2: []byte("From: bob@example.com\r\nSubject: Other\r\nMessage-ID: <changed@x>\r\n\r\nhi\r\n"),
	}
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return box, nil }
	var sent struct {
		from string
		rcpt []string
		msg  string
	}
	s.send = func(_ context.Context, _ imapc.SMTPServer, _ imapc.Login, from string, rcpt []string, msg []byte) error {
		sent.from, sent.rcpt, sent.msg = from, rcpt, string(msg)
		return nil
	}

	if _, err := s.Reply(ctx, ReplyRequest{Message: store.MessageKey{AccountID: id, UIDValidity: 3, UID: 1}, Text: "  "}); !errors.As(err, new(*InvalidError)) {
		t.Fatalf("empty: %v", err)
	}
	res, err := s.Reply(ctx, ReplyRequest{Message: store.MessageKey{AccountID: id, UIDValidity: 3, UID: 1}, Text: "Yes, 12:30.", All: true})
	if err != nil {
		t.Fatal(err)
	}
	if sent.from != "me@example.com" || strings.Join(sent.rcpt, ",") != "ann@example.com,bob@example.com,carol@example.com" ||
		strings.Join(res.To, ",") != "Ann <ann@example.com>" || len(res.Cc) != 2 {
		t.Fatalf("sent %+v result %+v", sent, res)
	}
	r := imapc.ReadMessage([]byte(sent.msg))
	if r.Subject != "Re: Lunch" || r.InReplyTo != "a@x" || strings.Join(r.References, ",") != "z@x,a@x" || !strings.Contains(r.Full, "> Friday?") {
		t.Fatalf("message %+v", r)
	}
	entries, _ := s.Audit(ctx)
	if len(entries) == 0 || entries[0].Kind != "reply" || !strings.Contains(entries[0].Description, "Ann <ann@example.com>") {
		t.Fatalf("audit %+v", entries)
	}

	sent.msg = ""
	if _, err := s.Reply(ctx, ReplyRequest{Message: store.MessageKey{AccountID: id, UIDValidity: 3, UID: 2}, Text: "hi"}); !errors.As(err, new(*ConflictError)) || sent.msg != "" {
		t.Fatalf("changed message: %v, sent %q", err, sent.msg)
	}
}
