package account

import (
	"context"
	"fmt"
	"os"
	"strings"

	"cockatoo/internal/files"
	imap "cockatoo/internal/mail"
	"cockatoo/internal/store"
)

// Save folders, and saving the attachments of chosen messages into one. Writing files is an
// action: audited before (which messages, where to) and after (each file's full path and size).

// AddSaveFolder adds a save folder: a name, and the path of a folder that already exists.
func (s *Service) AddSaveFolder(ctx context.Context, name, path string) (store.SaveFolder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return store.SaveFolder{}, &InvalidError{"Give the folder a name, for example Expenses."}
	}
	root, err := files.CheckRoot(path)
	if err != nil {
		return store.SaveFolder{}, err
	}
	return s.st.AddSaveFolder(ctx, name, root)
}

// Browse is a place inside a save folder: its path and the folders in it.
type Browse struct {
	FolderID int64    `json:"folderId"`
	Name     string   `json:"name"`    // the save folder's name
	Root     string   `json:"root"`    // the save folder's path
	Path     string   `json:"path"`    // relative, "/"-separated; "" is the save folder itself
	Full     string   `json:"full"`    // the full path on disk, to show
	Folders  []string `json:"folders"` // the folders directly inside
}

// BrowseSaveFolder lists the folders inside a place in a save folder.
func (s *Service) BrowseSaveFolder(ctx context.Context, id int64, rel string) (Browse, error) {
	f, err := s.st.GetSaveFolder(ctx, id)
	if err != nil {
		return Browse{}, err
	}
	full, err := files.Resolve(f.Path, rel)
	if err != nil {
		return Browse{}, err
	}
	sub, err := files.Subfolders(f.Path, rel)
	if err != nil {
		return Browse{}, err
	}
	return Browse{FolderID: id, Name: f.Name, Root: f.Path, Path: strings.Trim(rel, "/"), Full: full, Folders: sub}, nil
}

// MakeSaveSubfolder makes a folder inside a place in a save folder, and browses into it.
func (s *Service) MakeSaveSubfolder(ctx context.Context, id int64, rel, name string) (Browse, error) {
	f, err := s.st.GetSaveFolder(ctx, id)
	if err != nil {
		return Browse{}, err
	}
	newRel, err := files.MakeSubfolder(f.Path, rel, name)
	if err != nil {
		return Browse{}, err
	}
	return s.BrowseSaveFolder(ctx, id, newRel)
}

// SaveRequest is what the user chose: which messages, which save folder, where in it.
type SaveRequest struct {
	FolderID int64              `json:"folderId"`
	Path     string             `json:"path"`
	Messages []store.MessageKey `json:"messages"`
}

// SavedMessage is what happened to one message.
type SavedMessage struct {
	Key     store.MessageKey   `json:"key"`
	Subject string             `json:"subject"`
	Files   []store.ActionFile `json:"files"`
	Skipped []string           `json:"skipped,omitempty"` // things in it that were not saved, and why
	Problem string             `json:"problem,omitempty"` // why nothing could be saved from it
}

// SaveResult is what a save did.
type SaveResult struct {
	Folder   string         `json:"folder"` // the full path saved to
	Files    int            `json:"files"`
	Messages []SavedMessage `json:"messages"`
}

// SaveAttachments saves the attachments of the chosen messages into a place in a save folder.
// Messages are read with a peek (nothing is marked read), checked to still be the messages the
// user chose, and their files written under safe names, never over an existing file. It runs to
// its end even if the window closes, and the app waits for it before stopping.
func (s *Service) SaveAttachments(ctx context.Context, req SaveRequest) (SaveResult, error) {
	if len(req.Messages) == 0 {
		return SaveResult{}, &InvalidError{"Choose at least one message."}
	}
	f, err := s.st.GetSaveFolder(ctx, req.FolderID)
	if err != nil {
		return SaveResult{}, err
	}
	dir, err := files.Resolve(f.Path, req.Path)
	if err != nil {
		return SaveResult{}, err
	}
	s.sync.mu.Lock()
	if s.stopped {
		s.sync.mu.Unlock()
		return SaveResult{}, ErrBusy
	}
	s.actions.Add(1)
	s.sync.mu.Unlock()
	defer s.actions.Done()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), actionTimeout)
	defer cancel()

	byAccount := map[int64][]store.MessageKey{}
	for _, k := range req.Messages {
		byAccount[k.AccountID] = append(byAccount[k.AccountID], k)
	}
	out := SaveResult{Folder: dir, Messages: []SavedMessage{}}
	for accountID, keys := range byAccount {
		saved, err := s.saveFromAccount(ctx, accountID, keys, dir)
		if err != nil {
			for _, k := range keys {
				out.Messages = append(out.Messages, SavedMessage{Key: k, Files: []store.ActionFile{}, Problem: err.Error()})
			}
			continue
		}
		out.Messages = append(out.Messages, saved...)
	}
	for _, m := range out.Messages {
		out.Files += len(m.Files)
	}
	return out, nil
}

func (s *Service) saveFromAccount(ctx context.Context, accountID int64, keys []store.MessageKey, dir string) ([]SavedMessage, error) {
	a, sealed, err := s.st.AccountSecret(ctx, accountID)
	if err != nil {
		return nil, err
	}
	known, err := s.st.MessagesByKey(ctx, keys)
	if err != nil {
		return nil, err
	}
	items := make([]store.Message, 0, len(keys))
	results := make([]SavedMessage, len(keys))
	for i, k := range keys {
		results[i] = SavedMessage{Key: k, Files: []store.ActionFile{}}
		if m, ok := known[k]; ok {
			items = append(items, m)
			results[i].Subject = m.Subject
		}
	}

	actionID, err := s.st.StartAction(ctx, store.ActionStart{
		AccountID: accountID, AccountEmail: a.Email, Kind: store.KindSave, Criteria: store.Criteria{Kind: "messages"},
		Description: fmt.Sprintf("Save the attachments of %d messages to %s (%s)", len(keys), dir, a.Email),
		Source:      "you", Folder: dir,
	}, items)
	if err != nil {
		return nil, err
	}
	finish := func(outcome, errText string, written []store.ActionFile, notDone []uint32) {
		_ = s.st.RecordActionFiles(ctx, actionID, written)
		_ = s.st.FinishAction(ctx, actionID, store.ActionResult{Outcome: outcome, Done: len(written), NotDone: notDone, Error: errText})
	}

	login, err := s.loginFor(ctx, a, sealed)
	if err != nil {
		finish("failed", err.Error(), nil, nil)
		return nil, err
	}
	box, err := s.dial(ctx, s.server, login)
	if err != nil {
		err = explainMail(err)
		finish("failed", err.Error(), nil, nil)
		return nil, err
	}
	defer box.Close()
	validity, err := box.Inbox()
	if err != nil {
		finish("failed", err.Error(), nil, nil)
		return nil, err
	}

	var uids []uint32
	for i, k := range keys {
		if k.UIDValidity != validity.UIDValidity {
			results[i].Problem = "The mailbox has changed since cockatoo last read it; let it check again and choose the message again."
			continue
		}
		uids = append(uids, k.UID)
	}
	raw, tooBig, err := box.Raw(uids)
	if err != nil {
		finish("failed", err.Error(), nil, nil)
		return nil, err
	}

	var written []store.ActionFile
	var notDone []uint32
	var problems []string
	for i := range results {
		r := &results[i]
		k := r.Key
		if r.Problem != "" {
			notDone = append(notDone, k.UID)
			continue
		}
		if size, big := tooBig[k.UID]; big {
			r.Problem = fmt.Sprintf("The message is %d MB, more than cockatoo reads (%d MB).", size>>20, imap.MaxMessageBytes>>20)
		} else if body, ok := raw[k.UID]; !ok {
			r.Problem = "The message is no longer in the inbox."
		} else {
			atts, skipped := imap.ExtractAttachments(body)
			r.Skipped = skipped
			if len(atts) == 0 {
				r.Problem = "The message has no attachments."
			}
			for n, att := range atts {
				name := files.SafeName(att.Name)
				if name == "" {
					name = fmt.Sprintf("attachment %d", n+1)
				}
				path, err := files.Write(dir, name, att.Data)
				if err != nil {
					r.Skipped = append(r.Skipped, fmt.Sprintf("%s could not be written: %v", name, err))
					problems = append(problems, err.Error())
					continue
				}
				file := store.ActionFile{UIDValidity: k.UIDValidity, UID: k.UID, Path: path, Bytes: int64(len(att.Data))}
				r.Files = append(r.Files, file)
				written = append(written, file)
			}
		}
		if len(r.Files) == 0 {
			notDone = append(notDone, k.UID)
		}
	}

	outcome := "done"
	switch {
	case len(written) == 0:
		outcome = "failed"
	case len(notDone) > 0 || len(problems) > 0:
		outcome = "partial"
	}
	finish(outcome, strings.Join(problems, "; "), written, notDone)
	return results, nil
}

// RemoveSaveFolder forgets a save folder. The folder and its files on disk are not touched.
func (s *Service) RemoveSaveFolder(ctx context.Context, id int64) error {
	return s.st.DeleteSaveFolder(ctx, id)
}

// ListSaveFolders returns the save folders, each marked when its path is no longer there.
func (s *Service) ListSaveFolders(ctx context.Context) ([]SaveFolderStatus, error) {
	list, err := s.st.ListSaveFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SaveFolderStatus, len(list))
	for i, f := range list {
		out[i] = SaveFolderStatus{SaveFolder: f}
		if info, err := os.Stat(f.Path); err != nil || !info.IsDir() {
			out[i].Missing = true
		}
	}
	return out, nil
}

// SaveFolderStatus is a save folder, and whether its path is still there.
type SaveFolderStatus struct {
	store.SaveFolder
	Missing bool `json:"missing"`
}
