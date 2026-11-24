package mail

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
)

const goodToken = "ya29.good-token"

// tokenSession is an in-memory IMAP session that also signs in with XOAUTH2, as Gmail does. A
// refused token gets Gmail's behaviour: a JSON challenge first, then NO after the empty answer.
type tokenSession struct {
	imapserver.Session
	password string
}

func (s *tokenSession) AuthenticateMechanisms() []string { return []string{"XOAUTH2"} }

func (s *tokenSession) Authenticate(mech string) (sasl.Server, error) {
	if mech != "XOAUTH2" {
		return nil, errors.New("unsupported mechanism")
	}
	return &xoauth2Server{s: s}, nil
}

type xoauth2Server struct {
	s        *tokenSession
	refusing bool
}

func (x *xoauth2Server) Next(response []byte) ([]byte, bool, error) {
	if x.refusing {
		return nil, true, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Invalid credentials (Failure)"}
	}
	var user, token string
	for _, part := range strings.Split(string(response), "\x01") {
		if v, ok := strings.CutPrefix(part, "user="); ok {
			user = v
		}
		if v, ok := strings.CutPrefix(part, "auth=Bearer "); ok {
			token = v
		}
	}
	if token != goodToken {
		x.refusing = true
		return []byte(`{"status":"400","schemes":"Bearer","scope":"https://mail.google.com/"}`), false, nil
	}
	if err := x.s.Login(user, x.s.password); err != nil {
		return nil, true, err
	}
	return nil, true, nil
}

func serveTokens(t *testing.T, user *imapmemserver.User) Server {
	t.Helper()
	mem := imapmemserver.New()
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &tokenSession{Session: mem.NewSession(), password: "pw"}, nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return Server{Addr: ln.Addr().String()}
}

func TestTokenLogin(t *testing.T) {
	user := newUser(t)
	putRaw(t, user, "From: a@example.com\r\nSubject: hi\r\n\r\nx\r\n")
	srv := serveTokens(t, user)
	ctx := context.Background()

	asked := 0
	login := Login{Username: "me@example.com", Token: func(context.Context) (string, error) {
		asked++
		return goodToken, nil
	}}
	inbox, err := CheckInbox(ctx, srv, login)
	if err != nil || inbox.Messages != 1 {
		t.Fatalf("good token: %+v %v", inbox, err)
	}
	// A fresh token for every connection.
	s, err := Open(ctx, srv, login)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if asked != 2 {
		t.Errorf("token asked %d times, want 2", asked)
	}

	// A refused token: a TokenError carrying the server's words and its challenge.
	_, err = CheckInbox(ctx, srv, Login{Username: "me@example.com", Token: func(context.Context) (string, error) { return "expired", nil }})
	var te *TokenError
	if !errors.As(err, &te) || !strings.Contains(te.Text, "Invalid credentials") || !strings.Contains(te.Text, `"status":"400"`) {
		t.Fatalf("refused token: %v", err)
	}

	// No token to be had (Google refused the refresh): a TokenError, and no connection is tried.
	_, err = CheckInbox(ctx, srv, Login{Username: "me@example.com", Token: func(context.Context) (string, error) {
		return "", &TokenError{Text: "invalid_grant"}
	}})
	if !errors.As(err, &te) || te.Text != "invalid_grant" {
		t.Fatalf("no token: %v", err)
	}

	// A server that does not offer token sign-in.
	plain := serve(t, newUser(t))
	if _, err := CheckInbox(ctx, plain, login); err == nil || !strings.Contains(err.Error(), "does not offer sign-in with a Google token") {
		t.Fatalf("no XOAUTH2: %v", err)
	}
}
