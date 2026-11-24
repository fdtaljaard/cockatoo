package mail

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// fakeSMTP is a tiny SMTP server: it takes PLAIN with password "pw", and keeps what it was given.
type fakeSMTP struct {
	mu   sync.Mutex
	auth string
	from string
	rcpt []string
	data string
	addr string
}

func startSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f := &fakeSMTP{addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(s string) { conn.Write([]byte(s + "\r\n")) }
	say("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		f.mu.Lock()
		switch {
		case cmd == "EHLO":
			say("250-fake\r\n250 AUTH PLAIN XOAUTH2")
		case strings.HasPrefix(strings.ToUpper(line), "AUTH PLAIN "):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN "):]))
			f.auth = string(raw)
			if strings.HasSuffix(f.auth, "\x00pw") {
				say("235 ok")
			} else {
				say("535 5.7.8 Username and Password not accepted")
			}
		case cmd == "MAIL":
			f.from = line
			say("250 ok")
		case cmd == "RCPT":
			f.rcpt = append(f.rcpt, line)
			say("250 ok")
		case cmd == "DATA":
			say("354 go on")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.data = b.String()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			f.mu.Unlock()
			return
		default:
			say("250 ok")
		}
		f.mu.Unlock()
	}
}

func TestBuildAndSendReply(t *testing.T) {
	when := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	msg, id, err := BuildReply(Reply{
		From:       &gomail.Address{Name: "Me", Address: "me@example.com"},
		To:         []*gomail.Address{{Name: "Ann Smith", Address: "ann@example.com"}},
		Cc:         []*gomail.Address{{Address: "bob@example.com"}},
		Subject:    ReplySubject("RE: Re: Lunch"),
		InReplyTo:  "a@x",
		References: []string{"z@x"},
		Text:       "Friday works — 12:30?",
		Quote:      "Lunch on Friday?\nKind regards\nAnn",
		QuoteFrom:  "Ann Smith <ann@example.com>",
		QuoteDate:  when,
	}, when)
	if err != nil || !strings.HasSuffix(id, "@example.com") {
		t.Fatalf("build: %v %q", err, id)
	}
	// Read back: the headers that keep it in its thread, and the text with the quote below.
	r := ReadMessage(msg)
	if r.Subject != "Re: Lunch" || r.InReplyTo != "a@x" || strings.Join(r.References, ",") != "z@x,a@x" || r.MessageID != id ||
		r.FromAddr != "me@example.com" || len(r.To) != 1 || len(r.Cc) != 1 {
		t.Fatalf("headers %+v", r)
	}
	if !strings.Contains(r.Full, "Friday works — 12:30?") || !strings.Contains(r.Full, "> Lunch on Friday?") || r.Text != "Friday works — 12:30?" {
		t.Fatalf("text %q / %q", r.Text, r.Full)
	}

	srv := startSMTP(t)
	ctx := context.Background()
	if err := Send(ctx, SMTPServer{Addr: srv.addr}, PasswordLogin("me@example.com", "pw"), "me@example.com",
		[]string{"ann@example.com", "bob@example.com"}, msg); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	if srv.auth != "\x00me@example.com\x00pw" || !strings.Contains(srv.from, "<me@example.com>") || len(srv.rcpt) != 2 ||
		!strings.Contains(srv.data, "In-Reply-To: <a@x>") {
		t.Fatalf("server saw auth %q from %q rcpt %v data\n%s", srv.auth, srv.from, srv.rcpt, srv.data)
	}
	srv.mu.Unlock()

	err = Send(ctx, SMTPServer{Addr: srv.addr}, PasswordLogin("me@example.com", "wrong"), "me@example.com", []string{"ann@example.com"}, msg)
	var le *LoginError
	if !errors.As(err, &le) || !strings.Contains(le.Text, "not accepted") {
		t.Fatalf("wrong password: %v", err)
	}
}
