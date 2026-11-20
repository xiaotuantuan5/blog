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
