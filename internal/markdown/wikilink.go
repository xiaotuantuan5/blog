package markdown

import (
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Wikilink support: `[[笔记标题]]` links to another note by title. The
// renderer emits a placeholder anchor with the title in a data attribute;
// core resolves it against the notes table after sanitization.

var kindWikilink = ast.NewNodeKind("Wikilink")

const maxWikilinkTitle = 200

type wikilink struct {
	ast.BaseInline
	title []byte
}

func (n *wikilink) Kind() ast.NodeKind            { return kindWikilink }
func (n *wikilink) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }
func (n *wikilink) IsRaw() bool                   { return true }

type wikilinkExtension struct{}

func (wikilinkExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(wikilinkParser{}, 50)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(wikilinkRenderer{}, 50)))
}

// wikilinkParser must run before the standard link parser (priority 100)
// because both trigger on '['.
type wikilinkParser struct{}

func (wikilinkParser) Trigger() []byte { return []byte{'['} }

func (wikilinkParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 4 || line[1] != '[' {
		return nil
	}
	// Find the closing "]]"; a single ']' inside the title is allowed.
	for i := 2; i+1 < len(line); i++ {
		if line[i] == ']' && line[i+1] == ']' {
			title := trimSpace(line[2:i])
			if len(title) == 0 || utf8.RuneCount(title) > maxWikilinkTitle {
				return nil
			}
			block.Advance(i + 2)
			return &wikilink{title: title}
		}
	}
	return nil
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && isSpace(b[start]) {
		start++
	}
	for end > start && isSpace(b[end-1]) {
		end--
	}
	return b[start:end]
}

type wikilinkRenderer struct{}

func (wikilinkRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindWikilink, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			title := n.(*wikilink).title
			_, _ = w.WriteString(`<a class="wikilink" data-wikilink="`)
			_, _ = w.Write(util.EscapeHTML(title))
			_, _ = w.WriteString(`">`)
			_, _ = w.Write(util.EscapeHTML(title))
			_, _ = w.WriteString(`</a>`)
		}
		return ast.WalkSkipChildren, nil
	})
}
