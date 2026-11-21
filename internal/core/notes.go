package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xiaotuantuan5/blog/internal/db"
	"github.com/xiaotuantuan5/blog/internal/markdown"
)

const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"

	// FormatMD is a note written in Markdown; FormatHTML is authored as
	// (sanitized) HTML — the shape generated notes usually come in.
	FormatMD   = "md"
	FormatHTML = "html"

	summaryLen = 120
	maxTags    = 20
)

// NoteSummary is a note without its body, used in lists.
type NoteSummary struct {
	ID          int64    `json:"id"`
	NotebookID  *int64   `json:"notebookId"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Summary     string   `json:"summary"`
	Format      string   `json:"format"`
	Visibility  string   `json:"visibility"`
	Pinned      bool     `json:"pinned"`
	Favorite    bool     `json:"favorite"`
	Tags        []string `json:"tags"`
	Version     int      `json:"version"`
	PublishedAt db.Time  `json:"publishedAt"`
	CreatedAt   db.Time  `json:"createdAt"`
	UpdatedAt   db.Time  `json:"updatedAt"`
	// DeletedAt is set while the note sits in the trash.
	DeletedAt db.Time `json:"deletedAt"`
	// Snippet is an HTML-escaped excerpt with <mark> around search hits.
	Snippet string `json:"snippet,omitempty"`
}

// Note is a full note.
type Note struct {
	NoteSummary
	ManualSummary string             `json:"manualSummary"`
	ContentMD     string             `json:"contentMd"`
	ContentHTML   string             `json:"contentHtml"`
	TOC           []markdown.Heading `json:"toc"`
	// Backlinks lists the notes that reference this one through [[title]].
	Backlinks []Backlink `json:"backlinks"`
}

// NoteInput is what the editor sends when creating or updating a note.
type NoteInput struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
	// NotebookID is only used on create; moves go through SetNoteFlags so
	// they don't conflict with the editor's autosave.
	NotebookID *int64   `json:"notebookId"`
	ContentMD  string   `json:"contentMd"`
	Format     string   `json:"format"`
	Summary    string   `json:"summary"`
	Visibility string   `json:"visibility"`
	Tags       []string `json:"tags"`
	// Version must match the stored version on update.
	Version int `json:"version"`
	// Manual marks an explicit save (Ctrl+S / the save button): it always
	// snapshots a revision, unlike autosave's 10-minute interval.
	Manual bool `json:"manual"`
}

// NoteFlags changes list-level properties without touching the note
// version, so they never conflict with an open editor.
type NoteFlags struct {
	Pinned   *bool `json:"pinned"`
	Favorite *bool `json:"favorite"`
	// Notebook moves the note when present; null means "no notebook".
	Notebook OptionalID `json:"notebookId"`
}

// OptionalID distinguishes an absent JSON field from an explicit null.
type OptionalID struct {
	Set   bool
	Value *int64
}

func (o *OptionalID) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v int64
	if err := json.Unmarshal(b, &v); err != nil {
		return invalid("无效的笔记本")
	}
	o.Value = &v
	return nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func cleanTags(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		t = strings.TrimPrefix(t, "#")
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > 30 {
			return nil, invalid("标签“" + t + "”太长（最多 30 个字）")
		}
		if strings.IndexFunc(t, unicode.IsControl) >= 0 || strings.ContainsAny(t, "/,") {
			return nil, invalid("标签不能包含 / 或 , 等特殊字符")
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	if len(out) > maxTags {
		return nil, invalid("每篇笔记最多 20 个标签")
	}
	return out, nil
}

func (in *NoteInput) clean() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Summary = strings.TrimSpace(in.Summary)
	if in.Title == "" {
		return invalid("标题不能为空")
	}
	if utf8.RuneCountInString(in.Title) > 200 {
		return invalid("标题太长（最多 200 个字）")
	}
	if in.Slug != "" && (len(in.Slug) > 80 || !slugRe.MatchString(in.Slug)) {
		return invalid("访问地址只能包含小写字母、数字和连字符，最长 80 个字符")
	}
	if utf8.RuneCountInString(in.Summary) > 500 {
		return invalid("摘要太长（最多 500 个字）")
	}
	switch in.Visibility {
	case "":
		in.Visibility = VisibilityPrivate
	case VisibilityPrivate, VisibilityPublic:
	default:
		return invalid("无效的可见性")
	}
	switch in.Format {
	case "":
		in.Format = FormatMD
	case FormatMD, FormatHTML:
	default:
		return invalid("无效的笔记格式")
	}
	tags, err := cleanTags(in.Tags)
	if err != nil {
		return err
	}
	in.Tags = tags
	return nil
}

// renderNote renders note source in the note's format. HTML notes skip the
// Markdown pipeline entirely; [[wikilinks]] are only interpreted in Markdown.
func renderNote(format, src string) (markdown.Result, error) {
	if format == FormatHTML {
		return markdown.RenderHTML(src)
	}
	return markdown.Render(src)
}

// CreateNote creates a note.
func (a *App) CreateNote(ctx context.Context, in NoteInput) (*Note, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	rendered, err := renderNote(in.Format, in.ContentMD)
	if err != nil {
		return nil, err
	}
	var id int64
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		if err := checkNotebook(ctx, tx, in.NotebookID); err != nil {
			return err
		}
		if in.Slug == "" {
			if in.Slug, err = uniqueSlug(ctx, tx); err != nil {
				return err
			}
		} else if err := checkSlugFree(ctx, tx, in.Slug, 0); err != nil {
			return err
		}
		now := db.Now()
		var published db.Time
		if in.Visibility == VisibilityPublic {
			published = now
		}
		html, links, err := resolveWikilinks(ctx, tx, rendered.HTML)
		if err != nil {
			return err
		}
		toc, _ := json.Marshal(nonNilTOC(rendered.TOC))
		res, err := tx.ExecContext(ctx, `
			INSERT INTO notes (notebook_id, title, slug, format, content_md, content_html, content_text, toc_json,
			                   summary, visibility, published_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			in.NotebookID, in.Title, in.Slug, in.Format, in.ContentMD, html, rendered.Text, string(toc),
			in.Summary, in.Visibility, published, now, now)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		if err := syncNoteRefs(ctx, tx, id, in.Tags, rendered.Files); err != nil {
			return err
		}
		if err := syncNoteLinks(ctx, tx, id, links); err != nil {
			return err
		}
		return recordActivity(ctx, tx, id, "create")
	})
	if err != nil {
		return nil, err
	}
	return a.GetNote(ctx, id)
}

// UpdateNote replaces a note's content and metadata. It fails with a
// ConflictError when in.Version is stale.
func (a *App) UpdateNote(ctx context.Context, id int64, in NoteInput) (*Note, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	rendered, err := renderNote(in.Format, in.ContentMD)
	if err != nil {
		return nil, err
	}
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		var (
			curSlug   string
			curVer    int
			published db.Time
		)
		err := tx.QueryRowContext(ctx, `SELECT slug, version, published_at FROM notes WHERE id = ?`, id).
			Scan(&curSlug, &curVer, &published)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if in.Version != curVer {
			return conflict("这篇笔记已在其他地方被修改，请刷新后再编辑")
		}
		if err := snapshotRevision(ctx, tx, id, in.Manual); err != nil {
			return err
		}
		if in.Slug == "" {
			in.Slug = curSlug
		} else if in.Slug != curSlug {
			if err := checkSlugFree(ctx, tx, in.Slug, id); err != nil {
				return err
			}
		}
		now := db.Now()
		if in.Visibility == VisibilityPublic && published.IsZero() {
			published = now
		}
		html, links, err := resolveWikilinks(ctx, tx, rendered.HTML)
		if err != nil {
			return err
		}
		toc, _ := json.Marshal(nonNilTOC(rendered.TOC))
		_, err = tx.ExecContext(ctx, `
			UPDATE notes SET title = ?, slug = ?, format = ?, content_md = ?, content_html = ?,
			       content_text = ?, toc_json = ?, summary = ?, visibility = ?, published_at = ?,
			       updated_at = ?, version = version + 1
			WHERE id = ?`,
			in.Title, in.Slug, in.Format, in.ContentMD, html, rendered.Text, string(toc),
			in.Summary, in.Visibility, published, now, id)
		if err != nil {
			return err
		}
		if err := syncNoteRefs(ctx, tx, id, in.Tags, rendered.Files); err != nil {
			return err
		}
		if err := syncNoteLinks(ctx, tx, id, links); err != nil {
			return err
		}
		return recordActivity(ctx, tx, id, "edit")
	})
	if err != nil {
		return nil, err
	}
	return a.GetNote(ctx, id)
}

// SetNoteFlags updates pinned/favorite and moves the note between notebooks.
func (a *App) SetNoteFlags(ctx context.Context, id int64, f NoteFlags) error {
	return a.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE notes SET pinned = COALESCE(?, pinned), favorite = COALESCE(?, favorite) WHERE id = ?`,
			f.Pinned, f.Favorite, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		if f.Notebook.Set {
			if err := checkNotebook(ctx, tx, f.Notebook.Value); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE notes SET notebook_id = ? WHERE id = ?`, f.Notebook.Value, id)
		}
		return err
	})
}

// DeleteNote moves a note to the trash. Trashed notes are purged
// automatically after trashRetention.
func (a *App) DeleteNote(ctx context.Context, id int64) error {
	return a.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE notes SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`, db.Now(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// RestoreNote takes a note out of the trash.
func (a *App) RestoreNote(ctx context.Context, id int64) error {
	res, err := a.DB.ExecContext(ctx, `UPDATE notes SET deleted_at = NULL WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeNote permanently deletes a trashed note (or an untrashed one when
// force is set), cascading to tags, links, revisions and annotations.
func (a *App) PurgeNote(ctx context.Context, id int64, force bool) error {
	return a.inTx(ctx, func(tx *sql.Tx) error {
		query := `DELETE FROM notes WHERE id = ?`
		if !force {
			query += ` AND deleted_at IS NOT NULL`
		}
		res, err := tx.ExecContext(ctx, query, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM note_tags)`)
		return err
	})
}

// TrashItem is a note waiting in the trash.
type TrashItem struct {
	NoteSummary
	NotebookName string `json:"notebookName"`
}

// ListTrash returns trashed notes, most recently deleted first.
func (a *App) ListTrash(ctx context.Context) ([]TrashItem, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT `+noteCols+`, COALESCE(nb.name, '')
		FROM notes n LEFT JOIN notebooks nb ON nb.id = n.notebook_id
		WHERE n.deleted_at IS NOT NULL
		ORDER BY n.deleted_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []TrashItem{}
	for rows.Next() {
		var it TrashItem
		s, _, err := scanSummary(rows, &it.NotebookName)
		if err != nil {
			return nil, err
		}
		it.NoteSummary = s
		list = append(list, it)
	}
	return list, rows.Err()
}

func nonNilTOC(t []markdown.Heading) []markdown.Heading {
	if t == nil {
		return []markdown.Heading{}
	}
	return t
}

func checkNotebook(ctx context.Context, q querier, id *int64) error {
	if id == nil {
		return nil
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM notebooks WHERE id = ?`, *id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return invalid("笔记本不存在")
	}
	return nil
}

func checkSlugFree(ctx context.Context, q querier, slug string, selfID int64) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE slug = ? AND id <> ?`, slug, selfID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return invalid("访问地址已被其他笔记使用")
	}
	return nil
}

func uniqueSlug(ctx context.Context, q querier) (string, error) {
	for i := 0; i < 10; i++ {
		s := randomString(8)
		if err := checkSlugFree(ctx, q, s, 0); err == nil {
			return s, nil
		} else if _, ok := err.(*ValidationError); !ok {
			return "", err
		}
	}
	return "", errors.New("could not generate slug")
}

// syncNoteRefs rewrites a note's tags and attachment references.
func syncNoteRefs(ctx context.Context, tx *sql.Tx, id int64, tags, files []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_tags WHERE note_id = ?`, id); err != nil {
		return err
	}
	for _, t := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags (name) VALUES (?) ON CONFLICT(name) DO NOTHING`, t); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_tags (note_id, tag_id) SELECT ?, id FROM tags WHERE name = ?`, id, t); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM note_tags)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_files WHERE note_id = ?`, id); err != nil {
		return err
	}
	for _, h := range files {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_files (note_id, file_id) SELECT ?, id FROM files WHERE hash = ?`, id, h); err != nil {
			return err
		}
	}
	return nil
}

const noteCols = `n.id, n.notebook_id, n.title, n.slug, n.summary, n.format, substr(n.content_text, 1, 200),
		n.visibility, n.pinned, n.favorite, n.version, n.published_at, n.created_at, n.updated_at, n.deleted_at,
		COALESCE((SELECT group_concat(t.name, char(31)) FROM note_tags nt JOIN tags t ON t.id = nt.tag_id WHERE nt.note_id = n.id), '')`

type scanner interface{ Scan(dest ...any) error }

func scanSummary(row scanner, extra ...any) (NoteSummary, string, error) {
	var (
		s       NoteSummary
		textPre string
		tags    string
	)
	dest := []any{&s.ID, &s.NotebookID, &s.Title, &s.Slug, &s.Summary, &s.Format, &textPre,
		&s.Visibility, &s.Pinned, &s.Favorite, &s.Version, &s.PublishedAt, &s.CreatedAt, &s.UpdatedAt,
		&s.DeletedAt, &tags}
	err := row.Scan(append(dest, extra...)...)
	manual := s.Summary
	if s.Summary == "" {
		s.Summary = markdown.Summary(textPre, summaryLen)
	}
	s.Tags = []string{}
	if tags != "" {
		s.Tags = strings.Split(tags, "\x1f")
		sort.Slice(s.Tags, func(i, j int) bool { return strings.ToLower(s.Tags[i]) < strings.ToLower(s.Tags[j]) })
	}
	return s, manual, err
}

func (a *App) getNoteWhere(ctx context.Context, where string, arg any) (*Note, error) {
	row := a.DB.QueryRowContext(ctx, `SELECT `+noteCols+`, n.content_md, n.content_html, n.toc_json FROM notes n WHERE `+where, arg)
	var (
		n   Note
		toc string
	)
	sum, manual, err := scanSummary(row, &n.ContentMD, &n.ContentHTML, &toc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	n.NoteSummary = sum
	n.ManualSummary = manual
	if err := json.Unmarshal([]byte(toc), &n.TOC); err != nil || n.TOC == nil {
		n.TOC = []markdown.Heading{}
	}
	return &n, nil
}

// GetNote returns any note by ID (admin), including trashed ones, with its
// backlinks.
func (a *App) GetNote(ctx context.Context, id int64) (*Note, error) {
	n, err := a.getNoteWhere(ctx, `n.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if n.Backlinks, err = a.Backlinks(ctx, id); err != nil {
		return nil, err
	}
	return n, nil
}

// PublicNote is a public note plus its neighbours in publish order.
// The Markdown source and manual summary are not exposed.
type PublicNote struct {
	*Note
	ContentMD     string    `json:"contentMd,omitempty"`
	ManualSummary string    `json:"manualSummary,omitempty"`
	Prev          *NoteLink `json:"prev"`
	Next          *NoteLink `json:"next"`
}

type NoteLink struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// GetPublicNote returns a public note by slug. Markdown source is omitted
// and wikilinks to non-public notes collapse to plain text.
func (a *App) GetPublicNote(ctx context.Context, slug string) (*PublicNote, error) {
	n, err := a.getNoteWhere(ctx, `n.slug = ? AND n.visibility = 'public' AND n.deleted_at IS NULL`, slug)
	if err != nil {
		return nil, err
	}
	n.ContentMD = ""
	n.ManualSummary = ""
	n.Backlinks = nil
	if n.ContentHTML, err = a.rewirePublicWikilinks(ctx, n.ContentHTML); err != nil {
		return nil, err
	}
	// The shadowing fields above stay empty, so they're omitted from JSON.
	pn := &PublicNote{Note: n}
	neighbour := func(cmp, order string) (*NoteLink, error) {
		var l NoteLink
		err := a.DB.QueryRowContext(ctx, `
			SELECT title, slug FROM notes
			WHERE visibility = 'public' AND deleted_at IS NULL
			  AND (published_at `+cmp+` ? OR (published_at = ? AND id `+cmp+` ?))
			ORDER BY published_at `+order+`, id `+order+` LIMIT 1`,
			n.PublishedAt, n.PublishedAt, n.ID).Scan(&l.Title, &l.Slug)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return &l, err
	}
	if pn.Prev, err = neighbour("<", "DESC"); err != nil {
		return nil, err
	}
	if pn.Next, err = neighbour(">", "ASC"); err != nil {
		return nil, err
	}
	return pn, nil
}
