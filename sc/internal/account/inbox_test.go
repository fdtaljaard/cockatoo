package account

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

func newService(t *testing.T) (*Service, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "sync.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	a, err := st.AddAccount(ctx, store.Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{st: st, open: func([]byte) ([]byte, error) { return []byte("pw"), nil }, dial: dialSession}
	return s, a.ID
}

type literal struct{ *strings.Reader }

func (l literal) Size() int64 { return l.Reader.Size() }

// A real sync over IMAP of a few thousand messages spread over years: everything ends up in the
// copy, newest batch first, and the overview sees it all.
func TestSyncAccountAtScale(t *testing.T) {
	const n = 2600
	user := imapmemserver.NewUser("me@example.com", "pw")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2013, 3, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * 36 * time.Hour)
		msg := fmt.Sprintf("From: Sender %d <s%d@example.com>\r\nSubject: message %d\r\n\r\nbody\r\n", i%40, i%40, i)
		var flags []imap.Flag
		if i%4 != 0 {
			flags = []imap.Flag{imap.FlagSeen}
		}
		if _, err := user.Append("INBOX", literal{strings.NewReader(msg)}, &imap.AppendOptions{Time: at, Flags: flags}); err != nil {
			t.Fatal(err)
		}
	}
	mem := imapmemserver.New()
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	s, id := newService(t)
	s.server = imapc.Server{Addr: ln.Addr().String()}

	var reports []AccountSync
	start := time.Now()
	if err := s.SyncAccount(context.Background(), id, func(p AccountSync) { reports = append(reports, p) }); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d messages synced in %v", n, time.Since(start).Round(time.Millisecond))
	last := reports[len(reports)-1]
	if last.State != "done" || last.Read != n || last.ToRead != n || last.Skipped != 0 {
		t.Fatalf("last report %+v", last)
	}

	o, err := s.Overview(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if o.Messages != n || o.Unread != n/4 || !o.Sync.Complete || o.Sync.ServerTotal != n || o.Oldest.Year() != 2013 {
		t.Fatalf("overview: %d messages, %d unread, sync %+v, oldest %v", o.Messages, o.Unread, o.Sync, o.Oldest)
	}
	if len(o.Years) < 10 {
		t.Errorf("%d years, want ten or more", len(o.Years))
	}

	// A second sync reads nothing new.
	reports = nil
	if err := s.SyncAccount(context.Background(), id, func(p AccountSync) { reports = append(reports, p) }); err != nil {
		t.Fatal(err)
	}
	if last := reports[len(reports)-1]; last.ToRead != 0 || last.State != "done" {
		t.Fatalf("second sync %+v", last)
	}
}

// fakeBox is a mail server that can go offline and that cannot read some messages.
type fakeBox struct {
	inbox   imapc.InboxState
	headers map[uint32]imapc.Message
	broken  map[uint32]bool
	offline bool
	raw     map[uint32][]byte // whole messages, for saving attachments
	big     map[uint32]int64  // messages too big to read
}

func (f *fakeBox) Inbox() (imapc.InboxState, error) {
	if f.offline {
		return imapc.InboxState{}, errors.New("could not reach imap.gmail.com:993")
	}
	return f.inbox, nil
}

func (f *fakeBox) Headers(uids []uint32) ([]imapc.Message, error) {
	out := []imapc.Message{}
	for _, u := range uids {
		if f.broken[u] {
			return nil, errors.New("parse error in a FETCH response")
		}
		if m, ok := f.headers[u]; ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeBox) Texts(uids []uint32, max int) (map[uint32]string, error) {
	if f.offline {
		return nil, errors.New("could not reach imap.gmail.com:993")
	}
	out := map[uint32]string{}
	for _, u := range uids {
		if m, ok := f.headers[u]; ok {
			out[u] = "text of " + m.Subject
		}
	}
	return out, nil
}

func (f *fakeBox) Folder() string { return "INBOX" }

func (f *fakeBox) Raw(uids []uint32) (map[uint32][]byte, map[uint32]int64, error) {
	if f.offline {
		return nil, nil, errors.New("could not reach imap.gmail.com:993")
	}
	raw, big := map[uint32][]byte{}, map[uint32]int64{}
	for _, u := range uids {
		if b, ok := f.raw[u]; ok {
			raw[u] = b
		} else if n, ok := f.big[u]; ok {
			big[u] = n
		}
	}
	return raw, big, nil
}

func (f *fakeBox) Close() {}

func newFake(n int) *fakeBox {
	f := &fakeBox{inbox: imapc.InboxState{UIDValidity: 3}, headers: map[uint32]imapc.Message{}, broken: map[uint32]bool{}}
	for i := 1; i <= n; i++ {
		u := uint32(i)
		f.inbox.Messages = append(f.inbox.Messages, imapc.UIDSeen{UID: u})
		f.headers[u] = imapc.Message{UIDValidity: 3, UID: u, Subject: fmt.Sprint("m", i), FromAddr: "a@x.com",
			Received: time.Now().UTC().Add(-time.Duration(n-i) * time.Hour)}
	}
	return f
}

// One message the server cannot give is skipped and counted; the rest are read.
func TestSyncSkipsAMessageTheServerCannotRead(t *testing.T) {
	s, id := newService(t)
	box := newFake(1200)
	box.broken[700] = true
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return box, nil }

	var last AccountSync
	if err := s.SyncAccount(context.Background(), id, func(p AccountSync) { last = p }); err != nil {
		t.Fatal(err)
	}
	if last.State != "done" || last.Skipped != 1 || last.Read != 1200 {
		t.Fatalf("last report %+v", last)
	}
	o, _ := s.Overview(context.Background(), id)
	if o.Messages != 1199 {
		t.Fatalf("%d messages in the copy, want 1199", o.Messages)
	}
}

// An account whose server cannot be reached keeps its copy, is reported as failed, and its copy
// is not marked complete or trimmed.
func TestSyncOfflineKeepsTheCopy(t *testing.T) {
	s, id := newService(t)
	box := newFake(30)
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return box, nil }
	if err := s.SyncAccount(context.Background(), id, nil); err != nil {
		t.Fatal(err)
	}

	box.offline = true
	var last AccountSync
	err := s.SyncAccount(context.Background(), id, func(p AccountSync) { last = p })
	if err == nil || last.State != "failed" || !strings.Contains(last.Error, "could not reach") {
		t.Fatalf("offline: %v, %+v", err, last)
	}
	list, err := s.StoredInbox(context.Background())
	if err != nil || len(list.Messages) != 24 { // m7..m30 arrived within 24 hours
		t.Fatalf("stored inbox: %d messages, %v", len(list.Messages), err)
	}
}

// The background sync runs once at a time, and StopSync waits for it.
func TestStartAndStopSync(t *testing.T) {
	s, _ := newService(t)
	release := make(chan struct{})
	s.dial = func(ctx context.Context, _ imapc.Server, _ imapc.Login) (mailbox, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return newFake(5), nil
	}
	if st := s.StartSync(); !st.Running || len(st.Accounts) != 1 || st.Accounts[0].State != "waiting" && st.Accounts[0].State != "listing" {
		t.Fatalf("started: %+v", st)
	}
	if st := s.StartSync(); !st.Running {
		t.Fatalf("second start: %+v", st)
	}
	s.StopSync()
	st := s.SyncStatus()
	if st.Running || st.Accounts[0].State != "failed" {
		t.Fatalf("after stop: %+v", st)
	}
	if again := s.StartSync(); again.Running {
		t.Fatal("a sync started after StopSync")
	}
	close(release)
}

// A sync also reads the Sent folder: who the user writes to, from which addresses, and what they
// answered. A server without one is not an error.
func TestSyncReadsSent(t *testing.T) {
	user := imapmemserver.NewUser("me@example.com", "pw")
	for _, name := range []string{"INBOX", "[Gmail]/Sent Mail"} {
		if err := user.Create(name, nil); err != nil {
			t.Fatal(err)
		}
	}
	put := func(box, raw string) {
		if _, err := user.Append(box, literal{strings.NewReader(raw)}, &imap.AppendOptions{Time: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	put("INBOX", "From: Ann <ann@example.com>\r\nTo: me@example.com\r\nMessage-ID: <q1@x>\r\nIn-Reply-To: <s1@x>\r\nSubject: re: budget\r\n\r\nok\r\n")
	put("[Gmail]/Sent Mail", "From: me@example.com\r\nTo: ann@example.com\r\nMessage-ID: <s1@x>\r\nSubject: budget\r\n\r\nhi\r\n")
	mem := imapmemserver.New()
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	s, id := newService(t)
	s.server = imapc.Server{Addr: ln.Addr().String()}
	s.dialSent = dialSentFolder
	var last AccountSync
	if err := s.SyncAccount(context.Background(), id, func(p AccountSync) { last = p }); err != nil {
		t.Fatal(err)
	}
	if last.State != "done" || last.SentError != "" {
		t.Fatalf("last %+v", last)
	}
	f, err := s.st.SentFactsFor(context.Background(), id)
	if err != nil || !f.Complete || f.Correspondents["ann@example.com"] != 1 || !f.SentIDs["s1@x"] || !f.MyAddresses["me@example.com"] {
		t.Fatalf("facts %+v %v", f, err)
	}
	c, _, err := s.st.TriageCandidates(context.Background(), []int64{id}, time.Now().Add(-time.Hour), 10)
	if err != nil || len(c) != 1 || c[0].InReplyTo != "s1@x" || strings.Join(c[0].To, ",") != "me@example.com" {
		t.Fatalf("candidate %+v %v", c, err)
	}
}
