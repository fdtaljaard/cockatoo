package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenMigratesAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.duckdb")

	for range 2 { // the second open finds the schema in place and applies nothing
		s, err := Open(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		st, err := s.Status(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st.SchemaVersion != len(migrations) {
			t.Errorf("schema version %d, want %d", st.SchemaVersion, len(migrations))
		}
		if st.DuckDBVersion == "" {
			t.Error("no DuckDB version")
		}
		if len(st.Tables) != 4 { // account, ai_provider, schema_version, setting
			t.Errorf("tables %v, want account, ai_provider, schema_version and setting", st.Tables)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
