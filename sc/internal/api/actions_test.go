//go:build windows

package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"cockatoo/internal/account"
	"cockatoo/internal/store"
)

func waitForSync(t *testing.T, h http.Handler) account.SyncStatus {
	t.Helper()
	var status account.SyncStatus
	for deadline := time.Now().Add(10 * time.Second); ; {
		_, b := do(t, h, "GET", "/api/sync", nil)
		if err := json.Unmarshal([]byte(b), &status); err != nil {
			t.Fatal(err)
		}
		if !status.Running {
			return status
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync still running: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCleanupActions(t *testing.T) {
	now := time.Now()
	var user *imapmemserver.User
	seed := func(u *imapmemserver.User) {
		user = u
		if err := u.Create("[Gmail]/Trash", nil); err != nil {
			t.Fatal(err)
		}
		for i, m := range []string{
			"From: Deals <deals@shop.example>\r\nSubject: sale 1\r\nList-Unsubscribe: <https://shop.example/u?id=7>\r\n\r\nx\r\n",
			"From: Deals <deals@shop.example>\r\nSubject: sale 2\r\nList-Unsubscribe: <https://shop.example/u?id=7>\r\n\r\nx\r\n",
			"From: Ann <ann@example.com>\r\nSubject: lunch\r\n\r\nhi\r\n",
		} {
			at := now.Add(-time.Duration(3-i) * time.Hour)
			if _, err := u.Append("INBOX", body{strings.NewReader(m)}, &imap.AppendOptions{Time: at}); err != nil {
				t.Fatal(err)
			}
		}
	}
	h, _ := newApp(t, seed)
	if code, b := do(t, h, "POST", "/api/accounts", account.Input{Email: testEmail, Password: testPassword}); code != http.StatusCreated {
		t.Fatalf("add: %d %s", code, b)
	}
	do(t, h, "POST", "/api/sync", nil)
	if st := waitForSync(t, h); st.Accounts[0].State != "done" {
		t.Fatalf("sync %+v", st)
	}

	// The overview offers the deals sender's unsubscribe page.
	_, b := do(t, h, "GET", "/api/accounts/1/overview", nil)
	var o store.Overview
	if err := json.Unmarshal([]byte(b), &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Unsubscribable) != 1 || o.Unsubscribable[0].Unsubscribe.URL != "https://shop.example/u?id=7" {
		t.Fatalf("unsubscribable %s", b)
	}

	// Preview, then apply with the count the preview gave.
	deals := store.Criteria{Kind: "sender", Address: "deals@shop.example"}
	code, b := do(t, h, "POST", "/api/accounts/1/actions/preview", deals)
	var p account.Preview
	if err := json.Unmarshal([]byte(b), &p); err != nil || code != 200 || p.Messages != 2 {
		t.Fatalf("preview: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/accounts/1/actions", account.ActionRequest{Kind: "trash", Criteria: deals, Expected: 3}); code != http.StatusConflict {
		t.Fatalf("stale count: %d", code)
	}
	code, b = do(t, h, "POST", "/api/accounts/1/actions", account.ActionRequest{Kind: "trash", Criteria: deals, Expected: 2})
	var out account.ActionOutcome
	if err := json.Unmarshal([]byte(b), &out); err != nil || code != 200 || out.Outcome != "done" || out.Moved != 2 {
		t.Fatalf("apply: %d %s", code, b)
	}

	// On the server: two in Trash, one left in the inbox, still unread.
	trash, err := user.Status("[Gmail]/Trash", &imap.StatusOptions{NumMessages: true})
	if err != nil || *trash.NumMessages != 2 {
		t.Fatalf("trash %+v %v", trash, err)
	}
	inbox, err := user.Status("INBOX", &imap.StatusOptions{NumMessages: true, NumUnseen: true})
	if err != nil || *inbox.NumMessages != 1 || *inbox.NumUnseen != 1 {
		t.Fatalf("inbox %+v %v", inbox, err)
	}
	_, b = do(t, h, "GET", "/api/inbox", nil)
	if strings.Contains(b, "sale") || !strings.Contains(b, "lunch") {
		t.Fatalf("inbox list after the move: %s", b)
	}

	// Opening an unsubscribe page is audited; one the sender did not give is refused.
	if code, _ := do(t, h, "POST", "/api/accounts/1/unsubscribe-opened", map[string]string{"address": "deals@shop.example", "target": "https://elsewhere.example"}); code != http.StatusBadRequest {
		t.Fatalf("foreign target: %d", code)
	}

	// The audit holds the move with its criteria.
	_, b = do(t, h, "GET", "/api/audit", nil)
	var audit []store.AuditEntry
	if err := json.Unmarshal([]byte(b), &audit); err != nil || len(audit) != 1 {
		t.Fatalf("audit: %s", b)
	}
	if e := audit[0]; e.Kind != "trash" || e.Criteria != deals || e.Matched != 2 || e.Outcome != "done" || e.Folder != "[Gmail]/Trash" {
		t.Fatalf("audit entry %+v", e)
	}
}
