package account

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cockatoo/internal/store"
)

// Chosen messages go to Trash, are taken out of the copy and are audited; a sync that is running
// is waited for, not refused.
func TestDeleteMessages(t *testing.T) {
	s, id, box, _ := actionSetup(t, 5)
	ctx := context.Background()
	keys := []store.MessageKey{{AccountID: id, UIDValidity: 3, UID: 9999}, {AccountID: id, UIDValidity: 3, UID: 1}, {AccountID: id, UIDValidity: 3, UID: 424242}}

	// A sync is running: the delete waits for it.
	s.sync.mu.Lock()
	s.sync.running = true
	s.sync.mu.Unlock()
	done := make(chan struct{})
	var res DeleteResult
	var err error
	go func() {
		res, err = s.DeleteMessages(ctx, keys)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("did not wait for the sync")
	default:
	}
	s.sync.mu.Lock()
	s.sync.running = false
	s.sync.mu.Unlock()
	<-done
	if err != nil || res.Moved != 2 || res.NotMoved != 0 || res.Outcome != "done" {
		t.Fatalf("delete %+v %v", res, err)
	}
	for _, m := range box.inbox.Messages {
		if m.UID == 9999 || m.UID == 1 {
			t.Fatalf("UID %d still in the inbox", m.UID)
		}
	}
	if left, _ := s.st.MessagesByKey(ctx, keys); len(left) != 0 {
		t.Fatalf("still in the copy: %v", left)
	}
	entries, err := s.Audit(ctx)
	if err != nil || len(entries) == 0 || entries[0].Kind != "trash" || !strings.Contains(entries[0].Description, "2 chosen messages") {
		t.Fatalf("audit %+v %v", entries, err)
	}

	if _, err := s.DeleteMessages(ctx, keys); !errors.As(err, new(*ConflictError)) {
		t.Fatalf("already gone: %v", err)
	}
	if _, err := s.DeleteMessages(ctx, nil); !errors.As(err, new(*InvalidError)) {
		t.Fatalf("nothing chosen: %v", err)
	}
}
