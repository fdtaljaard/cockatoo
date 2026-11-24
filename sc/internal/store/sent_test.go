package store

import (
	"context"
	"testing"
	"time"
)

func TestSentCopyAndFacts(t *testing.T) {
	ctx := context.Background()
	s, me, _ := openTest(t)
	now := time.Now().UTC()

	if f, err := s.SentFactsFor(ctx, me); err != nil || f.Complete || len(f.Correspondents) != 0 {
		t.Fatalf("before any sync: %+v %v", f, err)
	}
	if err := s.BeginSentSync(ctx, me, "[Gmail]/Sent Mail", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSent(ctx, me, []SentMessage{
		{UIDValidity: 3, UID: 1, MessageID: "a@x", FromAddr: "Me@Example.com", To: []string{"Ann@example.com"}, SentAt: now},
		{UIDValidity: 3, UID: 2, MessageID: "b@x", FromAddr: "me.alias@example.com", To: []string{"ann@example.com"}, Cc: []string{"bob@example.com"},
			InReplyTo: "q1@example.com", SentAt: now},
		{UIDValidity: 3, UID: 3, MessageID: "c@x", FromAddr: "me@example.com", To: []string{"gone@example.com"}, SentAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	// UID 3 left the Sent folder before the listing finished.
	if err := s.FinishSentSync(ctx, me, map[uint32]bool{1: true, 2: true}); err != nil {
		t.Fatal(err)
	}
	f, err := s.SentFactsFor(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Complete || f.Correspondents["ann@example.com"] != 2 || f.Correspondents["bob@example.com"] != 1 || f.Correspondents["gone@example.com"] != 0 {
		t.Fatalf("correspondents %+v", f)
	}
	if !f.MyAddresses["me@example.com"] || !f.MyAddresses["me.alias@example.com"] || !f.SentIDs["a@x"] || !f.Answered["q1@example.com"] {
		t.Fatalf("facts %+v", f)
	}
	// A new UID validity starts the copy again.
	if err := s.BeginSentSync(ctx, me, "[Gmail]/Sent Mail", 4); err != nil {
		t.Fatal(err)
	}
	if uids, _ := s.StoredSentUIDs(ctx, me); len(uids) != 0 {
		t.Fatalf("after a new validity: %v", uids)
	}
}
