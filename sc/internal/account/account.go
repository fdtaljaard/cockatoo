// Package account is the use cases around mail accounts: test the details, save them with the
// password encrypted, read an account's inbox, remove it. It is the only caller of the mail
// server and of the encryption.
package account

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/secret"
	"cockatoo/internal/store"
)

// Input is what the setup form sends.
type Input struct {
	Name     string `json:"name"`     // shown on the account; the address when left empty
	Email    string `json:"email"`    // the Gmail address, which is also the sign-in name
	Password string `json:"password"` // a Google app password
}

// InvalidError says what is wrong with the form, in words for the user.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// Service wires the store, the mail server and the encryption together.
type Service struct {
	st     *store.Store
	server imap.Server
	check  func(ctx context.Context, srv imap.Server, login imap.Login) (imap.Inbox, error)
	dial   func(ctx context.Context, srv imap.Server, login imap.Login) (mailbox, error)
	// dialSent opens the Sent folder; nil in tests that do not need it.
	dialSent func(ctx context.Context, srv imap.Server, login imap.Login) (mailbox, bool, error)
	seal     func([]byte) ([]byte, error)
	open     func([]byte) ([]byte, error)

	openMover func(ctx context.Context, srv imap.Server, login imap.Login) (mover, error)

	google  googleState
	sync    syncer
	acting  bool           // an action is running (guarded by sync.mu)
	stopped bool           // StopSync was called: nothing new starts (guarded by sync.mu)
	actions sync.WaitGroup // running actions, for StopSync to wait on
}

// New returns the service for Gmail accounts.
func New(st *store.Store) *Service { return NewFor(st, imap.Gmail) }

// NewFor returns the service for another IMAP server. The tests use it to stand in for Gmail.
func NewFor(st *store.Store, server imap.Server) *Service {
	return &Service{
		st: st, server: server, check: imap.CheckInbox, dial: dialSession, dialSent: dialSentFolder, openMover: openMover,
		seal: secret.Seal, open: secret.Open,
		google: googleState{endpoint: googleEndpoint, open: openInBrowser, pending: map[string]*signIn{}},
	}
}

// timeout bounds one sign-in and inbox count.
const timeout = 30 * time.Second

// List returns the saved accounts, without passwords.
func (s *Service) List(ctx context.Context) ([]store.Account, error) {
	return s.st.ListAccounts(ctx)
}

// Test signs in with the details as typed and counts the inbox. Nothing is saved.
func (s *Service) Test(ctx context.Context, in Input) (imap.Inbox, error) {
	in, err := clean(in)
	if err != nil {
		return imap.Inbox{}, err
	}
	return s.connect(ctx, in.Email, in.Password)
}

// Add tests the details, and only when the sign-in works encrypts the password and saves the
// account. A failed test saves nothing.
func (s *Service) Add(ctx context.Context, in Input) (store.Account, error) {
	in, err := clean(in)
	if err != nil {
		return store.Account{}, err
	}
	if _, err := s.connect(ctx, in.Email, in.Password); err != nil {
		return store.Account{}, err
	}
	sealed, err := s.seal([]byte(in.Password))
	if err != nil {
		return store.Account{}, fmt.Errorf("encrypt the password: %w", err)
	}
	return s.st.AddAccount(ctx, store.Account{Provider: "gmail", Name: in.Name, Email: in.Email}, sealed)
}

// Inbox signs in to a saved account with its stored password and counts the inbox.
func (s *Service) Inbox(ctx context.Context, id int64) (imap.Inbox, error) {
	a, sealed, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return imap.Inbox{}, err
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return imap.Inbox{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inbox, err := s.check(ctx, s.server, login)
	if err != nil {
		return imap.Inbox{}, explainMail(err)
	}
	return inbox, nil
}

// Remove deletes a saved account and its encrypted password. The mailbox itself is not touched.
func (s *Service) Remove(ctx context.Context, id int64) error {
	return s.st.DeleteAccount(ctx, id)
}

func (s *Service) connect(ctx context.Context, email, password string) (imap.Inbox, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inbox, err := s.check(ctx, s.server, imap.PasswordLogin(email, password))
	if err != nil {
		return imap.Inbox{}, explainMail(err)
	}
	return inbox, nil
}

// explainMail puts a refused sign-in into words for the user; other errors pass through.
func explainMail(err error) error {
	var token *imap.TokenError
	if errors.As(err, &token) {
		return fmt.Errorf("Google sign-in for this account no longer works. Sign in with Google again. Google said: %s", token.Text)
	}
	var refused *imap.LoginError
	if errors.As(err, &refused) {
		return fmt.Errorf("Gmail did not accept this address and app password. Check the address, and that this is "+
			"an app password and not your Google password. Gmail said: %s", refused.Text)
	}
	return err
}

// clean trims the form and says what is missing. Google shows an app password in four groups
// with spaces; the spaces are not part of it.
func clean(in Input) (Input, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	in.Password = strings.Join(strings.Fields(in.Password), "")

	if in.Email == "" {
		return in, &InvalidError{"Enter your Gmail address."}
	}
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email {
		return in, &InvalidError{"That does not look like an email address."}
	}
	if in.Password == "" {
		return in, &InvalidError{"Enter the app password."}
	}
	if in.Name == "" {
		in.Name = in.Email
	}
	return in, nil
}
