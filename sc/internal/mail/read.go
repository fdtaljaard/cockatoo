package mail

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// ReadBytes is how much of a message is fetched to show it: the headers and the text of almost
// any message. Attachments that come after the text are not downloaded; their names come from
// the server's description of the message (BODYSTRUCTURE).
const ReadBytes = 512 * 1024

// Body is what is fetched to show one message.
type Body struct {
	Raw         []byte           // the start of the message, at most ReadBytes
	Size        int64            // the whole message, in bytes
	Attachments []AttachmentInfo // files attached, by name
}

// AttachmentInfo names a file attached to a message.
type AttachmentInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"` // about, in bytes
}

// Bodies reads the start of the messages with these UIDs in the selected folder, and the names
// of their attachments. It peeks: nothing is marked as read. Nothing is kept.
func (s *Session) Bodies(uids []uint32) (map[uint32]Body, error) {
	out := map[uint32]Body{}
	section := &imap.FetchItemBodySection{Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: ReadBytes}}
	for start := 0; start < len(uids); start += 20 {
		part := uids[start:min(start+20, len(uids))]
		set := imap.UIDSet{}
		for _, u := range part {
			set.AddNum(imap.UID(u))
		}
		var fetched []*imapclient.FetchMessageBuffer
		err := s.timed(func() error {
			var err error
			fetched, err = s.c.Fetch(set, &imap.FetchOptions{
				UID: true, RFC822Size: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
				BodySection: []*imap.FetchItemBodySection{section},
			}).Collect()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("read messages on %s: %w", s.addr, err)
		}
		for _, m := range fetched {
			raw := m.FindBodySection(section)
			if raw == nil && len(m.BodySection) == 1 {
				raw = m.BodySection[0].Bytes
			}
			out[uint32(m.UID)] = Body{Raw: raw, Size: m.RFC822Size, Attachments: attachmentInfo(m.BodyStructure)}
		}
	}
	return out, nil
}

// Validity is the UIDVALIDITY of the folder the session has open: UIDs from another validity are
// not the same messages.
func (s *Session) Validity() uint32 { return s.selected.UIDValidity }

// attachmentInfo lists the files in a message by the same rule as ExtractAttachments: parts
// marked as attachments, and named parts that are not text, except pictures shown in the
// message itself (inline, with a Content-ID).
func attachmentInfo(bs imap.BodyStructure) []AttachmentInfo {
	out := []AttachmentInfo{}
	if bs == nil {
		return out
	}
	bs.Walk(func(_ []int, part imap.BodyStructure) bool {
		p, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		media := p.MediaType()
		name := decodeWords(p.Filename())
		attached := false
		if d := p.Disposition(); d != nil && strings.EqualFold(d.Value, "attachment") {
			attached = true
		}
		inlinePicture := !attached && p.ID != "" && strings.HasPrefix(media, "image/")
		switch {
		case inlinePicture:
		case attached, name != "" && !strings.HasPrefix(media, "text/"), media == "message/rfc822":
			if name == "" {
				name = unnamed(media)
			}
			size := int64(p.Size)
			if strings.EqualFold(p.Encoding, "base64") {
				size = size * 3 / 4
			}
			out = append(out, AttachmentInfo{Name: name, Size: size})
		}
		return media != "message/rfc822" // the parts of a forwarded message are its own
	})
	return out
}

// Attachments reads only the structure of these messages, not their text: the names and sizes of
// the files they carry. It is cheap enough to ask for many messages at once.
func (s *Session) Attachments(uids []uint32) (map[uint32][]AttachmentInfo, error) {
	out := map[uint32][]AttachmentInfo{}
	for start := 0; start < len(uids); start += 100 {
		part := uids[start:min(start+100, len(uids))]
		set := imap.UIDSet{}
		for _, u := range part {
			set.AddNum(imap.UID(u))
		}
		var fetched []*imapclient.FetchMessageBuffer
		err := s.timed(func() error {
			var err error
			fetched, err = s.c.Fetch(set, &imap.FetchOptions{
				UID: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
			}).Collect()
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("read attachment names on %s: %w", s.addr, err)
		}
		for _, m := range fetched {
			out[uint32(m.UID)] = attachmentInfo(m.BodyStructure)
		}
	}
	return out, nil
}
