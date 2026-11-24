package mail

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/emersion/go-imap/v2"
)

// Destination is where an action moves mail: the provider's Trash or Spam folder.
type Destination string

const (
	Trash Destination = "trash"
	Spam  Destination = "spam"
)

// Folder is one mailbox as the server lists it.
type Folder struct {
	Name  string
	Attrs []string // e.g. \Trash, \Junk (RFC 6154 special-use)
}

// The special-use attribute for each destination, and the names Gmail gives these folders when a
// server does not mark them (localised names included). Never a folder that is not there:
// cockatoo does not create folders.
var (
	destinationAttr = map[Destination]string{Trash: `\Trash`, Spam: `\Junk`}
	destinationName = map[Destination][]string{
		Trash: {"[Gmail]/Trash", "[Gmail]/Bin", "[Google Mail]/Trash", "[Google Mail]/Bin"},
		Spam:  {"[Gmail]/Spam", "[Google Mail]/Spam"},
	}
)

// chooseFolder picks the folder for a destination from a LIST result: the folder marked with the
// special-use attribute first, then a known name. When neither is there it refuses.
func chooseFolder(folders []Folder, d Destination) (string, error) {
	attr, ok := destinationAttr[d]
	if !ok {
		return "", fmt.Errorf("unknown destination %q", d)
	}
	for _, f := range folders {
		for _, a := range f.Attrs {
			if strings.EqualFold(a, attr) {
				return f.Name, nil
			}
		}
	}
	for _, name := range destinationName[d] {
		for _, f := range folders {
			if strings.EqualFold(f.Name, name) {
				return f.Name, nil
			}
		}
	}
	return "", fmt.Errorf("the mail server has no %s folder that cockatoo can recognise; nothing was moved", d)
}

// Mover is one signed-in connection with the inbox open for changes. It is used only to move
// messages a user confirmed; opening the inbox this way does not mark anything as read.
type Mover struct {
	s *Session
}

// OpenMover signs in and opens the inbox read-write. It refuses a server without the MOVE
// command: the fallback (copy, flag deleted, expunge) can remove other messages already flagged
// for deletion.
func OpenMover(ctx context.Context, srv Server, login Login) (*Mover, error) {
	c, done, err := signIn(ctx, srv, login)
	if err != nil {
		return nil, err
	}
	s := &Session{c: c, done: done, addr: srv.Addr, ctx: ctx, CommandTimeout: defaultCommandTimeout}
	if !c.Caps().Has(imap.CapMove) && !c.Caps().Has(imap.CapIMAP4rev2) {
		s.Close()
		return nil, errors.New("the mail server does not support moving messages safely (no MOVE command); nothing was moved")
	}
	err = s.timed(func() error {
		var err error
		s.selected, err = c.Select("INBOX", nil).Wait()
		return err
	})
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("open the inbox on %s: %w", srv.Addr, err)
	}
	return &Mover{s: s}, nil
}

// Close signs out.
func (m *Mover) Close() { m.s.Close() }

// UIDValidity is the inbox's UID validity. A different value from the one the copy was made
// with means the UIDs no longer name the same messages.
func (m *Mover) UIDValidity() uint32 { return m.s.selected.UIDValidity }

// Folder finds the folder for a destination.
func (m *Mover) Folder(d Destination) (string, error) {
	var folders []Folder
	err := m.s.timed(func() error {
		list, err := m.s.c.List("", "*", nil).Collect()
		if err != nil {
			return err
		}
		for _, l := range list {
			f := Folder{Name: l.Mailbox}
			for _, a := range l.Attrs {
				f.Attrs = append(f.Attrs, string(a))
			}
			folders = append(folders, f)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("list folders on %s: %w", m.s.addr, err)
	}
	return chooseFolder(folders, d)
}

// Move moves the messages with these UIDs from the inbox to folder, in one command.
func (m *Mover) Move(uids []uint32, folder string) error {
	if len(uids) == 0 {
		return nil
	}
	set := imap.UIDSet{}
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	err := m.s.timed(func() error {
		_, err := m.s.c.Move(set, folder).Wait()
		return err
	})
	if err != nil {
		return fmt.Errorf("move messages on %s: %w", m.s.addr, err)
	}
	return nil
}

// Present returns which of these UIDs are still in the inbox: after a move, the proof of what
// actually left.
func (m *Mover) Present(uids []uint32) (map[uint32]bool, error) {
	out := map[uint32]bool{}
	for start := 0; start < len(uids); start += 500 {
		part := uids[start:min(start+500, len(uids))]
		set := imap.UIDSet{}
		for _, u := range part {
			set.AddNum(imap.UID(u))
		}
		err := m.s.timed(func() error {
			found, err := m.s.c.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{set}}, nil).Wait()
			if err != nil {
				return err
			}
			for _, u := range found.AllUIDs() {
				if slices.Contains(part, uint32(u)) {
					out[uint32(u)] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("check the inbox on %s: %w", m.s.addr, err)
		}
	}
	return out, nil
}
