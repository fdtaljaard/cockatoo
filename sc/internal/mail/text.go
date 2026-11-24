package mail

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	gomail "github.com/emersion/go-message/mail"
)

// textBytes is how much of a message is fetched to find the start of its text: enough for the
// headers and the first part of almost any message, without its attachments.
const textBytes = 64 * 1024

// Texts reads the start of the text of the messages with these UIDs, at most max characters
// each. It peeks: nothing is marked as read. The text is returned, never stored.
func (s *Session) Texts(uids []uint32, max int) (map[uint32]string, error) {
	out := map[uint32]string{}
	section := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: textBytes}}
	for start := 0; start < len(uids); start += 50 {
		part := uids[start:min(start+50, len(uids))]
		set := imap.UIDSet{}
		for _, u := range part {
			set.AddNum(imap.UID(u))
		}
		var fetched []*imapclient.FetchMessageBuffer
		err := s.timed(func() error {
			var err error
			fetched, err = s.c.Fetch(set, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("read message text on %s: %w", s.addr, err)
		}
		for _, m := range fetched {
			raw := m.FindBodySection(section)
			if raw == nil && len(m.BodySection) == 1 {
				raw = m.BodySection[0].Bytes
			}
			out[uint32(m.UID)] = ExtractText(raw, max)
		}
	}
	return out, nil
}

// ExtractText returns the start of what a person wrote in a message, from its raw bytes (which
// may be cut short): the first plain-text part, or else the first HTML part made plain, without
// quoted replies, at most max characters. Empty when there is no text to be had. Transfer
// encodings and charsets are decoded by go-message (the charset package is imported in mail.go).
func ExtractText(raw []byte, max int) string {
	if len(raw) == 0 {
		return ""
	}
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if r == nil {
		_ = err
		return ""
	}
	var fromHTML string
	for {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		h, ok := p.Header.(*gomail.InlineHeader)
		if !ok {
			continue
		}
		ct, _, _ := h.ContentType()
		body, _ := io.ReadAll(io.LimitReader(p.Body, textBytes)) // a part cut short still gives what came
		switch strings.ToLower(ct) {
		case "text/plain", "":
			if t := tidy(string(body), max); t != "" {
				return t
			}
		case "text/html":
			if fromHTML == "" {
				fromHTML = tidy(plainFromHTML(string(body)), max)
			}
		}
	}
	return fromHTML
}

var (
	scriptStyle = regexp.MustCompile(`(?is)<(script|style|head)\b[^>]*>.*?</(script|style|head)\s*>`)
	breaks      = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/tr|/li|/h[1-6])\s*/?>`)
	tags        = regexp.MustCompile(`(?s)<[^>]*>`)
	quoteStart  = regexp.MustCompile(`(?i)^(on .{4,200} wrote:|-{2,}\s*original message\s*-{2,}|_{10,}|from: .+@.+)$`)
)

// plainFromHTML is crude on purpose: enough to read the first lines of an HTML-only message.
func plainFromHTML(s string) string {
	s = scriptStyle.ReplaceAllString(s, " ")
	s = breaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, " ")
	return html.UnescapeString(s)
}

// tidy keeps what the sender wrote: it stops where an earlier message is quoted, drops quoted
// lines and collapses white space.
func tidy(s string, max int) string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if quoteStart.MatchString(t) && len(kept) > 0 {
			break
		}
		if strings.HasPrefix(t, ">") || t == "" {
			continue
		}
		kept = append(kept, t)
	}
	out := strings.Join(strings.Fields(strings.Join(kept, " ")), " ")
	if r := []rune(out); len(r) > max {
		out = string(r[:max]) + "…"
	}
	return out
}
