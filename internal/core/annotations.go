package core

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/xiaotuantuan5/blog/internal/db"
	"github.com/xiaotuantuan5/blog/internal/markdown"
)

const (
	annotationCtxLen = 32
	maxExactRunes    = 2000
	maxCommentRunes  = 5000
)

var annotationColorsSet = map[string]bool{
	"yellow": true, "green": true, "blue": true, "red": true,
}

// Annotation is a text highlight with an optional comment. Its position is
// stored as text anchors (exact/prefix/suffix), following the W3C Web
// Annotation data model, so it survives most edits of the surrounding text.
type Annotation struct {
	ID          int64   `json:"id"`
	NoteID      int64   `json:"noteId"`
	Exact       string  `json:"exact"`
	Prefix      string  `json:"prefix"`
	Suffix      string  `json:"suffix"`
	StartOffset int     `json:"startOffset"`
	EndOffset   int     `json:"endOffset"`
	Color       string  `json:"color"`
	CommentMD   string  `json:"commentMd"`
	CommentHTML string  `json:"commentHtml"`
	Orphaned    bool    `json:"orphaned"`
	CreatedAt   db.Time `json:"createdAt"`
	UpdatedAt   db.Time `json:"updatedAt"`
}

// AnnotationListItem adds note context for the annotations overview page.
type AnnotationListItem struct {
	Annotation
	NoteTitle     string  `json:"noteTitle"`
	NotebookID    *int64  `json:"notebookId"`
	NoteUpdatedAt db.Time `json:"noteUpdatedAt"`
}

// AnnotationInput creates or replaces an annotation.
type AnnotationInput struct {
	Exact       string `json:"exact"`
	Prefix      string `json:"prefix"`
	Suffix      string `json:"suffix"`
	StartOffset int    `json:"startOffset"`
	EndOffset   int    `json:"endOffset"`
	Color       string `json:"color"`
	CommentMD   string `json:"commentMd"`
	// Orphaned is set by the client when it could not re-locate the anchors.
	Orphaned bool `json:"orphaned"`
}

func (in *AnnotationInput) clean() error {
	in.Exact = strings.TrimSpace(in.Exact)
	if in.Exact == "" {
		return invalid("高亮内容不能为空")
	}
	if utf8.RuneCountInString(in.Exact) > maxExactRunes {
		return invalid("高亮内容太长（最多 2000 字）")
	}
	// Keep only a window of context around the exact text.
	in.Prefix = tailRunes(in.Prefix, annotationCtxLen)
	in.Suffix = headRunes(in.Suffix, annotationCtxLen)
	if in.StartOffset < 0 || in.EndOffset < in.StartOffset {
		return invalid("无效的位置信息")
	}
	if in.Color == "" {
		in.Color = "yellow"
	}
	if !annotationColorsSet[in.Color] {
		return invalid("无效的高亮颜色")
	}
	if utf8.RuneCountInString(in.CommentMD) > maxCommentRunes {
		return invalid("批注太长（最多 5000 字）")
	}
	return nil
}

func headRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

const annotationCols = `id, note_id, exact, prefix, suffix, start_offset, end_offset,
	color, comment_md, comment_html, orphaned, created_at, updated_at`

func scanAnnotation(row scanner) (Annotation, error) {
	var an Annotation
	err := row.Scan(&an.ID, &an.NoteID, &an.Exact, &an.Prefix, &an.Suffix,
		&an.StartOffset, &an.EndOffset, &an.Color, &an.CommentMD, &an.CommentHTML,
		&an.Orphaned, &an.CreatedAt, &an.UpdatedAt)
	return an, err
}

// ListNoteAnnotations returns a note's annotations in document order.
func (a *App) ListNoteAnnotations(ctx context.Context, noteID int64) ([]Annotation, error) {
	rows, err := a.DB.QueryContext(ctx,
		`SELECT `+annotationCols+` FROM annotations WHERE note_id = ? ORDER BY start_offset, id`, noteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Annotation{}
	for rows.Next() {
		an, err := scanAnnotation(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, an)
	}
	return list, rows.Err()
}

// CreateAnnotation adds an annotation to a note.
func (a *App) CreateAnnotation(ctx context.Context, noteID int64, in AnnotationInput) (*Annotation, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	commentHTML, commentText, err := renderComment(in.CommentMD)
	if err != nil {
		return nil, err
	}
	var id int64
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		var deleted sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT deleted_at FROM notes WHERE id = ?`, noteID).Scan(&deleted)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && deleted.Valid) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO annotations (note_id, exact, prefix, suffix, start_offset, end_offset,
			                         color, comment_md, comment_html, comment_text, orphaned, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			noteID, in.Exact, in.Prefix, in.Suffix, in.StartOffset, in.EndOffset,
			in.Color, in.CommentMD, commentHTML, commentText, in.Orphaned, db.Now(), db.Now())
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	if err != nil {
		return nil, err
	}
	var an Annotation
	err = a.DB.QueryRowContext(ctx, `SELECT `+annotationCols+` FROM annotations WHERE id = ?`, id).
		Scan(&an.ID, &an.NoteID, &an.Exact, &an.Prefix, &an.Suffix,
			&an.StartOffset, &an.EndOffset, &an.Color, &an.CommentMD, &an.CommentHTML,
			&an.Orphaned, &an.CreatedAt, &an.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &an, nil
}

// UpdateAnnotation replaces an annotation's anchors, color and comment.
func (a *App) UpdateAnnotation(ctx context.Context, id int64, in AnnotationInput) (*Annotation, error) {
	if err := in.clean(); err != nil {
		return nil, err
	}
	commentHTML, commentText, err := renderComment(in.CommentMD)
	if err != nil {
		return nil, err
	}
	err = a.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE annotations SET exact = ?, prefix = ?, suffix = ?, start_offset = ?, end_offset = ?,
			       color = ?, comment_md = ?, comment_html = ?, comment_text = ?, orphaned = ?, updated_at = ?
			WHERE id = ?`,
			in.Exact, in.Prefix, in.Suffix, in.StartOffset, in.EndOffset,
			in.Color, in.CommentMD, commentHTML, commentText, in.Orphaned, db.Now(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var an Annotation
	err = a.DB.QueryRowContext(ctx, `SELECT `+annotationCols+` FROM annotations WHERE id = ?`, id).
		Scan(&an.ID, &an.NoteID, &an.Exact, &an.Prefix, &an.Suffix,
			&an.StartOffset, &an.EndOffset, &an.Color, &an.CommentMD, &an.CommentHTML,
			&an.Orphaned, &an.CreatedAt, &an.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &an, nil
}

// DeleteAnnotation removes an annotation.
func (a *App) DeleteAnnotation(ctx context.Context, id int64) error {
	res, err := a.DB.ExecContext(ctx, `DELETE FROM annotations WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// renderComment returns (HTML, plain text) of a comment. The plain text
// feeds the FTS index so markdown syntax doesn't break trigram matching.
func renderComment(md string) (string, string, error) {
	res, err := markdown.Render(md)
	if err != nil {
		return "", "", err
	}
	return res.HTML, res.Text, nil
}

// AnnotationFilter selects annotations for the overview page.
type AnnotationFilter struct {
	Color      string
	NotebookID *int64
	Q          string
	Page       int
	PageSize   int
}

// ListAnnotations returns annotations across all living notes.
func (a *App) ListAnnotations(ctx context.Context, f AnnotationFilter) (*AnnotationPage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 50
	}

	where := []string{"n.deleted_at IS NULL"}
	args := []any{}
	if f.Color != "" {
		if !annotationColorsSet[f.Color] {
			return nil, invalid("无效的高亮颜色")
		}
		where = append(where, "an.color = ?")
		args = append(args, f.Color)
	}
	if f.NotebookID != nil {
		if *f.NotebookID == 0 {
			where = append(where, "n.notebook_id IS NULL")
		} else {
			where = append(where, `n.notebook_id IN (
				WITH RECURSIVE sub(id) AS (
					SELECT ? UNION ALL SELECT nb.id FROM notebooks nb JOIN sub ON nb.parent_id = sub.id
				) SELECT id FROM sub)`)
			args = append(args, *f.NotebookID)
		}
	}

	fts, like, _ := searchTerms(f.Q)
	joins := ""
	if len(fts) > 0 {
		joins = "JOIN annotations_fts ON annotations_fts.rowid = an.id"
		where = append(where, "annotations_fts MATCH ?")
		args = append(args, strings.Join(fts, " AND "))
	}
	for _, t := range like {
		where = append(where, `(an.exact LIKE ? ESCAPE '\' OR an.comment_text LIKE ? ESCAPE '\')`)
		p := likePattern(t)
		args = append(args, p, p)
	}

	from := ` FROM annotations an JOIN notes n ON n.id = an.note_id ` + joins +
		" WHERE " + strings.Join(where, " AND ")

	var total int
	if err := a.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+from, args...).Scan(&total); err != nil {
		return nil, err
	}
	query := `SELECT an.id, an.note_id, an.exact, an.prefix, an.suffix, an.start_offset, an.end_offset,
		an.color, an.comment_md, an.comment_html, an.orphaned, an.created_at, an.updated_at,
		n.title, n.notebook_id, n.updated_at` + from +
		` ORDER BY an.updated_at DESC, an.id DESC LIMIT ? OFFSET ?`
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := a.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &AnnotationPage{Items: []AnnotationListItem{}, Total: total, Page: f.Page, PageSize: f.PageSize}
	for rows.Next() {
		var it AnnotationListItem
		err := rows.Scan(&it.ID, &it.NoteID, &it.Exact, &it.Prefix, &it.Suffix,
			&it.StartOffset, &it.EndOffset, &it.Color, &it.CommentMD, &it.CommentHTML,
			&it.Orphaned, &it.CreatedAt, &it.UpdatedAt, &it.NoteTitle, &it.NotebookID, &it.NoteUpdatedAt)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, it)
	}
	return page, rows.Err()
}

// AnnotationPage is one page of the annotations overview.
type AnnotationPage struct {
	Items    []AnnotationListItem `json:"items"`
	Total    int                  `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"pageSize"`
}
