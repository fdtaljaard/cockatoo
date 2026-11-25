package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// Skills start with the two starters; every change is a new version; a removed skill is gone from the
// list but its history stays.
func TestSkills(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "skills.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.ListSkills(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "What I keep" || list[1].MadeBy != SkillByCockatoo {
		t.Fatalf("starters: %+v %v", list, err)
	}
	k, err := s.SaveSkill(ctx, Skill{Name: "Shops", Applies: "mail from shops", Instructions: "Invoices to Save.", MadeBy: SkillByYou})
	if err != nil || k.ID < 3 {
		t.Fatalf("new: %+v %v", k, err)
	}
	k.Instructions = "Invoices to Save. Shipping notices need nothing."
	k.MadeBy, k.Note = SkillByAgent, "from a correction"
	if _, err := s.SaveSkill(ctx, k); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSkill(ctx, Skill{ID: 99, Name: "x", MadeBy: SkillByYou}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown skill: %v", err)
	}
	list, _ = s.ListSkills(ctx)
	if len(list) != 3 || list[2].Instructions != k.Instructions || list[2].MadeBy != SkillByAgent {
		t.Fatalf("after edit: %+v", list)
	}
	if err := s.RemoveSkill(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSkill(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed twice: %v", err)
	}
	list, _ = s.ListSkills(ctx)
	var versions int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM skill_version`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != 2 || versions != 5 {
		t.Fatalf("after remove: %+v, %d versions", list, versions)
	}
}
