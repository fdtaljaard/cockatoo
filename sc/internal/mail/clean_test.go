package mail

import (
	"strings"
	"testing"
)

func mailOf(subject, contentType, body string) []byte {
	return []byte("From: Ann Smith <Ann@Example.com>\r\nSubject: " + subject + "\r\nMessage-ID: <m1@example.com>\r\n" +
		"In-Reply-To: <m0@example.com>\r\nReferences: <a@x> <m0@example.com>\r\nDate: Mon, 23 Nov 2026 10:00:00 +0200\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: " + contentType + "; charset=utf-8\r\n\r\n" + body)
}

func TestReadMessage(t *testing.T) {
	for _, c := range []struct {
		name, subject, ct, body string
		want                    string // the cleaned text, exactly
		notIn                   []string
	}{
		{"gmail reply", "Re: Lunch", "text/html",
			`<div dir="ltr">Sounds good, see you then.<br><br>Kind regards<br>Ann<br>Ann Smith | Director<br>+27 82 000 0000<br>www.acme.example</div>` +
				`<div class="gmail_signature"><img src="https://x/logo.png">Ann Smith</div>` +
				`<br><div class="gmail_quote"><div dir="ltr" class="gmail_attr">On Mon, 23 Nov 2026 at 09:00, Bob &lt;bob@x&gt; wrote:<br></div>` +
				`<blockquote class="gmail_quote"><div>Lunch on Friday?</div><blockquote><div>older</div></blockquote></blockquote></div>`,
			"Sounds good, see you then.\n\nKind regards\nAnn", []string{"Lunch on Friday", "Director"}},
		{"outlook reply", "RE: Budget", "text/html",
			`<html><head><style>p{margin:0}</style></head><body><div>Hi Bob,</div><div>Approved.</div>` +
				`<div id="Signature"><p>Bob Jones<br>CFO</p><img src="cid:x"></div><div id="appendonsend"></div><hr>` +
				`<div id="divRplyFwdMsg"><b>From:</b> Ann<br><b>Sent:</b> Monday</div><div>Please approve the budget</div></body></html>`,
			"Hi Bob,\nApproved.", []string{"CFO", "Please approve"}},
		{"phone reply", "Re: Lunch", "text/plain",
			"Yes please\r\n\r\nSent from my iPhone\r\n\r\n> On 24 Nov 2026, at 10:00, Ann <ann@x> wrote:\r\n> \r\n> Lunch?\r\n",
			"Yes please", nil},
		{"gmail plain, attribution on two lines", "Re: x", "text/plain",
			"Fine by me.\r\n\r\nOn Mon, 23 Nov 2026 at 09:00, Bob Jones <bob@example.com>\r\nwrote:\r\n\r\n> Shall we?\r\n",
			"Fine by me.", nil},
		{"outlook plain", "RE: x", "text/plain",
			"Approved.\r\n\r\n________________________________\r\nFrom: Ann <ann@x>\r\nSent: Monday\r\nSubject: Re: x\r\n\r\nold text\r\n",
			"Approved.", []string{"old text"}},
		{"signature line", "x", "text/plain", "Thanks!\r\n-- \r\nAnn\r\nACME Ltd\r\n", "Thanks!", nil},
		{"disclaimer", "x", "text/plain",
			"Done.\r\n\r\nThis email and any attachments are confidential and intended solely for the addressee. If you have received it in error, delete it.\r\n",
			"Done.", nil},
	} {
		r := ReadMessage(mailOf(c.subject, c.ct, c.body))
		if r.Text != c.want {
			t.Errorf("%s: text\n%q\nwant\n%q", c.name, r.Text, c.want)
		}
		for _, n := range c.notIn {
			if strings.Contains(r.Text, n) {
				t.Errorf("%s: %q left in the text", c.name, n)
			}
		}
		if r.Full == "" || len(r.Full) < len(r.Text) {
			t.Errorf("%s: full text %q", c.name, r.Full)
		}
	}
}

func TestReadMessageKeepsContent(t *testing.T) {
	// Answers written between the quoted lines are kept, quotes and all.
	inline := ReadMessage(mailOf("Re: Questions", "text/plain",
		"See below.\r\n\r\nOn Mon, Bob wrote:\r\n> Question one?\r\nAnswer one.\r\n> Question two?\r\nAnswer two.\r\n"))
	if !strings.Contains(inline.Text, "Answer one.") || !strings.Contains(inline.Text, "Answer two.") || !strings.Contains(inline.Text, "> Question two?") {
		t.Errorf("inline reply: %q", inline.Text)
	}
	// A forward's quoted message is its content.
	fwd := ReadMessage(mailOf("Fwd: Invoice", "text/html",
		`<div>FYI</div><div class="gmail_quote"><div class="gmail_attr">---------- Forwarded message ---------<br>From: Billing</div><div>Please pay R 1,200 by Friday.</div></div>`))
	if !strings.Contains(fwd.Text, "Please pay R 1,200 by Friday.") {
		t.Errorf("forward: %q", fwd.Text)
	}
	// Only a quote: the text falls back to everything, never empty.
	quote := ReadMessage(mailOf("Re: x", "text/plain", "> old\r\n> stuff\r\n"))
	if quote.Text == "" || quote.Text != quote.Full {
		t.Errorf("only a quote: %q / %q", quote.Text, quote.Full)
	}
	// HTML only, with pictures and entities: text, no pictures.
	promo := ReadMessage(mailOf("Sale", "text/html", `<table><tr><td><img src="https://track.example/p.gif">Big&nbsp;sale &amp; more</td></tr></table><p>Ends <b>Friday</b></p>`))
	if promo.Text != "Big sale & more\n\nEnds Friday" {
		t.Errorf("html only: %q", promo.Text)
	}
	// Headers.
	if promo.MessageID != "m1@example.com" || promo.InReplyTo != "m0@example.com" || len(promo.References) != 2 ||
		promo.FromAddr != "ann@example.com" || promo.FromName != "Ann Smith" || promo.Date.IsZero() {
		t.Errorf("headers %+v", promo)
	}
	// Cut short in the middle: whatever came is read, without a panic.
	raw := mailOf("x", "text/html", `<div>The first part arrived</div><div class="gmail_quote"><blockquote>and then the`)
	if r := ReadMessage(raw[:len(raw)-5]); r.Text != "The first part arrived" {
		t.Errorf("cut short: %q", r.Text)
	}
	if !SameMessageID("<a@b>", "a@b") {
		t.Error("message ids")
	}
}
