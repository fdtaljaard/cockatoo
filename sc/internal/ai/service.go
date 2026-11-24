package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"cockatoo/internal/secret"
	"cockatoo/internal/store"
)

// provider is the only AI provider today.
const provider = "anthropic"

var (
	// ErrOff means AI is turned off in the app.
	ErrOff = errors.New("AI is turned off. Turn it on under AI.")
	// ErrNoKey means AI is on but no API key is saved.
	ErrNoKey = errors.New("No Anthropic API key is saved. Add one under AI.")
)

// InvalidError says what is wrong with what the user typed.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

// Service keeps the AI settings and hands out the client.
type Service struct {
	st    *store.Store
	seal  func([]byte) ([]byte, error)
	open  func([]byte) ([]byte, error)
	extra []option.RequestOption // the tests point the client at a stand-in server
}

// NewService returns the AI settings service.
func NewService(st *store.Store) *Service { return NewServiceFor(st) }

// NewServiceFor passes extra options to every client it makes. The tests use it.
func NewServiceFor(st *store.Store, extra ...option.RequestOption) *Service {
	return &Service{st: st, seal: secret.Seal, open: secret.Open, extra: extra}
}

const (
	checkTimeout = 30 * time.Second
	askTimeout   = 3 * time.Minute
)

// Settings returns the AI settings. Before any setup, AI is off with the default model.
func (s *Service) Settings(ctx context.Context) (store.AIProvider, error) {
	p, _, found, err := s.st.AIProviderSecret(ctx, provider)
	if err != nil {
		return store.AIProvider{}, err
	}
	if !found {
		return store.AIProvider{Provider: provider, Model: DefaultModel}, nil
	}
	return p, nil
}

// SetEnabled turns AI on or off. Turning it off keeps the key.
func (s *Service) SetEnabled(ctx context.Context, on bool) (store.AIProvider, error) {
	if err := s.st.SetAIEnabled(ctx, provider, DefaultModel, on); err != nil {
		return store.AIProvider{}, err
	}
	return s.Settings(ctx)
}

// SaveKey checks the key with Anthropic and, only when it is accepted, encrypts and saves it and
// turns AI on. A key that is not accepted changes nothing: a key saved before stays.
func (s *Service) SaveKey(ctx context.Context, key string) (store.AIProvider, error) {
	key = strings.Join(strings.Fields(key), "") // a pasted key often brings a space or a line break
	if key == "" {
		return store.AIProvider{}, &InvalidError{"Paste your Anthropic API key."}
	}
	current, err := s.Settings(ctx)
	if err != nil {
		return store.AIProvider{}, err
	}

	checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	if err := NewClient(key, current.Model, s.extra...).Check(checkCtx); err != nil {
		return store.AIProvider{}, err
	}

	sealed, err := s.seal([]byte(key))
	if err != nil {
		return store.AIProvider{}, fmt.Errorf("encrypt the key: %w", err)
	}
	if err := s.st.SaveAIKey(ctx, provider, current.Model, sealed, hint(key)); err != nil {
		return store.AIProvider{}, err
	}
	return s.Settings(ctx)
}

// RemoveKey deletes the saved key. Whether AI is on is kept.
func (s *Service) RemoveKey(ctx context.Context) (store.AIProvider, error) {
	if err := s.st.RemoveAIKey(ctx, provider); err != nil {
		return store.AIProvider{}, err
	}
	return s.Settings(ctx)
}

// Client is the one way in for every feature that uses AI: it returns a client only when AI is
// on and the saved key opens.
func (s *Service) Client(ctx context.Context) (*Client, error) {
	p, sealed, found, err := s.st.AIProviderSecret(ctx, provider)
	if err != nil {
		return nil, err
	}
	if !found || !p.Enabled {
		return nil, ErrOff
	}
	if len(sealed) == 0 {
		return nil, ErrNoKey
	}
	key, err := s.open(sealed)
	if err != nil {
		return nil, fmt.Errorf("the saved Anthropic key could not be read; remove it and add it again: %w", err)
	}
	return NewClient(string(key), p.Model, s.extra...), nil
}

// TestMessage sends one short message with the saved key and returns Claude's reply: the proof
// that a real request works end to end, which the free key check does not give.
func (s *Service) TestMessage(ctx context.Context) (Reply, error) {
	c, err := s.Client(ctx)
	if err != nil {
		return Reply{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	return c.Ask(ctx, "You are the assistant inside cockatoo, a simple email client.",
		"Reply with one short, friendly sentence confirming that cockatoo can talk to you.")
}

// hint is enough of the key to recognise it: its start and its last four characters.
func hint(key string) string {
	if len(key) < 16 {
		return "…" + key[max(0, len(key)-2):]
	}
	return key[:7] + "…" + key[len(key)-4:]
}
