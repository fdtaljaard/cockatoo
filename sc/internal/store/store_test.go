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
		if len(st.Tables) != 16 { // the fourteen of migration 9, plus action_file and save_folder
			t.Errorf("tables %v, want the sixteen tables of migration 10", st.Tables)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
