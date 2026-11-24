package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

const (
	testClientID = "123-abc.apps.googleusercontent.com"
	goodCode     = "4/good-code"
	goodRefresh  = "1//refresh-token"
	goodAccess   = "ya29.access-token"
)

// fakeGoogle stands in for Google's token endpoint. It checks what cockatoo sends: the client,
// the PKCE verifier for the code it issued, and the refresh token.
type fakeGoogle struct {
	mu        sync.Mutex
	challenge string // from the sign-in URL, to check the verifier against
	email     string
	noRefresh bool
	revoked   bool
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()
	fail := func(code, desc string) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
	}
	if r.Form.Get("client_id") != testClientID || r.Form.Get("client_secret") != "the-secret" {
		fail("invalid_client", "The OAuth client was not found.")
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		if r.Form.Get("code") != goodCode || r.Form.Get("code_verifier") == "" || !strings.HasSuffix(r.Form.Get("redirect_uri"), "/oauth/google/callback") {
			fail("invalid_grant", "Bad Request")
			return
		}
		claims, _ := json.Marshal(map[string]any{"email": f.email, "email_verified": true})
		idToken := "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
		answer := map[string]any{"access_token": goodAccess, "expires_in": 3599, "token_type": "Bearer", "id_token": idToken}
		if !f.noRefresh {
			answer["refresh_token"] = goodRefresh
		}
		_ = json.NewEncoder(w).Encode(answer)
	case "refresh_token":
		if f.revoked || r.Form.Get("refresh_token") != goodRefresh {
			fail("invalid_grant", "Token has been expired or revoked.")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": goodAccess, "expires_in": 3599, "token_type": "Bearer"})
	default:
		fail("unsupported_grant_type", "")
	}
}

// googleSetup is a service with the Google client saved, Google stood in for, and a mailbox that
// takes only goodAccess.
func googleSetup(t *testing.T) (*Service, *fakeGoogle, *[]string) {
	t.Helper()
	s, _ := newService(t)
	s.seal = func(b []byte) ([]byte, error) { return append([]byte("sealed:"), b...), nil }
	s.open = func(b []byte) ([]byte, error) { return []byte(strings.TrimPrefix(string(b), "sealed:")), nil }
	g := &fakeGoogle{email: "Me@Work.example"}
	srv := httptest.NewServer(g)
	t.Cleanup(srv.Close)
	var opened []string
	s.UseGoogleEndpoints(srv.URL+"/auth", srv.URL+"/token", func(u string) error { opened = append(opened, u); return nil })
	s.SetBaseURL("http://127.0.0.1:7332")
	s.check = func(ctx context.Context, _ imapc.Server, l imapc.Login) (imapc.Inbox, error) {
		if l.Token == nil {
			return imapc.Inbox{}, errors.New("expected a token login")
		}
		tok, err := l.Token(ctx)
		if err != nil {
			return imapc.Inbox{}, err
		}
		if tok != goodAccess {
			return imapc.Inbox{}, &imapc.TokenError{Text: "Invalid credentials"}
		}
		return imapc.Inbox{Messages: 3}, nil
	}
	return s, g, &opened
}

func stateOf(t *testing.T, authURL string) (state, challenge string) {
	t.Helper()
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("access_type") != "offline" || q.Get("prompt") != "consent" ||
		!strings.Contains(q.Get("scope"), "https://mail.google.com/") || q.Get("redirect_uri") != "http://127.0.0.1:7332/oauth/google/callback" {
		t.Fatalf("sign-in URL %s", authURL)
	}
	return q.Get("state"), q.Get("code_challenge")
}

func TestGoogleSignIn(t *testing.T) {
	s, g, opened := googleSetup(t)
	ctx := context.Background()

	if _, err := s.StartGoogleSignIn(ctx, ""); !errors.As(err, new(*ConflictError)) {
		t.Fatalf("before setup: %v", err)
	}
	if _, err := s.SaveGoogleClient(ctx, "not-a-client-id", "x"); !errors.As(err, new(*InvalidError)) {
		t.Fatalf("bad client id: %v", err)
	}
	if settings, err := s.SaveGoogleClient(ctx, " "+testClientID+" ", " the-secret "); err != nil || !settings.Configured || settings.ClientID != testClientID {
		t.Fatalf("save client: %+v %v", settings, err)
	}

	start, err := s.StartGoogleSignIn(ctx, "")
	if err != nil || !start.Opened || len(*opened) != 1 {
		t.Fatalf("start: %+v %v", start, err)
	}
	state, _ := stateOf(t, start.URL)
	if state != start.State {
		t.Fatalf("state %q, want %q", state, start.State)
	}
	if st := s.GoogleSignInStatus(state); st.State != "waiting" {
		t.Fatalf("status before the callback: %+v", st)
	}

	// Google sends the browser back: the account is saved, as a Google account, with the
	// address from Google's answer, its refresh token encrypted.
	st := s.FinishGoogleSignIn(ctx, state, goodCode, "")
	if st.State != "done" || st.Account == nil || st.Account.Email != "me@work.example" || st.Account.Auth != store.AuthGoogle {
		t.Fatalf("finish: %+v", st)
	}
	_, sealed, err := s.st.AccountSecret(ctx, st.Account.ID)
	if err != nil || string(sealed) != "sealed:"+goodRefresh {
		t.Fatalf("stored secret %q %v", sealed, err)
	}
	if polled := s.GoogleSignInStatus(state); polled.State != "done" {
		t.Fatalf("status after: %+v", polled)
	}

	// The same link twice is refused.
	if again := s.FinishGoogleSignIn(ctx, state, goodCode, ""); again.State != "failed" || !strings.Contains(again.Error, "already used") {
		t.Fatalf("reused state: %+v", again)
	}
	if unknown := s.FinishGoogleSignIn(ctx, "made-up", goodCode, ""); unknown.State != "failed" {
		t.Fatalf("unknown state: %+v", unknown)
	}

	// The saved account connects with a fresh access token from its refresh token.
	if inbox, err := s.Inbox(ctx, st.Account.ID); err != nil || inbox.Messages != 3 {
		t.Fatalf("inbox: %+v %v", inbox, err)
	}

	// Revoked: the error says to sign in again, with Google's words.
	g.mu.Lock()
	g.revoked = true
	g.mu.Unlock()
	_, err = s.Inbox(ctx, st.Account.ID)
	if err == nil || !strings.Contains(err.Error(), "Sign in with Google again") || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("revoked: %v", err)
	}

	// Signing in again updates the same account, not a new one.
	g.mu.Lock()
	g.revoked = false
	g.mu.Unlock()
	start, _ = s.StartGoogleSignIn(ctx, "me@work.example")
	if !strings.Contains(start.URL, "login_hint=me%40work.example") {
		t.Fatalf("hint missing: %s", start.URL)
	}
	if st2 := s.FinishGoogleSignIn(ctx, start.State, goodCode, ""); st2.State != "done" || st2.Account.ID != st.Account.ID {
		t.Fatalf("sign in again: %+v", st2)
	}
	if list, _ := s.List(ctx); len(list) != 2 { // the password account from newService, and this one
		t.Fatalf("%d accounts, want 2", len(list))
	}
}

func TestGoogleSignInRefusals(t *testing.T) {
	s, g, _ := googleSetup(t)
	ctx := context.Background()
	if _, err := s.SaveGoogleClient(ctx, testClientID, "the-secret"); err != nil {
		t.Fatal(err)
	}
	try := func(code, googleErr string) SignInStatus {
		start, err := s.StartGoogleSignIn(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		return s.FinishGoogleSignIn(ctx, start.State, code, googleErr)
	}

	if st := try("", "access_denied"); st.State != "failed" || !strings.Contains(st.Error, "did not allow") {
		t.Errorf("denied: %+v", st)
	}
	if st := try("4/wrong", ""); st.State != "failed" || !strings.Contains(st.Error, "invalid_grant") {
		t.Errorf("bad code: %+v", st)
	}
	g.noRefresh = true
	if st := try(goodCode, ""); st.State != "failed" || !strings.Contains(st.Error, "lasting sign-in") {
		t.Errorf("no refresh token: %+v", st)
	}
	g.noRefresh = false

	// Signing in again as someone else does not change the account.
	start, _ := s.StartGoogleSignIn(ctx, "someone.else@work.example")
	if st := s.FinishGoogleSignIn(ctx, start.State, goodCode, ""); st.State != "failed" || !strings.Contains(st.Error, "not someone.else@work.example") {
		t.Errorf("other account: %+v", st)
	}
	if list, _ := s.List(ctx); len(list) != 1 { // only newService's password account
		t.Errorf("%d accounts after refusals, want 1", len(list))
	}
}

// Google's chooser lists every account the browser knows. Picking one that cockatoo already has
// with an app password must not swap its working password for a token.
func TestGoogleSignInKeepsPasswordAccounts(t *testing.T) {
	s, g, _ := googleSetup(t)
	ctx := context.Background()
	if _, err := s.SaveGoogleClient(ctx, testClientID, "the-secret"); err != nil {
		t.Fatal(err)
	}
	g.email = "me@example.com" // newService's password account
	start, _ := s.StartGoogleSignIn(ctx, "")
	st := s.FinishGoogleSignIn(ctx, start.State, goodCode, "")
	if st.State != "failed" || !strings.Contains(st.Error, "already added, with an app password") {
		t.Fatalf("password account: %+v", st)
	}
	list, _ := s.List(ctx)
	if len(list) != 1 || list[0].Auth != store.AuthPassword {
		t.Fatalf("accounts %+v", list)
	}

	// Asked for on purpose (signing that address in with Google), it is updated in place.
	start, _ = s.StartGoogleSignIn(ctx, "me@example.com")
	if st := s.FinishGoogleSignIn(ctx, start.State, goodCode, ""); st.State != "done" || st.Account.ID != list[0].ID || st.Account.Auth != store.AuthGoogle {
		t.Fatalf("on purpose: %+v", st)
	}
}

// Google signs the user in but Gmail will not open the mailbox (IMAP off): say that, not "sign in again".
func TestGoogleSignInMailboxRefused(t *testing.T) {
	s, _, _ := googleSetup(t)
	ctx := context.Background()
	if _, err := s.SaveGoogleClient(ctx, testClientID, "the-secret"); err != nil {
		t.Fatal(err)
	}
	s.check = func(context.Context, imapc.Server, imapc.Login) (imapc.Inbox, error) {
		return imapc.Inbox{}, &imapc.TokenError{Text: "IMAP access is disabled for your domain"}
	}
	start, _ := s.StartGoogleSignIn(ctx, "")
	st := s.FinishGoogleSignIn(ctx, start.State, goodCode, "")
	if st.State != "failed" || !strings.Contains(st.Error, "would not open the mailbox") || !strings.Contains(st.Error, "IMAP access is disabled") ||
		strings.Contains(st.Error, "Sign in with Google again") {
		t.Fatalf("refused mailbox: %+v", st)
	}
	if list, _ := s.List(ctx); len(list) != 1 {
		t.Fatalf("%d accounts, want 1", len(list))
	}
}
