// Package core holds the application's domain logic and SQL.
package core

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"path/filepath"
)

var (
	ErrNotFound = errors.New("not found")
	// ErrConflict means the request can't be applied to the current state,
	// e.g. a stale note version or deleting a non-empty notebook.
	ErrConflict = errors.New("conflict")
)

// ValidationError carries a user-facing (Chinese) message.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(msg string) error { return &ValidationError{Msg: msg} }

// ConflictError is ErrConflict with a user-facing message.
type ConflictError struct{ Msg string }

func (e *ConflictError) Error() string { return e.Msg }
func (e *ConflictError) Unwrap() error { return ErrConflict }

func conflict(msg string) error { return &ConflictError{Msg: msg} }

// App is the entry point for all domain operations.
type App struct {
	DB      *sql.DB
	DataDir string
	// MaxUpload is the maximum upload size in bytes.
	MaxUpload int64
}

func New(db *sql.DB, dataDir string, maxUpload int64) *App {
	return &App{DB: db, DataDir: dataDir, MaxUpload: maxUpload}
}

func (a *App) filesDir() string { return filepath.Join(a.DataDir, "files") }

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// inTx runs fn in a transaction, committing on success.
func (a *App) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

const alnum = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return string(b)
}
