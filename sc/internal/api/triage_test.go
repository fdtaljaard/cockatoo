//go:build windows

package api

import (
	"net/http"
	"strings"
	"testing"

	"cockatoo/internal/account"
)

func TestTriageRoutes(t *testing.T) {
	h, _ := newApp(t)
	if code, b := do(t, h, "POST", "/api/accounts", account.Input{Email: testEmail, Password: testPassword}); code != http.StatusCreated {
		t.Fatalf("add: %d %s", code, b)
	}

	// No answer yet.
	if code, b := do(t, h, "GET", "/api/triage?scope=all", nil); code != 200 || strings.TrimSpace(b) != "null" {
		t.Fatalf("before: %d %s", code, b)
	}
	// Not an account of yours.
	if code, _ := do(t, h, "GET", "/api/triage?scope=42", nil); code != http.StatusBadRequest {
		t.Fatalf("unknown scope: %d", code)
	}
	// Nothing unread: the answer says so, and nothing is sent to the AI (it is not even on).
	code, b := do(t, h, "POST", "/api/triage", map[string]string{"scope": "all"})
	if code != 200 || !strings.Contains(b, "Nothing unread") || !strings.Contains(b, `"id":0`) {
		t.Fatalf("empty inbox: %d %s", code, b)
	}
}
