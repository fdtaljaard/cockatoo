// Package mail talks to a mailbox over IMAP: sign in and count the inbox (the proof that an
// account's details work), list every inbox message and read their headers. It only reads: the
// inbox is opened read-only, so nothing is marked as read.
package mail

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

// Server is an IMAP server to connect to.
type Server struct {
	Addr string // host:port
	TLS  bool   // true for every real server; false only for the server the tests start
}

// Gmail is Google's IMAP server, for a personal Gmail and a Google Workspace address alike.
var Gmail = Server{Addr: "imap.gmail.com:993", TLS: true}

// Inbox is what the server says about the inbox.
type Inbox struct {
	Messages uint32 `json:"messages"`
	Unseen   uint32 `json:"unseen"`
}

// LoginError means the server was reached and refused the address and password.
type LoginError struct {
	Text string // the server's own words
}

func (e *LoginError) Error() string {
	return "the mail server did not accept this address and password: " + e.Text
}

// CheckInbox signs in to srv and counts the inbox. Nothing on the server is changed.
func CheckInbox(ctx context.Context, srv Server, login Login) (Inbox, error) {
	c, done, err := signIn(ctx, srv, login)
	if err != nil {
		return Inbox{}, err
	}
	defer done()

	data, err := c.Status("INBOX", &imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait()
	if err != nil {
		return Inbox{}, fmt.Errorf("read the inbox on %s: %w", srv.Addr, cause(ctx, err))
	}
	_ = c.Logout().Wait()

	var inbox Inbox
	if data.NumMessages != nil {
		inbox.Messages = *data.NumMessages
	}
	if data.NumUnseen != nil {
		inbox.Unseen = *data.NumUnseen
	}
	return inbox, nil
}

// Message is the header of one inbox message: enough to list it. No body is read.
type Message struct {
	UIDValidity uint32    // with UID, names the message in this mailbox; a new UIDValidity means new UIDs
	UID         uint32    //
	MessageID   string    // the Message-ID header, without angle brackets
	FromName    string    //
	FromAddr    string    // lower case
	Subject     string    // decoded to UTF-8 where the charset is known; as sent otherwise
	Received    time.Time // when the server received it (INTERNALDATE), in UTC
	Seen        bool      // read
	Unsubscribe Unsubscribe
}

// InboxState is every message in the inbox, by UID, with whether it is read.
type InboxState struct {
	UIDValidity uint32
	Messages    []UIDSeen
}

// UIDSeen is one inbox message's UID and read state.
type UIDSeen struct {
	UID  uint32
	Seen bool
}

// Session is one signed-in connection with the inbox open read-only: nothing it does marks mail
// as read or changes the mailbox. One session serves a whole sync.
type Session struct {
	c         *imapclient.Client
	done      func()
	addr      string
	ctx       context.Context
	selected  *imap.SelectData
	closeOnce sync.Once
	// Each command must finish within this; a stalled connection is closed rather than waited on.
	CommandTimeout time.Duration
}

// defaultCommandTimeout is how long one command may take before the connection is closed.
const defaultCommandTimeout = 2 * time.Minute

// Open signs in and opens the inbox read-only. Close the session when finished.
func Open(ctx context.Context, srv Server, login Login) (*Session, error) {
	c, done, err := signIn(ctx, srv, login)
	if err != nil {
		return nil, err
	}
	s := &Session{c: c, done: done, addr: srv.Addr, ctx: ctx, CommandTimeout: defaultCommandTimeout}
	err = s.timed(func() error {
		var err error
		s.selected, err = c.Select("INBOX", &imap.SelectOptions{ReadOnly: true}).Wait()
		return err
	})
	if err != nil {
		done()
		return nil, fmt.Errorf("open the inbox on %s: %w", srv.Addr, err)
	}
	return s, nil
}

// Close signs out and closes the connection. Closing twice is harmless.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		_ = s.c.Logout().Wait()
		s.done()
	})
}

// Inbox lists every message in the inbox with its read state. Only UIDs and flags are read, so
// this is quick even for a large inbox.
func (s *Session) Inbox() (InboxState, error) {
	state := InboxState{UIDValidity: s.selected.UIDValidity, Messages: []UIDSeen{}}
	if s.selected.NumMessages == 0 {
		return state, nil // "1:*" on an empty mailbox is an error on some servers
	}
	var fetched []*imapclient.FetchMessageBuffer
	err := s.timed(func() error {
		var err error
		all := imap.SeqSet{}
		all.AddRange(1, 0) // 1:*
		fetched, err = s.c.Fetch(all, &imap.FetchOptions{UID: true, Flags: true}).Collect()
		return err
	})
	if err != nil {
		return InboxState{}, fmt.Errorf("list the inbox on %s: %w", s.addr, err)
	}
	for _, m := range fetched {
		if m.UID == 0 {
			continue
		}
		state.Messages = append(state.Messages, UIDSeen{UID: uint32(m.UID), Seen: slices.Contains(m.Flags, imap.FlagSeen)})
	}
	return state, nil
}

// Headers reads the headers of the messages with these UIDs. A UID no longer in the inbox is
// left out of the answer.
func (s *Session) Headers(uids []uint32) ([]Message, error) {
	out := []Message{}
	if len(uids) == 0 {
		return out, nil
	}
	set := imap.UIDSet{}
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	// PEEK: reading these header fields must not mark the message as read.
	unsub := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: unsubscribeFields, Peek: true}
	var fetched []*imapclient.FetchMessageBuffer
	err := s.timed(func() error {
		var err error
		fetched, err = s.c.Fetch(set, &imap.FetchOptions{
			UID: true, Envelope: true, Flags: true, InternalDate: true, BodySection: []*imap.FetchItemBodySection{unsub},
		}).Collect()
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read headers on %s: %w", s.addr, err)
	}
	for _, m := range fetched {
		msg := Message{
			UIDValidity: s.selected.UIDValidity,
			UID:         uint32(m.UID),
			Received:    m.InternalDate.UTC(),
			Seen:        slices.Contains(m.Flags, imap.FlagSeen),
			Unsubscribe: parseUnsubscribe(headerSection(m, unsub)),
		}
		if env := m.Envelope; env != nil {
			msg.Subject = env.Subject
			msg.MessageID = env.MessageID
			if len(env.From) > 0 {
				msg.FromName = env.From[0].Name
				msg.FromAddr = strings.ToLower(env.From[0].Addr())
			}
		}
		out = append(out, msg)
	}
	return out, nil
}

// timed runs one command, closing the connection if it takes longer than CommandTimeout. The
// error then says the command timed out rather than that the connection closed.
func (s *Session) timed(command func() error) error {
	var expired atomic.Bool
	t := time.AfterFunc(s.CommandTimeout, func() {
		expired.Store(true)
		s.c.Close()
	})
	err := command()
	t.Stop()
	if err != nil {
		if expired.Load() {
			return fmt.Errorf("no answer within %s", s.CommandTimeout)
		}
		return cause(s.ctx, err)
	}
	return nil
}

// signIn connects and signs in. done closes the connection; call it when finished.
func signIn(ctx context.Context, srv Server, login Login) (c *imapclient.Client, done func(), err error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	opts := &imapclient.Options{
		Dialer: dialer,
		// Subjects and names in charsets other than UTF-8 (windows-1252, iso-2022-jp, ...).
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	if srv.TLS {
		c, err = imapclient.DialTLS(srv.Addr, opts)
	} else {
		c, err = imapclient.DialInsecure(srv.Addr, opts)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("could not reach %s: %w", srv.Addr, err)
	}

	// The client's commands take no context: closing the connection is how a cancelled or
	// timed-out request stops waiting.
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-stop:
		}
	}()
	var once sync.Once
	done = func() {
		once.Do(func() {
			close(stop)
			c.Close()
		})
	}

	if err := authenticate(ctx, c, login); err != nil {
		done()
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("sign in to %s: %w", srv.Addr, ctx.Err())
		}
		var refused *LoginError
		var token *TokenError
		if errors.As(err, &refused) || errors.As(err, &token) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("sign in to %s: %w", srv.Addr, err)
	}
	return c, done, nil
}

// cause prefers the context's error: a command cut short by a timeout fails with a closed
// connection, which says less than "deadline exceeded".
func cause(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// headerSection is the header block the server returned for the request. The library matches the
// echoed field list exactly (in order); a server that echoes it differently still sent one block,
// so that one is used.
func headerSection(m *imapclient.FetchMessageBuffer, asked *imap.FetchItemBodySection) []byte {
	if b := m.FindBodySection(asked); b != nil {
		return b
	}
	if len(m.BodySection) == 1 {
		return m.BodySection[0].Bytes
	}
	return nil
}
