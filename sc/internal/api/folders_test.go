//go:build windows

package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"cockatoo/internal/store"
)

func TestFolderRoutes(t *testing.T) {
	h, _ := newApp(t)
	root := t.TempDir()

	if code, b := do(t, h, "POST", "/api/folders", map[string]string{"name": "Expenses", "path": "not\absolute"}); code != http.StatusBadRequest {
		t.Fatalf("relative path: %d %s", code, b)
	}
	code, b := do(t, h, "POST", "/api/folders", map[string]string{"name": "Expenses", "path": root})
	if code != http.StatusCreated || !strings.Contains(b, `"id":1`) {
		t.Fatalf("add: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/folders", map[string]string{"name": "expenses", "path": root}); code != http.StatusConflict {
		t.Fatalf("duplicate: %d", code)
	}
	if code, b := do(t, h, "POST", "/api/folders/1/subfolders", map[string]string{"path": "", "name": "2026-11"}); code != http.StatusCreated || !strings.Contains(b, `"path":"2026-11"`) {
		t.Fatalf("subfolder: %d %s", code, b)
	}
	if code, b := do(t, h, "GET", "/api/folders/1/browse?path=", nil); code != 200 || !strings.Contains(b, `"folders":["2026-11"]`) {
		t.Fatalf("browse: %d %s", code, b)
	}
	if code, _ := do(t, h, "GET", "/api/folders/1/browse?path=..", nil); code != http.StatusBadRequest {
		t.Fatalf("browse up: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/folders/1/subfolders", map[string]string{"path": "", "name": "../x"}); code != http.StatusBadRequest {
		t.Fatalf("bad name: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/attachments/save", map[string]any{"folderId": 1, "path": "", "messages": []any{}}); code != http.StatusBadRequest {
		t.Fatalf("nothing chosen: %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/api/folders/1", nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d", code)
	}
}

// Done and not done are recorded in the audit and shown on the Inbox list.
func TestMarkDone(t *testing.T) {
	h, st := newApp(t)
	ctx := context.Background()
	a, err := st.AddAccount(ctx, store.Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddMessages(ctx, a.ID, []store.Message{{UIDValidity: 7, UID: 5, MessageID: "m@x", FromAddr: "ann@example.com",
		Subject: "lunch", ReceivedAt: time.Now().UTC()}}); err != nil {
		t.Fatal(err)
	}
	key := map[string]any{"accountId": a.ID, "uidValidity": 7, "uid": 5}
	if code, b := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key}, "done": true}); code != 200 || !strings.Contains(b, `"marked":1`) {
		t.Fatalf("done: %d %s", code, b)
	}
	if code, b := do(t, h, "GET", "/api/inbox", nil); code != 200 || !strings.Contains(b, `"doneAt"`) {
		t.Fatalf("inbox after done: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key}, "done": false}); code != 200 {
		t.Fatalf("not done: %d", code)
	}
	if _, b := do(t, h, "GET", "/api/inbox", nil); strings.Contains(b, `"handled"`) {
		t.Fatalf("inbox after not done: %s", b)
	}
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{}, "done": true}); code != http.StatusBadRequest {
		t.Fatalf("nothing chosen: %d", code)
	}
	if _, b := do(t, h, "GET", "/api/audit", nil); !strings.Contains(b, `"kind":"done"`) || !strings.Contains(b, `"kind":"not_done"`) {
		t.Fatalf("audit: %s", b)
	}
}

// A routine points out the mail it is for, in the Inbox's waiting list, until it is dealt with.
func TestRoutineRoutes(t *testing.T) {
	h, st := newApp(t)
	ctx := context.Background()
	if code, b := do(t, h, "POST", "/api/folders", map[string]string{"name": "Expenses", "path": t.TempDir()}); code != http.StatusCreated {
		t.Fatalf("add folder: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/routines", map[string]any{"from": "nodot", "folderId": 1}); code != http.StatusBadRequest {
		t.Fatalf("bad sender: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/routines", map[string]any{"from": "acme.example", "folderId": 9}); code != http.StatusNotFound {
		t.Fatalf("unknown folder: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/routines", map[string]any{"from": "acme.example", "folderId": 1, "path": "../x"}); code != http.StatusBadRequest {
		t.Fatalf("path out of the folder: %d", code)
	}
	routine := map[string]any{"from": "@ACME.example", "subject": "  Invoice ", "folderId": 1, "path": "", "dated": "month"}
	if code, b := do(t, h, "POST", "/api/routines", routine); code != http.StatusCreated || !strings.Contains(b, `"from":"acme.example"`) || !strings.Contains(b, `"subject":"invoice"`) {
		t.Fatalf("add: %d %s", code, b)
	}
	// A second rule for the same mail replaces the first: the same mail never has two lists.
	if code, _ := do(t, h, "POST", "/api/routines", routine); code != http.StatusCreated {
		t.Fatalf("replace: %d", code)
	}
	if _, b := do(t, h, "GET", "/api/routines", nil); strings.Count(b, `"from":"acme.example"`) != 1 {
		t.Fatalf("replaced, not added: %s", b)
	}

	a, err := st.AddAccount(ctx, store.Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	arrived := time.Now().UTC().Add(time.Second) // after the routine was made
	if err := st.AddMessages(ctx, a.ID, []store.Message{
		{UIDValidity: 7, UID: 1, MessageID: "i@x", FromAddr: "billing@acme.example", Subject: "Your invoice", ReceivedAt: arrived},
		{UIDValidity: 7, UID: 2, MessageID: "j@x", FromAddr: "billing@acme.example", Subject: "Newsletter", ReceivedAt: arrived},
	}); err != nil {
		t.Fatal(err)
	}
	month := arrived.Local().Format("2006-01")
	code, b := do(t, h, "GET", "/api/inbox", nil)
	if code != 200 || strings.Count(b, `"routineId":2`) != 2 || !strings.Contains(b, `"name":"`+month+`"`) || !strings.Contains(b, `"waiting":[{`) {
		t.Fatalf("inbox: %d %s", code, b) // once on the row, once in the waiting list
	}
	key := map[string]any{"accountId": a.ID, "uidValidity": 7, "uid": 1}
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key}, "done": true}); code != 200 {
		t.Fatalf("done: %d", code)
	}
	if _, b := do(t, h, "GET", "/api/inbox", nil); !strings.Contains(b, `"waiting":[]`) || strings.Contains(b, `"routineId"`) {
		t.Fatalf("after done: %s", b)
	}
	if code, b := do(t, h, "GET", "/api/routines", nil); code != 200 || !strings.Contains(b, `"folderName":"Expenses"`) || !strings.Contains(b, `"missing":false`) {
		t.Fatalf("list: %d %s", code, b)
	}
	if code, _ := do(t, h, "DELETE", "/api/routines/2", nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d", code)
	}
}

// Search reads the whole copy, not only the last 24 hours, with the marks the Inbox shows.
func TestSearchRoute(t *testing.T) {
	h, st := newApp(t)
	ctx := context.Background()
	a, err := st.AddAccount(ctx, store.Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddMessages(ctx, a.ID, []store.Message{
		{UIDValidity: 7, UID: 1, MessageID: "o@x", FromAddr: "billing@acme.example", Subject: "Invoice October", ReceivedAt: time.Now().UTC().Add(-40 * 24 * time.Hour)},
		{UIDValidity: 7, UID: 2, MessageID: "l@x", FromAddr: "ann@example.com", Subject: "Lunch", ReceivedAt: time.Now().UTC()},
	}); err != nil {
		t.Fatal(err)
	}
	key := map[string]any{"accountId": a.ID, "uidValidity": 7, "uid": 1}
	if code, _ := do(t, h, "POST", "/api/messages/done", map[string]any{"messages": []any{key}, "done": true}); code != 200 {
		t.Fatalf("done: %d", code)
	}
	code, b := do(t, h, "GET", "/api/inbox/search?q=acme+invoice&account=all", nil)
	if code != 200 || !strings.Contains(b, "Invoice October") || strings.Contains(b, "Lunch") || !strings.Contains(b, `"doneAt"`) || !strings.Contains(b, `"more":false`) {
		t.Fatalf("search: %d %s", code, b)
	}
	if code, _ := do(t, h, "GET", "/api/inbox/search?q=x&account=nope", nil); code != http.StatusBadRequest {
		t.Fatalf("bad account: %d", code)
	}
}

func TestConversationRoute(t *testing.T) {
	h, _ := newApp(t)
	if code, _ := do(t, h, "GET", "/api/conversation?account=1&uidValidity=x&uid=1", nil); code != http.StatusBadRequest {
		t.Fatalf("bad key: %d", code)
	}
	if code, _ := do(t, h, "GET", "/api/conversation?account=9&uidValidity=1&uid=1", nil); code != http.StatusNotFound {
		t.Fatalf("unknown account: %d", code)
	}
}
