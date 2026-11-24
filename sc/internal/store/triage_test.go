package store

import (
	"context"
	"testing"
	"time"
)

func TestTriageCandidatesAndRecord(t *testing.T) {
	ctx := context.Background()
	s, me, work := openTest(t)
	now := time.Now().UTC().Truncate(time.Second)

	news := msg(1, now.Add(-2*time.Hour), "deals@shop.example", "sale", false)
	news.Unsubscribe = Unsubscribe{URL: "https://shop.example/u"}
	if err := s.AddMessages(ctx, me, []Message{
		news,
		msg(2, now.Add(-3*24*time.Hour), "boss@example.com", "budget by Friday?", false),
		msg(3, now.Add(-4*24*time.Hour), "boss@example.com", "read one", true),
		msg(4, now.Add(-40*24*time.Hour), "boss@example.com", "too old", false),
		msg(5, now.Add(-time.Hour), "deals@shop.example", "another sale", true),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, work, []Message{msg(1, now.Add(-time.Hour), "it@work.example", "password expires", false)}); err != nil {
		t.Fatal(err)
	}

	since := now.Add(-30 * 24 * time.Hour)
	got, left, err := s.TriageCandidates(ctx, []int64{me}, since, 10)
	if err != nil {
		t.Fatal(err)
	}
	if left != 0 || len(got) != 2 || got[0].Subject != "sale" || got[1].Subject != "budget by Friday?" {
		t.Fatalf("candidates %+v, left %d", got, left)
	}
	if !got[0].Bulk || got[0].SenderMessages != 2 || got[0].SenderRead != 1 {
		t.Errorf("shop sender facts %+v", got[0])
	}
	if got[1].Bulk || got[1].SenderMessages != 3 || got[1].SenderRead != 1 {
		t.Errorf("boss sender facts %+v", got[1])
	}

	// The limit keeps the newest and says how many were left out; both accounts together.
	got, left, err = s.TriageCandidates(ctx, []int64{me, work}, since, 2)
	if err != nil || len(got) != 2 || left != 1 || got[0].Subject != "password expires" {
		t.Fatalf("limited %+v left %d %v", got, left, err)
	}

	// The record: insert-only, the latest answered one is found again.
	id, err := s.RecordAIRequest(ctx, AIRequest{CreatedAt: now, Kind: "triage", Scope: "all", Model: "m", InputTokens: 900,
		OutputTokens: 80, Sent: 2, LeftOut: 1, Answer: `{"summary":"x"}`},
		[]AIRequestItem{{Ref: "m1", MessageKey: MessageKey{AccountID: work, UIDValidity: 7, UID: 1}, FromAddr: "it@work.example", Subject: "password expires"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordAIRequest(ctx, AIRequest{CreatedAt: now, Kind: "triage", Scope: "all", Error: "refused"}, nil); err != nil {
		t.Fatal(err)
	}
	r, found, err := s.LatestAIRequest(ctx, "triage", "all")
	if err != nil || !found || r.ID != id || r.Answer != `{"summary":"x"}` || r.InputTokens != 900 {
		t.Fatalf("latest %+v %v %v", r, found, err)
	}
	if _, found, _ := s.LatestAIRequest(ctx, "triage", "1"); found {
		t.Error("an answer for another scope")
	}
	var items int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM ai_request_item WHERE request_id = ?`, id).Scan(&items); err != nil || items != 1 {
		t.Fatalf("%d items %v", items, err)
	}

	byKey, err := s.MessagesByKey(ctx, []MessageKey{{AccountID: me, UIDValidity: 7, UID: 2}, {AccountID: me, UIDValidity: 7, UID: 99}})
	if err != nil || len(byKey) != 1 {
		t.Fatalf("by key %+v %v", byKey, err)
	}
}
