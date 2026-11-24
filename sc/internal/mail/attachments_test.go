package mail

import (
	"context"
	"strings"
	"testing"
)

const invoiceMail = "From: Billing <billing@shop.example>\r\nSubject: Your invoice\r\nMIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=outer\r\n\r\n" +
	"--outer\r\nContent-Type: multipart/related; boundary=rel\r\n\r\n" +
	"--rel\r\nContent-Type: text/html\r\n\r\n<p>Thanks <img src=\"cid:logo\"></p>\r\n" +
	"--rel\r\nContent-Type: image/png; name=logo.png\r\nContent-ID: <logo>\r\nContent-Disposition: inline; filename=logo.png\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\niVBORw0KGgo=\r\n--rel--\r\n" +
	"--outer\r\nContent-Type: application/pdf; name=\"invoice.pdf\"\r\nContent-Disposition: attachment; filename=\"invoice.pdf\"\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n" +
	"--outer\r\nContent-Type: application/pdf\r\nContent-Disposition: inline; filename*=UTF-8''Rechnung%20M%C3%BCller.pdf\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n" +
	"--outer\r\nContent-Type: image/jpeg; name=\"=?UTF-8?B?cmXDp3UuanBn?=\"\r\nContent-Disposition: inline\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\n/9j/4AAQ\r\n" +
	"--outer\r\nContent-Type: message/rfc822\r\nContent-Disposition: attachment\r\n\r\n" +
	"From: a@b.c\r\nSubject: fwd\r\n\r\nforwarded body\r\n" +
	"--outer\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment\r\n\r\nbytes\r\n" +
	"--outer--\r\n"

func TestExtractAttachments(t *testing.T) {
	atts, skipped := ExtractAttachments([]byte(invoiceMail))
	var names []string
	for _, a := range atts {
		names = append(names, a.Name)
	}
	want := "invoice.pdf,Rechnung Müller.pdf,reçu.jpg,forwarded message.eml,attachment.bin"
	if strings.Join(names, ",") != want {
		t.Fatalf("names %q, want %q", strings.Join(names, ","), want)
	}
	if string(atts[0].Data) != "%PDF-1.4\n" {
		t.Errorf("invoice bytes %q", atts[0].Data)
	}
	if !strings.Contains(string(atts[3].Data), "forwarded body") {
		t.Errorf("forwarded message %q", atts[3].Data)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "logo.png") {
		t.Errorf("skipped %v", skipped)
	}

	if atts, _ := ExtractAttachments([]byte("Content-Type: text/plain\r\n\r\njust text\r\n")); len(atts) != 0 {
		t.Errorf("a plain message has attachments: %+v", atts)
	}
	if atts, _ := ExtractAttachments(nil); len(atts) != 0 {
		t.Errorf("nothing has attachments: %+v", atts)
	}
}

func TestRawPeeksAndCaps(t *testing.T) {
	user := newUser(t)
	putRaw(t, user, invoiceMail)
	srv := serve(t, user)
	ctx := context.Background()
	login := PasswordLogin("me@example.com", "pw")
	s, err := Open(ctx, srv, login)
	if err != nil {
		t.Fatal(err)
	}
	inbox, _ := s.Inbox()
	uid := inbox.Messages[0].UID
	raw, tooBig, err := s.Raw([]uint32{uid, 9999})
	if err != nil || len(tooBig) != 0 || len(raw) != 1 || !strings.Contains(string(raw[uid]), "invoice.pdf") {
		t.Fatalf("raw %d %v %v", len(raw), tooBig, err)
	}
	s.Close()
	if count, _ := CheckInbox(ctx, srv, login); count.Unseen != 1 {
		t.Fatalf("reading the message marked it read: %+v", count)
	}
}
