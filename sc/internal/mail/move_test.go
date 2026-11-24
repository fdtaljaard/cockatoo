package mail

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

func serveCaps(t *testing.T, user *imapmemserver.User, caps imap.CapSet) Server {
	t.Helper()
	mem := imapmemserver.New()
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         caps,
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return Server{Addr: ln.Addr().String()}
}

func putRaw(t *testing.T, user *imapmemserver.User, raw string) {
	t.Helper()
	if _, err := user.Append("INBOX", literal{strings.NewReader(raw)}, &imap.AppendOptions{Time: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func TestUnsubscribeHeadersAndMove(t *testing.T) {
	user := newUser(t)
	if err := user.Create("[Gmail]/Trash", nil); err != nil {
		t.Fatal(err)
	}
	putRaw(t, user, "From: Deals <deals@shop.example>\r\nSubject: sale\r\n"+
		"List-Unsubscribe: <mailto:leave@shop.example>,\r\n <https://shop.example/u?x=1>\r\n"+
		"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nbuy\r\n")
	putRaw(t, user, "From: Ann <ann@example.com>\r\nSubject: lunch\r\n\r\nhi\r\n")
	putRaw(t, user, "From: Deals <deals@shop.example>\r\nSubject: sale again\r\n\r\nbuy\r\n")
	srv := serveCaps(t, user, imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapMove: {}})
	ctx := context.Background()

	// The unsubscribe fields come with the headers, and reading them marks nothing as read.
	s, err := Open(ctx, srv, PasswordLogin("me@example.com", "pw"))
	if err != nil {
		t.Fatal(err)
	}
	inbox, err := s.Inbox()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Headers([]uint32{inbox.Messages[0].UID, inbox.Messages[1].UID})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	want := Unsubscribe{URL: "https://shop.example/u?x=1", Mailto: "mailto:leave@shop.example", OneClick: true}
	if got[0].Unsubscribe != want || !got[1].Unsubscribe.Empty() {
		t.Fatalf("unsubscribe %+v / %+v", got[0].Unsubscribe, got[1].Unsubscribe)
	}
	if count, _ := CheckInbox(ctx, srv, PasswordLogin("me@example.com", "pw")); count.Unseen != 3 {
		t.Fatalf("%d unread after reading headers, want 3", count.Unseen)
	}

	// Move the two deals to Trash; the other message stays.
	m, err := OpenMover(ctx, srv, PasswordLogin("me@example.com", "pw"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.UIDValidity() != inbox.UIDValidity {
		t.Errorf("validity %d, want %d", m.UIDValidity(), inbox.UIDValidity)
	}
	folder, err := m.Folder(Trash)
	if err != nil || folder != "[Gmail]/Trash" {
		t.Fatalf("trash folder %q %v", folder, err)
	}
	if _, err := m.Folder(Spam); err == nil {
		t.Error("found a spam folder that is not there")
	}
	deals := []uint32{inbox.Messages[0].UID, inbox.Messages[2].UID}
	if err := m.Move(deals, folder); err != nil {
		t.Fatal(err)
	}
	present, err := m.Present([]uint32{inbox.Messages[0].UID, inbox.Messages[1].UID, inbox.Messages[2].UID})
	if err != nil {
		t.Fatal(err)
	}
	if len(present) != 1 || !present[inbox.Messages[1].UID] {
		t.Fatalf("still in the inbox: %v", present)
	}
	m.Close()

	trash, err := CheckInbox(ctx, srv, PasswordLogin("me@example.com", "pw"))
	if err != nil || trash.Messages != 1 {
		t.Fatalf("inbox after the move: %+v %v", trash, err)
	}
	if st, err := user.Status("[Gmail]/Trash", &imap.StatusOptions{NumMessages: true}); err != nil || *st.NumMessages != 2 {
		t.Fatalf("trash: %+v %v", st, err)
	}
}

func TestOpenMoverRefusesWithoutMove(t *testing.T) {
	srv := serveCaps(t, newUser(t), imap.CapSet{imap.CapIMAP4rev1: {}})
	if _, err := OpenMover(context.Background(), srv, PasswordLogin("me@example.com", "pw")); err == nil || !strings.Contains(err.Error(), "MOVE") {
		t.Fatalf("got %v, want a refusal naming MOVE", err)
	}
}
