package account

import (
	"context"
	"errors"
	"strings"
	"testing"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// fakeMover is a mail server for actions: it removes moved messages from the fake inbox, and can
// fail on a given MOVE or when asked what is left.
type fakeMover struct {
	box         *fakeBox
	validity    uint32
	moves       int
	failOnMove  int  // 1-based; 0 never. A failed move takes the connection with it.
	failCheck   bool // asking what is left always fails, on every connection
	reopenFails bool // signing in again fails
	dead        bool
	folders     map[imapc.Destination]string
}

func (f *fakeMover) UIDValidity() uint32 { return f.validity }
func (f *fakeMover) Folder(d imapc.Destination) (string, error) {
	if name, ok := f.folders[d]; ok {
		return name, nil
	}
	return "", errors.New("no such folder")
}
func (f *fakeMover) Move(uids []uint32, folder string) error {
	f.moves++
	if f.dead {
		return errors.New("connection closed")
	}
	if f.moves == f.failOnMove {
		f.dead = true
		return errors.New("connection reset")
	}
	gone := map[uint32]bool{}
	for _, u := range uids {
		gone[u] = true
	}
	kept := f.box.inbox.Messages[:0]
	for _, m := range f.box.inbox.Messages {
		if !gone[m.UID] {
			kept = append(kept, m)
		}
	}
	f.box.inbox.Messages = kept
	return nil
}
func (f *fakeMover) Present(uids []uint32) (map[uint32]bool, error) {
	if f.failCheck || f.dead {
		return nil, errors.New("connection closed")
	}
	in := map[uint32]bool{}
	for _, m := range f.box.inbox.Messages {
		in[m.UID] = true
	}
	out := map[uint32]bool{}
	for _, u := range uids {
		if in[u] {
			out[u] = true
		}
	}
	return out, nil
}
func (f *fakeMover) Close() {}

// actionSetup is a synced account of n messages, all from deals@shop.example with an unsubscribe
// address, plus one from ann@example.com.
func actionSetup(t *testing.T, n int) (*Service, int64, *fakeBox, *fakeMover) {
	t.Helper()
	s, id := newService(t)
	box := newFake(n)
	for u, m := range box.headers {
		m.FromAddr = "deals@shop.example"
		m.Unsubscribe = imapc.Unsubscribe{URL: "https://shop.example/u", Mailto: "mailto:off@shop.example"}
		box.headers[u] = m
	}
	box.inbox.Messages = append(box.inbox.Messages, imapc.UIDSeen{UID: 9999})
	box.headers[9999] = imapc.Message{UIDValidity: 3, UID: 9999, FromAddr: "ann@example.com", Subject: "hi"}
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return box, nil }
	if err := s.SyncAccount(context.Background(), id, nil); err != nil {
		t.Fatal(err)
	}
	m := &fakeMover{box: box, validity: 3, folders: map[imapc.Destination]string{imapc.Trash: "[Gmail]/Trash", imapc.Spam: "[Gmail]/Spam"}}
	// The first sign-in of an action gets m; signing in again (after a dropped connection) gets a
	// fresh connection to the same mailbox, unless m says that fails too.
	opened := 0
	s.openMover = func(context.Context, imapc.Server, imapc.Login) (mover, error) {
		opened++
		if opened == 1 || !m.dead && !m.failCheck {
			return m, nil
		}
		if m.reopenFails {
			return nil, errors.New("could not reach imap.gmail.com:993")
		}
		return &fakeMover{box: box, validity: m.validity, folders: m.folders, failCheck: m.failCheck}, nil
	}
	return s, id, box, m
}

var deals = store.Criteria{Kind: "sender", Address: "Deals@Shop.example"}

func TestApplyAction(t *testing.T) {
	s, id, _, _ := actionSetup(t, 1200)
	ctx := context.Background()

	p, err := s.PreviewAction(ctx, id, deals)
	if err != nil || p.Messages != 1200 || len(p.Subjects) != 3 {
		t.Fatalf("preview %+v %v", p, err)
	}

	// A count that no longer matches changes nothing.
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 1199}); !errors.As(err, new(*ConflictError)) {
		t.Fatalf("stale count: %v", err)
	}
	if audit, _ := s.Audit(ctx); len(audit) != 0 {
		t.Fatalf("a refused action was audited: %+v", audit)
	}

	// The request is cancelled (the window closed); the action still runs to its end.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	out, err := s.ApplyAction(cancelled, id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 1200})
	if err != nil || out.Outcome != "done" || out.Moved != 1200 || out.NotMoved != 0 {
		t.Fatalf("apply: %+v %v", out, err)
	}
	if !strings.Contains(out.Description, "Move 1200 messages from deals@shop.example to Trash") {
		t.Errorf("description %q", out.Description)
	}
	if o, _ := s.Overview(ctx, id); o.Messages != 1 {
		t.Fatalf("%d messages left in the copy, want 1", o.Messages)
	}
	audit, err := s.Audit(ctx)
	if err != nil || len(audit) != 1 || audit[0].Outcome != "done" || audit[0].Matched != 1200 || audit[0].Folder != "[Gmail]/Trash" {
		t.Fatalf("audit %+v %v", audit, err)
	}
}

func TestApplyActionFailsHalfway(t *testing.T) {
	s, id, _, m := actionSetup(t, 1200)
	m.failOnMove = 2 // the second batch of 500
	out, err := s.ApplyAction(context.Background(), id, ActionRequest{Kind: "spam", Criteria: deals, Expected: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != "partial" || out.Moved != 500 || out.NotMoved != 700 || !strings.Contains(out.Error, "connection reset") {
		t.Fatalf("outcome %+v", out)
	}
	// The copy matches the server: only what left the inbox is gone from it.
	if o, _ := s.Overview(context.Background(), id); o.Messages != 701 {
		t.Fatalf("%d in the copy, want 701", o.Messages)
	}
	if audit, _ := s.Audit(context.Background()); audit[0].Outcome != "partial" || audit[0].Done != 500 {
		t.Fatalf("audit %+v", audit[0])
	}
}

func TestApplyActionCannotCheck(t *testing.T) {
	s, id, _, m := actionSetup(t, 10)
	m.failCheck = true
	out, err := s.ApplyAction(context.Background(), id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 10})
	if err != nil {
		t.Fatal(err)
	}
	// It cannot tell what moved, so it does not claim anything.
	if out.Outcome != "unknown" || out.Moved != 0 || out.NotMoved != 0 {
		t.Fatalf("outcome %+v", out)
	}
	if audit, _ := s.Audit(context.Background()); audit[0].Outcome != "unknown" {
		t.Fatalf("audit %+v", audit[0])
	}
	// Not knowing, the copy is left as it was for the next sync to set right.
	if o, _ := s.Overview(context.Background(), id); o.Messages != 11 {
		t.Fatalf("%d in the copy, want 11", o.Messages)
	}
}

func TestApplyActionRefusals(t *testing.T) {
	s, id, _, m := actionSetup(t, 10)
	ctx := context.Background()

	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "delete-forever", Criteria: deals, Expected: 10}); !errors.As(err, new(*InvalidError)) {
		t.Errorf("unknown kind: %v", err)
	}
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "spam", Criteria: store.Criteria{Kind: "year", Year: 2026}, Expected: 11}); !errors.As(err, new(*InvalidError)) {
		t.Errorf("spam by year: %v", err)
	}
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "trash", Criteria: store.Criteria{Kind: "sender", Address: ""}, Expected: 0}); !errors.As(err, new(*InvalidError)) {
		t.Errorf("no sender: %v", err)
	}

	// A sync is running: wait.
	s.sync.mu.Lock()
	s.sync.running = true
	s.sync.mu.Unlock()
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 10}); !errors.Is(err, ErrBusy) {
		t.Errorf("during a sync: %v", err)
	}
	s.sync.mu.Lock()
	s.sync.running = false
	s.sync.mu.Unlock()

	// The mailbox was rebuilt since the copy was made.
	m.validity = 4
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 10}); !errors.As(err, new(*ConflictError)) {
		t.Errorf("new UID validity: %v", err)
	}
	m.validity = 3

	// No Spam folder on this server: refuse, never create one.
	delete(m.folders, imapc.Spam)
	if _, err := s.ApplyAction(ctx, id, ActionRequest{Kind: "spam", Criteria: deals, Expected: 10}); err == nil {
		t.Error("moved to a spam folder that is not there")
	}
	if audit, _ := s.Audit(ctx); len(audit) != 0 {
		t.Fatalf("refused actions were audited: %+v", audit)
	}
}

func TestLogUnsubscribe(t *testing.T) {
	s, id, _, _ := actionSetup(t, 3)
	ctx := context.Background()
	if err := s.LogUnsubscribe(ctx, id, "deals@shop.example", "https://evil.example/"); !errors.As(err, new(*InvalidError)) {
		t.Fatalf("an address the sender did not give: %v", err)
	}
	if err := s.LogUnsubscribe(ctx, id, "deals@shop.example", "https://shop.example/u"); err != nil {
		t.Fatal(err)
	}
	if err := s.LogUnsubscribe(ctx, id, "deals@shop.example", "mailto:off@shop.example"); err != nil {
		t.Fatal(err)
	}
	audit, _ := s.Audit(ctx)
	if len(audit) != 2 || audit[1].Kind != "unsubscribe" || audit[1].Outcome != "opened" || audit[1].Target != "https://shop.example/u" ||
		!strings.Contains(audit[0].Description, "email") {
		t.Fatalf("audit %+v", audit)
	}
}

// The connection drops on the second batch and every later command on it fails: signing in again
// shows the first batch did move.
func TestApplyActionConnectionDrops(t *testing.T) {
	s, id, _, m := actionSetup(t, 1200)
	m.failOnMove = 2
	out, err := s.ApplyAction(context.Background(), id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != "partial" || out.Moved != 500 || out.NotMoved != 700 {
		t.Fatalf("outcome %+v", out)
	}
	if o, _ := s.Overview(context.Background(), id); o.Messages != 701 {
		t.Fatalf("%d in the copy, want 701", o.Messages)
	}
}

// The connection drops and signing in again fails too: the outcome is unknown, not "nothing moved",
// and the copy is left for the next sync to set right.
func TestApplyActionConnectionDropsAndStaysDown(t *testing.T) {
	s, id, _, m := actionSetup(t, 1200)
	m.failOnMove = 2
	m.reopenFails = true
	out, err := s.ApplyAction(context.Background(), id, ActionRequest{Kind: "trash", Criteria: deals, Expected: 1200})
	if err != nil {
		t.Fatal(err)
	}
	if out.Outcome != "unknown" || !strings.Contains(out.Error, "connection reset") {
		t.Fatalf("outcome %+v", out)
	}
	if o, _ := s.Overview(context.Background(), id); o.Messages != 1201 {
		t.Fatalf("%d in the copy, want 1201 (unchanged)", o.Messages)
	}
}
