package mail

import (
	"context"
	"strings"
	"testing"
)

// Bodies peeks at the start of a message and names its attachments from BODYSTRUCTURE, by the
// same rule as saving them.
func TestBodiesPeeksAndNamesAttachments(t *testing.T) {
	user := newUser(t)
	putRaw(t, user, invoiceMail)
	srv := serve(t, user)
	ctx := context.Background()
	login := PasswordLogin("me@example.com", "pw")
	s, err := Open(ctx, srv, login)
	if err != nil {
		t.Fatal(err)
	}
	inbox, _ := s.Inbox()
	uid := inbox.Messages[0].UID
	bodies, err := s.Bodies([]uint32{uid, 9999})
	if err != nil || len(bodies) != 1 {
		t.Fatalf("bodies %d %v", len(bodies), err)
	}
	b := bodies[uid]
	if !strings.Contains(string(b.Raw), "Thanks") || b.Size == 0 {
		t.Fatalf("raw %q size %d", b.Raw, b.Size)
	}
	var names []string
	for _, a := range b.Attachments {
		names = append(names, a.Name)
	}
	if got := strings.Join(names, ","); got != "invoice.pdf,Rechnung Müller.pdf,reçu.jpg,forwarded message.eml,attachment.bin" {
		t.Fatalf("attachments %q", got)
	}
	s.Close()
	if count, _ := CheckInbox(ctx, srv, login); count.Unseen != 1 {
		t.Fatalf("reading the message marked it read: %+v", count)
	}
}
