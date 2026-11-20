package core

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/xiaotuantuan5/blog/internal/db"
	"github.com/xiaotuantuan5/blog/internal/markdown"
)

// Daily notes live in a top-level "日记" notebook, one per day, titled by
// the date.
const dailyNotebookName = "日记"

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// GetOrCreateDaily returns the note for the given date (YYYY-MM-DD),
// creating it (and the 日记 notebook) on first use.
func (a *App) GetOrCreateDaily(ctx context.Context, date string) (*Note, error) {
	if !dateRe.MatchString(date) {
		return nil, invalid("日期格式应为 YYYY-MM-DD")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return nil, invalid("无效的日期")
	}
	var id int64
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		var notebookID int64
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM notebooks WHERE name = ? AND parent_id IS NULL`, dailyNotebookName).
			Scan(&notebookID)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO notebooks (parent_id, name, sort, created_at) VALUES (NULL, ?, 0, ?)`,
				dailyNotebookName, db.Now())
			if err != nil {
				return err
			}
			if notebookID, err = res.LastInsertId(); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx,
			`SELECT id FROM notes WHERE is_daily = 1 AND title = ? AND notebook_id = ?`, date, notebookID).
			Scan(&id)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		content := "# " + date + "\n\n"
		rendered, err := markdown.Render(content)
		if err != nil {
			return err
		}
		slug, err := uniqueSlug(ctx, tx)
		if err != nil {
			return err
		}
		now := db.Now()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO notes (notebook_id, title, slug, content_md, content_html, content_text, toc_json,
			                   visibility, is_daily, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'private', 1, ?, ?)`,
			notebookID, date, slug, content, rendered.HTML, rendered.Text, "[]", now, now)
		if err != nil {
			return err
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return recordActivity(ctx, tx, id, "create")
	})
	if err != nil {
		return nil, err
	}
	return a.GetNote(ctx, id)
}
