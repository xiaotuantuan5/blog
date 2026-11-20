package markdown

import (
	"bytes"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Math support: `$...$` inline and `$$ ... $$` blocks. The TeX source is
// emitted escaped inside marker elements and typeset by KaTeX in the browser.

var (
	kindMathInline = ast.NewNodeKind("MathInline")
	kindMathBlock  = ast.NewNodeKind("MathBlock")
)

type mathInline struct {
	ast.BaseInline
	literal []byte
}

func (n *mathInline) Kind() ast.NodeKind            { return kindMathInline }
func (n *mathInline) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }
func (n *mathInline) IsRaw() bool                   { return true }

type mathBlock struct{ ast.BaseBlock }

func (n *mathBlock) Kind() ast.NodeKind            { return kindMathBlock }
func (n *mathBlock) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }
func (n *mathBlock) IsRaw() bool                   { return true }

type mathExtension struct{}

func (mathExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithInlineParsers(util.Prioritized(mathInlineParser{}, 150)),
		parser.WithBlockParsers(util.Prioritized(mathBlockParser{}, 150)),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{}, 150)))
}

type mathInlineParser struct{}

func (mathInlineParser) Trigger() []byte { return []byte{'$'} }

func (mathInlineParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	// Opening `$` must be followed by a non-space character; `$$` is not inline math.
	if len(line) < 3 || line[1] == '$' || isSpace(line[1]) {
		return nil
	}
	for i := 1; i < len(line); i++ {
		switch line[i] {
		case '\\':
			i++
		case '$':
			// Closing `$` must follow a non-space and must not precede a digit,
			// so prices like "$5 and $10" stay plain text.
			if isSpace(line[i-1]) || (i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9') {
				continue
			}
			lit := make([]byte, i-1)
			copy(lit, line[1:i])
			block.Advance(i + 1)
			return &mathInline{literal: lit}
		case '\n':
			return nil
		}
	}
	return nil
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

type mathBlockParser struct{}

var mathBlockDoneKey = parser.NewContextKey()

func (mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (mathBlockParser) Open(_ ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	pos := pc.BlockOffset()
	if pos < 0 || pos+1 >= len(line) || line[pos] != '$' || line[pos+1] != '$' {
		return nil, parser.NoChildren
	}
	base := segment.Start - segment.Padding
	start := pos + 2
	end := len(line) - util.TrimRightSpaceLength(line)
	node := &mathBlock{}
	pc.Set(mathBlockDoneKey, false)
	if end-start >= 2 && line[end-1] == '$' && line[end-2] == '$' {
		// Single line: $$ ... $$
		node.Lines().Append(text.NewSegment(base+start, base+end-2))
		pc.Set(mathBlockDoneKey, true)
	} else if start < end {
		node.Lines().Append(text.NewSegment(base+start, base+end))
	}
	return node, parser.NoChildren
}

func (mathBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	if done, _ := pc.Get(mathBlockDoneKey).(bool); done {
		return parser.Close
	}
	line, segment := reader.PeekLine()
	if line == nil {
		return parser.Close
	}
	trimmed := bytes.TrimSpace(line)
	if bytes.HasSuffix(trimmed, []byte("$$")) {
		end := len(line) - util.TrimRightSpaceLength(line) - 2
		if content := bytes.TrimSpace(line[:end]); len(content) > 0 {
			node.Lines().Append(text.NewSegment(segment.Start, segment.Start+end))
		}
		reader.AdvanceToEOL()
		return parser.Close
	}
	seg := segment
	seg.ForceNewline = true
	node.Lines().Append(seg)
	reader.AdvanceToEOL()
	return parser.Continue | parser.NoChildren
}

func (mathBlockParser) Close(_ ast.Node, _ text.Reader, pc parser.Context) {
	pc.Set(mathBlockDoneKey, nil)
}

func (mathBlockParser) CanInterruptParagraph() bool { return true }
func (mathBlockParser) CanAcceptIndentedLine() bool { return false }

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMathInline, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<span class="math math-inline">`)
			_, _ = w.Write(util.EscapeHTML(n.(*mathInline).literal))
			_, _ = w.WriteString(`</span>`)
		}
		return ast.WalkSkipChildren, nil
	})
	reg.Register(kindMathBlock, func(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(`<div class="math math-display">`)
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				_, _ = w.Write(util.EscapeHTML(seg.Value(source)))
			}
			_, _ = w.WriteString("</div>\n")
		}
		return ast.WalkSkipChildren, nil
	})
}
