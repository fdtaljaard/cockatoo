package triage

import (
	"context"
	"strings"
	"testing"

	"cockatoo/internal/account"
	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// Each message is asked about once; attachment names go with it; only answers about messages
// that were sent, with a known list, are kept.
func TestSuggest(t *testing.T) {
	s, st, fake, id := setup(t, true)
	ctx := context.Background()
	var asked []uint32
	s.snippets = func(_ context.Context, _ int64, uids []uint32, _ int, withText bool) (map[uint32]account.Snippet, error) {
		asked = append(asked, uids...)
		out := map[uint32]account.Snippet{}
		for _, u := range uids {
			sn := account.Snippet{}
			if u == 2 {
				sn.Attachments = []imapc.AttachmentInfo{{Name: "budget-2027.xlsx", Size: 9000}}
			}
			if withText {
				sn.Text = "first lines"
			}
			out[u] = sn
		}
		return out, nil
	}
	key := func(uid uint32) store.MessageKey { return store.MessageKey{AccountID: id, UIDValidity: 7, UID: uid} }
	// Newest first: m1 the sale (uid 1), m2 the hostile subject (uid 3), m3 the budget (uid 2).
	fake.answer = `{"items":[
		{"ref":"m1","list":"done","reason":"A shop sale."},
		{"ref":"m3","list":"reply","reason":"Your manager asks for the budget figures by Friday.","skill":"when i reply"},
		{"ref":"m2","list":"later","reason":"not a list"},
		{"ref":"m99","list":"save","reason":"made up"}]}`
	r, err := s.Suggest(ctx, []store.MessageKey{key(1), key(2), key(3)}, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sent != 3 || len(r.Suggestions) != 2 || fake.calls != 1 {
		t.Fatalf("result %+v, calls %d", r, fake.calls)
	}
	if !strings.Contains(fake.got.Prompt, "attachments: budget-2027.xlsx") || strings.Contains(fake.got.Prompt, "first lines") {
		t.Fatalf("prompt: %s", fake.got.Prompt)
	}
	got := map[uint32]string{}
	for _, g := range r.Suggestions {
		got[g.Key.UID] = g.Suggestion.List
	}
	if got[1] != store.SuggestDone || got[2] != store.ListReply {
		t.Fatalf("suggestions %v", got)
	}
	for _, g := range r.Suggestions {
		if g.Key.UID == 2 && g.Suggestion.Skill != "When I reply" {
			t.Fatalf("the skill it followed, by its own name: %+v", g.Suggestion)
		}
	}
	if !strings.Contains(fake.got.Context, "<skills>") {
		t.Fatalf("skills sent: %q", fake.got.Context)
	}

	// Asked again: the two kept are not sent again; only the one without an answer is.
	fake.answer = `{"items":[{"ref":"m1","list":"attention","reason":"Odd subject."}]}`
	r, err = s.Suggest(ctx, []store.MessageKey{key(1), key(2), key(3)}, true)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sent != 1 || len(r.Suggestions) != 3 || fake.calls != 2 || !strings.Contains(fake.got.Prompt, "first lines") {
		t.Fatalf("second: %+v calls %d", r, fake.calls)
	}
	kept, err := st.Suggestions(ctx)
	if err != nil || len(kept) != 3 {
		t.Fatalf("kept %v %v", kept, err)
	}

	// All known: nothing is sent.
	if r, err := s.Suggest(ctx, []store.MessageKey{key(2)}, false); err != nil || r.Sent != 0 || fake.calls != 2 || len(r.Suggestions) != 1 {
		t.Fatalf("all known: %+v %v", r, err)
	}
	if len(asked) != 4 {
		t.Fatalf("read attachments of %v", asked)
	}
}
