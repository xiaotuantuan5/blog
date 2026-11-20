// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open opens (creating if needed) the database at path and migrates it.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	q := url.Values{}
	q.Set("_txlock", "immediate")
	for _, p := range []string{
		"foreign_keys(1)",
		"journal_mode(WAL)",
		"synchronous(NORMAL)",
		"busy_timeout(5000)",
	} {
		q.Add("_pragma", p)
	}
	dsn := "file:" + path + "?" + q.Encode()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// LatestVersion is the highest embedded migration version.
func LatestVersion() int {
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return 0
	}
	latest := 0
	for _, e := range entries {
		if v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0]); err == nil && v > latest {
			latest = v
		}
	}
	return latest
}

// CheckBackup opens the database at path read-only, verifies it is a healthy
// SQLite file with our schema, and reports its migration version and
// instance id (empty when the database has none yet).
func CheckBackup(ctx context.Context, path string) (schemaVersion int, instanceID string, err error) {
	q := url.Values{}
	q.Set("mode", "ro")
	for _, p := range []string{"busy_timeout(5000)"} {
		q.Add("_pragma", p)
	}
	d, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return 0, "", err
	}
	defer d.Close()
	if err := d.PingContext(ctx); err != nil {
		return 0, "", err
	}
	var check string
	if err := d.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&check); err != nil {
		return 0, "", err
	}
	if check != "ok" {
		return 0, "", fmt.Errorf("quick_check: %s", check)
	}
	if err := d.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		return 0, "", err
	}
	if err := d.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'instance_id'`).Scan(&instanceID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	return schemaVersion, instanceID, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var current int
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	type mig struct {
		version int
		name    string
	}
	var list []mig
	for _, e := range entries {
		v, err := strconv.Atoi(strings.SplitN(e.Name(), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("bad migration name %q", e.Name())
		}
		list = append(list, mig{v, e.Name()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].version < list[j].version })

	for _, m := range list {
		if m.version <= current {
			continue
		}
		body, err := migrations.ReadFile("migrations/" + m.name)
		if err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.version, Now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
