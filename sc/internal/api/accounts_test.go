//go:build windows

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"cockatoo/internal/account"
	"cockatoo/internal/ai"
	"cockatoo/internal/mail"
	"cockatoo/internal/secret"
	"cockatoo/internal/store"
)

const (
	testEmail    = "me@example.com"
	testPassword = "abcdefghijklmnop"
)

// newApp is the whole backend against a temp DuckDB file and an in-memory IMAP server that
// stands in for Gmail. The encryption is the real one (Windows DPAPI).
func newApp(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	user := imapmemserver.NewUser(testEmail, testPassword)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem := imapmemserver.New()
	mem.AddUser(user)
	imapSrv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = imapSrv.Serve(ln) }()
	t.Cleanup(func() { _ = imapSrv.Close() })

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	accounts := account.NewFor(st, mail.Server{Addr: ln.Addr().String(), TLS: false})
	ui := fstest.MapFS{"index.html": {Data: []byte("<html></html>")}}
	return LocalOnly(Handler(st, accounts, ai.NewService(st), ui, "test")), st
}

func do(t *testing.T, h http.Handler, method, path string, body any) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:7332"+path, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestAccountLifecycle(t *testing.T) {
	h, _ := newApp(t)
	// The app password as Google shows it, in four groups; the spaces are not part of it.
	form := account.Input{Name: "Me", Email: " Me@Example.com ", Password: "abcd efgh ijkl mnop"}

	if code, body := do(t, h, "GET", "/api/accounts", nil); code != 200 || strings.TrimSpace(body) != "[]" {
		t.Fatalf("empty list: %d %s", code, body)
	}

	// A wrong password is refused and nothing is saved.
	bad := form
	bad.Password = "wrong"
	if code, body := do(t, h, "POST", "/api/accounts", bad); code != http.StatusBadGateway || !strings.Contains(body, "did not accept") {
		t.Fatalf("wrong password: %d %s", code, body)
	}
	if _, body := do(t, h, "GET", "/api/accounts", nil); strings.TrimSpace(body) != "[]" {
		t.Fatalf("a failed sign-in saved an account: %s", body)
	}

	// An empty form says what is missing.
	if code, body := do(t, h, "POST", "/api/accounts/test", account.Input{}); code != 400 || !strings.Contains(body, "Gmail address") {
		t.Fatalf("empty form: %d %s", code, body)
	}

	// Test, then save.
	if code, body := do(t, h, "POST", "/api/accounts/test", form); code != 200 || !strings.Contains(body, `"messages":0`) {
		t.Fatalf("test: %d %s", code, body)
	}
	code, body := do(t, h, "POST", "/api/accounts", form)
	if code != http.StatusCreated {
		t.Fatalf("add: %d %s", code, body)
	}
	var saved store.Account
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Email != testEmail || saved.Name != "Me" || saved.Provider != "gmail" || saved.ID == 0 {
		t.Fatalf("saved %+v", saved)
	}
	if strings.Contains(body, testPassword) || strings.Contains(strings.ToLower(body), "password") {
		t.Fatalf("the answer carries the password: %s", body)
	}

	// The same address twice is a conflict.
	if code, body := do(t, h, "POST", "/api/accounts", form); code != http.StatusConflict {
		t.Fatalf("duplicate: %d %s", code, body)
	}

	// The saved account signs in with its stored, encrypted password.
	if code, body := do(t, h, "GET", "/api/accounts/1/inbox", nil); code != 200 || !strings.Contains(body, `"messages":0`) {
		t.Fatalf("inbox: %d %s", code, body)
	}
	if code, body := do(t, h, "GET", "/api/accounts", nil); code != 200 || strings.Contains(body, testPassword) {
		t.Fatalf("list: %d %s", code, body)
	}

	// Remove, then it is gone.
	if code, body := do(t, h, "DELETE", "/api/accounts/1", nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", code, body)
	}
	if code, _ := do(t, h, "GET", "/api/accounts/1/inbox", nil); code != http.StatusNotFound {
		t.Fatalf("inbox after delete: %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/api/accounts/1", nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}
}

// The password must not be readable in the DuckDB file: what is stored is not the password, and
// it opens back to the password only through the encryption.
func TestPasswordIsEncryptedInTheFile(t *testing.T) {
	h, st := newApp(t)
	form := account.Input{Email: testEmail, Password: testPassword}
	if code, body := do(t, h, "POST", "/api/accounts", form); code != http.StatusCreated {
		t.Fatalf("add: %d %s", code, body)
	}
	_, sealed, err := st.AccountSecret(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sealed) == 0 || bytes.Contains(sealed, []byte(testPassword)) {
		t.Fatal("the stored secret is empty or contains the plain password")
	}
	plain, err := secret.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != testPassword {
		t.Fatalf("the stored secret opens to %q", plain)
	}
}

func TestLocalOnly(t *testing.T) {
	h, _ := newApp(t)

	// A page on another site that points its own name at 127.0.0.1.
	req := httptest.NewRequest("GET", "http://evil.example:7332/api/accounts", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign host: %d", rec.Code)
	}

	// A page on another site posting to the right address.
	req = httptest.NewRequest("POST", "http://127.0.0.1:7332/api/accounts", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign origin: %d", rec.Code)
	}

	// The app's own page.
	req = httptest.NewRequest("POST", "http://127.0.0.1:7332/api/accounts/test", strings.NewReader(`{}`))
	req.Header.Set("Origin", "http://127.0.0.1:7332")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest { // reached the handler: the empty form is refused there
		t.Errorf("own origin: %d", rec.Code)
	}
}
