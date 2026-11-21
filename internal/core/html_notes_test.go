package core

import (
	"context"
	"strings"
	"testing"
)

func TestHTMLNoteLifecycle(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)

	n := mustCreate(t, a, NoteInput{
		Title: "HTML 笔记", Format: FormatHTML,
		ContentMD: `<h2>章节</h2><p style="color:red">红色段落</p><script>alert(1)</script>`,
	})
	if n.Format != FormatHTML {
		t.Fatalf("format = %q", n.Format)
	}
	if !strings.Contains(n.ContentHTML, `style="color:red"`) || !strings.Contains(n.ContentHTML, "红色段落") {
		t.Fatalf("html = %s", n.ContentHTML)
	}
	if strings.Contains(n.ContentHTML, "alert") || strings.Contains(n.ContentHTML, "<script") {
		t.Fatalf("script leaked: %s", n.ContentHTML)
	}
	if len(n.TOC) != 1 || n.TOC[0].Text != "章节" {
		t.Fatalf("toc = %+v", n.TOC)
	}
	// Text extracted from the DOM feeds search and summaries.
	if !strings.Contains(n.Summary, "红色段落") {
		t.Fatalf("summary = %q", n.Summary)
	}
	list, err := a.ListNotes(ctx, NoteFilter{PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Items[0].Format != FormatHTML {
		t.Fatalf("list format = %q", list.Items[0].Format)
	}
	// Search finds HTML note content through the extracted text.
	found, err := a.ListNotes(ctx, NoteFilter{Q: "红色段落", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if found.Total != 1 || found.Items[0].ID != n.ID {
		t.Fatalf("search = %+v", found.Items)
	}

	// Wikilinks stay literal in HTML notes even when the target exists.
	mustCreate(t, a, NoteInput{Title: "目标", ContentMD: "内容"})
	h := mustCreate(t, a, NoteInput{Title: "链接笔记", Format: FormatHTML, ContentMD: `<p>看看 [[目标]] 这篇</p>`})
	if strings.Contains(h.ContentHTML, "wikilink") || len(h.Backlinks) != 0 {
		t.Fatalf("wikilink interpreted in html note: %s", h.ContentHTML)
	}

	// Default format is Markdown.
	md := mustCreate(t, a, NoteInput{Title: "普通笔记", ContentMD: "# 标题\n\n正文"})
	if md.Format != FormatMD {
		t.Fatalf("default format = %q", md.Format)
	}

	// Invalid format is rejected.
	if _, err := a.CreateNote(ctx, NoteInput{Title: "坏格式", Format: "pdf", ContentMD: "x"}); err == nil {
		t.Fatal("invalid format accepted")
	}
}

func TestHTMLNotePreview(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	res, err := a.RenderPreview(ctx, FormatHTML, `<h2>预览</h2><div style="display:flex">内容</div>`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `<div style="display:flex">`) || len(res.TOC) != 1 {
		t.Fatalf("preview = %s %+v", res.HTML, res.TOC)
	}
	// Markdown source through the same endpoint is not re-parsed as HTML.
	res, err = a.RenderPreview(ctx, FormatMD, "## 标题\n\n内容")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "<h2") || !strings.Contains(res.HTML, "标题") {
		t.Fatalf("md preview = %s", res.HTML)
	}
	if _, err := a.RenderPreview(ctx, "pdf", "x"); err == nil {
		t.Fatal("invalid format accepted")
	}
}

func TestHTMLNoteFormatSwitchAndRevisions(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)

	// Start as Markdown, then switch the same note to HTML.
	n := mustCreate(t, a, NoteInput{Title: "切换", ContentMD: "## 旧标题\n\n旧内容"})
	updated, err := a.UpdateNote(ctx, n.ID, NoteInput{
		Title: "切换", Format: FormatHTML, Version: n.Version, Manual: true,
		ContentMD: `<h2 id="新标题">新标题</h2><p>新内容</p>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Format != FormatHTML || !strings.Contains(updated.ContentHTML, `id="新标题"`) {
		t.Fatalf("updated = %+v %s", updated.NoteSummary, updated.ContentHTML)
	}

	// The pre-switch revision keeps its own format.
	revs, err := a.ListRevisions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0].Format != FormatMD || !strings.Contains(revs[0].ContentMD, "旧内容") {
		t.Fatalf("revisions = %+v", revs)
	}
	restored, err := a.RestoreRevision(ctx, n.ID, revs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Format != FormatMD || !strings.Contains(restored.ContentHTML, "旧内容") {
		t.Fatalf("restored = %+v %s", restored.NoteSummary, restored.ContentHTML)
	}

	// The pre-restore HTML state was itself snapshotted (newest html revision).
	h2, err := a.UpdateNote(ctx, n.ID, NoteInput{
		Title: "切换", Format: FormatHTML, Version: restored.Version, Manual: true,
		ContentMD: `<p>又改回 html</p>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if h2.Format != FormatHTML {
		t.Fatalf("format = %q", h2.Format)
	}
	revs, err = a.ListRevisions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	var htmlRev int64
	for _, r := range revs { // newest first: the first html revision wins
		if r.Format == FormatHTML && htmlRev == 0 {
			htmlRev = r.ID
		}
	}
	if htmlRev == 0 {
		t.Fatalf("no html revision: %+v", revs)
	}
	back, err := a.RestoreRevision(ctx, n.ID, htmlRev)
	if err != nil {
		t.Fatal(err)
	}
	if back.Format != FormatHTML || !strings.Contains(back.ContentHTML, `id="新标题"`) {
		t.Fatalf("back = %+v %s", back.NoteSummary, back.ContentHTML)
	}
}
