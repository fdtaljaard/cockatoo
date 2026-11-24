package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// put appends a message that arrived at the given time.
func put(t *testing.T, user *imapmemserver.User, at time.Time, from, subject string, flags ...imap.Flag) {
	t.Helper()
	msg := "From: " + from + "\r\nTo: me@example.com\r\nSubject: " + subject +
		"\r\nMessage-ID: <" + at.Format("150405.000") + "@example.com>\r\n\r\nbody\r\n"
	if _, err := user.Append("INBOX", literal{strings.NewReader(msg)}, &imap.AppendOptions{Time: at, Flags: flags}); err != nil {
		t.Fatal(err)
	}
}

func newUser(t *testing.T) *imapmemserver.User {
	t.Helper()
	user := imapmemserver.NewUser("me@example.com", "pw")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestSession(t *testing.T) {
	// The server's clock is in a zone two hours ahead of UTC, like this machine's.
	sast := time.FixedZone("SAST", 2*60*60)
	now := time.Now().In(sast)

	user := newUser(t)
	put(t, user, now.Add(-9*365*24*time.Hour), "Old <OLD@Example.com>", "years ago", imap.FlagSeen)
	put(t, user, now.Add(-2*time.Hour), "=?UTF-8?B?Sm9zw6kgR8OzbWV6?= <jose@example.com>", "=?UTF-8?Q?Caf=C3=A9_au_lait?=")
	put(t, user, now.Add(-1*time.Hour), "=?windows-1252?Q?Ren=E9e?= <renee@example.com>", "=?windows-1252?Q?Pr=E9cis_=96_draft?=")
	// Years of real mail include charsets nobody knows and broken encoded words.
	put(t, user, now.Add(-30*time.Minute), "Odd <odd@example.com>", "=?x-no-such-charset?Q?hello?=")
	put(t, user, now.Add(-20*time.Minute), "Broken <broken@example.com>", "=?UTF-8?B?###not-base64###?=")
	srv := serve(t, user)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := Open(ctx, srv, PasswordLogin("me@example.com", "pw"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	inbox, err := s.Inbox()
	if err != nil {
		t.Fatal(err)
	}
	if inbox.UIDValidity == 0 || len(inbox.Messages) != 5 {
		t.Fatalf("inbox %+v, want 5 messages", inbox)
	}
	if !inbox.Messages[0].Seen || inbox.Messages[1].Seen {
		t.Errorf("seen flags %+v", inbox.Messages)
	}

	uids := []uint32{}
	for _, m := range inbox.Messages {
		uids = append(uids, m.UID)
	}
	got, err := s.Headers(append(uids, 9999)) // a UID that is not there is left out
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("%d headers, want 5", len(got))
	}
	byUID := map[uint32]Message{}
	for _, m := range got {
		byUID[m.UID] = m
		if m.Received.Location() != time.UTC || m.UIDValidity != inbox.UIDValidity {
			t.Errorf("message %d: received %v validity %d", m.UID, m.Received, m.UIDValidity)
		}
	}
	want := []struct{ name, addr, subject string }{
		{"Old", "old@example.com", "years ago"},
		{"José Gómez", "jose@example.com", "Café au lait"},
		{"Renée", "renee@example.com", "Précis – draft"},
		{"Odd", "odd@example.com", "=?x-no-such-charset?Q?hello?="},
		{"Broken", "broken@example.com", "=?UTF-8?B?###not-base64###?="},
	}
	for i, w := range want {
		m := byUID[uids[i]]
		if m.FromName != w.name || m.FromAddr != w.addr || m.Subject != w.subject {
			t.Errorf("message %d: %q <%s> %q, want %q <%s> %q", i, m.FromName, m.FromAddr, m.Subject, w.name, w.addr, w.subject)
		}
	}
	if got := byUID[uids[0]].Received; got.Year() != now.Add(-9*365*24*time.Hour).UTC().Year() {
		t.Errorf("old message received %v", got)
	}
	s.Close()

	// Reading marked nothing as read.
	count, err := CheckInbox(ctx, srv, PasswordLogin("me@example.com", "pw"))
	if err != nil {
		t.Fatal(err)
	}
	if count.Unseen != 4 {
		t.Errorf("%d unread on the server, want 4", count.Unseen)
	}
}

func TestSessionEmptyInbox(t *testing.T) {
	srv := serve(t, newUser(t))
	s, err := Open(context.Background(), srv, PasswordLogin("me@example.com", "pw"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	inbox, err := s.Inbox()
	if err != nil {
		t.Fatal(err)
	}
	if inbox.Messages == nil || len(inbox.Messages) != 0 {
		t.Fatalf("got %#v, want an empty, non-nil list", inbox.Messages)
	}
	if got, err := s.Headers(nil); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("headers of nothing: %#v %v", got, err)
	}
}
