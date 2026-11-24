package store

import (
	"context"
	"testing"
)

func TestSaveFolders(t *testing.T) {
	ctx := context.Background()
	s, me, _ := openTest(t)
	f, err := s.AddSaveFolder(ctx, "Expenses", `C:\Docs\Expenses`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddSaveFolder(ctx, "expenses", `C:\Other`); err != ErrDuplicateFolder {
		t.Errorf("same name: %v", err)
	}
	if _, err := s.AddSaveFolder(ctx, "Other", `c:\docs\expenses`); err != ErrDuplicateFolder {
		t.Errorf("same path: %v", err)
	}
	if list, _ := s.ListSaveFolders(ctx); len(list) != 1 || list[0].Path != `C:\Docs\Expenses` {
		t.Fatalf("list %+v", list)
	}

	id, err := s.StartAction(ctx, ActionStart{AccountID: me, AccountEmail: "me@example.com", Kind: "save_attachments",
		Criteria: Criteria{Kind: "messages"}, Description: "Save", Source: "you", Folder: `C:\Docs\Expenses\2026-11`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordActionFiles(ctx, id, []ActionFile{{UIDValidity: 7, UID: 1, Path: `C:\Docs\Expenses\2026-11\invoice.pdf`, Bytes: 1234}}); err != nil {
		t.Fatal(err)
	}
	// The audit outlives the save folder.
	if err := s.DeleteSaveFolder(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM action_file WHERE action_id = ?`, id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("%d audited files %v", n, err)
	}
	if err := s.DeleteSaveFolder(ctx, f.ID); err != ErrNotFound {
		t.Errorf("delete twice: %v", err)
	}
}
