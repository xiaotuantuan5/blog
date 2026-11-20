// Command chromacss writes the syntax highlighting stylesheet used by the
// frontend. Run via `make chroma-css` after changing styles.
package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"github.com/alecthomas/chroma/v2/styles"

	"github.com/xiaotuantuan5/blog/internal/markdown"
)

// Each generated rule starts with "/* Comment */ .selector"; prefix the
// selector so dark rules only apply under html.dark.
var selectorRe = regexp.MustCompile(`(?m)^(/\*[^*]*\*/ )?\.`)

func main() {
	var light, dark bytes.Buffer
	if err := markdown.Formatter.WriteCSS(&light, styles.Get("github")); err != nil {
		panic(err)
	}
	if err := markdown.Formatter.WriteCSS(&dark, styles.Get("github-dark")); err != nil {
		panic(err)
	}
	darkCSS := selectorRe.ReplaceAllString(dark.String(), "$1.dark .")
	out := fmt.Sprintf("/* 由 tools/chromacss 生成，请勿手动修改 */\n\n%s\n%s", light.String(), darkCSS)
	if err := os.WriteFile(os.Args[1], []byte(out), 0o644); err != nil {
		panic(err)
	}
}
