package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"regexp"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// SMTPServer is a mail server to send through.
type SMTPServer struct {
	Addr string // host:port
	TLS  bool   // true for every real server (TLS from the start); false only for the tests' server
}

// GmailSMTP is Google's server for sending, for a personal Gmail and Google Workspace alike. It
// takes the same app password or Google token as IMAP, and keeps a copy in the Sent folder.
var GmailSMTP = SMTPServer{Addr: "smtp.gmail.com:465", TLS: true}

// Reply is a reply to write.
type Reply struct {
	From       *gomail.Address
	To, Cc     []*gomail.Address
	Subject    string   // already "Re: ..."
	InReplyTo  string   // the Message-ID answered, without < >
	References []string // the thread so far, then InReplyTo
	Text       string   // what the user wrote
	Quote      string   // the message answered, as text: quoted below the reply
	QuoteFrom  string   // "Ann Smith <ann@example.com>"
	QuoteDate  time.Time
}

// BuildReply writes a reply as a plain-text message, UTF-8, quoted-printable, with the headers
// that keep it in its thread. It returns the message and its Message-ID (without < >).
func BuildReply(r Reply, now time.Time) ([]byte, string, error) {
	var h gomail.Header
	h.SetDate(now)
	h.SetAddressList("From", []*gomail.Address{r.From})
	h.SetAddressList("To", r.To)
	if len(r.Cc) > 0 {
		h.SetAddressList("Cc", r.Cc)
	}
	h.SetSubject(r.Subject)
	host := "cockatoo.local"
	if at := strings.LastIndex(r.From.Address, "@"); at >= 0 {
		host = r.From.Address[at+1:]
	}
	if err := h.GenerateMessageIDWithHostname(host); err != nil {
		return nil, "", err
	}
	id, _ := h.MessageID()
	if r.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{r.InReplyTo})
		h.SetMsgIDList("References", append(append([]string{}, r.References...), r.InReplyTo))
	}
	h.Set("MIME-Version", "1.0")
	h.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	h.Set("Content-Transfer-Encoding", "quoted-printable")

	body := strings.TrimRight(strings.ReplaceAll(r.Text, "\r\n", "\n"), "\n ") + "\n"
	if q := strings.TrimSpace(r.Quote); q != "" {
		body += "\n" + fmt.Sprintf("On %s, %s wrote:", r.QuoteDate.Local().Format("Mon, 2 Jan 2006 at 15:04"), r.QuoteFrom) + "\n"
		for _, line := range strings.Split(q, "\n") {
			body += strings.TrimRight("> "+line, " ") + "\n"
		}
	}
	var buf bytes.Buffer
	w, err := gomail.CreateSingleInlineWriter(&buf, h)
	if err != nil {
		return nil, "", err
	}
	if _, err := w.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n"))); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), id, nil
}

var rePrefix = regexp.MustCompile(`(?i)^\s*((re|aw|antw|sv|vs)\s*(\[\d+\])?\s*:\s*)+`)

// ReplySubject is "Re: " and the subject, without stacking "Re: Re:".
func ReplySubject(subject string) string {
	return "Re: " + strings.TrimSpace(rePrefix.ReplaceAllString(subject, ""))
}

// SendError means the server would not take the message; its own words are kept.
type SendError struct{ Text string }

func (e *SendError) Error() string { return "Gmail did not take the message: " + e.Text }

// Send hands a message to the server for these recipients. from is the envelope sender.
func Send(ctx context.Context, srv SMTPServer, login Login, from string, rcpt []string, msg []byte) error {
	host, _, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(60 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	if srv.TLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", srv.Addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", srv.Addr)
	}
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", srv.Addr, err)
	}
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("talk to %s: %w", srv.Addr, err)
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		return err
	}

	var auth smtp.Auth
	if login.Token == nil {
		auth = &plainAuth{user: login.Username, password: login.Password}
	} else {
		token, err := login.Token(ctx)
		if err != nil {
			var te *TokenError
			if errors.As(err, &te) {
				return err
			}
			return &TokenError{Text: err.Error()}
		}
		auth = &xoauth2SMTP{user: login.Username, token: token}
	}
	if err := c.Auth(auth); err != nil {
		var se *textproto.Error
		if errors.As(err, &se) && (se.Code == 535 || se.Code == 534) {
			if login.Token != nil {
				return &TokenError{Text: se.Msg}
			}
			return &LoginError{Text: se.Msg}
		}
		return fmt.Errorf("sign in to %s: %w", srv.Addr, err)
	}
	if err := c.Mail(from); err != nil {
		return refused(err)
	}
	for _, r := range rcpt {
		if err := c.Rcpt(r); err != nil {
			return refused(err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return refused(err)
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return refused(err)
	}
	_ = c.Quit()
	return nil
}

func refused(err error) error {
	var se *textproto.Error
	if errors.As(err, &se) {
		return &SendError{Text: se.Msg}
	}
	return err
}

// plainAuth is PLAIN over a connection that is TLS from the start (or the tests' local server);
// net/smtp's own refuses anything it did not upgrade itself.
type plainAuth struct{ user, password string }

func (a *plainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.password), nil
}

func (a *plainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected challenge from the server")
	}
	return nil, nil
}

// xoauth2SMTP is Google's XOAUTH2 for SMTP. On a refused token the server sends a challenge with
// the reason; the answer is empty, and the server then says no.
type xoauth2SMTP struct{ user, token string }

func (a *xoauth2SMTP) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.user + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

func (a *xoauth2SMTP) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return []byte{}, nil
	}
	return nil, nil
}
