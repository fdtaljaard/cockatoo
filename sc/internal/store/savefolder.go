package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SaveFolder is a place on disk the user saves attachments to.
type SaveFolder struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

// ListSaveFolders returns the save folders, by name.
func (s *Store) ListSaveFolders(ctx context.Context) ([]SaveFolder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, path, created_at FROM save_folder ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("list save folders: %w", err)
	}
	defer rows.Close()
	out := []SaveFolder{}
	for rows.Next() {
		var f SaveFolder
		if err := rows.Scan(&f.ID, &f.Name, &f.Path, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("list save folders: %w", err)
		}
		f.CreatedAt = f.CreatedAt.UTC()
		out = append(out, f)
	}
	return out, rows.Err()
}

// GetSaveFolder returns one save folder; ErrNotFound when there is none.
func (s *Store) GetSaveFolder(ctx context.Context, id int64) (SaveFolder, error) {
	var f SaveFolder
	err := s.db.QueryRowContext(ctx, `SELECT id, name, path, created_at FROM save_folder WHERE id = ?`, id).
		Scan(&f.ID, &f.Name, &f.Path, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SaveFolder{}, ErrNotFound
	}
	if err != nil {
		return SaveFolder{}, fmt.Errorf("read save folder %d: %w", id, err)
	}
	f.CreatedAt = f.CreatedAt.UTC()
	return f, nil
}

// ErrDuplicateFolder means a save folder with that name or path is already there.
var ErrDuplicateFolder = errors.New("a save folder with this name or path is already added")

// AddSaveFolder saves a folder. The path must already be checked (an existing directory).
func (s *Store) AddSaveFolder(ctx context.Context, name, path string) (SaveFolder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM save_folder WHERE lower(name) = ? OR lower(path) = ?`,
		strings.ToLower(name), strings.ToLower(path)).Scan(&n); err != nil {
		return SaveFolder{}, fmt.Errorf("add save folder: %w", err)
	}
	if n > 0 {
		return SaveFolder{}, ErrDuplicateFolder
	}
	f := SaveFolder{Name: name, Path: path, CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.db.QueryRowContext(ctx, `INSERT INTO save_folder (name, path, created_at) VALUES (?, ?, ?) RETURNING id`,
		name, path, f.CreatedAt).Scan(&f.ID); err != nil {
		return SaveFolder{}, fmt.Errorf("add save folder: %w", err)
	}
	return f, nil
}

// DeleteSaveFolder forgets a save folder. Nothing on disk is touched.
func (s *Store) DeleteSaveFolder(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var gone int64
	err := s.db.QueryRowContext(ctx, `DELETE FROM save_folder WHERE id = ? RETURNING id`, id).Scan(&gone)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("remove save folder %d: %w", id, err)
	}
	return nil
}

// ActionFile is one file an action wrote.
type ActionFile struct {
	UIDValidity uint32 `json:"-"`
	UID         uint32 `json:"-"`
	Path        string `json:"path"` // the full path
	Bytes       int64  `json:"bytes"`
}

// RecordActionFiles adds the files an action wrote to its audit. Insert-only.
func (s *Store) RecordActionFiles(ctx context.Context, actionID int64, files []ActionFile) error {
	if len(files) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values := make([]string, len(files))
	args := make([]any, 0, len(files)*5)
	for i, f := range files {
		values[i] = "(?, ?, ?, ?, ?)"
		args = append(args, actionID, int64(f.UIDValidity), int64(f.UID), f.Path, f.Bytes)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO action_file (action_id, uid_validity, uid, path, bytes) VALUES `+
		strings.Join(values, ", "), args...); err != nil {
		return fmt.Errorf("record the files of action %d: %w", actionID, err)
	}
	return nil
}
