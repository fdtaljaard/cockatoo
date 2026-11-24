// Package store owns the DuckDB file: schema, the single writer, and queries.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/duckdb/duckdb-go/v2"
)

// Store is the one handle on the DuckDB file. DuckDB wants a single writer,
// so every write goes through mu. Reads are concurrent.
type Store struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

// Open opens or creates the DuckDB file at path and brings the schema up to date.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %s: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open duckdb %s: %w", path, err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the file.
func (s *Store) Close() error { return s.db.Close() }

// migrations run in order; each one is applied once. Keep them additive.
var migrations = []string{
	// 1: settings of this data file, as key and value
	`CREATE TABLE IF NOT EXISTS setting (
	   key   VARCHAR PRIMARY KEY,
	   value VARCHAR NOT NULL
	 );`,
	// 2: mail accounts. secret is the account's password, encrypted by internal/secret before it
	// reaches the store; the plain password is never written to this file.
	`CREATE SEQUENCE IF NOT EXISTS seq_account START 1;
	 CREATE TABLE IF NOT EXISTS account (
	   id         INTEGER PRIMARY KEY DEFAULT nextval('seq_account'),
	   provider   VARCHAR NOT NULL,
	   name       VARCHAR NOT NULL,
	   email      VARCHAR NOT NULL UNIQUE,
	   secret     BLOB NOT NULL,
	   created_at TIMESTAMP NOT NULL
	 );`,
	// 3: AI providers, one row each (Anthropic first). secret is the API key, encrypted by
	// internal/secret; key_hint is the start and end of the key, to show which one is saved.
	`CREATE TABLE IF NOT EXISTS ai_provider (
	   provider   VARCHAR PRIMARY KEY,
	   enabled    BOOLEAN NOT NULL,
	   model      VARCHAR NOT NULL,
	   secret     BLOB,
	   key_hint   VARCHAR,
	   updated_at TIMESTAMP NOT NULL
	 );`,
}

func (s *Store) migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL, applied_at TIMESTAMP NOT NULL DEFAULT current_timestamp)`); err != nil {
		return fmt.Errorf("schema_version: %w", err)
	}
	var current int
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM schema_version`).Scan(&current); err != nil {
		return fmt.Errorf("schema_version: %w", err)
	}
	for i := current; i < len(migrations); i++ {
		if _, err := s.db.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_version (version) VALUES (?)`, i+1); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}

// Status is what the data file says about itself: proof the app is talking to DuckDB.
type Status struct {
	Path          string   `json:"path"`
	DuckDBVersion string   `json:"duckdbVersion"`
	SchemaVersion int      `json:"schemaVersion"`
	Tables        []string `json:"tables"`
}

// Status reads the DuckDB version, the schema version and the table names.
func (s *Store) Status(ctx context.Context) (Status, error) {
	st := Status{Path: s.path, Tables: []string{}}
	if err := s.db.QueryRowContext(ctx, `SELECT version()`).Scan(&st.DuckDBVersion); err != nil {
		return st, fmt.Errorf("duckdb version: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM schema_version`).Scan(&st.SchemaVersion); err != nil {
		return st, fmt.Errorf("schema version: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = 'main' ORDER BY table_name`)
	if err != nil {
		return st, fmt.Errorf("tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return st, fmt.Errorf("tables: %w", err)
		}
		st.Tables = append(st.Tables, name)
	}
	return st, rows.Err()
}
