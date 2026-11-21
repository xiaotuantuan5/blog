package markdown

import (
	"strings"
	"testing"
)

func TestRenderHTMLFragment(t *testing.T) {
	hashA := strings.Repeat("a", 64)
	hashB := strings.Repeat("b", 64)
	src := `<h2 id="sec-1">第一章</h2><p>段落 <b>加粗</b></p>
<h2>第二章</h2><h3>子节</h3>
<div class="card" style="color: red">卡片 <span>内容</span></div>
<img src="/files/` + hashA + `" alt="图">
<a href="/files/` + hashB + `#page=1">附件</a>`
	res, err := RenderHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("html: %s", res.HTML)
	t.Logf("toc: %+v", res.TOC)
	if len(res.TOC) != 3 {
		t.Fatalf("toc = %+v", res.TOC)
	}
	if res.TOC[0] != (Heading{Level: 2, ID: "sec-1", Text: "第一章"}) ||
		res.TOC[1] != (Heading{Level: 2, ID: "第二章", Text: "第二章"}) ||
		res.TOC[2] != (Heading{Level: 3, ID: "子节", Text: "子节"}) {
		t.Fatalf("toc = %+v", res.TOC)
	}
	if !strings.Contains(res.Text, "第一章") || !strings.Contains(res.Text, "卡片 内容") {
		t.Fatalf("text = %q", res.Text)
	}
	if len(res.Files) != 2 || res.Files[0] != hashA || res.Files[1] != hashB {
		t.Fatalf("files = %v", res.Files)
	}
	for _, want := range []string{`id="第二章"`, `class="card"`, `style="color: red"`, `src="/files/` + hashA + `"`} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("html missing %q: %s", want, res.HTML)
		}
	}
}

func TestRenderHTMLSanitizes(t *testing.T) {
	src := `<script>alert(1)</script><p onclick="evil()">hi</p>
<img src="x" onerror="alert(1)"><a href="javascript:evil()">x</a><iframe src="https://evil"></iframe>`
	res, err := RenderHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("html: %s", res.HTML)
	low := strings.ToLower(res.HTML)
	for _, bad := range []string{"alert", "onclick", "onerror", "javascript:", "<script", "<iframe"} {
		if strings.Contains(low, bad) {
			t.Errorf("html leaked %q: %s", bad, res.HTML)
		}
	}
	if !strings.Contains(res.HTML, "hi") {
		t.Errorf("text content dropped: %s", res.HTML)
	}
	if strings.Contains(res.Text, "alert") {
		t.Errorf("text includes script content: %q", res.Text)
	}
}

func TestRenderHTMLFullDocument(t *testing.T) {
	src := `<!DOCTYPE html><html><head><title>页面标题</title><style>.grid > div { color: red }</style></head>
<body><h2>章</h2><p>正文文本</p></body></html>`
	res, err := RenderHTML(src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("html: %s", res.HTML)
	if strings.Contains(res.Text, "页面标题") {
		t.Errorf("title text leaked into search text: %q", res.Text)
	}
	if !strings.Contains(res.Text, "正文文本") {
		t.Errorf("text = %q", res.Text)
	}
	low := strings.ToLower(res.HTML)
	for _, wrapper := range []string{"<html", "<head", "<body", "<!doctype", "<title"} {
		if strings.Contains(low, wrapper) {
			t.Errorf("document wrapper survived: %s", res.HTML)
		}
	}
	if !strings.Contains(res.HTML, `<style>`) || !strings.Contains(res.HTML, ".grid > div") {
		t.Errorf("style block mangled: %s", res.HTML)
	}
}
