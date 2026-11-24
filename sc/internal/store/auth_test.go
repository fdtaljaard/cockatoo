package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A data file made before migration 7, holding a password account with a copy of its inbox and
// an audit entry: after the migration the account reads as a password account, nothing is lost,
// and a Google account can be added and signed in again in place.
func TestMigrationSeven(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "six.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT current_timestamp)`); err != nil {
		t.Fatal(err)
	}
	for i, m := range migrations[:6] {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (provider, name, email, secret, created_at) VALUES ('gmail', 'Me', 'me@example.com', '\x01\x02'::BLOB, ?)`, []any{now}},
		{`INSERT INTO message (account_id, uid_validity, uid, subject, received_at, seen, unsub_one_click, refreshed_at) VALUES (1, 7, 1, 'kept', ?, false, false, ?)`, []any{now, now}},
		{`INSERT INTO action (started_at, account_id, account_email, kind, criteria, description, source, matched) VALUES (?, 1, 'me@example.com', 'trash', '{}', 'x', 'you', 1)`, []any{now}},
	} {
		if _, err := db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	accounts, err := s.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].Auth != AuthPassword {
		t.Fatalf("accounts after migration: %+v %v", accounts, err)
	}
	if seen, err := s.StoredSeen(ctx, 1); err != nil || len(seen) != 1 {
		t.Fatalf("copy after migration: %v %v", seen, err)
	}
	if audit, err := s.Audit(ctx, 10); err != nil || len(audit) != 1 {
		t.Fatalf("audit after migration: %v %v", audit, err)
	}

	// A Google account, then signing in again: the same row, a new secret.
	g, err := s.AddAccount(ctx, Account{Provider: "gmail", Name: "Work", Email: "me@work.example", Auth: AuthGoogle}, []byte("token-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateAccountLogin(ctx, g.ID, AuthGoogle, []byte("token-2")); err != nil {
		t.Fatal(err)
	}
	got, sealed, err := s.AccountSecret(ctx, g.ID)
	if err != nil || got.Auth != AuthGoogle || string(sealed) != "token-2" {
		t.Fatalf("after signing in again: %+v %q %v", got, sealed, err)
	}
	if found, ok, _ := s.AccountByEmail(ctx, "me@work.example"); !ok || found.ID != g.ID {
		t.Fatalf("by email: %+v %v", found, ok)
	}
	if err := s.UpdateAccountLogin(ctx, 99, AuthGoogle, []byte("x")); err != ErrNotFound {
		t.Errorf("unknown account: %v", err)
	}

	// The OAuth client: saved, replaced.
	if _, _, found, _ := s.GetOAuthClient(ctx, "google"); found {
		t.Fatal("a client before any was saved")
	}
	if err := s.SaveOAuthClient(ctx, "google", "one.apps.googleusercontent.com", []byte("s1")); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveOAuthClient(ctx, "google", "two.apps.googleusercontent.com", []byte("s2")); err != nil {
		t.Fatal(err)
	}
	c, secret, found, err := s.GetOAuthClient(ctx, "google")
	if err != nil || !found || c.ClientID != "two.apps.googleusercontent.com" || string(secret) != "s2" {
		t.Fatalf("client %+v %q %v %v", c, secret, found, err)
	}
}
