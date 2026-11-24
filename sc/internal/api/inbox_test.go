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

type body struct{ *strings.Reader }

func (b body) Size() int64 { return b.Reader.Size() }

func TestInboxAndOverview(t *testing.T) {
	now := time.Now()
	seed := func(u *imapmemserver.User) {
		for _, m := range []struct {
			ago     time.Duration
			from    string
			subject string
			seen    bool
		}{
			{6 * 365 * 24 * time.Hour, "Shop <shop@example.com>", "old sale", false},
			{30 * time.Hour, "Shop <shop@example.com>", "yesterday's sale", true},
			{3 * time.Hour, "Ann <ann@example.com>", "Lunch?", false},
			{10 * time.Minute, "=?UTF-8?Q?Ren=C3=A9e?= <renee@example.com>", "=?UTF-8?Q?Caf=C3=A9?=", false},
		} {
			var flags []imap.Flag
			if m.seen {
				flags = []imap.Flag{imap.FlagSeen}
			}
			msg := "From: " + m.from + "\r\nTo: " + testEmail + "\r\nSubject: " + m.subject + "\r\n\r\nhi\r\n"
			if _, err := u.Append("INBOX", body{strings.NewReader(msg)}, &imap.AppendOptions{Time: now.Add(-m.ago), Flags: flags}); err != nil {
				t.Fatal(err)
			}
		}
	}
	h, _ := newApp(t, seed)

	// No account: an empty list, never null.
	if code, b := do(t, h, "GET", "/api/inbox", nil); code != 200 || !strings.Contains(b, `"messages":[]`) {
		t.Fatalf("empty: %d %s", code, b)
	}
	if code, b := do(t, h, "POST", "/api/accounts", account.Input{Email: testEmail, Password: testPassword}); code != http.StatusCreated {
		t.Fatalf("add: %d %s", code, b)
	}

	// Start the sync, then wait for it.
	if code, b := do(t, h, "POST", "/api/sync", nil); code != http.StatusAccepted || !strings.Contains(b, `"running":true`) {
		t.Fatalf("start sync: %d %s", code, b)
	}
	var status account.SyncStatus
	for deadline := time.Now().Add(10 * time.Second); ; {
		_, b := do(t, h, "GET", "/api/sync", nil)
		if err := json.Unmarshal([]byte(b), &status); err != nil {
			t.Fatal(err)
		}
		if !status.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync still running: %s", b)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(status.Accounts) != 1 || status.Accounts[0].State != "done" || status.Accounts[0].Read != 4 {
		t.Fatalf("sync status %+v", status)
	}

	// The Inbox list is the last 24 hours of the copy.
	_, b := do(t, h, "GET", "/api/inbox", nil)
	var list account.InboxList
	if err := json.Unmarshal([]byte(b), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Messages) != 2 || list.Messages[0].Subject != "Café" || list.Messages[0].FromName != "Renée" {
		t.Fatalf("inbox: %s", b)
	}

	// The overview covers the whole inbox.
	code, b := do(t, h, "GET", "/api/accounts/1/overview", nil)
	var o store.Overview
	if err := json.Unmarshal([]byte(b), &o); err != nil {
		t.Fatal(err)
	}
	if code != 200 || o.Messages != 4 || o.Unread != 3 || !o.Sync.Complete || len(o.Years) < 2 {
		t.Fatalf("overview: %d %s", code, b)
	}
	if o.Senders[0].Address != "shop@example.com" || o.Senders[0].Messages != 2 || o.Senders[0].Unread != 1 {
		t.Errorf("top sender: %+v", o.Senders[0])
	}
	if code, _ := do(t, h, "GET", "/api/accounts/99/overview", nil); code != http.StatusNotFound {
		t.Errorf("unknown account: %d", code)
	}
}
