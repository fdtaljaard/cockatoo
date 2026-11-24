package store

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

// The provider row is created once and then updated in place: turning on and off, replacing the
// key and removing it all touch the same row and keep what they do not change.
func TestAIProviderRow(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "ai.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	get := func() (AIProvider, []byte, bool) {
		t.Helper()
		p, sealed, found, err := s.AIProviderSecret(ctx, "anthropic")
		if err != nil {
			t.Fatal(err)
		}
		return p, sealed, found
	}

	if _, _, found := get(); found {
		t.Fatal("a provider before any setup")
	}

	if err := s.SetAIEnabled(ctx, "anthropic", "model-a", true); err != nil {
		t.Fatal(err)
	}
	if p, _, found := get(); !found || !p.Enabled || p.HasKey || p.Model != "model-a" {
		t.Fatalf("after on: %+v", p)
	}

	if err := s.SaveAIKey(ctx, "anthropic", "model-b", []byte("sealed-1"), "sk-ant-…0001"); err != nil {
		t.Fatal(err)
	}
	p, sealed, _ := get()
	if !p.HasKey || p.KeyHint != "sk-ant-…0001" || !bytes.Equal(sealed, []byte("sealed-1")) {
		t.Fatalf("after key: %+v %q", p, sealed)
	}
	if p.Model != "model-a" {
		t.Errorf("saving a key changed the model to %q", p.Model)
	}

	// Off keeps the key; on again keeps it too.
	if err := s.SetAIEnabled(ctx, "anthropic", "model-a", false); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := get(); p.Enabled || !p.HasKey {
		t.Fatalf("after off: %+v", p)
	}

	// A new key replaces the old one and turns the provider on.
	if err := s.SaveAIKey(ctx, "anthropic", "model-a", []byte("sealed-2"), "sk-ant-…0002"); err != nil {
		t.Fatal(err)
	}
	if p, sealed, _ := get(); !p.Enabled || p.KeyHint != "sk-ant-…0002" || !bytes.Equal(sealed, []byte("sealed-2")) {
		t.Fatalf("after replace: %+v %q", p, sealed)
	}

	// Remove deletes the key and keeps the provider on.
	if err := s.RemoveAIKey(ctx, "anthropic"); err != nil {
		t.Fatal(err)
	}
	if p, sealed, _ := get(); !p.Enabled || p.HasKey || p.KeyHint != "" || sealed != nil {
		t.Fatalf("after remove: %+v %q", p, sealed)
	}

	var rows int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM ai_provider`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("%d rows, want 1", rows)
	}
}
