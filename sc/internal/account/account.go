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
	check  func(ctx context.Context, srv imap.Server, username, password string) (imap.Inbox, error)
	seal   func([]byte) ([]byte, error)
	open   func([]byte) ([]byte, error)
}

// New returns the service for Gmail accounts.
func New(st *store.Store) *Service { return NewFor(st, imap.Gmail) }

// NewFor returns the service for another IMAP server. The tests use it to stand in for Gmail.
func NewFor(st *store.Store, server imap.Server) *Service {
	return &Service{st: st, server: server, check: imap.CheckInbox, seal: secret.Seal, open: secret.Open}
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
	password, err := s.open(sealed)
	if err != nil {
		return imap.Inbox{}, fmt.Errorf("the saved password for %s could not be read; remove the account and add it again: %w", a.Email, err)
	}
	return s.connect(ctx, a.Email, string(password))
}

// Remove deletes a saved account and its encrypted password. The mailbox itself is not touched.
func (s *Service) Remove(ctx context.Context, id int64) error {
	return s.st.DeleteAccount(ctx, id)
}

func (s *Service) connect(ctx context.Context, email, password string) (imap.Inbox, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inbox, err := s.check(ctx, s.server, email, password)
	var refused *imap.LoginError
	if errors.As(err, &refused) {
		return imap.Inbox{}, fmt.Errorf("Gmail did not accept this address and app password. Check the address, and that this is "+
			"an app password and not your Google password. Gmail said: %s", refused.Text)
	}
	return inbox, err
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
