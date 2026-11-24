package mail

import (
	"bytes"
	"html"
	"io"
	"regexp"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// Reading a message: what the person wrote, without what clutters it. Quoted earlier messages,
// signatures, pictures, "Sent from my phone" lines and trailing disclaimers are taken out of
// Text; Full keeps everything as text, so nothing is ever lost. Mail HTML is never shown as HTML:
// it is turned into text here, so no remote picture (or tracking pixel) is ever loaded.

// Read is one message, as the conversation shows it.
type Read struct {
	Subject    string
	MessageID  string // without < >
	InReplyTo  string
	References []string
	FromName   string
	FromAddr   string
	ReplyTo    []*gomail.Address // where replies go, when the sender says
	To, Cc     []*gomail.Address
	Date       time.Time
	Text       string // what the person wrote
	Full       string // the whole message, as text
}

// maxChars bounds the text of one message.
const maxChars = 100_000

// ReadMessage reads a message from its raw bytes (which may be cut short).
func ReadMessage(raw []byte) Read {
	var out Read
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if r == nil {
		_ = err
		return out
	}
	out.Subject, _ = r.Header.Subject()
	out.MessageID = msgID(r.Header.Get("Message-Id"))
	out.InReplyTo = msgID(r.Header.Get("In-Reply-To"))
	for _, id := range strings.Fields(strings.NewReplacer("<", " ", ">", " ").Replace(r.Header.Get("References"))) {
		out.References = append(out.References, id)
	}
	if from, err := r.Header.AddressList("From"); err == nil && len(from) > 0 {
		out.FromName, out.FromAddr = from[0].Name, strings.ToLower(from[0].Address)
	}
	out.Date, _ = r.Header.Date()
	out.ReplyTo, _ = r.Header.AddressList("Reply-To")
	out.To, _ = r.Header.AddressList("To")
	out.Cc, _ = r.Header.AddressList("Cc")

	var plain, htmlBody string
	for plain == "" || htmlBody == "" {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		h, ok := p.Header.(*gomail.InlineHeader)
		if !ok {
			continue
		}
		ct, _, _ := h.ContentType()
		body, _ := io.ReadAll(io.LimitReader(p.Body, ReadBytes)) // a part cut short still gives what came
		switch strings.ToLower(ct) {
		case "text/plain", "":
			if plain == "" {
				plain = string(body)
			}
		case "text/html":
			if htmlBody == "" {
				htmlBody = string(body)
			}
		}
	}
	forward := forwardSubject.MatchString(strings.TrimSpace(out.Subject))
	// HTML first: quote and signature blocks are marked there in any language; the plain part has
	// only text rules to go on.
	if strings.TrimSpace(htmlBody) != "" {
		out.Full = limit(collapse(htmlText(htmlBody, false, false)))
		out.Text = limit(cleanText(htmlText(htmlBody, true, forward), forward))
	} else {
		out.Full = limit(collapse(plain))
		out.Text = limit(cleanText(plain, forward))
	}
	if out.Text == "" {
		out.Text = out.Full
	}
	return out
}

func msgID(s string) string { return strings.Trim(strings.TrimSpace(s), "<>") }

// SameMessageID compares two Message-IDs as stored by different parts of cockatoo.
func SameMessageID(a, b string) bool { return msgID(a) == msgID(b) }

func limit(s string) string {
	if r := []rune(s); len(r) > maxChars {
		return string(r[:maxChars]) + "…"
	}
	return s
}

var forwardSubject = regexp.MustCompile(`(?i)^(fwd?|fw|wg|tr|doorgestuur|vs)\s*:`)

// The marks mail programs put on quoted messages and signatures in HTML. Extend as real mail shows
// more.
var (
	quoteClasses = set("gmail_quote", "gmail_quote_container", "gmail_extra", "x_gmail_quote", "moz-cite-prefix",
		"yahoo_quoted", "protonmail_quote", "outlookmessageheader", "applemailoriginalcontents")
	signatureClasses = set("gmail_signature", "gmail_signature_prefix", "x_gmail_signature", "moz-signature",
		"protonmail_signature_block", "protonmail_signature_block-user", "signature")
	signatureIDs = set("signature", "x_signature", "ms-outlook-mobile-signature", "x_ms-outlook-mobile-signature",
		"zm-_id_-sgn", "outlook-signature")
	// From these on, the rest of the message is the one being answered (Outlook).
	cutIDs = set("divrplyfwdmsg", "x_divrplyfwdmsg", "appendonsend", "x_appendonsend")
)

func set(names ...string) map[string]bool {
	out := map[string]bool{}
	for _, n := range names {
		out[n] = true
	}
	return out
}

var (
	comments  = regexp.MustCompile(`(?s)<!--.*?-->`)
	hidden    = regexp.MustCompile(`(?is)<(script|style|head|title)\b[^>]*>.*?</(script|style|head|title)\s*>`)
	tagRe     = regexp.MustCompile(`(?s)<(/?)([a-zA-Z][a-zA-Z0-9:-]*)([^>]*)>`)
	classAttr = regexp.MustCompile(`(?i)\bclass\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	idAttr    = regexp.MustCompile(`(?i)\bid\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	voidTags  = set("br", "img", "hr", "meta", "link", "input", "wbr", "col", "area", "base", "source", "embed")
	lineTags  = set("div", "tr", "li", "h1", "h2", "h3", "h4", "h5", "h6", "table", "ul", "ol", "blockquote", "pre", "center")
)

func attr(re *regexp.Regexp, attrs string) string {
	m := re.FindStringSubmatch(attrs)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1] + m[2] + m[3])
}

// htmlText turns mail HTML into text. With clean, quoted messages, signatures and the rest after
// an Outlook reply header are left out (quotes are kept in a forward: they are its content).
// Pictures are always left out. A small tag scanner, not a full parser: mail HTML is often not
// well formed, and only blocks need to be found.
func htmlText(s string, clean, forward bool) string {
	s = comments.ReplaceAllString(s, " ")
	s = hidden.ReplaceAllString(s, " ")
	var b strings.Builder
	skipping, depth := "", 0
	last := 0
	for _, m := range tagRe.FindAllStringSubmatchIndex(s, -1) {
		if skipping == "" {
			b.WriteString(s[last:m[0]])
		}
		last = m[1]
		closing := m[3] > m[2]
		name := strings.ToLower(s[m[4]:m[5]])
		attrs := s[m[6]:m[7]]
		selfClosing := voidTags[name] || strings.HasSuffix(strings.TrimSpace(attrs), "/")
		if skipping != "" {
			if name == skipping && !selfClosing {
				if closing {
					depth--
				} else {
					depth++
				}
				if depth == 0 {
					skipping = ""
				}
			}
			continue
		}
		if !closing && clean {
			id := attr(idAttr, attrs)
			if cutIDs[id] && !forward && strings.TrimSpace(html.UnescapeString(b.String())) != "" {
				last = len(s) // the rest is the message being answered
				break
			}
			if !selfClosing && (signatureIDs[id] || (!forward && name == "blockquote") || classMatch(attrs, forward)) {
				skipping, depth = name, 1
				continue
			}
		}
		switch {
		case name == "br":
			b.WriteString("\n")
		case name == "p":
			endLine(&b)
			b.WriteString("\n")
		case name == "li" && !closing:
			endLine(&b)
			b.WriteString("• ")
		case lineTags[name]:
			endLine(&b)
		case name == "td" || name == "th":
			b.WriteString(" ")
		}
	}
	if skipping == "" && last < len(s) {
		b.WriteString(s[last:])
	}
	text := html.UnescapeString(b.String())
	text = strings.ReplaceAll(text, " ", " ")
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		lines = append(lines, strings.Join(strings.Fields(line), " "))
	}
	return strings.Join(lines, "\n")
}

// endLine starts a new line, unless the text is at the start of one already. Spaces are
// trimmed later, so only a real line ending counts.
func endLine(b *strings.Builder) {
	t := strings.TrimRight(b.String(), " \t\r")
	if t != "" && !strings.HasSuffix(t, "\n") {
		b.WriteString("\n")
	}
}

func classMatch(attrs string, forward bool) bool {
	for _, c := range strings.Fields(attr(classAttr, attrs)) {
		if signatureClasses[c] || (!forward && quoteClasses[c]) {
			return true
		}
	}
	return false
}

var (
	attribution  = regexp.MustCompile(`(?i)^(on .{4,200} wrote:?|am .{4,200} schrieb .{1,120}:|le .{4,200} a écrit\s?:|el .{4,200} escribió:|op .{4,200} (schreef|het .{1,80} geskryf):?|.{4,200} wrote:)$`)
	originalLine = regexp.MustCompile(`(?i)^-{2,}\s*(original message|ursprüngliche nachricht|oorspronklike boodskap|message d'origine)\s*-{2,}$`)
	fromLine     = regexp.MustCompile(`(?i)^\*?(from|von|de|van)\*?:\s*\S`)
	sentLine     = regexp.MustCompile(`(?i)^\*?(sent|date|gesendet|envoyé|verzonden|datum)\*?:\s*\S`)
	underline    = regexp.MustCompile(`^_{10,}$`)
	deviceLine   = regexp.MustCompile(`(?i)^(sent from my .{1,40}|get outlook for (ios|android).*|sent from (mail|outlook) for windows.*|sent from yahoo mail.*|sent with proton mail.*|verstuur vanaf my .{1,40}|verstuurd vanaf mijn .{1,40}|gesendet von meinem .{1,40}|envoyé de mon .{1,40})$`)
	disclaimer   = regexp.MustCompile(`(?i)^(this e-?mail|this message|this communication|the information (contained|in this)|confidential(ity)?( notice| note| statement)?\b|disclaimer|important:? this|privileged|the content of this e-?mail|please consider the environment|think before you print|e-?mail disclaimer)`)
	signOff      = regexp.MustCompile(`(?i)^(kind regards|warm regards|best regards|regards|best wishes|best|many thanks|thanks( again)?|thank you|cheers|sincerely|yours sincerely|yours faithfully|groete|vriendelike groete|beste groete|mvg|met vriendelijke groet|liebe grüße|mit freundlichen grüßen|cordialement)[,.!]*$`)
)

// cleanText keeps what the person wrote: it stops where an earlier message is quoted (unless the
// answers are written between the quoted lines, or the message is a forward), at a signature
// line ("-- "), and drops device lines, trailing disclaimers, and the contact details below a
// sign-off. Every cut needs text above it, so a message is never cleaned away.
func cleanText(s string, forward bool) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	inline := inlineReply(lines)
	hasText := func(ls []string) bool {
		for _, l := range ls {
			if t := strings.TrimSpace(l); t != "" && !strings.HasPrefix(t, ">") {
				return true
			}
		}
		return false
	}

	var kept []string
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !forward && !inline && hasText(kept) && quoteHeader(lines, i) {
			break
		}
		if (t == "--" || line == "-- ") && hasText(kept) {
			break
		}
		if strings.HasPrefix(t, ">") && !inline && !forward {
			continue
		}
		if deviceLine.MatchString(t) {
			continue
		}
		kept = append(kept, line)
	}
	kept = dropDisclaimers(kept)
	kept = trimSignOff(kept)
	return collapse(strings.Join(kept, "\n"))
}

// quoteHeader says whether line i starts the quote of an earlier message.
func quoteHeader(lines []string, i int) bool {
	t := strings.TrimSpace(lines[i])
	if attribution.MatchString(t) || originalLine.MatchString(t) {
		return true
	}
	// "On Mon, 24 Nov 2026 at 10:00, Ann <ann@example.com>" and "wrote:" on the next line.
	if i+1 < len(lines) && len(t) < 200 && attribution.MatchString(t+" "+strings.TrimSpace(lines[i+1])) &&
		strings.HasSuffix(strings.TrimSpace(lines[i+1]), ":") && len(strings.TrimSpace(lines[i+1])) < 40 {
		return true
	}
	if underline.MatchString(t) || fromLine.MatchString(t) {
		for j := i + 1; j < len(lines) && j <= i+5; j++ {
			n := strings.TrimSpace(lines[j])
			if sentLine.MatchString(n) || (underline.MatchString(t) && fromLine.MatchString(n)) {
				return true
			}
		}
	}
	return false
}

// inlineReply says whether new text is written between quoted lines, after the first quote.
func inlineReply(lines []string) bool {
	quoted, textAfter := false, false
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, ">"):
			if textAfter {
				return true
			}
			quoted = true
		case t != "" && quoted:
			textAfter = true
		}
	}
	return false
}

// dropDisclaimers takes off trailing paragraphs that are legal notices.
func dropDisclaimers(lines []string) []string {
	for {
		end := len(lines)
		for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
			end--
		}
		start := end
		for start > 0 && strings.TrimSpace(lines[start-1]) != "" {
			start--
		}
		para := strings.Join(lines[start:end], " ")
		if start == 0 || len(para) < 80 || !disclaimer.MatchString(strings.TrimSpace(para)) {
			return lines[:end]
		}
		lines = lines[:start]
	}
}

// trimSignOff keeps a sign-off ("Kind regards") and the name below it, and drops the few short
// lines after that: a title, phone numbers, an address.
func trimSignOff(lines []string) []string {
	var idx []int
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			idx = append(idx, i)
		}
	}
	from := max(0, len(idx)-10)
	for k := len(idx) - 1; k >= from; k-- {
		i := idx[k]
		if !signOff.MatchString(strings.TrimSpace(lines[i])) {
			continue
		}
		after := idx[k+1:]
		if len(after) == 0 || len(after) > 8 || k == 0 {
			return lines
		}
		for _, j := range after {
			if len([]rune(strings.TrimSpace(lines[j]))) > 70 {
				return lines
			}
		}
		return lines[:after[0]+1]
	}
	return lines
}

// collapse trims the text and keeps at most one blank line between paragraphs.
func collapse(s string) string {
	var out []string
	blank := false
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		l = strings.TrimRight(l, " \t")
		if strings.TrimSpace(l) == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		blank = false
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
