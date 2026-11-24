package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AIProvider is an AI provider's settings as the app shows them. The API key is not part of
// it: it is kept in the secret column, encrypted, and only AIProviderSecret hands it out.
type AIProvider struct {
	Provider  string    `json:"provider"` // "anthropic"
	Enabled   bool      `json:"enabled"`
	Model     string    `json:"model"`
	HasKey    bool      `json:"hasKey"`
	KeyHint   string    `json:"keyHint,omitempty"` // "sk-ant-…a1B2"
	UpdatedAt time.Time `json:"updatedAt"`
}

// AIProviderSecret returns a provider's settings and its encrypted key (nil when none is saved).
// found is false when the provider has never been set up.
func (s *Store) AIProviderSecret(ctx context.Context, provider string) (p AIProvider, sealed []byte, found bool, err error) {
	var hint sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT provider, enabled, model, secret, key_hint, updated_at FROM ai_provider WHERE provider = ?`, provider).
		Scan(&p.Provider, &p.Enabled, &p.Model, &sealed, &hint, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AIProvider{}, nil, false, nil
	}
	if err != nil {
		return AIProvider{}, nil, false, fmt.Errorf("read AI provider %s: %w", provider, err)
	}
	p.HasKey = len(sealed) > 0
	p.KeyHint = hint.String
	return p, sealed, true, nil
}

// SetAIEnabled turns a provider on or off. The first time, it creates the row with model;
// after that the saved model and key are kept as they are.
func (s *Store) SetAIEnabled(ctx context.Context, provider, model string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_provider (provider, enabled, model, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (provider) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		provider, enabled, model, now())
	if err != nil {
		return fmt.Errorf("turn %s on or off: %w", provider, err)
	}
	return nil
}

// SaveAIKey saves a provider's key, already encrypted (sealed), replacing any key before it.
// Saving a key also turns the provider on. It never sees the plain key.
func (s *Store) SaveAIKey(ctx context.Context, provider, model string, sealed []byte, hint string) error {
	if len(sealed) == 0 {
		return errors.New("save AI key: no encrypted key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ai_provider (provider, enabled, model, secret, key_hint, updated_at) VALUES (?, true, ?, ?, ?, ?)
		 ON CONFLICT (provider) DO UPDATE SET enabled = true, secret = excluded.secret,
		   key_hint = excluded.key_hint, updated_at = excluded.updated_at`,
		provider, model, sealed, hint, now())
	if err != nil {
		return fmt.Errorf("save the %s key: %w", provider, err)
	}
	return nil
}

// RemoveAIKey deletes a provider's encrypted key. Whether the provider is on is kept.
func (s *Store) RemoveAIKey(ctx context.Context, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`UPDATE ai_provider SET secret = NULL, key_hint = NULL, updated_at = ? WHERE provider = ?`, now(), provider)
	if err != nil {
		return fmt.Errorf("remove the %s key: %w", provider, err)
	}
	return nil
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }
