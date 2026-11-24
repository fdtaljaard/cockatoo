package account

import (
	"context"
	"strings"
	"testing"
	"time"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

func rawMail(id, from, inReplyTo, body string) []byte {
	h := "From: " + from + "\r\nSubject: Lunch\r\nMessage-ID: <" + id + ">\r\n"
	if inReplyTo != "" {
		h += "In-Reply-To: <" + inReplyTo + ">\r\n"
	}
	return []byte(h + "Content-Type: text/plain\r\n\r\n" + body)
}

// Opening a message shows its conversation from the inbox and the Sent folder, cleaned, the
// user's own mail marked as theirs; a UID that now holds another message shows a problem, never
// that message's text.
func TestConversation(t *testing.T) {
	ctx := context.Background()
	s, id := newService(t)
	now := time.Now().UTC()
	if err := s.st.AddMessages(ctx, id, []store.Message{
		{UIDValidity: 3, UID: 1, MessageID: "a@x", FromName: "Ann", FromAddr: "ann@example.com", Subject: "Lunch", ReceivedAt: now.Add(-3 * time.Hour)},
		{UIDValidity: 3, UID: 3, MessageID: "c@x", InReplyTo: "b@x", FromName: "Ann", FromAddr: "ann@example.com", Subject: "Re: Lunch", ReceivedAt: now.Add(-time.Hour)},
		{UIDValidity: 3, UID: 4, MessageID: "d@x", InReplyTo: "c@x", FromName: "Ann", FromAddr: "ann@example.com", Subject: "Re: Lunch", ReceivedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.st.BeginSentSync(ctx, id, "Sent", 5); err != nil {
		t.Fatal(err)
	}
	if err := s.st.AddSent(ctx, id, []store.SentMessage{{UIDValidity: 5, UID: 9, MessageID: "b@x", InReplyTo: "a@x", FromAddr: "me@example.com", SentAt: now.Add(-2 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	inbox := newFake(0)
	inbox.raw = map[uint32][]byte{
		1: rawMail("a@x", "Ann <ann@example.com>", "", "Lunch on Friday?\r\n\r\nKind regards\r\nAnn\r\n+27 82 000 0000\r\n"),
		3: rawMail("c@x", "Ann <ann@example.com>", "b@x", "Great, see you there.\r\n\r\nOn Mon, me wrote:\r\n> Yes, 12:30?\r\n"),
		4: rawMail("someone-else@x", "Bob <bob@example.com>", "", "not this one"),
	}
	sent := &fakeBox{inbox: imapc.InboxState{UIDValidity: 5}, raw: map[uint32][]byte{
		9: rawMail("b@x", "Me <me@example.com>", "a@x", "Yes, 12:30?\r\n\r\nSent from my iPhone\r\n"),
	}}
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return inbox, nil }
	s.dialSent = func(context.Context, imapc.Server, imapc.Login) (mailbox, bool, error) { return sent, true, nil }

	c, err := s.Conversation(ctx, store.MessageKey{AccountID: id, UIDValidity: 3, UID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Messages) != 4 || c.Subject != "Re: Lunch" || c.MessageID != "c@x" || c.Account != "me@example.com" || !c.SentPartial {
		t.Fatalf("conversation %+v", c)
	}
	var got []string
	for _, m := range c.Messages {
		got = append(got, m.Folder+"|"+m.Text+"|"+boolWord(m.Mine, "mine")+boolWord(m.Opened, "opened")+"|"+m.Problem)
	}
	want := []string{
		"inbox|Lunch on Friday?\n\nKind regards\nAnn||",
		"sent|Yes, 12:30?|mine|",
		"inbox|Great, see you there.|opened|",
		"inbox||| Your mailbox changed since cockatoo last read it. Refresh, then open it again.",
	}
	for i := range want {
		if strings.TrimSpace(got[i]) != strings.TrimSpace(strings.Replace(want[i], "|| ", "||", 1)) {
			t.Errorf("message %d:\n%q\nwant\n%q", i, got[i], want[i])
		}
	}
	if c.Messages[1].Key != nil || c.Messages[2].Key == nil || c.Messages[2].Full == "" {
		t.Errorf("keys and full text: %+v", c.Messages)
	}
	if strings.Contains(c.Messages[3].Text+c.Messages[3].Full, "not this one") {
		t.Error("another message's text was shown")
	}
}

func boolWord(b bool, w string) string {
	if b {
		return w
	}
	return ""
}
