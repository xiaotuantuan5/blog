package core

import (
	"context"
	"html"
	"strings"
	"unicode/utf8"
)

// NoteFilter selects notes for lists and search.
type NoteFilter struct {
	PublicOnly bool
	// NotebookID filters by notebook including sub-notebooks; 0 means
	// "no notebook".
	NotebookID *int64
	Tag        string
	Q          string
	Visibility string
	Favorite   bool
	Page       int
	PageSize   int
}

type NotePage struct {
	Items    []NoteSummary `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
}

// searchTerms splits a query into terms. Terms of 3+ characters go through
// the FTS5 trigram index; shorter ones (common for Chinese, e.g. "并发")
// fall back to LIKE because trigram can't match them.
func searchTerms(q string) (fts []string, like []string, all []string) {
	for _, t := range strings.Fields(q) {
		if len(all) >= 10 {
			break
		}
		all = append(all, t)
		if utf8.RuneCountInString(t) >= 3 {
			fts = append(fts, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
		} else {
			like = append(like, t)
		}
	}
	return
}

func likePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(s) + "%"
}

// ListNotes lists notes, optionally searching.
func (a *App) ListNotes(ctx context.Context, f NoteFilter) (*NotePage, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}

	var (
		joins []string
		where = []string{"n.deleted_at IS NULL"}
		args  []any
	)
	if f.PublicOnly {
		where = append(where, "n.visibility = 'public'")
	} else if f.Visibility != "" {
		where = append(where, "n.visibility = ?")
		args = append(args, f.Visibility)
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
	if f.Tag != "" {
		where = append(where, `EXISTS (SELECT 1 FROM note_tags nt JOIN tags t ON t.id = nt.tag_id
			WHERE nt.note_id = n.id AND t.name = ?)`)
		args = append(args, f.Tag)
	}
	if f.Favorite {
		where = append(where, "n.favorite = 1")
	}

	fts, like, all := searchTerms(f.Q)
	if len(fts) > 0 {
		joins = append(joins, "JOIN notes_fts ON notes_fts.rowid = n.id")
		where = append(where, "notes_fts MATCH ?")
		args = append(args, strings.Join(fts, " AND "))
	}
	for _, t := range like {
		where = append(where, `(n.title LIKE ? ESCAPE '\' OR n.content_text LIKE ? ESCAPE '\')`)
		p := likePattern(t)
		args = append(args, p, p)
	}

	from := " FROM notes n " + strings.Join(joins, " ") + " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := a.DB.QueryRowContext(ctx, "SELECT COUNT(*)"+from, args...).Scan(&total); err != nil {
		return nil, err
	}

	order := " ORDER BY n.pinned DESC, n.updated_at DESC, n.id DESC"
	if f.PublicOnly {
		order = " ORDER BY n.pinned DESC, n.published_at DESC, n.id DESC"
	}
	if len(fts) > 0 {
		order = " ORDER BY bm25(notes_fts, 5.0, 1.0), n.updated_at DESC"
	} else if len(all) > 0 {
		order = " ORDER BY (n.title LIKE ? ESCAPE '\\') DESC, n.updated_at DESC"
		args = append(args, likePattern(all[0]))
	}

	cols := noteCols
	searching := len(all) > 0
	if searching {
		cols += ", n.content_text"
	}
	query := "SELECT " + cols + from + order + " LIMIT ? OFFSET ?"
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := a.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	page := &NotePage{Items: []NoteSummary{}, Total: total, Page: f.Page, PageSize: f.PageSize}
	for rows.Next() {
		var (
			s    NoteSummary
			body string
			err  error
		)
		if searching {
			s, _, err = scanSummary(rows, &body)
		} else {
			s, _, err = scanSummary(rows)
		}
		if err != nil {
			return nil, err
		}
		if searching {
			s.Snippet = makeSnippet(body, all, 40)
		}
		page.Items = append(page.Items, s)
	}
	return page, rows.Err()
}

// makeSnippet returns HTML-escaped text around the first hit, with every
// term wrapped in <mark>.
func makeSnippet(text string, terms []string, radius int) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	lower := []rune(strings.ToLower(string(runes)))
	if len(lower) != len(runes) {
		// Rare case-mapping that changes length; match case-sensitively.
		lower = runes
	}
	hit := -1
	for _, t := range terms {
		if i := runeIndex(lower, []rune(strings.ToLower(t))); i >= 0 && (hit < 0 || i < hit) {
			hit = i
		}
	}
	start, end := 0, len(runes)
	if hit < 0 {
		hit = 0
	}
	if hit > radius {
		start = hit - radius
	}
	if end > hit+radius*3 {
		end = hit + radius*3
	}
	if end < start {
		end = start
	}

	seg := runes[start:end]
	segLower := lower[start:end]
	marked := make([]bool, len(seg))
	for _, t := range terms {
		tr := []rune(strings.ToLower(t))
		if len(tr) == 0 {
			continue
		}
		for i := 0; i+len(tr) <= len(segLower); {
			if j := runeIndex(segLower[i:], tr); j >= 0 {
				for k := 0; k < len(tr); k++ {
					marked[i+j+k] = true
				}
				i += j + len(tr)
			} else {
				break
			}
		}
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	in := false
	for i, r := range seg {
		if marked[i] && !in {
			b.WriteString("<mark>")
			in = true
		} else if !marked[i] && in {
			b.WriteString("</mark>")
			in = false
		}
		b.WriteString(html.EscapeString(string(r)))
	}
	if in {
		b.WriteString("</mark>")
	}
	if end < len(runes) {
		b.WriteString("…")
	}
	return b.String()
}

func runeIndex(s, sub []rune) int {
	if len(sub) == 0 {
		return 0
	}
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := range sub {
			if s[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
