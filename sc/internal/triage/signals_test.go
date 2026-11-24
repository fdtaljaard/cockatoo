package triage

import (
	"testing"

	"cockatoo/internal/store"
)

func TestClassifyAndLeaveOut(t *testing.T) {
	facts := store.SentFacts{
		Complete:       true,
		MyAddresses:    map[string]bool{"me.alias@example.com": true},
		Correspondents: map[string]int{"ann@example.com": 14, "colleague@work.example": 3},
		SentIDs:        map[string]bool{"mine@x": true},
		Answered:       map[string]bool{"done@x": true},
	}
	cases := []struct {
		name    string
		c       store.Candidate
		leave   bool
		why     string
		toYou   bool
		replies bool
	}{
		{"a person writes to me directly", store.Candidate{Message: store.Message{FromAddr: "Ann@example.com", To: []string{"me@example.com"}}},
			false, "", true, false},
		{"to my alias counts as me", store.Candidate{Message: store.Message{FromAddr: "new@example.com", To: []string{"me.alias@example.com"}}},
			false, "", true, false},
		{"a newsletter", store.Candidate{Message: store.Message{FromAddr: "news@shop.example", To: []string{"me@example.com"}}, HasUnsubscribe: true},
			true, "automated", true, false},
		{"a notification sender", store.Candidate{Message: store.Message{FromAddr: "notifications@github.example", To: []string{"me@example.com"}}},
			true, "automated", true, false},
		{"bulk precedence", store.Candidate{Message: store.Message{FromAddr: "x@y.example", Precedence: "bulk"}}, true, "automated", false, false},
		{"auto-submitted", store.Candidate{Message: store.Message{FromAddr: "x@y.example", AutoSubmitted: "auto-replied"}}, true, "automated", false, false},
		{"a colleague through a team list stays", store.Candidate{Message: store.Message{FromAddr: "colleague@work.example",
			To: []string{"team@work.example"}, ListID: "<team.work.example>"}}, false, "", false, false},
		{"a list reply to my own mail stays", store.Candidate{Message: store.Message{FromAddr: "stranger@lists.example",
			ListID: "<l.example>", InReplyTo: "mine@x"}}, false, "", false, true},
		{"already answered", store.Candidate{Message: store.Message{FromAddr: "ann@example.com", MessageID: "done@x", To: []string{"me@example.com"}}},
			true, "answered", true, false},
		{"a stranger, no list marks, stays for the AI", store.Candidate{Message: store.Message{FromAddr: "someone@new.example", Cc: []string{"me@example.com"}}},
			false, "", false, false},
	}
	for _, c := range cases {
		s := classify(c.c, "Me@Example.com", facts)
		leave, why := leaveOut(s)
		if leave != c.leave || why != c.why || s.ToYou != c.toYou || s.RepliesToYou != c.replies {
			t.Errorf("%s: leave %v %q, signals %+v", c.name, leave, why, s)
		}
	}
	if s := classify(store.Candidate{Message: store.Message{FromAddr: "a@b.c", Cc: []string{"me@example.com"}}}, "me@example.com", facts); !s.CCYou || s.ToYou {
		t.Errorf("cc: %+v", s)
	}
	if s := classify(store.Candidate{Message: store.Message{FromAddr: "ann@example.com"}}, "me@example.com", facts); s.Wrote != 14 {
		t.Errorf("wrote: %+v", s)
	}
}
