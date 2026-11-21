// Package markdown renders note Markdown into sanitized HTML and extracts
// the data the rest of the app needs (plain text, table of contents and
// referenced attachments).
package markdown

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Heading is one entry of a note's table of contents.
type Heading struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// Result is the output of Render.
type Result struct {
	HTML string
	// Text is the plain-text content, used for search and summaries.
	Text string
	TOC  []Heading
	// Files lists hashes of attachments referenced as /files/<hash>.
	Files []string
	// Wikilinks lists the titles of `[[title]]` references, deduplicated.
	Wikilinks []string
}

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		extension.Footnote,
		mathExtension{},
		wikilinkExtension{},
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
		parser.WithASTTransformers(util.Prioritized(calloutTransformer{}, 100)),
	),
	goldmark.WithRendererOptions(
		// Raw HTML is allowed in source and cleaned by the sanitizer afterwards.
		html.WithUnsafe(),
		renderer.WithNodeRenderers(util.Prioritized(codeRenderer{}, 100)),
	),
)

var fileRefRe = regexp.MustCompile(`^/files/([0-9a-f]{64})(?:[?#].*)?$`)

// Render converts Markdown source into sanitized HTML plus derived data.
func Render(src string) (Result, error) {
	source := []byte(src)
	ctx := parser.NewContext(parser.WithIDs(newIDs()))
	doc := md.Parser().Parse(text.NewReader(source), parser.WithContext(ctx))

	res := Result{}
	seenFiles := map[string]bool{}
	seenWiki := map[string]bool{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *wikilink:
			title := string(node.title)
			if !seenWiki[title] {
				seenWiki[title] = true
				res.Wikilinks = append(res.Wikilinks, title)
			}
		case *ast.Heading:
			if node.Level >= 2 && node.Level <= 4 {
				id, _ := node.AttributeString("id")
				idb, _ := id.([]byte)
				res.TOC = append(res.TOC, Heading{
					Level: node.Level,
					ID:    string(idb),
					Text:  strings.TrimSpace(inlineText(node, source)),
				})
			}
		case *ast.Image:
			addFileRef(string(node.Destination), seenFiles, &res.Files)
		case *ast.Link:
			addFileRef(string(node.Destination), seenFiles, &res.Files)
		}
		return ast.WalkContinue, nil
	})
	res.Text = plainText(doc, source)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, source, doc); err != nil {
		return Result{}, err
	}
	res.HTML = policy.Sanitize(buf.String())
	return res, nil
}

func addFileRef(dest string, seen map[string]bool, out *[]string) {
	m := fileRefRe.FindStringSubmatch(dest)
	if m == nil || seen[m[1]] {
		return
	}
	seen[m[1]] = true
	*out = append(*out, m[1])
}

// inlineText collects the visible text of a node's inline descendants.
func inlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := c.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *mathInline:
			b.Write(t.literal)
		case *wikilink:
			b.Write(t.title)
		}
		return ast.WalkContinue, nil
	})
	return b.String()
}

// plainText flattens the document into searchable text, one block per line.
func plainText(doc ast.Node, source []byte) string {
	var b strings.Builder
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if n.Type() == ast.TypeBlock && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteByte('\n')
			}
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(t.Value)
		case *mathInline:
			b.Write(t.literal)
		case *wikilink:
			b.Write(t.title)
		case *ast.FencedCodeBlock, *ast.CodeBlock, *mathBlock:
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(source))
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML, *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// Summary returns the first n runes of plain text, with whitespace collapsed.
func Summary(plain string, n int) string {
	fields := strings.FieldsFunc(plain, unicode.IsSpace)
	s := []rune(strings.Join(fields, " "))
	if len(s) <= n {
		return string(s)
	}
	return string(s[:n]) + "…"
}

// ids generates heading IDs that keep Chinese characters readable.
type ids struct{ used map[string]int }

func newIDs() *ids { return &ids{used: map[string]int{}} }

func (s *ids) Generate(value []byte, _ ast.NodeKind) []byte {
	return []byte(uniqueSlug(slugify(string(value)), s.used))
}

// slugify folds a heading's text into an id-safe slug, keeping letters and
// digits (Chinese included) and turning separators into dashes.
func slugify(value string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	base := strings.TrimRight(b.String(), "-")
	if base == "" {
		base = "heading"
	}
	return base
}

// uniqueSlug deduplicates a slug within one document.
func uniqueSlug(base string, used map[string]int) string {
	id := base
	if n, ok := used[base]; ok {
		for {
			n++
			id = base + "-" + strconv.Itoa(n)
			if _, taken := used[id]; !taken {
				break
			}
		}
		used[base] = n
	}
	used[id] = 0
	return id
}

func (s *ids) Put(value []byte) { s.used[string(value)] = 0 }
