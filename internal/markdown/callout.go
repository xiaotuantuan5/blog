package markdown

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Callouts use GitHub's syntax:
//
//	> [!TIP] 可选标题
//	> 内容
var calloutRe = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\][ \t]*(.*?)\s*$`)

var calloutTitles = map[string]string{
	"note":      "注意",
	"tip":       "提示",
	"important": "重要",
	"warning":   "警告",
	"caution":   "危险",
}

var kindCalloutTitle = ast.NewNodeKind("CalloutTitle")

type calloutTitle struct {
	ast.BaseBlock
	title string
}

func (n *calloutTitle) Kind() ast.NodeKind            { return kindCalloutTitle }
func (n *calloutTitle) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type calloutTransformer struct{}

func (calloutTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var quotes []*ast.Blockquote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if bq, ok := n.(*ast.Blockquote); ok && entering {
			quotes = append(quotes, bq)
		}
		return ast.WalkContinue, nil
	})
	for _, bq := range quotes {
		para, ok := bq.FirstChild().(*ast.Paragraph)
		if !ok || para.Lines().Len() == 0 {
			continue
		}
		first := para.Lines().At(0)
		m := calloutRe.FindSubmatch(first.Value(source))
		if m == nil {
			continue
		}
		kind := strings.ToLower(string(m[1]))
		title := string(m[2])
		if title == "" {
			title = calloutTitles[kind]
		}

		// Drop the inline nodes that belong to the marker line.
		for c := para.FirstChild(); c != nil; {
			next := c.NextSibling()
			if start := firstTextStart(c); start >= 0 && start < first.Stop {
				para.RemoveChild(para, c)
			}
			c = next
		}
		if para.ChildCount() == 0 {
			bq.RemoveChild(bq, para)
		}
		bq.SetAttributeString("class", []byte("callout callout-"+kind))
		if bq.FirstChild() == nil {
			bq.AppendChild(bq, &calloutTitle{title: title})
		} else {
			bq.InsertBefore(bq, bq.FirstChild(), &calloutTitle{title: title})
		}
	}
}

func firstTextStart(n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if s := firstTextStart(c); s >= 0 {
			return s
		}
	}
	return -1
}

func renderCalloutTitle(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<p class="callout-title">`)
		_, _ = w.Write(util.EscapeHTML([]byte(n.(*calloutTitle).title)))
		_, _ = w.WriteString("</p>\n")
	}
	return ast.WalkSkipChildren, nil
}
