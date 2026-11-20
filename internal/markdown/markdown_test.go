package markdown

import (
	"strings"
	"testing"
)

func render(t *testing.T, src string) Result {
	t.Helper()
	res, err := Render(src)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("output missing %q\n---\n%s", w, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, bads ...string) {
	t.Helper()
	for _, b := range bads {
		if strings.Contains(got, b) {
			t.Errorf("output unexpectedly contains %q\n---\n%s", b, got)
		}
	}
}

func TestHeadingsAndTOC(t *testing.T) {
	res := render(t, "# 标题\n\n## 并发 模型\n\n### Go Channel\n\n## 并发 模型\n")
	assertContains(t, res.HTML, `id="并发-模型"`, `id="go-channel"`, `id="并发-模型-1"`)
	if len(res.TOC) != 3 {
		t.Fatalf("toc = %+v", res.TOC)
	}
	if res.TOC[1].Text != "Go Channel" || res.TOC[1].Level != 3 {
		t.Errorf("toc[1] = %+v", res.TOC[1])
	}
}

func TestCodeHighlight(t *testing.T) {
	res := render(t, "```go\nfunc main() {}\n```\n")
	assertContains(t, res.HTML, `class="code-block" data-lang="go"`, `class="ch-chroma"`, `class="ch-kd"`)
	if !strings.Contains(res.Text, "func main()") {
		t.Errorf("code missing from plain text: %q", res.Text)
	}
}

func TestMermaidPassThrough(t *testing.T) {
	res := render(t, "```mermaid\ngraph TD; A-->B\n```\n")
	assertContains(t, res.HTML, `<pre class="mermaid">graph TD; A--&gt;B`)
}

func TestMath(t *testing.T) {
	res := render(t, "质能方程 $E = mc^2$，价格 $5 和 $10。\n\n$$\n\\sum_{i=1}^n x_i\n$$\n\n$$a_b$$\n")
	assertContains(t, res.HTML,
		`<span class="math math-inline">E = mc^2</span>`,
		`价格 $5 和 $10`,
		`<div class="math math-display">\sum_{i=1}^n x_i`,
		`<div class="math math-display">a_b</div>`,
	)
	assertNotContains(t, res.HTML, "<em>")
}

func TestCallout(t *testing.T) {
	res := render(t, "> [!TIP]\n> 多用 `context`\n\n> [!WARNING] 小心\n> 死锁\n\n> 普通引用\n")
	assertContains(t, res.HTML,
		`<blockquote class="callout callout-tip">`,
		`<p class="callout-title">提示</p>`,
		`<code>context</code>`,
		`<p class="callout-title">小心</p>`,
		"<blockquote>\n<p>普通引用",
	)
	assertNotContains(t, res.HTML, "[!TIP]", "[!WARNING]")
}

func TestGFMAndFootnote(t *testing.T) {
	res := render(t, "- [x] 完成\n- [ ] 未完成\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n脚注[^1]\n\n[^1]: 说明\n")
	assertContains(t, res.HTML, `type="checkbox"`, `checked`, "<table>", `id="fn:1"`)
}

func TestSanitize(t *testing.T) {
	res := render(t, "<script>alert(1)</script>\n\n<img src=x onerror=alert(1)>\n\n[x](javascript:alert(1))\n\n<details><summary>展开</summary>内容</details>\n")
	assertNotContains(t, res.HTML, "<script", "onerror", "javascript:")
	assertContains(t, res.HTML, "<details>", "<summary>展开</summary>")
}

func TestFileRefs(t *testing.T) {
	h := strings.Repeat("a", 64)
	h2 := strings.Repeat("b", 64)
	res := render(t, "![图](/files/"+h+") [附件](/files/"+h2+") ![重复](/files/"+h+") ![外部](https://x.com/a.png)")
	if len(res.Files) != 2 || res.Files[0] != h || res.Files[1] != h2 {
		t.Errorf("files = %v", res.Files)
	}
}

func TestSummary(t *testing.T) {
	if got := Summary("一二三\n\n四五六七", 5); got != "一二三 四…" {
		t.Errorf("summary = %q", got)
	}
}
