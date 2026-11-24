package triage

import (
	"strings"

	"cockatoo/internal/store"
)

// Signals are what cockatoo knows, without the AI, about whether a message needs the user.
type Signals struct {
	ToYou        bool `json:"toYou"`        // the user is in To
	CCYou        bool `json:"ccYou"`        // the user is in Cc (and not in To)
	RepliesToYou bool `json:"repliesToYou"` // it answers a message the user sent
	Wrote        int  `json:"wrote"`        // messages the user has sent to this sender
	Automated    bool `json:"automated"`    // list, bulk or machine-sent mail
	Answered     bool `json:"-"`            // the user already replied to it
}

// classify works out a message's signals from its headers and the facts of the Sent folder.
// accountEmail counts as one of the user's addresses even before the Sent folder is read.
func classify(c store.Candidate, accountEmail string, f store.SentFacts) Signals {
	me := func(a string) bool { return a == strings.ToLower(accountEmail) || f.MyAddresses[a] }
	var s Signals
	for _, a := range c.To {
		if me(a) {
			s.ToYou = true
		}
	}
	if !s.ToYou {
		for _, a := range c.Cc {
			if me(a) {
				s.CCYou = true
			}
		}
	}
	s.RepliesToYou = c.InReplyTo != "" && f.SentIDs[c.InReplyTo]
	s.Wrote = f.Correspondents[strings.ToLower(c.FromAddr)]
	s.Answered = c.MessageID != "" && f.Answered[c.MessageID]
	s.Automated = c.ListID != "" || c.HasUnsubscribe || noReply(c.FromAddr) ||
		c.Precedence == "bulk" || c.Precedence == "list" || c.Precedence == "junk" ||
		(c.AutoSubmitted != "" && c.AutoSubmitted != "no")
	return s
}

// leaveOut says whether a message can be left out before the AI is asked, and why. Only on
// positive evidence: mail the user already answered, and automated mail from someone they have
// never written to that does not answer them. Anything a person might be waiting on stays: a
// reply to the user's own mail, or mail from someone they write to, is never left out, even when
// it came through a list.
func leaveOut(s Signals) (bool, string) {
	switch {
	case s.Answered:
		return true, "answered"
	case s.Automated && !s.RepliesToYou && s.Wrote == 0:
		return true, "automated"
	}
	return false, ""
}

// noReply is an address no person reads: no-reply, notifications, bounces.
func noReply(addr string) bool {
	local, _, _ := strings.Cut(strings.ToLower(addr), "@")
	for _, w := range []string{"noreply", "no-reply", "no_reply", "donotreply", "do-not-reply", "do_not_reply",
		"notification", "mailer-daemon", "postmaster", "bounce"} {
		if strings.Contains(local, w) {
			return true
		}
	}
	return false
}
