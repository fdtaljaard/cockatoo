package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "inbox.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	a, err := s.AddAccount(ctx, Account{Provider: "gmail", Name: "Me", Email: "me@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddAccount(ctx, Account{Provider: "gmail", Name: "Work", Email: "work@example.com"}, []byte("sealed"))
	if err != nil {
		t.Fatal(err)
	}
	return s, a.ID, b.ID
}

func msg(uid uint32, at time.Time, from, subject string, seen bool) Message {
	return Message{UIDValidity: 7, UID: uid, MessageID: subject + "@x", FromName: "N", FromAddr: from,
		Subject: subject, ReceivedAt: at, Seen: seen}
}

func TestInboxSync(t *testing.T) {
	ctx := context.Background()
	s, me, work := openTest(t)
	// Times in a zone two hours ahead of UTC, as this machine's clock is.
	now := time.Now().In(time.FixedZone("SAST", 2*60*60)).Truncate(time.Second)

	if _, found, _ := s.GetInboxSync(ctx, me); found {
		t.Fatal("sync state before any sync")
	}

	// A first sync, interrupted after one batch: the copy is incomplete and keeps what it has.
	if err := s.BeginInboxSync(ctx, me, 7, 4); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, me, []Message{msg(4, now.Add(-time.Hour), "Ann@X.com", "d", false), msg(3, now.Add(-23*time.Hour), "ann@x.com", "c", false)}); err != nil {
		t.Fatal(err)
	}
	st, found, err := s.GetInboxSync(ctx, me)
	if err != nil || !found || st.Complete || st.ServerTotal != 4 || st.SyncedAt != nil {
		t.Fatalf("after one batch: %+v %v %v", st, found, err)
	}

	// The next sync reads the rest and finishes.
	if err := s.BeginInboxSync(ctx, me, 7, 4); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMessages(ctx, me, []Message{msg(2, now.AddDate(-3, 0, 0), "bob@x.com", "b", true), msg(1, now.AddDate(-9, 0, 0), "bob@x.com", "a", true)}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishInboxSync(ctx, me, map[uint32]bool{1: true, 2: true, 3: false, 4: false}); err != nil {
		t.Fatal(err)
	}
	if st, _, _ := s.GetInboxSync(ctx, me); !st.Complete || st.SyncedAt == nil {
		t.Fatalf("after finish: %+v", st)
	}
	if err := s.AddMessages(ctx, work, []Message{msg(1, now.Add(-time.Minute), "w@x.com", "work", false)}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListMessages(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if subjects(got) != "work,d,c" {
		t.Fatalf("last 24 hours: %s", subjects(got))
	}
	if got[1].FromAddr != "ann@x.com" || !got[1].ReceivedAt.Equal(now.Add(-time.Hour)) || got[1].ReceivedAt.Location() != time.UTC {
		t.Errorf("message d: %+v", got[1])
	}

	// In Gmail since: c was read, b was archived, a was marked unread again.
	if err := s.BeginInboxSync(ctx, me, 7, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishInboxSync(ctx, me, map[uint32]bool{1: false, 3: true, 4: false}); err != nil {
		t.Fatal(err)
	}
	seen, err := s.StoredSeen(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(seen) != "map[1:false 3:true 4:false]" {
		t.Fatalf("after changes in Gmail: %v", seen)
	}

	// A new UID validity: every stored UID means something else now, so the copy starts again.
	if err := s.BeginInboxSync(ctx, me, 8, 1); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.StoredSeen(ctx, me); len(seen) != 0 {
		t.Fatalf("after a new UID validity: %v", seen)
	}

	// Removing an account removes its copy and its sync state.
	if err := s.DeleteAccount(ctx, work); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.StoredSeen(ctx, work); len(seen) != 0 {
		t.Fatalf("messages left after removing the account: %v", seen)
	}
	if _, found, _ := s.GetInboxSync(ctx, work); found {
		t.Fatal("sync state left after removing the account")
	}
}

func TestInboxOverview(t *testing.T) {
	ctx := context.Background()
	s, me, _ := openTest(t)
	at := func(y int) time.Time { return time.Date(y, 6, 1, 12, 0, 0, 0, time.UTC) }

	empty, err := s.InboxOverview(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if empty.Messages != 0 || empty.Oldest != nil || empty.Years == nil || empty.Senders == nil {
		t.Fatalf("empty overview: %+v", empty)
	}

	if err := s.BeginInboxSync(ctx, me, 7, 5); err != nil {
		t.Fatal(err)
	}
	m := []Message{
		msg(1, at(2014), "shop@news.example.com", "sale", false),
		msg(2, at(2014), "shop@news.example.com", "sale", false),
		msg(3, at(2020), "SHOP@news.example.com", "sale", true),
		msg(4, at(2020), "ann@example.com", "hi", true),
		msg(5, at(2026), "", "no sender", false),
	}
	m[2].FromName = "Shop News" // the most recent name wins
	if err := s.AddMessages(ctx, me, m); err != nil {
		t.Fatal(err)
	}
	o, err := s.InboxOverview(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	if o.Messages != 5 || o.Unread != 3 || !o.Oldest.Equal(at(2014)) || o.Sync.Complete {
		t.Fatalf("totals: %+v", o)
	}
	if fmt.Sprint(o.Years) != "[{2014 2 2} {2020 2 0} {2026 1 1}]" {
		t.Errorf("years: %v", o.Years)
	}
	top := o.Senders[0]
	if top.Address != "shop@news.example.com" || top.Name != "Shop News" || top.Messages != 3 || top.Unread != 2 ||
		!top.Oldest.Equal(at(2014)) || !top.Latest.Equal(at(2020)) {
		t.Errorf("top sender: %+v", top)
	}
	if len(o.Senders) != 3 {
		t.Errorf("%d senders, want 3", len(o.Senders))
	}
}

// A large inbox: writing and summarising thousands of rows must stay quick.
func TestInboxScale(t *testing.T) {
	ctx := context.Background()
	s, me, _ := openTest(t)
	const n = 12000
	if err := s.BeginInboxSync(ctx, me, 7, n); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	base := time.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC)
	listed := map[uint32]bool{}
	for b := 0; b < n; b += 500 {
		batch := make([]Message, 0, 500)
		for i := b; i < b+500; i++ {
			batch = append(batch, msg(uint32(i+1), base.Add(time.Duration(i)*7*time.Hour), fmt.Sprintf("s%d@example.com", i%300), "subject", i%3 != 0))
			listed[uint32(i+1)] = i%3 != 0
		}
		if err := s.AddMessages(ctx, me, batch); err != nil {
			t.Fatal(err)
		}
	}
	wrote := time.Since(start)
	if err := s.FinishInboxSync(ctx, me, listed); err != nil {
		t.Fatal(err)
	}
	o, err := s.InboxOverview(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	total := time.Since(start)
	t.Logf("%d messages: written in %v, finished and summarised in %v", n, wrote.Round(time.Millisecond), total.Round(time.Millisecond))
	if o.Messages != n || o.Unread != n/3 || len(o.Senders) != topSenders || !o.Sync.Complete {
		t.Fatalf("overview: %d messages, %d unread, %d senders, complete %v", o.Messages, o.Unread, len(o.Senders), o.Sync.Complete)
	}
	if total > 20*time.Second {
		t.Errorf("%d messages took %v", n, total)
	}
}

func subjects(ms []Message) string {
	var out string
	for i, m := range ms {
		if i > 0 {
			out += ","
		}
		out += m.Subject
	}
	return out
}
