package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

const goodKey = "sk-ant-test-0123456789abcdef"

// fakeAnthropic stands in for api.anthropic.com. It answers the two calls the client makes and
// records the last message request, so a test can check what was sent.
type fakeAnthropic struct {
	mu       sync.Mutex
	lastBody map[string]any
	lastBeta string
	reply    string // JSON of the message to return
	status   int    // for a message call: an error status to return instead
}

func (f *fakeAnthropic) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// The SDK sends the key as a bearer token; x-api-key is the older header. Accept either.
	if r.Header.Get("X-Api-Key") != goodKey && r.Header.Get("Authorization") != "Bearer "+goodKey {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
		return
	}
	switch {
	case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/models/"):
		id := strings.TrimPrefix(r.URL.Path, "/v1/models/")
		if id != DefaultModel {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"type":"error","error":{"type":"not_found_error","message":"model not found"}}`)
			return
		}
		io.WriteString(w, `{"type":"model","id":"`+id+`","display_name":"Claude","created_at":"2026-01-01T00:00:00Z"}`)
	case r.Method == "POST" && r.URL.Path == "/v1/messages":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.lastBody, f.lastBeta = body, r.Header.Get("Anthropic-Beta")
		reply, status := f.reply, f.status
		f.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			io.WriteString(w, `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
			return
		}
		io.WriteString(w, reply)
	default:
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"type":"error","error":{"type":"not_found_error","message":"no route"}}`)
	}
}

func message(stopReason, text, extra string) string {
	content := `[{"type":"thinking","thinking":"","signature":"x"}`
	if text != "" {
		content += `,{"type":"text","text":` + quote(text) + `}`
	}
	content += `]`
	return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":` + content +
		`,"stop_reason":"` + stopReason + `","stop_details":` + extra + `,"usage":{"input_tokens":10,"output_tokens":5}}`
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func newFake(t *testing.T) (*fakeAnthropic, option.RequestOption) {
	t.Helper()
	f := &fakeAnthropic{reply: message("end_turn", "Hello from Claude.", "null")}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, option.WithBaseURL(srv.URL)
}

func TestCheck(t *testing.T) {
	_, base := newFake(t)
	ctx := context.Background()

	if err := NewClient(goodKey, DefaultModel, base).Check(ctx); err != nil {
		t.Fatalf("good key: %v", err)
	}
	err := NewClient("sk-ant-wrong", DefaultModel, base).Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "did not accept this API key") {
		t.Fatalf("bad key: %v", err)
	}
	err = NewClient(goodKey, "no-such-model", base).Check(ctx)
	if err == nil || !strings.Contains(err.Error(), "does not offer the model no-such-model") {
		t.Fatalf("unknown model: %v", err)
	}
}

func TestAsk(t *testing.T) {
	f, base := newFake(t)
	reply, err := NewClient(goodKey, DefaultModel, base).Ask(context.Background(), "be brief", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Text != "Hello from Claude." || reply.Model != DefaultModel {
		t.Fatalf("reply %+v", reply)
	}

	// What was sent: the model, room to think, the fallback setting and its beta header.
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lastBody["model"] != DefaultModel || f.lastBody["max_tokens"] != float64(maxTokens) || f.lastBody["fallbacks"] != "default" {
		t.Errorf("body %v", f.lastBody)
	}
	if !strings.Contains(f.lastBeta, "server-side-fallback-2026-07-01") {
		t.Errorf("beta header %q", f.lastBeta)
	}
	if _, ok := f.lastBody["thinking"]; ok {
		t.Errorf("thinking was set; the model rejects anything but its default")
	}
}

func TestAskRefusalAndEmpty(t *testing.T) {
	f, base := newFake(t)
	c := NewClient(goodKey, DefaultModel, base)

	f.reply = message("refusal", "", `{"type":"refusal","category":null,"explanation":"not allowed"}`)
	if _, err := c.Ask(context.Background(), "", "x"); err == nil || !strings.Contains(err.Error(), "declined") {
		t.Fatalf("refusal: %v", err)
	}

	f.reply = message("max_tokens", "", "null")
	if _, err := c.Ask(context.Background(), "", "x"); err == nil || !strings.Contains(err.Error(), "ran out of room") {
		t.Fatalf("max tokens: %v", err)
	}
}

func TestAskServerError(t *testing.T) {
	f, base := newFake(t)
	f.status = http.StatusInternalServerError
	c := NewClient(goodKey, DefaultModel, base, option.WithMaxRetries(0))
	if _, err := c.Ask(context.Background(), "", "x"); err == nil || !strings.Contains(err.Error(), "busy or down") {
		t.Fatalf("server error: %v", err)
	}
}

func TestHint(t *testing.T) {
	if got := hint("sk-ant-api03-abcdefghijklmnopWXYZ"); got != "sk-ant-…WXYZ" {
		t.Errorf("hint %q", got)
	}
	if got := hint("short"); got != "…rt" {
		t.Errorf("short hint %q", got)
	}
}
