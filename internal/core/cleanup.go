package core

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/xiaotuantuan5/blog/internal/db"
)

// trashRetention is how long trashed notes stay recoverable.
const trashRetention = 30 * 24 * time.Hour

// RunTrashCleanup purges trashed notes older than trashRetention, once at
// startup and then every few hours, until ctx is cancelled.
func (a *App) RunTrashCleanup(ctx context.Context) {
	if err := a.PurgeExpiredTrash(ctx); err != nil {
		slog.Error("回收站清理失败", "err", err)
	}
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.PurgeExpiredTrash(ctx); err != nil {
				slog.Error("回收站清理失败", "err", err)
			}
		}
	}
}

// PurgeExpiredTrash deletes trashed notes past the retention window.
func (a *App) PurgeExpiredTrash(ctx context.Context) error {
	cutoff := db.Now().Add(-trashRetention)
	var purged int64
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM notes WHERE deleted_at IS NOT NULL AND deleted_at < ?`, cutoff)
		if err != nil {
			return err
		}
		purged, _ = res.RowsAffected()
		_, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM note_tags)`)
		return err
	})
	if err == nil && purged > 0 {
		slog.Info("回收站自动清理", "purged", purged)
	}
	return err
}
