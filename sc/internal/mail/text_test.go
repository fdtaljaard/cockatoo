package mail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
)

func TestExtractText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "Subject: x\r\nContent-Type: text/plain\r\n\r\nCan you send the budget by Friday?\r\nThanks, Ann\r\n",
			"Can you send the budget by Friday? Thanks, Ann"},
		{"quoted-printable", "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nCaf=C3=A9 at 10 =E2=80=93 ok?\r\n",
			"Café at 10 – ok?"},
		{"base64", "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\nUGxlYXNlIHNpZ24gdGhlIGNvbnRyYWN0Lg==\r\n",
			"Please sign the contract."},
		{"legacy charset", "Content-Type: text/plain; charset=windows-1252\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nRen=E9e asks: is =93today=94 fine?\r\n",
			"Renée asks: is “today” fine?"},
		{"html only", "Content-Type: text/html\r\n\r\n<html><head><style>p{}</style></head><body><p>Hello&nbsp;team,</p><p>Meeting moved to <b>3pm</b>.</p><script>x()</script></body></html>\r\n",
			"Hello team, Meeting moved to 3pm ."},
		{"multipart prefers plain", "Content-Type: multipart/alternative; boundary=b\r\n\r\n--b\r\nContent-Type: text/html\r\n\r\n<p>html version</p>\r\n--b\r\nContent-Type: text/plain\r\n\r\nplain version\r\n--b--\r\n",
			"plain version"},
		{"quoted reply cut", "Content-Type: text/plain\r\n\r\nYes, Thursday works.\r\n\r\nOn Mon, 23 Nov 2026 at 10:00, Bob <bob@x.com> wrote:\r\n> Does Thursday work?\r\n",
			"Yes, Thursday works."},
		{"cut mid-part", "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\n\r\nThe first lines arrive before the cu",
			"The first lines arrive before the cu"},
		{"empty", "", ""},
		{"attachment only", "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=a.pdf\r\n\r\nJVBERi0x\r\n--b--\r\n", ""},
	}
	for _, c := range cases {
		if got := ExtractText([]byte(c.raw), 500); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	if got := ExtractText([]byte("Content-Type: text/plain\r\n\r\n"+strings.Repeat("ab ", 400)), 10); got != "ab ab ab a…" {
		t.Errorf("cap: %q", got)
	}
}

func TestParseMarkers(t *testing.T) {
	raw := "List-Id: Team <team.example.com>\r\nPrecedence: BULK\r\nAuto-Submitted: auto-generated\r\n"
	if got := parseMarkers([]byte(raw)); got != (Markers{ListID: "team <team.example.com>", Precedence: "bulk", AutoSubmitted: "auto-generated"}) {
		t.Errorf("markers %+v", got)
	}
	if got := parseMarkers(nil); got != (Markers{}) {
		t.Errorf("none %+v", got)
	}
}

func TestSignalsTextAndSent(t *testing.T) {
	user := newUser(t)
	if err := user.Create("[Gmail]/Sent Mail", nil); err != nil {
		t.Fatal(err)
	}
	putRaw(t, user, "From: Ann <ann@example.com>\r\nTo: Me <ME@example.com>, x@example.com\r\nCc: boss@example.com\r\n"+
		"Message-ID: <q1@example.com>\r\nIn-Reply-To: <mine@example.com>\r\nList-Id: <team.example.com>\r\nSubject: budget\r\n"+
		"Content-Type: text/plain\r\n\r\nCan you send the budget by Friday?\r\n")
	if _, err := user.Append("[Gmail]/Sent Mail", literal{strings.NewReader(
		"From: me@example.com\r\nTo: ann@example.com\r\nMessage-ID: <mine@example.com>\r\nSubject: budget\r\n\r\nhi\r\n")},
		&imap.AppendOptions{Time: time.Now(), Flags: []imap.Flag{imap.FlagSeen}}); err != nil {
		t.Fatal(err)
	}
	srv := serve(t, user)
	ctx := context.Background()
	login := PasswordLogin("me@example.com", "pw")

	s, err := Open(ctx, srv, login)
	if err != nil {
		t.Fatal(err)
	}
	inbox, _ := s.Inbox()
	uid := inbox.Messages[0].UID
	got, err := s.Headers([]uint32{uid})
	if err != nil {
		t.Fatal(err)
	}
	m := got[0]
	if strings.Join(m.To, ",") != "me@example.com,x@example.com" || strings.Join(m.Cc, ",") != "boss@example.com" ||
		m.InReplyTo != "mine@example.com" || m.Markers.ListID != "<team.example.com>" {
		t.Fatalf("signals %+v", m)
	}
	texts, err := s.Texts([]uint32{uid}, 200)
	if err != nil || texts[uid] != "Can you send the budget by Friday?" {
		t.Fatalf("text %q %v", texts[uid], err)
	}
	s.Close()
	if count, _ := CheckInbox(ctx, srv, login); count.Unseen != 1 {
		t.Fatalf("reading the text marked the message read: %+v", count)
	}

	// The Sent folder, found by Gmail's name when the server does not mark it.
	sent, found, err := OpenSent(ctx, srv, login)
	if err != nil || !found {
		t.Fatalf("sent %v %v", found, err)
	}
	defer sent.Close()
	list, err := sent.Inbox()
	if err != nil || len(list.Messages) != 1 {
		t.Fatalf("sent list %+v %v", list, err)
	}
	h, err := sent.Headers([]uint32{list.Messages[0].UID})
	if err != nil || h[0].MessageID != "mine@example.com" || strings.Join(h[0].To, ",") != "ann@example.com" || h[0].FromAddr != "me@example.com" {
		t.Fatalf("sent headers %+v %v", h, err)
	}

	// No Sent folder at all: not found, not an error.
	plain := serve(t, newUser(t))
	if _, found, err := OpenSent(ctx, plain, login); found || err != nil {
		t.Fatalf("no sent folder: %v %v", found, err)
	}
}
