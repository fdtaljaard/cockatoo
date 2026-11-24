package account

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	imapc "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

func invoice(subject, body string) []byte {
	return []byte("From: billing@shop.example\r\nSubject: " + subject + "\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain\r\n\r\nSee attached.\r\n" +
		"--b\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"invoice.pdf\"\r\n\r\n" + body + "\r\n--b--\r\n")
}

func TestSaveAttachments(t *testing.T) {
	s, id := newService(t)
	ctx := context.Background()
	box := newFake(6)
	box.raw = map[uint32][]byte{
		1: invoice("October", "pdf-1"),
		2: invoice("November", "pdf-2"),
		3: invoice("December", "pdf-3"),
		4: []byte("Subject: hello\r\nContent-Type: text/plain\r\n\r\nno files here\r\n"),
	}
	box.big = map[uint32]int64{5: 80 << 20}
	s.dial = func(context.Context, imapc.Server, imapc.Login) (mailbox, error) { return box, nil }
	if err := s.SyncAccount(ctx, id, nil); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if _, err := s.AddSaveFolder(ctx, "  ", root); !errors.As(err, new(*InvalidError)) {
		t.Fatalf("no name: %v", err)
	}
	f, err := s.AddSaveFolder(ctx, "Expenses", `"`+root+`"`)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.MakeSaveSubfolder(ctx, f.ID, "", "2026-11")
	if err != nil || b.Path != "2026-11" || len(b.Folders) != 0 {
		t.Fatalf("subfolder %+v %v", b, err)
	}
	if top, _ := s.BrowseSaveFolder(ctx, f.ID, ""); strings.Join(top.Folders, ",") != "2026-11" {
		t.Fatalf("browse %+v", top)
	}

	key := func(uid uint32) store.MessageKey { return store.MessageKey{AccountID: id, UIDValidity: 3, UID: uid} }
	res, err := s.SaveAttachments(ctx, SaveRequest{FolderID: f.ID, Path: "2026-11", Messages: []store.MessageKey{
		key(1), key(2), key(3), key(4), key(5), key(99), {AccountID: id, UIDValidity: 2, UID: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 3 || res.Folder != filepath.Join(root, "2026-11") {
		t.Fatalf("result %+v", res)
	}
	entries, _ := os.ReadDir(res.Folder)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "invoice (2).pdf,invoice (3).pdf,invoice.pdf" {
		t.Fatalf("files on disk %v", names)
	}
	problems := map[uint32]string{}
	for _, m := range res.Messages {
		if m.Key.UIDValidity == 3 {
			problems[m.Key.UID] = m.Problem
		}
	}
	if !strings.Contains(problems[4], "no attachments") || !strings.Contains(problems[5], "80 MB") ||
		!strings.Contains(problems[99], "no longer in the inbox") || problems[1] != "" {
		t.Fatalf("problems %v", problems)
	}
	for _, m := range res.Messages {
		if m.Key.UIDValidity == 2 && !strings.Contains(m.Problem, "mailbox has changed") {
			t.Fatalf("old validity: %+v", m)
		}
	}

	// The audit: the action with its destination, and each file's full path and size.
	audit, _ := s.Audit(ctx)
	if len(audit) != 1 || audit[0].Kind != "save_attachments" || audit[0].Outcome != "partial" || audit[0].Done != 3 ||
		audit[0].Folder != res.Folder {
		t.Fatalf("audit %+v", audit)
	}

	// A path that leaves the save folder is refused, and nothing is written.
	if _, err := s.SaveAttachments(ctx, SaveRequest{FolderID: f.ID, Path: "../..", Messages: []store.MessageKey{key(1)}}); err == nil {
		t.Fatal("saved outside the save folder")
	}
}
