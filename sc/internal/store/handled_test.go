package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// What the user did about a message is read from the audit and the Sent copy: saves, done and
// not-done marks (the latest wins), and replies.
func TestHandledFor(t *testing.T) {
	ctx := context.Background()
	s, me, work := openTest(t)
	now := time.Now().UTC()
	invoice, lunch, plain := msg(1, now, "bills@example.com", "invoice", false), msg(2, now, "ann@example.com", "lunch", false), msg(3, now, "bob@example.com", "plain", false)
	other := msg(1, now, "x@example.com", "other", false) // the same UID in the other account
	if err := s.AddMessages(ctx, me, []Message{invoice, lunch, plain}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, work, []Message{other}); err != nil {
		t.Fatal(err)
	}
	invoice.AccountID, lunch.AccountID, plain.AccountID, other.AccountID = me, me, me, work

	dir := filepath.Join(`C:\Saves`, "2026-11")
	save, err := s.StartAction(ctx, ActionStart{AccountID: me, AccountEmail: "me@example.com", Kind: KindSave, Criteria: Criteria{Kind: "messages"}}, []Message{invoice})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordActionFiles(ctx, save, []ActionFile{
		{UIDValidity: 7, UID: 1, Path: filepath.Join(dir, "invoice.pdf"), Bytes: 10},
		{UIDValidity: 7, UID: 1, Path: filepath.Join(dir, "statement.pdf"), Bytes: 20},
	}); err != nil {
		t.Fatal(err)
	}
	mark := func(kind string, m Message) {
		t.Helper()
		if _, err := s.StartAction(ctx, ActionStart{AccountID: m.AccountID, AccountEmail: "me@example.com", Kind: kind, Criteria: Criteria{Kind: "messages"}}, []Message{m}); err != nil {
			t.Fatal(err)
		}
	}
	mark(KindDone, plain)
	mark(KindDone, lunch)
	mark(KindNotDone, lunch)

	if err := s.BeginSentSync(ctx, me, "Sent", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSent(ctx, me, []SentMessage{{UIDValidity: 3, UID: 1, MessageID: "r@x", FromAddr: "me@example.com", InReplyTo: "lunch@x", SentAt: now}}); err != nil {
		t.Fatal(err)
	}

	h, err := s.HandledFor(ctx, []Message{invoice, lunch, plain, other})
	if err != nil {
		t.Fatal(err)
	}
	if got := h[keyOf(invoice)]; got.SavedFiles != 2 || len(got.SavedTo) != 1 || got.SavedTo[0] != dir || got.SavedAt == nil || got.DoneAt != nil {
		t.Fatalf("invoice %+v", got)
	}
	if got := h[keyOf(lunch)]; got.DoneAt != nil || !got.Replied || !got.Any() {
		t.Fatalf("lunch (done, then not done; replied) %+v", got)
	}
	if got := h[keyOf(plain)]; got.DoneAt == nil || got.Replied {
		t.Fatalf("plain %+v", got)
	}
	if got, ok := h[keyOf(other)]; ok {
		t.Fatalf("the other account's UID 1 is not the saved invoice: %+v", got)
	}
}

func keyOf(m Message) MessageKey {
	return MessageKey{AccountID: m.AccountID, UIDValidity: m.UIDValidity, UID: m.UID}
}
