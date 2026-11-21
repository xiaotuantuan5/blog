package markdown

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

var policy = newPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// Classes drive highlighting, callouts, math and mermaid.
	p.AllowStyling()
	p.AllowDataAttributes()
	// Heading IDs may contain Chinese characters.
	p.AllowAttrs("id").Matching(regexp.MustCompile(`^[\p{L}\p{N}_:.\-]+$`)).Globally()
	// GFM task list checkboxes.
	p.AllowElements("input")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^lazy$`)).OnElements("img")
	// Personal content: no rel=nofollow on our own links, but open
	// external links in a new tab.
	p.RequireNoFollowOnLinks(false)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}

// SanitizeHTML cleans HTML with the same policy used for rendered notes.
func SanitizeHTML(s string) string { return policy.Sanitize(s) }

var (
	styleValueRe   = regexp.MustCompile(`(?i)^[^<>]*$`)
	alignRe        = regexp.MustCompile(`(?i)^(left|right|center|justify)$`)
	valignRe       = regexp.MustCompile(`(?i)^(top|middle|bottom|baseline)$`)
	dimensionRe    = regexp.MustCompile(`(?i)^[0-9]+(%|px|em|rem|pt|vh|vw)?$`)
	numberColorRe  = regexp.MustCompile(`(?i)^[0-9]+$|^#[0-9a-f]{3,8}$`)
	dirRe          = regexp.MustCompile(`(?i)^(ltr|rtl|auto)$`)
	langRe         = regexp.MustCompile(`^[a-zA-Z]{2,3}(-[a-zA-Z0-9]+)*$`)
)

var htmlPolicy = newHTMLPolicy()

// newHTMLPolicy widens the Markdown policy for notes authored as HTML: they
// come from the user's own generators and carry layout markup, inline styles
// and document-level style blocks. Scripting stays forbidden either way.
func newHTMLPolicy() *bluemonday.Policy {
	p := newPolicy()
	p.AllowElements("div", "span", "section", "article", "header", "footer",
		"main", "aside", "nav", "figure", "figcaption", "picture", "details", "summary")
	// Document-level style blocks shipped by generated documents. AllowUnsafe
	// only lifts bluemonday's extra gate on style/script elements; script is
	// still not in the policy, so it keeps being stripped with its content.
	// Without the flag the CSS text would be HTML-escaped and break selectors.
	p.AllowUnsafe(true)
	p.AllowElements("style")
	// Presentational markup from table-era generators.
	p.AllowAttrs("style").Matching(styleValueRe).Globally()
	p.AllowAttrs("align").Matching(alignRe).Globally()
	p.AllowAttrs("valign").Matching(valignRe).Globally()
	p.AllowAttrs("width", "height").Matching(dimensionRe).Globally()
	p.AllowAttrs("bgcolor", "border", "cellpadding", "cellspacing").Matching(numberColorRe).Globally()
	p.AllowAttrs("dir").Matching(dirRe).Globally()
	p.AllowAttrs("lang").Matching(langRe).Globally()
	return p
}
