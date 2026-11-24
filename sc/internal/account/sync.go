package account

import (
	"context"
	"errors"
	"log"
	"slices"
	"sync"
	"time"

	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// mailbox is what a sync needs from a signed-in mail server. *mail.Session is the real one; the
// tests stand in for it.
type mailbox interface {
	Inbox() (imap.InboxState, error)
	Headers(uids []uint32) ([]imap.Message, error)
	Texts(uids []uint32, max int) (map[uint32]string, error)
	Raw(uids []uint32) (raw map[uint32][]byte, tooBig map[uint32]int64, err error)
	Bodies(uids []uint32) (map[uint32]imap.Body, error)
	Validity() uint32
	Folder() string
	Close()
}

func dialSession(ctx context.Context, srv imap.Server, login imap.Login) (mailbox, error) {
	return imap.Open(ctx, srv, login)
}

// dialSentFolder opens the Sent folder; found is false when the server has none.
func dialSentFolder(ctx context.Context, srv imap.Server, login imap.Login) (mailbox, bool, error) {
	s, found, err := imap.OpenSent(ctx, srv, login)
	if err != nil || !found {
		return nil, found, err
	}
	return s, true, nil
}

// headerBatch is how many messages' headers are asked for at once.
const headerBatch = 500

// syncTimeout bounds one account's whole sync. A first sync of a large inbox reads every
// message's headers, which can take minutes.
const syncTimeout = 30 * time.Minute

// SyncStatus is what the sync is doing, for the screen to show.
type SyncStatus struct {
	Running  bool          `json:"running"`
	Accounts []AccountSync `json:"accounts"`
}

// AccountSync is the state of one account's sync.
type AccountSync struct {
	AccountID int64  `json:"accountId"`
	Email     string `json:"email"`
	// "waiting", "listing" (asking the server what is in the inbox), "reading" (reading the
	// headers of messages not yet in the copy), "sent" (the same for the Sent folder), "done" or
	// "failed".
	State   string `json:"state"`
	Read    int    `json:"read"`    // headers read so far in this sync
	ToRead  int    `json:"toRead"`  // headers this sync has to read
	Skipped int    `json:"skipped"` // messages whose headers the server could not give
	Error   string `json:"error,omitempty"`
	// The Sent folder could not be read. The inbox is fine; only the "who you write to" facts
	// are missing until the next sync.
	SentError string `json:"sentError,omitempty"`
}

type syncer struct {
	mu      sync.Mutex
	running bool
	status  []AccountSync
	cancel  context.CancelFunc
	done    chan struct{}
}

// StartSync brings every account's copy of its inbox up to date in the background. It does
// nothing when a sync is already running. Progress is in SyncStatus.
func (s *Service) StartSync() SyncStatus {
	s.sync.mu.Lock()
	if s.sync.running || s.acting || s.stopped {
		s.sync.mu.Unlock()
		return s.SyncStatus()
	}
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		s.sync.mu.Unlock()
		log.Printf("sync: %v", err)
		return s.SyncStatus()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.sync.running, s.sync.cancel, s.sync.done = true, cancel, make(chan struct{})
	s.sync.status = make([]AccountSync, len(accounts))
	for i, a := range accounts {
		s.sync.status[i] = AccountSync{AccountID: a.ID, Email: a.Email, State: "waiting"}
	}
	s.sync.mu.Unlock()

	go func() {
		defer close(s.sync.done)
		defer cancel()
		for i, a := range accounts {
			err := s.SyncAccount(ctx, a.ID, func(p AccountSync) {
				s.sync.mu.Lock()
				p.AccountID, p.Email = a.ID, a.Email
				s.sync.status[i] = p
				s.sync.mu.Unlock()
			})
			if err != nil {
				log.Printf("sync of account %d: %v", a.ID, err) // the address and mail stay out of the log
			}
		}
		s.sync.mu.Lock()
		s.sync.running = false
		s.sync.mu.Unlock()
	}()
	return s.SyncStatus()
}

// SyncStatus returns what the sync is doing, or what the last one did.
func (s *Service) SyncStatus() SyncStatus {
	s.sync.mu.Lock()
	defer s.sync.mu.Unlock()
	return SyncStatus{Running: s.sync.running, Accounts: slices.Clone(append([]AccountSync{}, s.sync.status...))}
}

// StopSync stops a running sync and waits for it, and waits for a running action to finish and
// record its result, so the store can be closed after. Batches already read stay in the copy;
// the next sync carries on from there. Nothing new starts after this.
func (s *Service) StopSync() {
	s.sync.mu.Lock()
	s.stopped = true
	cancel, done := s.sync.cancel, s.sync.done
	s.sync.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
	s.actions.Wait()
}

// SyncAccount brings one account's copy of its inbox up to date, reporting progress as it goes.
// It lists the inbox, reads the headers of every message not yet in the copy (newest first, in
// batches that are each saved at once), and only after a complete pass updates read states and
// removes what left the inbox. Nothing on the mail server is changed.
func (s *Service) SyncAccount(ctx context.Context, id int64, progress func(AccountSync)) error {
	p := AccountSync{AccountID: id, State: "listing"}
	report := func() {
		if progress != nil {
			progress(p)
		}
	}
	fail := func(err error) error {
		p.State, p.Error = "failed", err.Error()
		report()
		return err
	}
	report()

	a, sealed, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return fail(err)
	}
	p.Email = a.Email
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return fail(err)
	}

	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	box, err := s.dial(ctx, s.server, login)
	if err != nil {
		return fail(explainMail(err))
	}
	defer box.Close()

	inbox, err := box.Inbox()
	if err != nil {
		return fail(err)
	}
	if err := s.st.BeginInboxSync(ctx, id, inbox.UIDValidity, len(inbox.Messages)); err != nil {
		return fail(err)
	}
	stored, err := s.st.StoredSeen(ctx, id)
	if err != nil {
		return fail(err)
	}
	listed := make(map[uint32]bool, len(inbox.Messages))
	var missing []uint32
	for _, m := range inbox.Messages {
		listed[m.UID] = m.Seen
		if _, have := stored[m.UID]; !have {
			missing = append(missing, m.UID)
		}
	}
	// Newest first: a higher UID arrived later, so the Inbox list fills before the old years do.
	slices.SortFunc(missing, func(x, y uint32) int { return int(y) - int(x) })

	p.State, p.ToRead = "reading", len(missing)
	report()
	for start := 0; start < len(missing); start += headerBatch {
		batch := missing[start:min(start+headerBatch, len(missing))]
		got, skipped, err := s.readHeaders(ctx, box, batch)
		if err != nil {
			return fail(err)
		}
		rows := make([]store.Message, len(got))
		for i, m := range got {
			rows[i] = store.Message{
				AccountID: id, UIDValidity: m.UIDValidity, UID: m.UID, MessageID: m.MessageID,
				FromName: m.FromName, FromAddr: m.FromAddr, Subject: m.Subject, ReceivedAt: m.Received, Seen: listed[m.UID],
				Unsubscribe: store.Unsubscribe{URL: m.Unsubscribe.URL, Mailto: m.Unsubscribe.Mailto, OneClick: m.Unsubscribe.OneClick},
				To:          m.To, Cc: m.Cc, InReplyTo: m.InReplyTo, ListID: m.Markers.ListID, Precedence: m.Markers.Precedence,
				AutoSubmitted: m.Markers.AutoSubmitted,
			}
		}
		if err := s.st.AddMessages(ctx, id, rows); err != nil {
			return fail(err)
		}
		p.Read += len(batch)
		p.Skipped += skipped
		report()
	}

	if err := s.st.FinishInboxSync(ctx, id, listed); err != nil {
		return fail(err)
	}
	box.Close()

	// The Sent folder: who the user writes to and what they answered. It never fails the sync.
	if err := s.syncSent(ctx, id, login, &p, report); err != nil {
		p.SentError = explainMail(err).Error()
		log.Printf("sync of account %d: sent folder: %v", id, err)
	}
	p.State = "done"
	report()
	return nil
}

// syncSent brings the copy of the Sent folder up to date, the same way as the inbox's: list,
// read the headers of what is not yet in the copy, and remove what left only after a full pass.
func (s *Service) syncSent(ctx context.Context, id int64, login imap.Login, p *AccountSync, report func()) error {
	if s.dialSent == nil {
		return nil
	}
	box, found, err := s.dialSent(ctx, s.server, login)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	defer box.Close()
	list, err := box.Inbox()
	if err != nil {
		return err
	}
	if err := s.st.BeginSentSync(ctx, id, box.Folder(), list.UIDValidity); err != nil {
		return err
	}
	stored, err := s.st.StoredSentUIDs(ctx, id)
	if err != nil {
		return err
	}
	listed := make(map[uint32]bool, len(list.Messages))
	var missing []uint32
	for _, m := range list.Messages {
		listed[m.UID] = true
		if !stored[m.UID] {
			missing = append(missing, m.UID)
		}
	}
	slices.SortFunc(missing, func(x, y uint32) int { return int(y) - int(x) })
	p.State, p.Read, p.ToRead = "sent", 0, len(missing)
	report()
	for start := 0; start < len(missing); start += headerBatch {
		batch := missing[start:min(start+headerBatch, len(missing))]
		got, skipped, err := s.readHeaders(ctx, box, batch)
		if err != nil {
			return err
		}
		rows := make([]store.SentMessage, len(got))
		for i, m := range got {
			rows[i] = store.SentMessage{UIDValidity: m.UIDValidity, UID: m.UID, MessageID: m.MessageID, FromAddr: m.FromAddr,
				To: m.To, Cc: m.Cc, InReplyTo: m.InReplyTo, SentAt: m.Received}
		}
		if err := s.st.AddSent(ctx, id, rows); err != nil {
			return err
		}
		p.Read += len(batch)
		p.Skipped += skipped
		report()
	}
	return s.st.FinishSentSync(ctx, id, listed)
}

// MessageTexts reads the start of the text of some of an account's inbox messages, at most max
// characters each, without marking them read. The text is returned, never stored.
func (s *Service) MessageTexts(ctx context.Context, id int64, uids []uint32, max int) (map[uint32]string, error) {
	a, sealed, err := s.st.AccountSecret(ctx, id)
	if err != nil {
		return nil, err
	}
	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		return nil, err
	}
	box, err := s.dial(ctx, s.server, login)
	if err != nil {
		return nil, explainMail(err)
	}
	defer box.Close()
	return box.Texts(uids, max)
}

// readHeaders reads one batch. When the server fails on a batch (one malformed message can do
// that), the batch is halved and tried again, down to single messages; a message that still
// fails is skipped and counted rather than stopping the sync. A dead connection or a cancelled
// sync is not retried.
func (s *Service) readHeaders(ctx context.Context, box mailbox, uids []uint32) ([]imap.Message, int, error) {
	got, err := box.Headers(uids)
	if err == nil {
		return got, 0, nil
	}
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}
	if len(uids) == 1 {
		log.Printf("sync: skipped one message the server could not read: %v", err)
		return nil, 1, nil
	}
	half := len(uids) / 2
	first, skippedFirst, err := s.readHeaders(ctx, box, uids[:half])
	if err != nil {
		return nil, 0, err
	}
	second, skippedSecond, err := s.readHeaders(ctx, box, uids[half:])
	if err != nil {
		return nil, 0, err
	}
	if skippedFirst+skippedSecond == len(uids) {
		// Nothing in the batch could be read: the connection is gone, not one bad message.
		return nil, 0, errors.New("the mail server stopped answering")
	}
	return append(first, second...), skippedFirst + skippedSecond, nil
}
