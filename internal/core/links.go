package core

import (
	"context"
	"database/sql"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/xiaotuantuan5/blog/internal/markdown"
)

// Backlink is a note that references another note through [[title]].
type Backlink struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Visibility string `json:"visibility"`
}

// Placeholder anchors come from the wikilink renderer; resolved anchors are
// what gets stored in notes.content_html. Both are self-generated markup,
// already sanitized, so rewriting them with regex is safe.
var (
	wikilinkAnchorRe = regexp.MustCompile(`<a class="wikilink" data-wikilink="([^"]*)">([^<]*)</a>`)
	resolvedAnchorRe = regexp.MustCompile(`<a class="wikilink" data-note="([0-9]+)" href="/app/notes/[0-9]+">([^<]*)</a>`)
)

// resolveWikilinks rewrites placeholder wikilink anchors against the notes
// table and returns the IDs of the referenced notes (sorted, deduplicated).
// Titles matching several notes link to the most recently updated one.
func resolveWikilinks(ctx context.Context, q querier, page string) (string, []int64, error) {
	matches := wikilinkAnchorRe.FindAllStringSubmatchIndex(page, -1)
	if len(matches) == 0 {
		return page, nil, nil
	}

	titles := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		t := html.UnescapeString(page[m[2]:m[3]])
		if !seen[t] {
			seen[t] = true
			titles = append(titles, t)
		}
	}

	args := make([]any, len(titles))
	for i, t := range titles {
		args[i] = t
	}
	byTitle := map[string]int64{}
	rows, err := q.QueryContext(ctx,
		`SELECT title, id FROM notes WHERE deleted_at IS NULL AND title IN (`+
			strings.TrimRight(strings.Repeat("?,", len(titles)), ",")+
			`) ORDER BY updated_at DESC, id DESC`, args...)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			title string
			id    int64
		)
		if err := rows.Scan(&title, &id); err != nil {
			return "", nil, err
		}
		if _, dup := byTitle[title]; !dup {
			byTitle[title] = id
		}
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	var b strings.Builder
	last := 0
	ids := map[int64]bool{}
	for _, m := range matches {
		title := html.UnescapeString(page[m[2]:m[3]])
		b.WriteString(page[last:m[0]])
		if id, ok := byTitle[title]; ok {
			fmt.Fprintf(&b, `<a class="wikilink" data-note="%d" href="/app/notes/%d">%s</a>`, id, id, page[m[4]:m[5]])
			ids[id] = true
		} else {
			b.WriteString(`<span class="wikilink wikilink-missing">`)
			b.WriteString(page[m[4]:m[5]])
			b.WriteString(`</span>`)
		}
		last = m[1]
	}
	b.WriteString(page[last:])

	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return b.String(), out, nil
}

// rewirePublicWikilinks adapts stored HTML for public readers: links to other
// public notes point at /n/:slug, everything else (private, trashed, purged)
// collapses to plain text so nothing about private notes leaks.
func (a *App) rewirePublicWikilinks(ctx context.Context, page string) (string, error) {
	matches := resolvedAnchorRe.FindAllStringSubmatchIndex(page, -1)
	if len(matches) == 0 {
		return page, nil
	}

	ids := map[int64]bool{}
	for _, m := range matches {
		var id int64
		_, _ = fmt.Sscanf(page[m[2]:m[3]], "%d", &id)
		ids[id] = true
	}
	args := make([]any, 0, len(ids))
	for id := range ids {
		args = append(args, id)
	}
	public := map[int64]string{}
	rows, err := a.DB.QueryContext(ctx,
		`SELECT id, slug FROM notes WHERE visibility = 'public' AND deleted_at IS NULL AND id IN (`+
			strings.TrimRight(strings.Repeat("?,", len(args)), ",")+`)`, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id   int64
			slug string
		)
		if err := rows.Scan(&id, &slug); err != nil {
			return "", err
		}
		public[id] = slug
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		var id int64
		_, _ = fmt.Sscanf(page[m[2]:m[3]], "%d", &id)
		b.WriteString(page[last:m[0]])
		if slug, ok := public[id]; ok {
			fmt.Fprintf(&b, `<a class="wikilink" href="/n/%s">%s</a>`, slug, page[m[4]:m[5]])
		} else {
			b.WriteString(page[m[4]:m[5]])
		}
		last = m[1]
	}
	b.WriteString(page[last:])
	return b.String(), nil
}

// syncNoteLinks rewrites a note's outgoing [[title]] references.
func syncNoteLinks(ctx context.Context, tx *sql.Tx, id int64, targets []int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM note_links WHERE source_id = ?`, id); err != nil {
		return err
	}
	for _, t := range targets {
		if t == id {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO note_links (source_id, target_id) VALUES (?, ?)`, id, t); err != nil {
			return err
		}
	}
	return nil
}

// Backlinks lists notes that link to id, most recently updated first.
func (a *App) Backlinks(ctx context.Context, id int64) ([]Backlink, error) {
	rows, err := a.DB.QueryContext(ctx, `
		SELECT l.source_id, n.title, n.visibility
		FROM note_links l JOIN notes n ON n.id = l.source_id
		WHERE l.target_id = ? AND n.deleted_at IS NULL
		ORDER BY n.updated_at DESC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Backlink{}
	for rows.Next() {
		var b Backlink
		if err := rows.Scan(&b.ID, &b.Title, &b.Visibility); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, rows.Err()
}

// RenderPreview renders note source like the editor preview, with wikilinks
// resolved against the notes table.
func (a *App) RenderPreview(ctx context.Context, format, src string) (markdown.Result, error) {
	if format == "" {
		format = FormatMD
	}
	if format != FormatMD && format != FormatHTML {
		return markdown.Result{}, invalid("无效的笔记格式")
	}
	res, err := renderNote(format, src)
	if err != nil {
		return markdown.Result{}, err
	}
	page, _, err := resolveWikilinks(ctx, a.DB, res.HTML)
	if err != nil {
		return markdown.Result{}, err
	}
	res.HTML = page
	return res, nil
}
