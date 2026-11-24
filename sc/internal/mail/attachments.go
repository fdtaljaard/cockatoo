package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	gomail "github.com/emersion/go-message/mail"
)

// MaxMessageBytes is the largest message whose attachments are fetched: bigger ones are reported,
// not read.
const MaxMessageBytes = 50 << 20

// Attachment is one file in a message.
type Attachment struct {
	Name        string // as the sender named it, decoded; may be empty
	ContentType string
	Data        []byte
}

// Raw reads whole messages by UID, one at a time so a large one does not hold up the rest. It
// peeks: nothing is marked read. Messages over MaxMessageBytes are not read; their sizes are
// returned in tooBig. A UID no longer in the folder is in neither map.
func (s *Session) Raw(uids []uint32) (raw map[uint32][]byte, tooBig map[uint32]int64, err error) {
	raw, tooBig = map[uint32][]byte{}, map[uint32]int64{}
	if len(uids) == 0 {
		return raw, tooBig, nil
	}
	set := imap.UIDSet{}
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	var sizes []*imapclient.FetchMessageBuffer
	err = s.timed(func() error {
		var err error
		sizes, err = s.c.Fetch(set, &imap.FetchOptions{UID: true, RFC822Size: true}).Collect()
		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read message sizes on %s: %w", s.addr, err)
	}
	whole := &imap.FetchItemBodySection{Peek: true}
	for _, m := range sizes {
		uid := uint32(m.UID)
		if m.RFC822Size > MaxMessageBytes {
			tooBig[uid] = m.RFC822Size
			continue
		}
		var got []*imapclient.FetchMessageBuffer
		err = s.timed(func() error {
			var err error
			got, err = s.c.Fetch(imap.UIDSetNum(m.UID), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{whole}}).Collect()
			return err
		})
		if err != nil {
			return nil, nil, fmt.Errorf("read a message on %s: %w", s.addr, err)
		}
		for _, g := range got {
			b := g.FindBodySection(whole)
			if b == nil && len(g.BodySection) == 1 {
				b = g.BodySection[0].Bytes
			}
			raw[uid] = b
		}
	}
	return raw, tooBig, nil
}

// ExtractAttachments returns the files in a message: every part sent as an attachment, every
// part with a file name that is not the message's own text (invoices and phone photos of
// receipts often come marked inline), and forwarded messages as .eml files. Pictures embedded in
// the message's own HTML (a logo, a signature) are not files; they are listed in skipped so
// nothing is left out silently.
func ExtractAttachments(raw []byte) (atts []Attachment, skipped []string) {
	atts = []Attachment{}
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if r == nil {
		_ = err
		return atts, nil
	}
	for {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		var name, ct string
		var embedded bool
		switch h := p.Header.(type) {
		case *gomail.AttachmentHeader:
			name, _ = h.Filename()
			ct, _, _ = h.ContentType()
		case *gomail.InlineHeader:
			var params map[string]string
			ct, params, _ = h.ContentType()
			_, disp, _ := h.ContentDisposition()
			name = disp["filename"]
			if name == "" {
				name = params["name"]
			}
			name = decodeWords(name)
			ct = strings.ToLower(ct)
			isText := ct == "" || ct == "text/plain" || ct == "text/html"
			if name == "" && isText {
				continue // the message's own text
			}
			if name == "" && ct != "message/rfc822" {
				continue // an unnamed inline part that is not a forwarded message
			}
			embedded = strings.HasPrefix(ct, "image/") && h.Get("Content-Id") != ""
		}
		data, _ := io.ReadAll(io.LimitReader(p.Body, MaxMessageBytes))
		if embedded {
			skipped = append(skipped, fmt.Sprintf("a picture shown in the message itself (%s)", name))
			continue
		}
		ct = strings.ToLower(ct)
		if name == "" {
			name = unnamed(ct)
		}
		if ct == "message/rfc822" && !strings.HasSuffix(strings.ToLower(name), ".eml") {
			name += ".eml"
		}
		atts = append(atts, Attachment{Name: name, ContentType: ct, Data: data})
	}
	return atts, skipped
}

// decodeWords decodes an RFC 2047 encoded file name ("=?UTF-8?B?...?=") as some mail programs
// send them; RFC 2231 names are decoded by the MIME parameter parser already.
func decodeWords(s string) string {
	d := mime.WordDecoder{CharsetReader: charset.Reader}
	if out, err := d.DecodeHeader(s); err == nil {
		return out
	}
	return s
}

// unnamed is the name for an attachment its sender did not name.
func unnamed(ct string) string {
	if ct == "message/rfc822" {
		return "forwarded message.eml"
	}
	if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
		return "attachment" + exts[0]
	}
	return "attachment"
}
