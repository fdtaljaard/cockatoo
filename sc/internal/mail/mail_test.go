package mail

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// literal is a message body with its size, as APPEND wants it.
type literal struct {
	*strings.Reader
}

func (l literal) Size() int64 { return l.Reader.Size() }

// startServer runs an in-memory IMAP server with one user and one unread message in the inbox.
func startServer(t *testing.T, username, password string) Server {
	t.Helper()
	user := imapmemserver.NewUser(username, password)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	msg := "From: a@example.com\r\nTo: " + username + "\r\nSubject: hello\r\n\r\nhi\r\n"
	if _, err := user.Append("INBOX", literal{strings.NewReader(msg)}, &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	mem := imapmemserver.New()
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true, // plain TCP on loopback, for the test only
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return Server{Addr: ln.Addr().String(), TLS: false}
}

func TestCheckInbox(t *testing.T) {
	srv := startServer(t, "me@example.com", "app-password")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inbox, err := CheckInbox(ctx, srv, "me@example.com", "app-password")
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Messages != 1 || inbox.Unseen != 1 {
		t.Errorf("inbox %+v, want 1 message, 1 unseen", inbox)
	}
}

func TestCheckInboxWrongPassword(t *testing.T) {
	srv := startServer(t, "me@example.com", "app-password")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := CheckInbox(ctx, srv, "me@example.com", "not-it")
	var refused *LoginError
	if !errors.As(err, &refused) {
		t.Fatalf("error %v, want a LoginError", err)
	}
}

func TestCheckInboxUnreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here now

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = CheckInbox(ctx, Server{Addr: addr}, "me@example.com", "x")
	if err == nil || !strings.Contains(err.Error(), "could not reach") {
		t.Fatalf("error %v, want could not reach", err)
	}
}
