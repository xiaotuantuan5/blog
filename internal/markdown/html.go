package markdown

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// RenderHTML renders a note authored as HTML: the source is parsed as a
// document (or fragment) and sanitized with the wider htmlPolicy, while the
// derived data — plain text, table of contents, attachment references —
// comes from the DOM instead of a Markdown AST. [[wikilinks]] are not
// interpreted in HTML notes.
func RenderHTML(src string) (Result, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return Result{}, err
	}

	var res Result
	seenFiles := map[string]bool{}
	usedIDs := map[string]int{}
	var text strings.Builder

	var collect func(n *html.Node)
	collect = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			if s := strings.TrimSpace(n.Data); s != "" {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(s)
			}
			return
		case html.CommentNode, html.DoctypeNode, html.DocumentNode:
		case html.ElementNode:
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Title, atom.Noscript, atom.Template:
				return // invisible content: no text, no headings, no refs
			case atom.Img:
				addFileRef(attrVal(n, "src"), seenFiles, &res.Files)
			case atom.A:
				addFileRef(attrVal(n, "href"), seenFiles, &res.Files)
			case atom.H2, atom.H3, atom.H4:
				heading := strings.TrimSpace(nodeText(n))
				id := attrVal(n, "id")
				if id == "" {
					id = uniqueSlug(slugify(heading), usedIDs)
					n.Attr = append(n.Attr, html.Attribute{Key: "id", Val: id})
				} else {
					usedIDs[id] = 0 // respect the author's ids; don't hand them out twice
				}
				res.TOC = append(res.TOC, Heading{
					Level: map[atom.Atom]int{atom.H2: 2, atom.H3: 3, atom.H4: 4}[n.DataAtom],
					ID:    id,
					Text:  heading,
				})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(doc)
	res.Text = strings.TrimSpace(text.String())

	// Serialize head <style> blocks plus the body's children; the html/head
	// wrappers themselves are dropped (the sanitizer strips them anyway).
	var body strings.Builder
	root := doc
	if htmlEl := findChild(doc, atom.Html); htmlEl != nil {
		root = htmlEl
	}
	for _, parent := range []atom.Atom{atom.Head, atom.Body} {
		if p := findChild(root, parent); p != nil {
			for c := p.FirstChild; c != nil; c = c.NextSibling {
				if parent == atom.Head && (c.Type != html.ElementNode || c.DataAtom != atom.Style) {
					continue
				}
				if err := html.Render(&body, c); err != nil {
					return Result{}, err
				}
			}
		}
	}
	res.HTML = htmlPolicy.Sanitize(body.String())
	return res, nil
}

// nodeText collects the visible text of a node's descendants.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Script, atom.Style, atom.Template:
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func findChild(n *html.Node, tag atom.Atom) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.DataAtom == tag {
			return c
		}
	}
	return nil
}
