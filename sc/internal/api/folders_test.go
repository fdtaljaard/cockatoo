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
