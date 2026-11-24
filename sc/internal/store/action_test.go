package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestMatchAndAudit(t *testing.T) {
	ctx := context.Background()
	s, me, _ := openTest(t)
	at := func(y int) time.Time { return time.Date(y, 6, 1, 12, 0, 0, 0, time.UTC) }
	if err := s.BeginInboxSync(ctx, me, 7, 4); err != nil {
		t.Fatal(err)
	}
	deals := msg(1, at(2015), "deals@shop.example", "sale", false)
	deals.Unsubscribe = Unsubscribe{URL: "https://shop.example/u", OneClick: true}
	if err := s.AddMessages(ctx, me, []Message{
		deals,
		msg(2, at(2019), "DEALS@shop.example", "sale", true),
		msg(3, at(2015), "ann@example.com", "hi", true),
		msg(4, at(2019), "", "no sender", false),
	}); err != nil {
		t.Fatal(err)
	}

	// The overview's sender row and the action's match agree.
	o, err := s.InboxOverview(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if o.Senders[0].Address != "deals@shop.example" || o.Senders[0].Messages != 2 {
		t.Fatalf("top sender %+v", o.Senders[0])
	}
	if len(o.Unsubscribable) != 1 || o.Unsubscribable[0].Unsubscribe == nil ||
		*o.Unsubscribable[0].Unsubscribe != (Unsubscribe{URL: "https://shop.example/u", OneClick: true}) {
		t.Fatalf("unsubscribable %+v", o.Unsubscribable)
	}
	if o.Senders[1].Unsubscribe != nil {
		t.Errorf("a sender without unsubscribe has one: %+v", o.Senders[1])
	}
	bySender, err := s.MatchMessages(ctx, me, Criteria{Kind: "sender", Address: "Deals@Shop.example"})
	if err != nil || len(bySender) != o.Senders[0].Messages {
		t.Fatalf("sender match: %d %v", len(bySender), err)
	}
	byYear, err := s.MatchMessages(ctx, me, Criteria{Kind: "year", Year: 2015})
	if err != nil || len(byYear) != o.Years[0].Messages {
		t.Fatalf("year match: %d (row says %d) %v", len(byYear), o.Years[0].Messages, err)
	}
	if _, err := s.MatchMessages(ctx, me, Criteria{Kind: "sender", Address: " "}); err == nil {
		t.Error("matched mail without a sender as one sender")
	}
	if _, err := s.MatchMessages(ctx, me, Criteria{Kind: "everything"}); err == nil {
		t.Error("matched an unknown kind")
	}

	// The audit: a start record before, a result after, kept when the account is removed.
	id, err := s.StartAction(ctx, ActionStart{AccountID: me, AccountEmail: "me@example.com", Kind: "trash",
		Criteria: Criteria{Kind: "sender", Address: "deals@shop.example"}, Description: "Move 2 messages from deals@shop.example to Trash",
		Source: "you", Folder: "[Gmail]/Trash"}, bySender)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := s.Audit(ctx, 10)
	if err != nil || len(audit) != 1 || audit[0].Outcome != "running" || audit[0].Matched != 2 {
		t.Fatalf("audit before the result: %+v %v", audit, err)
	}
	if err := s.FinishAction(ctx, id, ActionResult{Outcome: "partial", Done: 1, NotDone: []uint32{2}, Error: "server stopped"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, me); err != nil {
		t.Fatal(err)
	}
	audit, err = s.Audit(ctx, 10)
	if err != nil || len(audit) != 1 {
		t.Fatalf("audit after removing the account: %+v %v", audit, err)
	}
	e := audit[0]
	if e.Outcome != "partial" || e.Done != 1 || e.AccountEmail != "me@example.com" || e.Criteria.Address != "deals@shop.example" ||
		e.Folder != "[Gmail]/Trash" || e.FinishedAt == nil || e.Error != "server stopped" {
		t.Fatalf("audit entry %+v", e)
	}
	var items int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM action_item WHERE action_id = ?`, id).Scan(&items); err != nil || items != 2 {
		t.Fatalf("%d audited messages, %v", items, err)
	}
}

// A data file made before migration 6, with an account, an AI key and a copy of the inbox in it:
// the account and the key survive, and the copy is emptied for the next sync to refill.
func TestMigrationSixKeepsAccounts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE schema_version (version INTEGER NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT current_timestamp)`); err != nil {
		t.Fatal(err)
	}
	for i, m := range migrations[:5] {
		if _, err := db.Exec(m); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for _, q := range []string{
		`INSERT INTO account (provider, name, email, secret, created_at) VALUES ('gmail', 'Me', 'me@example.com', '\x01\x02'::BLOB, ?)`,
		`INSERT INTO ai_provider (provider, enabled, model, secret, key_hint, updated_at) VALUES ('anthropic', true, 'm', '\x03'::BLOB, 'sk-…', ?)`,
		`INSERT INTO message (account_id, uid_validity, uid, subject, received_at, seen, refreshed_at) VALUES (1, 7, 1, 'old', ?, false, ?)`,
		`INSERT INTO inbox_sync (account_id, uid_validity, server_total, complete, started_at) VALUES (1, 7, 1, true, ?)`,
	} {
		args := []any{now}
		if q[12:19] == "message" {
			args = []any{now, now}
		}
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if accounts, err := s.ListAccounts(ctx); err != nil || len(accounts) != 1 || accounts[0].Email != "me@example.com" {
		t.Fatalf("accounts after migration: %+v %v", accounts, err)
	}
	if _, sealed, err := s.AccountSecret(ctx, 1); err != nil || len(sealed) != 2 {
		t.Fatalf("account secret after migration: %v %v", sealed, err)
	}
	if p, sealed, found, err := s.AIProviderSecret(ctx, "anthropic"); err != nil || !found || !p.Enabled || len(sealed) != 1 {
		t.Fatalf("AI key after migration: %+v %v", p, err)
	}
	if seen, err := s.StoredSeen(ctx, 1); err != nil || len(seen) != 0 {
		t.Fatalf("copy after migration: %v %v", seen, err)
	}
	if _, found, _ := s.GetInboxSync(ctx, 1); found {
		t.Fatal("sync state survived; the next sync would not refill the copy")
	}
}
