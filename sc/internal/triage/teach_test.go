package triage

import (
	"context"
	"strings"
	"testing"

	"cockatoo/internal/store"
)

// A correction becomes a proposal, never a saved change: an edit names a real skill and carries the
// text before it; a skill that does not exist is refused; the skills go with the request.
func TestTeach(t *testing.T) {
	s, st, fake, id := setup(t, true)
	ctx := context.Background()
	key := store.MessageKey{AccountID: id, UIDValidity: 7, UID: 1}

	fake.answer = `{"action":"edit","skill_id":1,"name":"","applies":"","instructions":"Invoices go to Save. Sale mail needs nothing.","summary":"Sale mail from shops needs nothing."}`
	p, err := s.Teach(ctx, TeachInput{Message: key, From: "save", To: "ignore"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Action != "edit" || p.SkillID != 1 || p.Name != "What I keep" || p.Before == "" || !strings.Contains(p.Instructions, "Sale mail") {
		t.Fatalf("proposal %+v", p)
	}
	if !strings.Contains(fake.got.Prompt, "moved it to Nothing to do") || !strings.Contains(fake.got.Context, `name="What I keep"`) {
		t.Fatalf("request: %s | %s", fake.got.Prompt, fake.got.Context)
	}
	if list, _ := st.ListSkills(ctx); list[0].Instructions == p.Instructions {
		t.Fatal("a proposal must not be saved")
	}

	fake.answer = `{"action":"edit","skill_id":42,"name":"","applies":"","instructions":"x","summary":"x"}`
	if _, err := s.Teach(ctx, TeachInput{Message: key, From: "save", To: "ignore"}); err == nil {
		t.Fatal("an edit of a skill that does not exist must be refused")
	}
	fake.answer = `{"action":"none","skill_id":0,"name":"","applies":"","instructions":"","summary":""}`
	if p, err := s.Teach(ctx, TeachInput{Message: key, From: "reply", To: "attention"}); err != nil || p.Action != "none" || p.Summary == "" {
		t.Fatalf("none: %+v %v", p, err)
	}
	if _, err := s.Teach(ctx, TeachInput{Message: key, From: "save", To: "save"}); err == nil {
		t.Fatal("a move to the same list teaches nothing")
	}
}
