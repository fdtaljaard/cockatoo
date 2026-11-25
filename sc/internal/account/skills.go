package account

import (
	"context"
	"strings"

	"cockatoo/internal/store"
)

// SkillInput is a skill as the user saves it: written by them, or the agent's proposal they approved.
type SkillInput struct {
	ID           int64  `json:"id"` // 0 for a new skill
	Name         string `json:"name"`
	Applies      string `json:"applies"`
	Instructions string `json:"instructions"`
	Note         string `json:"note"`      // why it changed, for the history
	FromAgent    bool   `json:"fromAgent"` // the agent proposed it and the user approved it
}

// SaveSkill checks and saves a skill (SCOPE.md, section 7a). Only the user saves a skill: a change the
// agent proposes reaches here only when the user approves it.
func (s *Service) SaveSkill(ctx context.Context, in SkillInput) (store.Skill, error) {
	k := store.Skill{
		ID:           in.ID,
		Name:         strings.Join(strings.Fields(in.Name), " "),
		Applies:      strings.TrimSpace(in.Applies),
		Instructions: strings.TrimSpace(in.Instructions),
		Note:         strings.TrimSpace(in.Note),
		MadeBy:       store.SkillByYou,
	}
	if in.FromAgent {
		k.MadeBy = store.SkillByAgent
	}
	switch {
	case k.Name == "":
		return store.Skill{}, &InvalidError{"Give the skill a name, such as Shops I buy from."}
	case len([]rune(k.Name)) > 60:
		return store.Skill{}, &InvalidError{"Keep the name short: a few words."}
	case k.Instructions == "":
		return store.Skill{}, &InvalidError{"Say what to do, in your own words."}
	case len([]rune(k.Instructions)) > 2000 || len([]rune(k.Applies)) > 300 || len([]rune(k.Note)) > 300:
		return store.Skill{}, &InvalidError{"A skill is a few sentences. Split a long one into two skills."}
	}
	return s.st.SaveSkill(ctx, k)
}

// ListSkills returns the skills, oldest first.
func (s *Service) ListSkills(ctx context.Context) ([]store.Skill, error) { return s.st.ListSkills(ctx) }

// RemoveSkill removes a skill; its history stays.
func (s *Service) RemoveSkill(ctx context.Context, id int64) error { return s.st.RemoveSkill(ctx, id) }
