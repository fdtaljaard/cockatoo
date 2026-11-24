package mail

import (
	"bufio"
	"bytes"
	"net/textproto"
	"net/url"
	"strings"
)

// Unsubscribe is how a message says it can be unsubscribed from (RFC 2369 List-Unsubscribe,
// RFC 8058 one-click). The values come from the sender and are not trusted: only https and
// mailto addresses are kept, and nothing here fetches or sends anything.
type Unsubscribe struct {
	URL      string // an https address to open
	Mailto   string // a mailto: address
	OneClick bool   // the sender supports a one-click POST to URL (RFC 8058)
}

// Empty reports whether the message offers no way to unsubscribe.
func (u Unsubscribe) Empty() bool { return u.URL == "" && u.Mailto == "" }

// unsubscribeFields are the header fields read for every message.
var unsubscribeFields = []string{"List-Unsubscribe", "List-Unsubscribe-Post"}

// parseUnsubscribe reads the List-Unsubscribe fields from a raw header block (as returned for
// BODY.PEEK[HEADER.FIELDS (...)]). Folded lines, several addresses and junk are all handled; what
// cannot be used is dropped.
func parseUnsubscribe(raw []byte) Unsubscribe {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Unsubscribe{}
	}
	r := textproto.NewReader(bufio.NewReader(bytes.NewReader(append(bytes.TrimRight(raw, "\r\n"), "\r\n\r\n"...))))
	h, err := r.ReadMIMEHeader()
	if err != nil && len(h) == 0 {
		return Unsubscribe{}
	}
	var u Unsubscribe
	for _, value := range h.Values("List-Unsubscribe") {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(part, "<") || !strings.HasSuffix(part, ">") {
				continue
			}
			addr := strings.TrimSpace(part[1 : len(part)-1])
			parsed, err := url.Parse(addr)
			if err != nil {
				continue
			}
			switch strings.ToLower(parsed.Scheme) {
			case "https":
				if u.URL == "" && parsed.Host != "" && len(addr) <= 2048 {
					u.URL = addr
				}
			case "mailto":
				if u.Mailto == "" && parsed.Opaque != "" && len(addr) <= 1024 {
					u.Mailto = addr
				}
			}
		}
	}
	post := strings.ToLower(strings.Join(strings.Fields(h.Get("List-Unsubscribe-Post")), ""))
	u.OneClick = u.URL != "" && post == "list-unsubscribe=one-click"
	return u
}
