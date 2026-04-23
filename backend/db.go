package main

// Persistence layer. Two drivers are supported:
//
//   - SQLite (modernc.org/sqlite) for local development and small
//     self-hosted deploys that can mount a persistent volume.
//   - Postgres (github.com/jackc/pgx/v5/stdlib) for serverless / managed
//     deploys such as Vercel Postgres + Neon.
//
// The driver is chosen by inspecting DATABASE_URL (or legacy
// DATABASE_PATH). Migrations for each dialect live under
// migrations/<dialect>/*.sql and are applied idempotently on boot.

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

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

// defaultDBPath is used for SQLite when neither DATABASE_URL nor
// DATABASE_PATH is set. Relative to CWD so local dev puts the file next
// to the backend binary; production should mount a persistent volume
// and point DATABASE_PATH at it.
const defaultDBPath = "data/nast.db"

// pickDialect inspects DATABASE_URL to decide which driver to use. An
// empty or "sqlite://…" value → SQLite. A "postgres://" or
// "postgresql://" value → Postgres. The raw URL is returned so the
// caller can pass it directly to sql.Open (for Postgres) or parse the
// file path (for SQLite).
func pickDialect(raw string) (dialect, string) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		return dialectPostgres, raw
	case strings.HasPrefix(raw, "sqlite://"):
		return dialectSQLite, strings.TrimPrefix(strings.TrimPrefix(raw, "sqlite://"), "/")
	case raw == "":
		return dialectSQLite, ""
	default:
		// Unknown schemes fall back to SQLite treating the value as a
		// plain file path so we don't surprise self-hosted users.
		return dialectSQLite, raw
	}
}

// openDB opens the database, applies pending migrations and returns the
// handle. The caller is responsible for closing it. DATABASE_URL takes
// priority; DATABASE_PATH remains supported for backwards compat with
// self-hosted deploys that already set it.
func openDB() (*sql.DB, error) {
	dbURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	legacyPath := strings.TrimSpace(os.Getenv("DATABASE_PATH"))
	if dbURL == "" && legacyPath != "" {
		dbURL = legacyPath
	}
	d, rest := pickDialect(dbURL)
	currentDialect = d
	switch d {
	case dialectPostgres:
		return openPostgres(rest)
	default:
		return openSQLite(rest)
	}
}

func openSQLite(path string) (*sql.DB, error) {
	if path == "" {
		path = defaultDBPath
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxIdleTime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := applyMigrations(db, dialectSQLite); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	log.Printf("database: sqlite (path=%s)", path)
	return db, nil
}

func openPostgres(url string) (*sql.DB, error) {
	// Managed Postgres providers (Neon, Vercel Postgres, Supabase) all
	// enforce TLS by default. We trust the URL to set sslmode; if it's
	// missing we append require to stay safe.
	if !strings.Contains(url, "sslmode=") {
		if strings.Contains(url, "?") {
			url += "&sslmode=require"
		} else {
			url += "?sslmode=require"
		}
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	// Keep the pool small — many managed providers cap connections at
	// 20 or fewer and serverless instances multiply the pressure.
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(2 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := applyMigrations(db, dialectPostgres); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	log.Printf("database: postgres")
	return db, nil
}

// applyMigrations runs every migration in migrations/<dialect>/ in
// lexicographic order, once. A tiny schema_migrations table tracks what
// ran.
func applyMigrations(db *sql.DB, d dialect) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY,
		applied_at TIMESTAMP NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	dir := "migrations/" + string(d)
	entries, err := fs.ReadDir(migrationsFS, dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", dir, err)
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
		err := db.QueryRow(rb(`SELECT name FROM schema_migrations WHERE name=?`), name).Scan(&existing)
		if err == nil {
			continue // already applied
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check %s: %w", name, err)
		}
		sqlBytes, err := migrationsFS.ReadFile(dir + "/" + name)
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
		if _, err := tx.Exec(rb(
			`INSERT INTO schema_migrations(name, applied_at) VALUES(?,?)`),
			name, time.Now().UTC(),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}
		log.Printf("migration applied: %s/%s", d, name)
	}
	return nil
}
