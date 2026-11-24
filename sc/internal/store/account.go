package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Account is a mail account as the app shows it. The password or sign-in token is not part of
// it: it is kept in the secret column, encrypted, and only AccountSecret hands it out.
type Account struct {
	ID        int64     `json:"id"`
	Provider  string    `json:"provider"` // "gmail"
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Auth      string    `json:"auth"` // "password" (an app password) or "google" (Google's own sign-in)
	CreatedAt time.Time `json:"createdAt"`
}

// The ways an account signs in.
const (
	AuthPassword = "password"
	AuthGoogle   = "google"
)

var (
	// ErrNotFound means there is no account with that id.
	ErrNotFound = errors.New("no such account")
	// ErrDuplicate means an account with that address is already saved.
	ErrDuplicate = errors.New("this address is already added")
)

// ListAccounts returns every account, oldest first.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, provider, name, email, coalesce(auth, 'password'), created_at FROM account ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Provider, &a.Name, &a.Email, &a.Auth, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("list accounts: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AddAccount saves a new account with its password already encrypted (sealed). It never sees
// the plain password.
func (s *Store) AddAccount(ctx context.Context, a Account, sealed []byte) (Account, error) {
	if len(sealed) == 0 {
		return Account{}, errors.New("add account: no encrypted password")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM account WHERE email = ?`, a.Email).Scan(&n); err != nil {
		return Account{}, fmt.Errorf("add account: %w", err)
	}
	if n > 0 {
		return Account{}, ErrDuplicate
	}
	a.CreatedAt = time.Now().UTC().Truncate(time.Second)
	if a.Auth == "" {
		a.Auth = AuthPassword
	}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO account (provider, name, email, auth, secret, created_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		a.Provider, a.Name, a.Email, a.Auth, sealed, a.CreatedAt).Scan(&a.ID)
	if err != nil {
		return Account{}, fmt.Errorf("add account: %w", err)
	}
	return a, nil
}

// AccountSecret returns an account with its encrypted password.
func (s *Store) AccountSecret(ctx context.Context, id int64) (Account, []byte, error) {
	var a Account
	var sealed []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, provider, name, email, coalesce(auth, 'password'), created_at, secret FROM account WHERE id = ?`, id).
		Scan(&a.ID, &a.Provider, &a.Name, &a.Email, &a.Auth, &a.CreatedAt, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, nil, ErrNotFound
	}
	if err != nil {
		return Account{}, nil, fmt.Errorf("read account %d: %w", id, err)
	}
	return a, sealed, nil
}

// DeleteAccount removes an account, its encrypted password and its copy of the inbox from the file.
// The audit of its actions stays: it names the account by address, not only by id.
func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	defer tx.Rollback() //nolint:errcheck // after Commit this is a no-op

	var gone int64
	err = tx.QueryRowContext(ctx, `DELETE FROM account WHERE id = ? RETURNING id`, id).Scan(&gone)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM message WHERE account_id = ?`, id); err != nil {
		return fmt.Errorf("delete the messages of account %d: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM inbox_sync WHERE account_id = ?`, id); err != nil {
		return fmt.Errorf("delete the sync state of account %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	return nil
}

// AccountByEmail returns the account with this address; found is false when there is none.
func (s *Store) AccountByEmail(ctx context.Context, email string) (a Account, found bool, err error) {
	var id int64
	err = s.db.QueryRowContext(ctx, `SELECT id FROM account WHERE email = ?`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("find account %s: %w", email, err)
	}
	a, _, err = s.AccountSecret(ctx, id)
	return a, err == nil, err
}

// UpdateAccountLogin replaces how an account signs in, keeping the account itself: its id, its
// copy of the inbox and its audit. Used when the user signs in with Google again.
func (s *Store) UpdateAccountLogin(ctx context.Context, id int64, auth string, sealed []byte) error {
	if len(sealed) == 0 {
		return errors.New("update account: nothing encrypted to save")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var got int64
	err := s.db.QueryRowContext(ctx, `UPDATE account SET auth = ?, secret = ? WHERE id = ? RETURNING id`, auth, sealed, id).Scan(&got)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update account %d: %w", id, err)
	}
	return nil
}

// OAuthClient is the Google Cloud OAuth client cockatoo signs in with. The secret is kept apart,
// encrypted.
type OAuthClient struct {
	ClientID  string    `json:"clientId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GetOAuthClient returns a provider's OAuth client and its encrypted secret; found is false when
// none is saved.
func (s *Store) GetOAuthClient(ctx context.Context, provider string) (c OAuthClient, sealed []byte, found bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT client_id, secret, updated_at FROM oauth_client WHERE provider = ?`, provider).
		Scan(&c.ClientID, &sealed, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return OAuthClient{}, nil, false, nil
	}
	if err != nil {
		return OAuthClient{}, nil, false, fmt.Errorf("read the %s sign-in settings: %w", provider, err)
	}
	c.UpdatedAt = c.UpdatedAt.UTC()
	return c, sealed, true, nil
}

// SaveOAuthClient saves a provider's OAuth client, replacing any before it.
func (s *Store) SaveOAuthClient(ctx context.Context, provider, clientID string, sealed []byte) error {
	if len(sealed) == 0 {
		return errors.New("save sign-in settings: no encrypted secret")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_client (provider, client_id, secret, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (provider) DO UPDATE SET client_id = excluded.client_id, secret = excluded.secret, updated_at = excluded.updated_at`,
		provider, clientID, sealed, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save the %s sign-in settings: %w", provider, err)
	}
	return nil
}
