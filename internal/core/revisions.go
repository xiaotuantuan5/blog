package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/xiaotuantuan5/blog/internal/db"
)

// Revisions snapshot a note before each meaningful save so content can be
// diffed and restored. Autosaves only snapshot when the previous snapshot is
// older than revisionInterval; manual saves always do.
const (
	revisionInterval = 10 * time.Minute
	maxRevisions     = 100
)

// Revision is a stored snapshot of a note's title and content.
type Revision struct {
	ID        int64   `json:"id"`
	NoteID    int64   `json:"noteId"`
	Title     string  `json:"title"`
	ContentMD string  `json:"contentMd"`
	Format    string  `json:"format"`
	CreatedAt db.Time `json:"createdAt"`
}

// snapshotRevision stores the note's current title and content as a revision.
// Without force it skips snapshots taken less than revisionInterval ago.
func snapshotRevision(ctx context.Context, tx *sql.Tx, id int64, force bool) error {
	var (
		title   string
		content string
		format  string
	)
	err := tx.QueryRowContext(ctx, `SELECT title, content_md, format FROM notes WHERE id = ?`, id).
		Scan(&title, &content, &format)
	if err != nil {
		return err
	}
	if !force {
		var last db.Time
		err = tx.QueryRowContext(ctx,
			`SELECT created_at FROM note_revisions WHERE note_id = ? ORDER BY id DESC LIMIT 1`, id).Scan(&last)
		if errors.Is(err, sql.ErrNoRows) {
			// No snapshot yet: take one.
		} else if err != nil {
			return err
		} else if time.Since(last.Time) < revisionInterval {
			return nil
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO note_revisions (note_id, title, content_md, format, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, title, content, format, db.Now()); err != nil {
		return err
	}
	// Keep the newest maxRevisions snapshots.
	_, err = tx.ExecContext(ctx, `
		DELETE FROM note_revisions WHERE note_id = ? AND id NOT IN (
			SELECT id FROM note_revisions WHERE note_id = ? ORDER BY id DESC LIMIT ?)`, id, id, maxRevisions)
	return err
}

// ListRevisions returns a note's revisions, newest first.
func (a *App) ListRevisions(ctx context.Context, noteID int64) ([]Revision, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id, note_id, title, content_md, format, created_at FROM note_revisions
		 WHERE note_id = ? ORDER BY id DESC LIMIT ?`, noteID, maxRevisions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Revision{}
	for rows.Next() {
		var r Revision
		if err := rows.Scan(&r.ID, &r.NoteID, &r.Title, &r.ContentMD, &r.Format, &r.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// RestoreRevision restores a note's title and content from a revision.
// The current content is snapshotted first; this itself is a new version.
func (a *App) RestoreRevision(ctx context.Context, noteID, revID int64) (*Note, error) {
	err := a.inTx(ctx, func(tx *sql.Tx) error {
		var deleted sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM notes WHERE id = ?`, noteID).Scan(&deleted); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if deleted.Valid {
			return conflict("笔记在回收站中，请先恢复")
		}
		var rev Revision
		err := tx.QueryRowContext(ctx,
			`SELECT id, note_id, title, content_md, format, created_at FROM note_revisions WHERE id = ?`, revID).
			Scan(&rev.ID, &rev.NoteID, &rev.Title, &rev.ContentMD, &rev.Format, &rev.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && rev.NoteID != noteID) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if rev.Format == "" {
			rev.Format = FormatMD
		}

		res, err := renderNote(rev.Format, rev.ContentMD)
		if err != nil {
			return err
		}
		html, links, err := resolveWikilinks(ctx, tx, res.HTML)
		if err != nil {
			return err
		}
		if err := snapshotRevision(ctx, tx, noteID, true); err != nil {
			return err
		}
		toc, _ := json.Marshal(nonNilTOC(res.TOC))
		if _, err := tx.ExecContext(ctx, `
			UPDATE notes SET title = ?, format = ?, content_md = ?, content_html = ?, content_text = ?,
			       toc_json = ?, updated_at = ?, version = version + 1
			WHERE id = ?`,
			rev.Title, rev.Format, rev.ContentMD, html, res.Text, string(toc), db.Now(), noteID); err != nil {
			return err
		}
		// Tags and metadata stay as they are; only files and links follow content.
		tags, err := noteTags(ctx, tx, noteID)
		if err != nil {
			return err
		}
		if err := syncNoteRefs(ctx, tx, noteID, tags, res.Files); err != nil {
			return err
		}
		return syncNoteLinks(ctx, tx, noteID, links)
	})
	if err != nil {
		return nil, err
	}
	if err := recordActivity(ctx, a.DB, noteID, "edit"); err != nil {
		return nil, err
	}
	return a.GetNote(ctx, noteID)
}

// noteTags returns a note's current tag names.
func noteTags(ctx context.Context, q querier, id int64) ([]string, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT t.name FROM note_tags nt JOIN tags t ON t.id = nt.tag_id WHERE nt.note_id = ? ORDER BY t.name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
