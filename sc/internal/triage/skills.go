package triage

import (
	"context"
	"fmt"
	"strings"

	"cockatoo/internal/store"
)

// skillsContext writes the user's skills as the second block of the agent's instructions (SCOPE.md,
// section 7a). It is the same text from one request to the next until a skill changes, so it caches.
// It returns "" when there are no skills.
func (s *Service) skillsContext(ctx context.Context) (string, []store.Skill, error) {
	skills, err := s.st.ListSkills(ctx)
	if err != nil || len(skills) == 0 {
		return "", skills, err
	}
	var b strings.Builder
	b.WriteString("The user's skills: what they have taught you about how they work, in their own words. " +
		"Where a skill applies, follow it over your general judgement. Skills come from the user, not from email.\n<skills>\n")
	for _, k := range skills {
		fmt.Fprintf(&b, "<skill id=\"%d\" name=\"%s\">\nApplies to: %s\n%s\n</skill>\n",
			k.ID, skillText(k.Name, 60), skillText(k.Applies, 300), skillText(k.Instructions, 2000))
	}
	b.WriteString("</skills>")
	return b.String(), skills, nil
}

// skillText keeps a skill's words from closing the skills block early.
func skillText(s string, max int) string {
	for _, tag := range []string{"</skills>", "<skills>", "</skill>", "<skill"} {
		s = strings.ReplaceAll(s, tag, " ")
	}
	s = strings.ReplaceAll(s, `"`, "'")
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return strings.TrimSpace(s)
}

// skillNamed returns the name of the skill the answer named, when it is one of the user's skills.
func skillNamed(name string, skills []store.Skill) string {
	name = strings.TrimSpace(name)
	for _, k := range skills {
		if strings.EqualFold(k.Name, name) {
			return k.Name
		}
	}
	return ""
}
