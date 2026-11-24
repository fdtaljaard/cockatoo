package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRoutineMatching(t *testing.T) {
	made := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	after := made.Add(24 * time.Hour)
	domain := Routine{ID: 1, From: "acme.example", Action: RoutineSave, FolderID: 1, Path: "Bills", Dated: "month", CreatedAt: made}
	invoices := Routine{ID: 2, From: "acme.example", Subject: "invoice", Action: RoutineSave, FolderID: 1, CreatedAt: made}
	address := Routine{ID: 3, From: "billing@acme.example", Action: RoutineSave, FolderID: 2, CreatedAt: made}
	m := func(from, subject string, at time.Time) Message {
		return Message{FromAddr: from, Subject: subject, ReceivedAt: at}
	}

	for _, c := range []struct {
		r    Routine
		m    Message
		want bool
	}{
		{domain, m("Sales@ACME.example", "hello", after), true},
		{domain, m("x@mail.acme.example", "hello", after), false}, // the domain exactly, not a subdomain
		{domain, m("x@notacme.example", "hello", after), false},
		{invoices, m("a@acme.example", "Your INVOICE for November", after), true},
		{invoices, m("a@acme.example", "Your statement", after), false},
		{address, m("Billing@Acme.example", "x", after), true},
		{address, m("sales@acme.example", "x", after), false},
	} {
		if got := c.r.Matches(c.m); got != c.want {
			t.Errorf("%+v matches %+v: %v, want %v", c.r, c.m, got, c.want)
		}
	}

	folders := map[int64]SaveFolder{1: {ID: 1, Name: "Expenses"}}
	match := Matcher([]Routine{domain, invoices, address}, folders)
	if r := match(m("billing@acme.example", "invoice", after)); r == nil || r.RoutineID != 3 || !r.Missing {
		t.Fatalf("the address beats the subject, and its folder is gone: %+v", r)
	}
	if r := match(m("a@acme.example", "invoice", after)); r == nil || r.RoutineID != 2 || r.Target() != "" {
		t.Fatalf("the subject beats the bare domain: %+v", r)
	}
	r := match(m("a@acme.example", "hello", time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local)))
	if r == nil || r.RoutineID != 1 || r.Target() != "Bills/2026-11" || r.Describe() != "save its attachments to Expenses › Bills › 2026-11" {
		t.Fatalf("dated by the mail's month: %+v", r)
	}
	if r := match(m("a@acme.example", "hello", made.Add(-time.Hour))); r != nil {
		t.Fatalf("mail from before the routine was made: %+v", r)
	}
	if DatedName("day", time.Date(2026, 3, 9, 12, 0, 0, 0, time.Local)) != "2026-03-09" || DatedName("", after) != "" {
		t.Fatal("dated names")
	}
}

func TestRoutines(t *testing.T) {
	ctx := context.Background()
	s, _, _ := openTest(t)
	r, err := s.AddRoutine(ctx, Routine{From: "acme.example", Subject: "invoice", Action: RoutineSave, FolderID: 4, Path: "Bills", Dated: "month"})
	if err != nil || r.ID == 0 || r.CreatedAt.IsZero() {
		t.Fatalf("add: %+v %v", r, err)
	}
	if _, err := s.AddRoutine(ctx, Routine{From: "acme.example", Subject: "invoice", Action: RoutineSave, FolderID: 5}); !errors.Is(err, ErrDuplicateRoutine) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := s.AddRoutine(ctx, Routine{From: "acme.example", Action: RoutineSave, FolderID: 4}); err != nil {
		t.Fatalf("the same sender without the subject is another routine: %v", err)
	}
	list, err := s.ListRoutines(ctx)
	if err != nil || len(list) != 2 || list[0].Subject != "invoice" || list[0].Dated != "month" || list[1].Subject != "" || list[1].Dated != "" {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := s.DeleteRoutine(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRoutine(ctx, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
