package store

import (
	"context"
	"testing"
	"time"
)

func TestSearchMessages(t *testing.T) {
	ctx := context.Background()
	s, me, work := openTest(t)
	now := time.Now().UTC()
	if err := s.AddMessages(ctx, me, []Message{
		{UIDValidity: 7, UID: 1, FromName: "Acme Billing", FromAddr: "billing@acme.example", Subject: "Your invoice for October", ReceivedAt: now.Add(-40 * 24 * time.Hour)},
		{UIDValidity: 7, UID: 2, FromName: "Acme Billing", FromAddr: "billing@acme.example", Subject: "Your Invoice for November", ReceivedAt: now.Add(-time.Hour)},
		{UIDValidity: 7, UID: 3, FromName: "Ann", FromAddr: "ann@example.com", Subject: "100% done_now", ReceivedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, work, []Message{
		{UIDValidity: 9, UID: 1, FromName: "Acme", FromAddr: "sales@acme.example", Subject: "invoice query", ReceivedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	subjects := func(q string, account int64) []string {
		t.Helper()
		msgs, more, err := s.SearchMessages(ctx, q, account)
		if err != nil || more {
			t.Fatalf("search %q: %v %v", q, more, err)
		}
		out := []string{}
		for _, m := range msgs {
			out = append(out, m.Subject)
		}
		return out
	}
	if got := subjects("INVOICE acme", 0); len(got) != 3 || got[2] != "Your invoice for October" {
		t.Fatalf("every word, any case, sender or subject, newest first: %v", got)
	}
	if got := subjects("invoice", me); len(got) != 2 {
		t.Fatalf("one account: %v", got)
	}
	if got := subjects("billing november", 0); len(got) != 1 {
		t.Fatalf("words across sender and subject: %v", got)
	}
	if got := subjects("100%", 0); len(got) != 1 || got[0] != "100% done_now" {
		t.Fatalf("%% is itself: %v", got)
	}
	if got := subjects("e_n", 0); len(got) != 1 {
		t.Fatalf("_ is itself, so it matches only done_now: %v", got)
	}
	if got := subjects("   ", 0); len(got) != 0 {
		t.Fatalf("empty query: %v", got)
	}
}
