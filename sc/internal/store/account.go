package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Account is a mail account as the app shows it. The password is not part of it: it is kept
// in the secret column, encrypted, and only AccountSecret hands it out.
type Account struct {
	ID        int64     `json:"id"`
	Provider  string    `json:"provider"` // "gmail"
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	// ErrNotFound means there is no account with that id.
	ErrNotFound = errors.New("no such account")
	// ErrDuplicate means an account with that address is already saved.
	ErrDuplicate = errors.New("this address is already added")
)

// ListAccounts returns every account, oldest first.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, provider, name, email, created_at FROM account ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Provider, &a.Name, &a.Email, &a.CreatedAt); err != nil {
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
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO account (provider, name, email, secret, created_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		a.Provider, a.Name, a.Email, sealed, a.CreatedAt).Scan(&a.ID)
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
		`SELECT id, provider, name, email, created_at, secret FROM account WHERE id = ?`, id).
		Scan(&a.ID, &a.Provider, &a.Name, &a.Email, &a.CreatedAt, &sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, nil, ErrNotFound
	}
	if err != nil {
		return Account{}, nil, fmt.Errorf("read account %d: %w", id, err)
	}
	return a, sealed, nil
}

// DeleteAccount removes an account and its encrypted password from the file.
func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var gone int64
	err := s.db.QueryRowContext(ctx, `DELETE FROM account WHERE id = ? RETURNING id`, id).Scan(&gone)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	return nil
}
