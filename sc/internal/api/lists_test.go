package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"cockatoo/internal/store"
)

// Mail sits in exactly one list: where the user put it, else the most specific rule, else
// Unsorted. Dealt-with mail leaves its list; new unsorted mail is shown at the top.
func TestListRoutes(t *testing.T) {
	h, st := newApp(t)
	ctx := context.Background()
	a, err := st.AddAccount(ctx, store.Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.AddMessages(ctx, a.ID, []store.Message{
		{UIDValidity: 7, UID: 1, MessageID: "1@x", FromAddr: "boss@work.example", Subject: "Budget?", ReceivedAt: now.Add(-time.Hour)},
		{UIDValidity: 7, UID: 2, MessageID: "2@x", FromAddr: "billing@acme.example", Subject: "Invoice", ReceivedAt: now.Add(-48 * time.Hour)},
		{UIDValidity: 7, UID: 3, MessageID: "3@x", FromAddr: "news@paper.example", Subject: "Old news", ReceivedAt: now.Add(-40 * 24 * time.Hour)},
		{UIDValidity: 7, UID: 4, MessageID: "4@x", FromAddr: "ann@friend.example", Subject: "Lunch", ReceivedAt: now.Add(-2 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	// A domain rule, then a more specific address rule for the same domain.
	if code, b := do(t, h, "POST", "/api/routines", map[string]any{"from": "work.example", "list": "attention"}); code != http.StatusCreated {
		t.Fatalf("domain rule: %d %s", code, b)
	}
	if code, b := do(t, h, "POST", "/api/routines", map[string]any{"from": "boss@work.example", "list": "reply"}); code != http.StatusCreated {
		t.Fatalf("address rule: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/routines", map[string]any{"from": "x.example", "list": "later"}); code != http.StatusBadRequest {
		t.Fatalf("unknown list: %d", code)
	}

	type view struct {
		Counts   map[string]int  `json:"counts"`
		Done     int             `json:"done"`
		Messages []store.Message `json:"messages"`
		New      []store.Message `json:"new"`
	}
	get := func(q string) view {
		t.Helper()
		code, b := do(t, h, "GET", "/api/lists?"+q, nil)
		if code != 200 {
			t.Fatalf("lists %s: %d %s", q, code, b)
		}
		var v view
		if err := json.Unmarshal([]byte(b), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := get("list=reply")
	if len(v.Messages) != 1 || v.Messages[0].UID != 1 || v.Messages[0].Sort.By != store.ByRule {
		t.Fatalf("reply by the address rule: %+v", v.Messages)
	}
	if v.Counts["reply"] != 1 || v.Counts["unsorted"] != 2 || v.Counts["attention"] != 0 {
		t.Fatalf("counts in 30 days: %v", v.Counts)
	}
	if len(v.New) != 1 || v.New[0].UID != 4 {
		t.Fatalf("new unsorted mail: %+v", v.New)
	}
	if v := get("list=unsorted&all=1"); v.Counts["unsorted"] != 3 || len(v.Messages) != 3 {
		t.Fatalf("all mail: %v %d", v.Counts, len(v.Messages))
	}

	// The user's choice beats the rules.
	key := func(uid int) map[string]any { return map[string]any{"accountId": a.ID, "uidValidity": 7, "uid": uid} }
	if code, _ := do(t, h, "POST", "/api/messages/sort", map[string]any{"messages": []any{key(2)}, "list": "save"}); code != http.StatusNoContent {
		t.Fatalf("sort: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/messages/sort", map[string]any{"messages": []any{key(1)}, "list": "attention", "confirmed": true}); code != http.StatusNoContent {
		t.Fatalf("confirm: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/messages/sort", map[string]any{"messages": []any{key(1)}, "list": "nowhere"}); code != http.StatusBadRequest {
		t.Fatalf("unknown list: %d", code)
	}
	if v := get("list=save"); len(v.Messages) != 1 || v.Messages[0].UID != 2 || v.Messages[0].Sort.By != store.ByYou {
		t.Fatalf("saved by hand: %+v", v.Messages)
	}
	if v := get("list=attention"); len(v.Messages) != 1 || v.Messages[0].Sort.By != store.ByAI {
		t.Fatalf("confirmed suggestion: %+v", v.Messages)
	}

	// Done mail leaves its list and the counts, and comes back with done=1.
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key(4)}, "done": true}); code != 200 {
		t.Fatalf("done: %d", code)
	}
	v = get("list=unsorted")
	if v.Counts["unsorted"] != 0 || len(v.Messages) != 0 || v.Done != 1 || len(v.New) != 0 {
		t.Fatalf("after done: %+v", v)
	}
	if v := get("list=unsorted&done=1"); len(v.Messages) != 1 {
		t.Fatalf("show done: %+v", v.Messages)
	}
	// Nothing to do: a rule sends mail there, out of every other list, and a narrower rule still wins.
	if code, b := do(t, h, "POST", "/api/routines", map[string]any{"from": "acme.example", "list": "ignore"}); code != http.StatusCreated {
		t.Fatalf("ignore rule: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/messages/sort", map[string]any{"messages": []any{key(2)}, "list": "unsorted"}); code != http.StatusNoContent {
		t.Fatalf("back to unsorted: %d", code)
	}
	if v := get("list=ignore"); v.Counts["ignore"] != 0 || v.Counts["unsorted"] != 1 {
		t.Fatalf("your choice beats the ignore rule: %v", v.Counts)
	}
	if code, b := do(t, h, "POST", "/api/routines", map[string]any{"from": "ann@friend.example", "list": "ignore"}); code != http.StatusCreated {
		t.Fatalf("ignore rule: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key(4)}, "done": false}); code != 200 {
		t.Fatalf("not done: %d", code)
	}
	if v := get("list=ignore"); v.Counts["ignore"] != 1 || len(v.Messages) != 1 || v.Messages[0].UID != 4 || len(v.New) != 0 {
		t.Fatalf("ignored by rule: %+v", v)
	}
	if code, b := do(t, h, "GET", "/api/lists?list=later", nil); code != http.StatusBadRequest || !strings.Contains(b, "later") {
		t.Fatalf("unknown list: %d %s", code, b)
	}
}
