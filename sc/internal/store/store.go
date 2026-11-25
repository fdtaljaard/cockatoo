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
	// 4: inbox message headers, copied from the mail server. Unlike account and ai_provider this
	// table is a cache: the server is the source, so a later migration may drop and refill it.
	// Times are UTC. refreshed_at marks the rows the last refresh of an account saw.
	`CREATE TABLE IF NOT EXISTS message (
	   account_id   INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   message_id   VARCHAR,
	   from_name    VARCHAR,
	   from_addr    VARCHAR,
	   subject      VARCHAR,
	   received_at  TIMESTAMP NOT NULL,
	   seen         BOOLEAN NOT NULL,
	   refreshed_at TIMESTAMP NOT NULL,
	   PRIMARY KEY (account_id, uid_validity, uid)
	 );`,
	// 5: the message table now holds the whole inbox, not 24 hours of it (refreshed_at is when a
	// row was last written). inbox_sync says how far each account's copy goes: complete is true
	// only once every message the server listed has been read. Also a cache.
	`CREATE TABLE IF NOT EXISTS inbox_sync (
	   account_id   INTEGER PRIMARY KEY,
	   uid_validity BIGINT NOT NULL,
	   server_total INTEGER NOT NULL,
	   complete     BOOLEAN NOT NULL,
	   started_at   TIMESTAMP NOT NULL,
	   synced_at    TIMESTAMP
	 );`,
	// 6: the cache gains each message's unsubscribe address (List-Unsubscribe). Every message has
	// to be read again for it, so the two cache tables are dropped and made anew; the next sync
	// refills them. And the audit: every action cockatoo takes on a mailbox, insert-only, kept
	// when an account is removed. action is written before anything changes (what, why, which
	// messages), action_result after (what happened).
	`DROP TABLE IF EXISTS message;
	 DROP TABLE IF EXISTS inbox_sync;
	 CREATE TABLE message (
	   account_id      INTEGER NOT NULL,
	   uid_validity    BIGINT NOT NULL,
	   uid             BIGINT NOT NULL,
	   message_id      VARCHAR,
	   from_name       VARCHAR,
	   from_addr       VARCHAR,
	   subject         VARCHAR,
	   received_at     TIMESTAMP NOT NULL,
	   seen            BOOLEAN NOT NULL,
	   unsub_url       VARCHAR,
	   unsub_mailto    VARCHAR,
	   unsub_one_click BOOLEAN NOT NULL,
	   refreshed_at    TIMESTAMP NOT NULL,
	   PRIMARY KEY (account_id, uid_validity, uid)
	 );
	 CREATE TABLE inbox_sync (
	   account_id   INTEGER PRIMARY KEY,
	   uid_validity BIGINT NOT NULL,
	   server_total INTEGER NOT NULL,
	   complete     BOOLEAN NOT NULL,
	   started_at   TIMESTAMP NOT NULL,
	   synced_at    TIMESTAMP
	 );
	 CREATE SEQUENCE IF NOT EXISTS seq_action START 1;
	 CREATE TABLE IF NOT EXISTS action (
	   id            INTEGER PRIMARY KEY DEFAULT nextval('seq_action'),
	   started_at    TIMESTAMP NOT NULL,
	   account_id    INTEGER NOT NULL,
	   account_email VARCHAR NOT NULL,
	   kind          VARCHAR NOT NULL,
	   criteria      VARCHAR NOT NULL,
	   description   VARCHAR NOT NULL,
	   source        VARCHAR NOT NULL,
	   folder        VARCHAR,
	   target        VARCHAR,
	   matched       INTEGER NOT NULL
	 );
	 CREATE TABLE IF NOT EXISTS action_item (
	   action_id    INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   message_id   VARCHAR,
	   from_name    VARCHAR,
	   from_addr    VARCHAR,
	   subject      VARCHAR,
	   received_at  TIMESTAMP NOT NULL
	 );
	 CREATE TABLE IF NOT EXISTS action_result (
	   action_id   INTEGER NOT NULL,
	   finished_at TIMESTAMP NOT NULL,
	   outcome     VARCHAR NOT NULL,
	   done        INTEGER NOT NULL,
	   not_done    VARCHAR NOT NULL,
	   error       VARCHAR
	 );`,
	// 7: Google's own sign-in. account.auth says how an account signs in: 'password' (a Google
	// app password, every account before this) or 'google' (secret is then the refresh token,
	// encrypted the same way). oauth_client holds the Google Cloud OAuth client cockatoo signs in
	// with, its secret encrypted.
	`ALTER TABLE account ADD COLUMN IF NOT EXISTS auth VARCHAR DEFAULT 'password';
	 CREATE TABLE IF NOT EXISTS oauth_client (
	   provider   VARCHAR PRIMARY KEY,
	   client_id  VARCHAR NOT NULL,
	   secret     BLOB NOT NULL,
	   updated_at TIMESTAMP NOT NULL
	 );`,
	// 8: every request cockatoo sends to an AI provider, insert-only: what kind, for which
	// accounts, which model answered, the tokens it took, the answer (or why there was none), and
	// each message that was sent.
	`CREATE SEQUENCE IF NOT EXISTS seq_ai_request START 1;
	 CREATE TABLE IF NOT EXISTS ai_request (
	   id            INTEGER PRIMARY KEY DEFAULT nextval('seq_ai_request'),
	   created_at    TIMESTAMP NOT NULL,
	   kind          VARCHAR NOT NULL,
	   scope         VARCHAR NOT NULL,
	   model         VARCHAR,
	   input_tokens  BIGINT NOT NULL,
	   output_tokens BIGINT NOT NULL,
	   sent          INTEGER NOT NULL,
	   left_out      INTEGER NOT NULL,
	   partial       BOOLEAN NOT NULL,
	   answer        VARCHAR,
	   error         VARCHAR
	 );
	 CREATE TABLE IF NOT EXISTS ai_request_item (
	   request_id   INTEGER NOT NULL,
	   ref          VARCHAR NOT NULL,
	   account_id   INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   from_addr    VARCHAR,
	   subject      VARCHAR
	 );`,
	// 9: what tells mail that needs a reply from mail to skim. The cache gains each message's
	// recipients, the message it answers, and its list / automated markers; every message has to be
	// read again for them, so the two cache tables are made anew as in migration 6. sent_message is
	// a cache of the Sent folder's headers (who the user writes to, and what they answered), with
	// its own sync state. The AI request record gains whether message text was sent, and how much
	// mail was left out as automated or already answered.
	`DROP TABLE IF EXISTS message;
	 DROP TABLE IF EXISTS inbox_sync;
	 CREATE TABLE message (
	   account_id      INTEGER NOT NULL,
	   uid_validity    BIGINT NOT NULL,
	   uid             BIGINT NOT NULL,
	   message_id      VARCHAR,
	   from_name       VARCHAR,
	   from_addr       VARCHAR,
	   to_addrs        VARCHAR,
	   cc_addrs        VARCHAR,
	   in_reply_to     VARCHAR,
	   subject         VARCHAR,
	   received_at     TIMESTAMP NOT NULL,
	   seen            BOOLEAN NOT NULL,
	   unsub_url       VARCHAR,
	   unsub_mailto    VARCHAR,
	   unsub_one_click BOOLEAN NOT NULL,
	   list_id         VARCHAR,
	   precedence      VARCHAR,
	   auto_submitted  VARCHAR,
	   refreshed_at    TIMESTAMP NOT NULL,
	   PRIMARY KEY (account_id, uid_validity, uid)
	 );
	 CREATE TABLE inbox_sync (
	   account_id   INTEGER PRIMARY KEY,
	   uid_validity BIGINT NOT NULL,
	   server_total INTEGER NOT NULL,
	   complete     BOOLEAN NOT NULL,
	   started_at   TIMESTAMP NOT NULL,
	   synced_at    TIMESTAMP
	 );
	 CREATE TABLE IF NOT EXISTS sent_message (
	   account_id   INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   message_id   VARCHAR,
	   from_addr    VARCHAR,
	   to_addrs     VARCHAR,
	   cc_addrs     VARCHAR,
	   in_reply_to  VARCHAR,
	   sent_at      TIMESTAMP NOT NULL,
	   PRIMARY KEY (account_id, uid_validity, uid)
	 );
	 CREATE TABLE IF NOT EXISTS sent_sync (
	   account_id   INTEGER PRIMARY KEY,
	   folder       VARCHAR NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   complete     BOOLEAN NOT NULL,
	   synced_at    TIMESTAMP
	 );
	 ALTER TABLE ai_request ADD COLUMN IF NOT EXISTS text_sent BOOLEAN DEFAULT false;
	 ALTER TABLE ai_request ADD COLUMN IF NOT EXISTS text_chars INTEGER DEFAULT 0;
	 ALTER TABLE ai_request ADD COLUMN IF NOT EXISTS left_automated INTEGER DEFAULT 0;
	 ALTER TABLE ai_request ADD COLUMN IF NOT EXISTS left_answered INTEGER DEFAULT 0;`,
	// 10: save folders, the places on disk the user saves attachments to (a name and a path), and
	// the audit of every file an action wrote: its full path (so the record outlives the save
	// folder) and size. Not a cache: nothing here is dropped.
	`CREATE SEQUENCE IF NOT EXISTS seq_save_folder START 1;
	 CREATE TABLE IF NOT EXISTS save_folder (
	   id         INTEGER PRIMARY KEY DEFAULT nextval('seq_save_folder'),
	   name       VARCHAR NOT NULL,
	   path       VARCHAR NOT NULL,
	   created_at TIMESTAMP NOT NULL
	 );
	 CREATE TABLE IF NOT EXISTS action_file (
	   action_id    INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   path         VARCHAR NOT NULL,
	   bytes        BIGINT NOT NULL
	 );`,

	// 11: routines, what the user does every time a certain mail comes in: mail from an address
	// or a domain (and, if given, with text in its subject) and what to do with it. For now the
	// action is saving its attachments into a save folder, in a subfolder for the mail's month or
	// day when dated is set. No foreign key: a removed save folder leaves the routine, shown as
	// broken.
	`CREATE SEQUENCE IF NOT EXISTS seq_routine START 1;
	 CREATE TABLE IF NOT EXISTS routine (
	   id            INTEGER PRIMARY KEY DEFAULT nextval('seq_routine'),
	   match_from    VARCHAR NOT NULL,
	   match_subject VARCHAR,
	   action        VARCHAR NOT NULL,
	   folder_id     INTEGER,
	   path          VARCHAR,
	   dated         VARCHAR,
	   created_at    TIMESTAMP NOT NULL
	 );`,

	// 12: the lists mail is sorted into (Reply, Attention, Save, Unsorted). sort_mark is where the
	// user put a message, insert-only, the latest mark winning; suggestion is the AI's idea of
	// where it belongs, kept so the same message is never asked about twice. Rules live in the
	// routine table, its action naming the list.
	`CREATE SEQUENCE IF NOT EXISTS seq_sort_mark START 1;
	 CREATE TABLE IF NOT EXISTS sort_mark (
	   id           INTEGER PRIMARY KEY DEFAULT nextval('seq_sort_mark'),
	   account_id   INTEGER NOT NULL,
	   uid_validity BIGINT NOT NULL,
	   uid          BIGINT NOT NULL,
	   list         VARCHAR NOT NULL,
	   placed_by    VARCHAR NOT NULL,
	   created_at   TIMESTAMP NOT NULL
	 );
	 CREATE SEQUENCE IF NOT EXISTS seq_suggestion START 1;
	 CREATE TABLE IF NOT EXISTS suggestion (
	   id            INTEGER PRIMARY KEY DEFAULT nextval('seq_suggestion'),
	   account_id    INTEGER NOT NULL,
	   uid_validity  BIGINT NOT NULL,
	   uid           BIGINT NOT NULL,
	   list          VARCHAR NOT NULL,
	   reason        VARCHAR NOT NULL,
	   ai_request_id BIGINT,
	   created_at    TIMESTAMP NOT NULL
	 );`,

	// 13: the email agent's skills (SCOPE.md, section 7a): what it knows about how the user works, in
	// plain words. Insert-only: every change is a new version; the latest version of a skill_id is the
	// skill, and a version marked removed removes it. Two starter skills to adjust.
	`CREATE SEQUENCE IF NOT EXISTS seq_skill_version START 1;
	 CREATE SEQUENCE IF NOT EXISTS seq_skill START 3;
	 CREATE TABLE IF NOT EXISTS skill_version (
	   id           INTEGER PRIMARY KEY DEFAULT nextval('seq_skill_version'),
	   skill_id     INTEGER NOT NULL,
	   name         VARCHAR NOT NULL,
	   applies      VARCHAR NOT NULL,
	   instructions VARCHAR NOT NULL,
	   made_by      VARCHAR NOT NULL,
	   note         VARCHAR,
	   removed      BOOLEAN NOT NULL,
	   created_at   TIMESTAMP NOT NULL
	 );
	 ALTER TABLE suggestion ADD COLUMN skill VARCHAR;
	 INSERT INTO skill_version (skill_id, name, applies, instructions, made_by, note, removed, created_at) VALUES
	 (1, 'What I keep', 'Mail with attachments: invoices, statements, receipts and other documents.',
	  'Invoices, statements, payslips, contracts, policies and tax documents I need for my records go to Save. Order confirmations, shipping notices, tickets and marketing PDFs need nothing, even with an attachment.',
	  'cockatoo', 'a starter skill: change it to how you work', false, current_timestamp),
	 (2, 'When I reply', 'Mail from people, not machines.',
	  'If someone asks me a question, asks me to decide, approve or send something, or is waiting on me, it goes to Reply. If I am only on CC and nobody asks me anything by name, it goes to Attention at most.',
	  'cockatoo', 'a starter skill: change it to how you work', false, current_timestamp);`,
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
