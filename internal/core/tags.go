package core

import (
	"context"
	"database/sql"
	"errors"

	"github.com/xiaotuantuan5/blog/internal/db"
)

type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ListTags returns tags with note counts. With publicOnly, only public
// notes are counted and tags without public notes are omitted.
func (a *App) ListTags(ctx context.Context, publicOnly bool) ([]Tag, error) {
	cond := ""
	if publicOnly {
		cond = "WHERE n.visibility = 'public'"
	}
	rows, err := a.DB.QueryContext(ctx, `
		SELECT t.id, t.name, COUNT(n.id)
		FROM tags t JOIN note_tags nt ON nt.tag_id = t.id JOIN notes n ON n.id = nt.note_id
		`+cond+`
		GROUP BY t.id ORDER BY COUNT(n.id) DESC, t.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// RenameTag renames a tag, merging into an existing tag of the same name.
func (a *App) RenameTag(ctx context.Context, id int64, name string) error {
	names, err := cleanTags([]string{name})
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return invalid("标签名不能为空")
	}
	name = names[0]
	return a.inTx(ctx, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE id = ?`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrNotFound
		}
		var other int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM tags WHERE name = ? AND id <> ?`, name, id).Scan(&other)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `UPDATE tags SET name = ? WHERE id = ?`, name, id)
			return err
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_tags (note_id, tag_id) SELECT note_id, ? FROM note_tags WHERE tag_id = ?`, other, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
		return err
	})
}

func (a *App) DeleteTag(ctx context.Context, id int64) error {
	res, err := a.DB.ExecContext(ctx, `DELETE FROM tags WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type ArchiveItem struct {
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	PublishedAt db.Time `json:"publishedAt"`
}

// Archive lists all public notes, newest first.
func (a *App) Archive(ctx context.Context) ([]ArchiveItem, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT title, slug, published_at FROM notes
		WHERE visibility = 'public' ORDER BY published_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []ArchiveItem{}
	for rows.Next() {
		var it ArchiveItem
		if err := rows.Scan(&it.Title, &it.Slug, &it.PublishedAt); err != nil {
			return nil, err
		}
		list = append(list, it)
	}
	return list, rows.Err()
}

type Stats struct {
	Notes     int `json:"notes"`
	Public    int `json:"public"`
	Notebooks int `json:"notebooks"`
	Tags      int `json:"tags"`
	Files     int `json:"files"`
}

func (a *App) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	err := a.DB.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM notes),
		       (SELECT COUNT(*) FROM notes WHERE visibility = 'public'),
		       (SELECT COUNT(*) FROM notebooks),
		       (SELECT COUNT(*) FROM tags),
		       (SELECT COUNT(*) FROM files)`).Scan(&s.Notes, &s.Public, &s.Notebooks, &s.Tags, &s.Files)
	return s, err
}

// SitemapItem is a public note for the RSS feed and sitemap.
type FeedItem struct {
	Title       string
	Slug        string
	Summary     string
	ContentHTML string
	PublishedAt db.Time
	UpdatedAt   db.Time
}

// Feed returns up to limit public notes, newest first (limit <= 0: all).
func (a *App) Feed(ctx context.Context, limit int, withContent bool) ([]FeedItem, error) {
	content := "''"
	if withContent {
		content = "content_html"
	}
	if limit <= 0 {
		limit = -1
	}
	rows, err := a.DB.QueryContext(ctx, `
		SELECT title, slug, summary, substr(content_text, 1, 200), `+content+`, published_at, updated_at
		FROM notes WHERE visibility = 'public' ORDER BY published_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []FeedItem
	for rows.Next() {
		var (
			it  FeedItem
			pre string
		)
		if err := rows.Scan(&it.Title, &it.Slug, &it.Summary, &pre, &it.ContentHTML, &it.PublishedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		if it.Summary == "" {
			it.Summary = summarize(pre)
		}
		list = append(list, it)
	}
	return list, rows.Err()
}
