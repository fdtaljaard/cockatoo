//go:build windows

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cockatoo/internal/account"
)

func TestGoogleRoutes(t *testing.T) {
	h, _ := newApp(t)
	var opened []string
	testAccounts.UseGoogleEndpoints("https://accounts.example/auth", "https://accounts.example/token", func(u string) error {
		opened = append(opened, u)
		return nil
	})
	testAccounts.SetBaseURL("http://127.0.0.1:7332")

	if code, b := do(t, h, "GET", "/api/google", nil); code != 200 || !strings.Contains(b, `"configured":false`) {
		t.Fatalf("settings before: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/google/signin", map[string]string{}); code != http.StatusConflict {
		t.Fatalf("sign-in before setup: %d", code)
	}
	if code, _ := do(t, h, "PUT", "/api/google", map[string]string{"clientId": "x", "clientSecret": "y"}); code != http.StatusBadRequest {
		t.Fatalf("bad client id: %d", code)
	}
	code, b := do(t, h, "PUT", "/api/google", map[string]string{"clientId": "1-a.apps.googleusercontent.com", "clientSecret": "s3cret"})
	if code != 200 || strings.Contains(b, "s3cret") || !strings.Contains(b, `"configured":true`) {
		t.Fatalf("save client: %d %s", code, b)
	}

	code, b = do(t, h, "POST", "/api/google/signin", map[string]string{})
	var start account.SignInStart
	if err := json.Unmarshal([]byte(b), &start); err != nil || code != 200 || !start.Opened || len(opened) != 1 {
		t.Fatalf("start: %d %s", code, b)
	}

	// The user says no on Google's page; the browser comes back with an error and a hostile code.
	// The page says so in cockatoo's words and echoes nothing from the address.
	req := httptest.NewRequest("GET", "http://127.0.0.1:7332/oauth/google/callback?state="+start.State+
		"&error=access_denied&code=%3Cscript%3Ealert(1)%3C%2Fscript%3E", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	page := rec.Body.String()
	if rec.Code != http.StatusBadRequest || !strings.Contains(page, "did not allow") || strings.Contains(page, "<script>") ||
		strings.Contains(page, start.State) || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("callback page: %d %s", rec.Code, page)
	}
	if _, b := do(t, h, "GET", "/api/google/signin/"+start.State, nil); !strings.Contains(b, `"state":"failed"`) {
		t.Fatalf("status after denial: %s", b)
	}

	// A callback for a state cockatoo never started.
	req = httptest.NewRequest("GET", "http://127.0.0.1:7332/oauth/google/callback?state=nope&code=x", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not one cockatoo started") {
		t.Fatalf("unknown state: %d %s", rec.Code, rec.Body.String())
	}

	// Another site cannot drive the callback through the browser.
	req = httptest.NewRequest("GET", "http://evil.example:7332/oauth/google/callback?state="+start.State, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: %d", rec.Code)
	}
}
