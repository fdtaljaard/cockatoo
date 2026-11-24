// Package mail talks to a mailbox over IMAP. Today it does one thing: sign in and count the inbox,
// which is the proof that an account's details work.
package mail

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
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
func CheckInbox(ctx context.Context, srv Server, username, password string) (Inbox, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if deadline, ok := ctx.Deadline(); ok {
		dialer.Deadline = deadline
	}
	opts := &imapclient.Options{Dialer: dialer}

	var c *imapclient.Client
	var err error
	if srv.TLS {
		c, err = imapclient.DialTLS(srv.Addr, opts)
	} else {
		c, err = imapclient.DialInsecure(srv.Addr, opts)
	}
	if err != nil {
		return Inbox{}, fmt.Errorf("could not reach %s: %w", srv.Addr, err)
	}
	defer c.Close()

	// The client's commands take no context: closing the connection is how a cancelled or
	// timed-out request stops waiting.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()

	if err := c.Login(username, password).Wait(); err != nil {
		if ctx.Err() != nil {
			return Inbox{}, fmt.Errorf("sign in to %s: %w", srv.Addr, ctx.Err())
		}
		var refused *imap.Error
		if errors.As(err, &refused) {
			return Inbox{}, &LoginError{Text: refused.Text}
		}
		return Inbox{}, fmt.Errorf("sign in to %s: %w", srv.Addr, err)
	}

	data, err := c.Status("INBOX", &imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait()
	if err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return Inbox{}, fmt.Errorf("read the inbox on %s: %w", srv.Addr, err)
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
