package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// A conversation is followed both ways through the inbox and the Sent copy, in one account only.
func TestThread(t *testing.T) {
	ctx := context.Background()
	s, me, work := openTest(t)
	now := time.Now().UTC()
	if err := s.AddMessages(ctx, me, []Message{
		{UIDValidity: 7, UID: 1, MessageID: "a@x", FromAddr: "ann@example.com", Subject: "Lunch?", ReceivedAt: now.Add(-3 * time.Hour)},
		{UIDValidity: 7, UID: 3, MessageID: "c@x", InReplyTo: "b@x", FromAddr: "ann@example.com", Subject: "Re: Lunch?", ReceivedAt: now.Add(-time.Hour)},
		{UIDValidity: 7, UID: 4, MessageID: "z@x", FromAddr: "bob@example.com", Subject: "Other", ReceivedAt: now},
		{UIDValidity: 7, UID: 5, MessageID: "old@x", FromAddr: "ann@example.com", Subject: "Earlier", ReceivedAt: now.Add(-48 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, work, []Message{
		{UIDValidity: 9, UID: 1, MessageID: "w@x", InReplyTo: "a@x", FromAddr: "x@example.com", Subject: "Re: Lunch?", ReceivedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginSentSync(ctx, me, "Sent", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSent(ctx, me, []SentMessage{{UIDValidity: 3, UID: 1, MessageID: "b@x", FromAddr: "me@example.com", InReplyTo: "a@x", SentAt: now.Add(-2 * time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	order := func(items []ThreadItem) string {
		var ids []string
		for _, it := range items {
			ids = append(ids, it.Folder+":"+it.MessageID)
		}
		return strings.Join(ids, ",")
	}
	for _, from := range []MessageKey{{AccountID: me, UIDValidity: 7, UID: 3}, {AccountID: me, UIDValidity: 7, UID: 1}} {
		items, more, err := s.Thread(ctx, me, from, nil)
		if err != nil || more != 0 || order(items) != "inbox:a@x,sent:b@x,inbox:c@x" {
			t.Fatalf("from %v: %s %d %v", from, order(items), more, err)
		}
	}
	// The References a message names reach mail no In-Reply-To links to.
	items, _, err := s.Thread(ctx, me, MessageKey{AccountID: me, UIDValidity: 7, UID: 3}, []string{"<old@x>"})
	if err != nil || order(items) != "inbox:old@x,inbox:a@x,sent:b@x,inbox:c@x" {
		t.Fatalf("with references: %s %v", order(items), err)
	}
	if _, _, err := s.Thread(ctx, me, MessageKey{AccountID: me, UIDValidity: 7, UID: 99}, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown message: %v", err)
	}
}
