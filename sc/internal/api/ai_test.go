//go:build windows

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"cockatoo/internal/account"
	"cockatoo/internal/ai"
	"cockatoo/internal/secret"
	"cockatoo/internal/store"
)

const aiKey = "sk-ant-api03-test-key-0123456789WXYZ"

// newAIApp is the whole backend with a stand-in for api.anthropic.com that accepts aiKey.
func newAIApp(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+aiKey && r.Header.Get("X-Api-Key") != aiKey {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		if r.Method == "GET" {
			io.WriteString(w, `{"type":"model","id":"claude-opus-5-5","display_name":"Claude","created_at":"2026-01-01T00:00:00Z"}`)
			return
		}
		io.WriteString(w, `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5",`+
			`"content":[{"type":"text","text":"Hello from Claude."}],"stop_reason":"end_turn","stop_details":null,`+
			`"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	t.Cleanup(fake.Close)

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "ai.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := ai.NewServiceFor(st, option.WithBaseURL(fake.URL))
	ui := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	return LocalOnly(Handler(st, account.New(st), svc, ui, "test")), st
}

func settingsOf(t *testing.T, body string) store.AIProvider {
	t.Helper()
	var p store.AIProvider
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("%v: %s", err, body)
	}
	return p
}

func TestAISettings(t *testing.T) {
	h, st := newAIApp(t)

	// Before any setup: off, default model, no key.
	code, body := do(t, h, "GET", "/api/ai", nil)
	if p := settingsOf(t, body); code != 200 || p.Enabled || p.HasKey || p.Model != ai.DefaultModel {
		t.Fatalf("start: %d %s", code, body)
	}
	if code, _ := do(t, h, "POST", "/api/ai/test-message", nil); code != http.StatusConflict {
		t.Fatalf("test message while off: %d", code)
	}

	// On, without a key.
	if _, body := do(t, h, "PUT", "/api/ai/enabled", map[string]bool{"enabled": true}); !settingsOf(t, body).Enabled {
		t.Fatalf("on: %s", body)
	}
	if code, body := do(t, h, "POST", "/api/ai/test-message", nil); code != http.StatusConflict || !strings.Contains(body, "No Anthropic API key") {
		t.Fatalf("test message without key: %d %s", code, body)
	}

	// An empty key and a wrong key are refused, and nothing is saved.
	if code, _ := do(t, h, "PUT", "/api/ai/key", map[string]string{"key": "  "}); code != http.StatusBadRequest {
		t.Fatalf("empty key: %d", code)
	}
	if code, body := do(t, h, "PUT", "/api/ai/key", map[string]string{"key": "sk-ant-wrong"}); code != http.StatusBadGateway || !strings.Contains(body, "did not accept") {
		t.Fatalf("wrong key: %d %s", code, body)
	}
	if _, body := do(t, h, "GET", "/api/ai", nil); settingsOf(t, body).HasKey {
		t.Fatalf("a refused key was saved: %s", body)
	}

	// The right key, pasted with a line break, is accepted and saved encrypted; it never comes back.
	code, body = do(t, h, "PUT", "/api/ai/key", map[string]string{"key": " " + aiKey + "\n"})
	p := settingsOf(t, body)
	if code != 200 || !p.HasKey || !p.Enabled || p.KeyHint != "sk-ant-…WXYZ" {
		t.Fatalf("save key: %d %s", code, body)
	}
	if strings.Contains(body, aiKey) {
		t.Fatalf("the answer carries the key: %s", body)
	}
	_, sealed, _, err := st.AIProviderSecret(context.Background(), "anthropic")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sealed), aiKey) {
		t.Fatal("the key is stored in plain text")
	}
	if plain, err := secret.Open(sealed); err != nil || string(plain) != aiKey {
		t.Fatalf("the stored key opens to %q, %v", plain, err)
	}

	// A wrong key afterwards keeps the saved one.
	do(t, h, "PUT", "/api/ai/key", map[string]string{"key": "sk-ant-wrong"})
	if _, body := do(t, h, "GET", "/api/ai", nil); settingsOf(t, body).KeyHint != "sk-ant-…WXYZ" {
		t.Fatalf("a refused key replaced the saved one: %s", body)
	}

	// The test message goes through with the saved key.
	code, body = do(t, h, "POST", "/api/ai/test-message", nil)
	if code != 200 || !strings.Contains(body, "Hello from Claude.") {
		t.Fatalf("test message: %d %s", code, body)
	}

	// Off keeps the key and stops the client.
	_, body = do(t, h, "PUT", "/api/ai/enabled", map[string]bool{"enabled": false})
	if p := settingsOf(t, body); p.Enabled || !p.HasKey {
		t.Fatalf("off: %s", body)
	}
	if code, _ := do(t, h, "POST", "/api/ai/test-message", nil); code != http.StatusConflict {
		t.Fatalf("test message after off: %d", code)
	}

	// Remove deletes the key.
	if _, body := do(t, h, "DELETE", "/api/ai/key", nil); settingsOf(t, body).HasKey {
		t.Fatalf("remove: %s", body)
	}
}
