package main

// SQLite-backed persistence layer.
//
// The backend uses a pure-Go SQLite driver (modernc.org/sqlite) so binaries
// are statically linked and cross-compile cleanly on every platform we care
// about. This is the only third-party dependency in the Go module; a real
// database is required for anything resembling production operation —
// orders must survive a restart and payment webhooks rely on looking up the
// record they reference.
//
// Migrations are embedded at compile time and applied on startup in
// lexicographic order. Each migration is idempotent.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// defaultDBPath is used when DATABASE_PATH is unset. Relative to CWD so
// local dev puts the file next to the backend binary; production should
// mount a persistent volume and point DATABASE_PATH at it.
const defaultDBPath = "data/nast.db"

// openDB opens (or creates) the SQLite database at path, sets the pragmas
// we want, and applies pending migrations. Callers are responsible for
// closing the returned handle.
func openDB(path string) (*sql.DB, error) {
	if path == "" {
		path = defaultDBPath
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	// `_pragma=` is the modernc.org/sqlite DSN knob. We want WAL for
	// concurrent readers + a writer, foreign keys on, and a 5s busy
	// timeout so transient locks don't immediately error out.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite is single-writer; more than one open conn to a WAL DB is fine
	// for reads but buys us nothing for writes. Keep the pool modest.
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return db, nil
}

// applyMigrations runs every migration in migrations/ in lexicographic
// order, once. A tiny schema_migrations table tracks what ran.
func applyMigrations(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var existing string
		err := db.QueryRow(`SELECT name FROM schema_migrations WHERE name=?`, name).Scan(&existing)
		if err == nil {
			continue // already applied
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check %s: %w", name, err)
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(name, applied_at) VALUES(?,?)`,
			name, time.Now().UTC(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
		log.Printf("migration applied: %s", name)
	}
	return nil
}
