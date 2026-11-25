package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Skill is something the email agent knows about how the user works, in plain words: when it
// applies and what to do (SCOPE.md, section 7a). Skills are kept insert-only: every change is a new
// version, with who made it and why, and the latest version of each skill is the skill.
type Skill struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Applies      string    `json:"applies"`      // when it applies: "mail from shops I buy from"
	Instructions string    `json:"instructions"` // what to do, in the user's words
	MadeBy       string    `json:"madeBy"`       // who made this version: SkillByYou, SkillByAgent or SkillByCockatoo
	Note         string    `json:"note,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Who made a version of a skill.
const (
	SkillByYou      = "you"      // written or edited by the user
	SkillByAgent    = "agent"    // proposed by the agent from a correction, approved by the user
	SkillByCockatoo = "cockatoo" // one of the starter skills
)

// ListSkills returns the skills, as their latest versions, oldest skill first. Removed skills are
// left out.
func (s *Store) ListSkills(ctx context.Context) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT skill_id, name, applies, instructions, made_by, coalesce(note, ''), created_at
		 FROM (SELECT *, row_number() OVER (PARTITION BY skill_id ORDER BY id DESC) AS n FROM skill_version)
		 WHERE n = 1 AND NOT removed ORDER BY skill_id`)
	if err != nil {
		return nil, fmt.Errorf("list skills: %w", err)
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		var k Skill
		if err := rows.Scan(&k.ID, &k.Name, &k.Applies, &k.Instructions, &k.MadeBy, &k.Note, &k.UpdatedAt); err != nil {
			return nil, fmt.Errorf("list skills: %w", err)
		}
		k.UpdatedAt = k.UpdatedAt.UTC()
		out = append(out, k)
	}
	return out, rows.Err()
}

// SaveSkill writes a new version of a skill; with ID 0 it is a new skill. Its fields must already be
// checked.
func (s *Store) SaveSkill(ctx context.Context, k Skill) (Skill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k.ID == 0 {
		if err := s.db.QueryRowContext(ctx, `SELECT nextval('seq_skill')`).Scan(&k.ID); err != nil {
			return Skill{}, fmt.Errorf("save skill: %w", err)
		}
	} else if err := s.skillExists(ctx, k.ID); err != nil {
		return Skill{}, err
	}
	k.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO skill_version (skill_id, name, applies, instructions, made_by, note, removed, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, false, ?)`,
		k.ID, k.Name, k.Applies, k.Instructions, k.MadeBy, nullable(k.Note), k.UpdatedAt); err != nil {
		return Skill{}, fmt.Errorf("save skill: %w", err)
	}
	return k, nil
}

// RemoveSkill removes a skill, as a last version marked removed; its history stays.
func (s *Store) RemoveSkill(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.skillExists(ctx, id); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO skill_version (skill_id, name, applies, instructions, made_by, note, removed, created_at)
		 SELECT skill_id, name, applies, instructions, ?, 'removed', true, ?
		 FROM skill_version WHERE skill_id = ? ORDER BY id DESC LIMIT 1`,
		SkillByYou, time.Now().UTC(), id); err != nil {
		return fmt.Errorf("remove skill %d: %w", id, err)
	}
	return nil
}

// skillExists reports ErrNotFound for a skill that is not there or was removed. The caller holds
// s.mu.
func (s *Store) skillExists(ctx context.Context, id int64) error {
	var removed bool
	err := s.db.QueryRowContext(ctx,
		`SELECT removed FROM skill_version WHERE skill_id = ? ORDER BY id DESC LIMIT 1`, id).Scan(&removed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && removed) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read skill %d: %w", id, err)
	}
	return nil
}
