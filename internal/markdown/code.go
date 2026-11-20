package markdown

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// ClassPrefix is prepended to every chroma CSS class. The stylesheet in
// web/src/styles/chroma.css is generated with the same prefix.
const ClassPrefix = "ch-"

// Formatter is shared with the CSS generator so classes always match.
var Formatter = chromahtml.New(chromahtml.WithClasses(true), chromahtml.ClassPrefix(ClassPrefix))

// codeRenderer renders code blocks with server-side syntax highlighting
// (`mermaid` blocks are left for the browser to draw) and callout titles.
type codeRenderer struct{}

func (codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, renderCode)
	reg.Register(ast.KindCodeBlock, renderCode)
	reg.Register(kindCalloutTitle, renderCalloutTitle)
}

func renderCode(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	lang := ""
	if fc, ok := n.(*ast.FencedCodeBlock); ok && fc.Info != nil {
		info := string(fc.Info.Segment.Value(source))
		if f := strings.Fields(info); len(f) > 0 {
			lang = strings.ToLower(f[0])
		}
	}
	var code bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(source))
	}

	if lang == "mermaid" {
		_, _ = w.WriteString(`<pre class="mermaid">`)
		_, _ = w.Write(util.EscapeHTML(code.Bytes()))
		_, _ = w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}

	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	_, _ = w.WriteString(`<div class="code-block" data-lang="`)
	_, _ = w.Write(util.EscapeHTML([]byte(lang)))
	_, _ = w.WriteString(`">`)
	iter, err := lexer.Tokenise(nil, code.String())
	if err == nil {
		err = Formatter.Format(w, styles.Fallback, iter)
	}
	if err != nil {
		_, _ = w.WriteString("<pre><code>")
		_, _ = w.Write(util.EscapeHTML(code.Bytes()))
		_, _ = w.WriteString("</code></pre>")
	}
	_, _ = w.WriteString("</div>\n")
	return ast.WalkSkipChildren, nil
}
