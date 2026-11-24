package account

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// Google's own sign-in, for accounts that do not take an app password (Google Workspace). The
// desktop-app flow: cockatoo opens Google's sign-in page in the browser, Google sends the browser
// back to cockatoo on 127.0.0.1 with a one-time code, and cockatoo exchanges the code (with the
// PKCE verifier only it holds) for a refresh token. The refresh token is encrypted like a
// password; a fresh access token is asked for at every connection.

// googleScopes: full mail access (the only scope Gmail's IMAP takes), and the address of the
// account that signed in.
var googleScopes = []string{"https://mail.google.com/", "openid", "email"}

// googleEndpoint is Google's. The tests point it at a stand-in.
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// signInTimeout is how long a started sign-in waits for Google to send the browser back.
const signInTimeout = 10 * time.Minute

// googleState is the Google sign-in's own state on the service.
type googleState struct {
	mu       sync.Mutex
	endpoint oauth2.Endpoint
	baseURL  string // http://127.0.0.1:<port>, where Google sends the browser back
	open     func(url string) error
	pending  map[string]*signIn
}

type signIn struct {
	verifier string
	hint     string
	started  time.Time
	used     bool
	status   SignInStatus
}

// GoogleSettings is the Google sign-in setup as the app shows it. The client secret never leaves.
type GoogleSettings struct {
	Configured bool   `json:"configured"`
	ClientID   string `json:"clientId,omitempty"`
}

// SignInStart is a started sign-in: the page to open, and the state to ask about.
type SignInStart struct {
	State  string `json:"state"`
	URL    string `json:"url"`    // Google's sign-in page, for a link when the browser did not open
	Opened bool   `json:"opened"` // cockatoo opened it in the default browser
}

// SignInStatus is how a started sign-in is going.
type SignInStatus struct {
	State   string         `json:"state"` // "waiting", "done" or "failed"
	Account *store.Account `json:"account,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// SetBaseURL tells the service the address it is served on, which Google sends the browser back
// to. main sets it once the port is known.
func (s *Service) SetBaseURL(url string) {
	s.google.mu.Lock()
	defer s.google.mu.Unlock()
	s.google.baseURL = strings.TrimRight(url, "/")
}

// UseGoogleEndpoints points the sign-in at another server and replaces opening the browser. For
// the tests only.
func (s *Service) UseGoogleEndpoints(authURL, tokenURL string, open func(url string) error) {
	s.google.mu.Lock()
	defer s.google.mu.Unlock()
	s.google.endpoint = oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL, AuthStyle: oauth2.AuthStyleInParams}
	s.google.open = open
}

// CallbackPath is where Google sends the browser back to.
const CallbackPath = "/oauth/google/callback"

// GoogleSettings says whether the Google sign-in is set up.
func (s *Service) GoogleSettings(ctx context.Context) (GoogleSettings, error) {
	c, _, found, err := s.st.GetOAuthClient(ctx, "google")
	if err != nil || !found {
		return GoogleSettings{}, err
	}
	return GoogleSettings{Configured: true, ClientID: c.ClientID}, nil
}

// SaveGoogleClient saves the Google Cloud OAuth client (a "Desktop app" client) cockatoo signs in
// with. The secret is encrypted like a password.
func (s *Service) SaveGoogleClient(ctx context.Context, clientID, secret string) (GoogleSettings, error) {
	clientID, secret = strings.TrimSpace(clientID), strings.TrimSpace(secret)
	if !strings.HasSuffix(clientID, ".apps.googleusercontent.com") || strings.ContainsAny(clientID, " /?#") {
		return GoogleSettings{}, &InvalidError{"The client ID ends in .apps.googleusercontent.com. Copy it from the Google Cloud console."}
	}
	if secret == "" || strings.ContainsAny(secret, " \t\r\n") {
		return GoogleSettings{}, &InvalidError{"Paste the client secret from the same client in the Google Cloud console."}
	}
	sealed, err := s.seal([]byte(secret))
	if err != nil {
		return GoogleSettings{}, fmt.Errorf("encrypt the client secret: %w", err)
	}
	if err := s.st.SaveOAuthClient(ctx, "google", clientID, sealed); err != nil {
		return GoogleSettings{}, err
	}
	return s.GoogleSettings(ctx)
}

// googleConfig is the OAuth configuration from the saved client.
func (s *Service) googleConfig(ctx context.Context) (*oauth2.Config, error) {
	c, sealed, found, err := s.st.GetOAuthClient(ctx, "google")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &ConflictError{"Google sign-in is not set up yet. Add the client ID and secret first."}
	}
	secret, err := s.open(sealed)
	if err != nil {
		return nil, fmt.Errorf("the saved Google client secret could not be read; save it again: %w", err)
	}
	s.google.mu.Lock()
	defer s.google.mu.Unlock()
	if s.google.baseURL == "" {
		return nil, errors.New("cockatoo does not know its own address yet")
	}
	return &oauth2.Config{
		ClientID: c.ClientID, ClientSecret: string(secret), Endpoint: s.google.endpoint,
		RedirectURL: s.google.baseURL + CallbackPath, Scopes: googleScopes,
	}, nil
}

// StartGoogleSignIn opens Google's sign-in page in the default browser, where the user is
// probably already signed in to Google. hint, when given, is the address to sign in as (when
// signing in again).
func (s *Service) StartGoogleSignIn(ctx context.Context, hint string) (SignInStart, error) {
	cfg, err := s.googleConfig(ctx)
	if err != nil {
		return SignInStart{}, err
	}
	state := oauth2.GenerateVerifier() // 32 random bytes, as good for state as for PKCE
	verifier := oauth2.GenerateVerifier()
	opts := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"), // ask again, so Google gives a refresh token every time
		oauth2.S256ChallengeOption(verifier),
	}
	if hint = strings.TrimSpace(hint); hint != "" {
		opts = append(opts, oauth2.SetAuthURLParam("login_hint", hint))
	}
	url := cfg.AuthCodeURL(state, opts...)

	s.google.mu.Lock()
	if s.google.pending == nil {
		s.google.pending = map[string]*signIn{}
	}
	s.pruneSignIns()
	s.google.pending[state] = &signIn{verifier: verifier, hint: hint, started: time.Now(), status: SignInStatus{State: "waiting"}}
	open := s.google.open
	s.google.mu.Unlock()

	start := SignInStart{State: state, URL: url}
	if err := open(url); err == nil {
		start.Opened = true
	} else {
		log.Printf("google sign-in: could not open the browser: %v", err)
	}
	return start, nil
}

// GoogleSignInStatus says how a started sign-in is going.
func (s *Service) GoogleSignInStatus(state string) SignInStatus {
	s.google.mu.Lock()
	defer s.google.mu.Unlock()
	s.pruneSignIns()
	p, ok := s.google.pending[state]
	if !ok {
		return SignInStatus{State: "failed", Error: "This sign-in has expired. Start again."}
	}
	return p.status
}

// pruneSignIns forgets sign-ins started too long ago. Hold s.google.mu.
func (s *Service) pruneSignIns() {
	for k, p := range s.google.pending {
		if time.Since(p.started) > signInTimeout+5*time.Minute {
			delete(s.google.pending, k)
		}
	}
}

// FinishGoogleSignIn is where Google's answer arrives (the browser on the callback address). It
// checks the state, exchanges the code, tests the mailbox with the new token, and saves the
// account: a new one, or the same account again when the address is already there, so its copy
// and its audit stay. The answer is in words for the page the browser shows.
func (s *Service) FinishGoogleSignIn(ctx context.Context, state, code, googleError string) SignInStatus {
	s.google.mu.Lock()
	p, ok := s.google.pending[state]
	switch {
	case !ok || time.Since(p.started) > signInTimeout:
		s.google.mu.Unlock()
		return SignInStatus{State: "failed", Error: "This sign-in link has expired or is not one cockatoo started. Start again from cockatoo."}
	case p.used:
		s.google.mu.Unlock()
		return SignInStatus{State: "failed", Error: "This sign-in link was already used. Start again from cockatoo."}
	}
	p.used = true
	s.google.mu.Unlock()

	status := s.finish(ctx, p, code, googleError)
	s.google.mu.Lock()
	p.status = status
	s.google.mu.Unlock()
	return status
}

func (s *Service) finish(ctx context.Context, p *signIn, code, googleError string) SignInStatus {
	fail := func(msg string) SignInStatus { return SignInStatus{State: "failed", Error: msg} }
	if googleError != "" {
		if googleError == "access_denied" {
			return fail("You did not allow cockatoo to read your mail, so nothing was saved.")
		}
		return fail("Google did not sign you in (" + googleError + ").")
	}
	if code == "" {
		return fail("Google sent no sign-in code back.")
	}
	cfg, err := s.googleConfig(ctx)
	if err != nil {
		return fail(err.Error())
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(p.verifier))
	if err != nil {
		return fail("Google refused the sign-in: " + googleWords(err))
	}
	if tok.RefreshToken == "" {
		return fail("Google did not give cockatoo a lasting sign-in. Remove cockatoo under Third-party apps in your Google Account, then sign in again.")
	}
	email, err := emailFromIDToken(tok)
	if err != nil {
		return fail(err.Error())
	}
	if p.hint != "" && !strings.EqualFold(p.hint, email) {
		return fail(fmt.Sprintf("You signed in to Google as %s, not %s. Nothing was changed.", email, p.hint))
	}

	// The mailbox must open with the new token before anything is saved.
	access := tok.AccessToken
	if _, err := s.check(ctx, s.server, imap.Login{Username: email, Token: func(context.Context) (string, error) { return access, nil }}); err != nil {
		var te *imap.TokenError
		if errors.As(err, &te) {
			return fail(fmt.Sprintf("Google signed you in as %s, but Gmail would not open the mailbox with it. If your organisation's "+
				"admin has turned IMAP off, it has to be turned on first. Gmail said: %s", email, te.Text))
		}
		return fail(fmt.Sprintf("Google signed you in as %s, but the mailbox did not open: %s", email, err))
	}

	sealed, err := s.seal([]byte(tok.RefreshToken))
	if err != nil {
		return fail("Could not encrypt the sign-in: " + err.Error())
	}
	existing, found, err := s.st.AccountByEmail(ctx, email)
	if err != nil {
		return fail(err.Error())
	}
	if found && existing.Auth != store.AuthGoogle && p.hint == "" {
		return fail(fmt.Sprintf("%s is already added, with an app password. Nothing was changed.", email))
	}
	var saved store.Account
	if found {
		if err := s.st.UpdateAccountLogin(ctx, existing.ID, store.AuthGoogle, sealed); err != nil {
			return fail(err.Error())
		}
		saved, _, err = s.st.AccountSecret(ctx, existing.ID)
	} else {
		saved, err = s.st.AddAccount(ctx, store.Account{Provider: "gmail", Name: email, Email: email, Auth: store.AuthGoogle}, sealed)
	}
	if err != nil {
		return fail(err.Error())
	}
	return SignInStatus{State: "done", Account: &saved}
}

// emailFromIDToken reads the signed-in address from the ID token Google returned with the tokens.
// It came straight from Google's token endpoint over TLS, so its signature is not checked again
// (as Google's documentation allows for that case).
func emailFromIDToken(tok *oauth2.Token) (string, error) {
	raw, _ := tok.Extra("id_token").(string)
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", errors.New("Google did not say which account signed in")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.New("Google's answer about the account could not be read")
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Email == "" {
		return "", errors.New("Google did not say which address signed in")
	}
	if !claims.EmailVerified {
		return "", errors.New("Google says this account's address is not verified")
	}
	return strings.ToLower(claims.Email), nil
}

// googleWords is Google's own error code and description, without the response body.
func googleWords(err error) string {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		if re.ErrorDescription != "" {
			return re.ErrorCode + ": " + re.ErrorDescription
		}
		if re.ErrorCode != "" {
			return re.ErrorCode
		}
		if re.Response != nil {
			return re.Response.Status
		}
	}
	return err.Error()
}

// loginFor is how to sign in to a saved account: its app password, or a fresh Google access token
// from its refresh token.
func (s *Service) loginFor(ctx context.Context, a store.Account, sealed []byte) (imap.Login, error) {
	secret, err := s.open(sealed)
	if err != nil {
		return imap.Login{}, fmt.Errorf("the saved sign-in for %s could not be read; remove the account and add it again: %w", a.Email, err)
	}
	if a.Auth != store.AuthGoogle {
		return imap.PasswordLogin(a.Email, string(secret)), nil
	}
	cfg, err := s.googleConfig(ctx)
	if err != nil {
		return imap.Login{}, err
	}
	refresh := string(secret)
	return imap.Login{Username: a.Email, Token: func(ctx context.Context) (string, error) {
		tok, err := cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refresh}).Token()
		if err != nil {
			var re *oauth2.RetrieveError
			if errors.As(err, &re) {
				return "", &imap.TokenError{Text: googleWords(err)}
			}
			return "", fmt.Errorf("could not reach Google to sign in: %w", err)
		}
		return tok.AccessToken, nil
	}}, nil
}

// openInBrowser opens a web address in the user's default browser.
func openInBrowser(url string) error {
	if runtime.GOOS != "windows" {
		return errors.New("opening the browser is only done on Windows")
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
