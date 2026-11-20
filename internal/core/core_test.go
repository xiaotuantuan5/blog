package core

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaotuantuan5/blog/internal/db"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	conn, err := db.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn, dir, 1<<20)
}

func mustCreate(t *testing.T, a *App, in NoteInput) *Note {
	t.Helper()
	n, err := a.CreateNote(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestNoteLifecycle(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	nb, err := a.CreateNotebook(ctx, NotebookInput{Name: "Go"})
	if err != nil {
		t.Fatal(err)
	}
	n := mustCreate(t, a, NoteInput{
		Title: "Goroutine 笔记", NotebookID: &nb.ID, ContentMD: "## 调度\n\n内容", Tags: []string{"Go", "go", " 并发 "},
	})
	if n.Visibility != VisibilityPrivate || len(n.Slug) != 8 || n.Version != 1 {
		t.Fatalf("unexpected note %+v", n.NoteSummary)
	}
	if strings.Join(n.Tags, ",") != "Go,并发" {
		t.Errorf("tags = %v", n.Tags)
	}
	if len(n.TOC) != 1 || n.TOC[0].ID != "调度" {
		t.Errorf("toc = %+v", n.TOC)
	}
	if !n.PublishedAt.IsZero() {
		t.Error("private note should not have publishedAt")
	}

	up := NoteInput{Title: "Goroutine 笔记", Slug: "goroutine", NotebookID: &nb.ID, ContentMD: "新内容",
		Visibility: VisibilityPublic, Tags: []string{"Go"}, Version: 1}
	n2, err := a.UpdateNote(ctx, n.ID, up)
	if err != nil {
		t.Fatal(err)
	}
	if n2.Version != 2 || n2.Slug != "goroutine" || n2.PublishedAt.IsZero() {
		t.Errorf("after update: %+v", n2.NoteSummary)
	}
	// The "并发" tag is no longer used and should be removed.
	tags, _ := a.ListTags(ctx, false)
	if len(tags) != 1 || tags[0].Name != "Go" {
		t.Errorf("tags = %+v", tags)
	}

	// Stale version.
	_, err = a.UpdateNote(ctx, n.ID, up)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("stale update err = %v", err)
	}

	// Moving between notebooks doesn't bump the version.
	if err := a.SetNoteFlags(ctx, n.ID, NoteFlags{Notebook: OptionalID{Set: true}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := a.GetNote(ctx, n.ID); got.NotebookID != nil || got.Version != 2 {
		t.Errorf("after move to root: notebook=%v version=%d", got.NotebookID, got.Version)
	}
	if err := a.SetNoteFlags(ctx, n.ID, NoteFlags{Notebook: OptionalID{Set: true, Value: &nb.ID}}); err != nil {
		t.Fatal(err)
	}

	// Notebook with notes can't be deleted.
	if err := a.DeleteNotebook(ctx, nb.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("delete non-empty notebook err = %v", err)
	}
	if err := a.DeleteNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteNotebook(ctx, nb.ID); err != nil {
		t.Errorf("delete empty notebook: %v", err)
	}
}

func TestNotebookCycle(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	p, _ := a.CreateNotebook(ctx, NotebookInput{Name: "父"})
	c, _ := a.CreateNotebook(ctx, NotebookInput{Name: "子", ParentID: &p.ID})
	_, err := a.UpdateNotebook(ctx, p.ID, NotebookInput{Name: "父", ParentID: &c.ID})
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("cycle err = %v", err)
	}
}

func TestSearch(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	nb, _ := a.CreateNotebook(ctx, NotebookInput{Name: "后端"})
	child, _ := a.CreateNotebook(ctx, NotebookInput{Name: "Go", ParentID: &nb.ID})
	mustCreate(t, a, NoteInput{Title: "并发编程", NotebookID: &child.ID, ContentMD: "goroutine 和 channel 是 Go 并发的基础。"})
	mustCreate(t, a, NoteInput{Title: "数据库索引", ContentMD: "B+ 树索引原理，<b>覆盖索引</b>", Visibility: VisibilityPublic})
	mustCreate(t, a, NoteInput{Title: "随笔", ContentMD: "今天学习了 channel 的关闭"})

	cases := []struct {
		q    string
		want int
	}{
		{"channel", 2},    // trigram
		{"并发", 1},         // 2 chars: LIKE fallback
		{"B+ 树索引原理", 1},   // mixed
		{"覆盖索引", 1},       // trigram, CJK
		{"channel 关闭", 1}, // AND of both
		{"不存在的内容", 0},
		{`"quote`, 0}, // FTS syntax characters are escaped
	}
	for _, c := range cases {
		page, err := a.ListNotes(ctx, NoteFilter{Q: c.q})
		if err != nil {
			t.Fatalf("q=%q: %v", c.q, err)
		}
		if page.Total != c.want || len(page.Items) != c.want {
			t.Errorf("q=%q: total=%d items=%d, want %d", c.q, page.Total, len(page.Items), c.want)
		}
	}

	page, _ := a.ListNotes(ctx, NoteFilter{Q: "覆盖索引"})
	if s := page.Items[0].Snippet; !strings.Contains(s, "<mark>覆盖索引</mark>") || strings.Contains(s, "<b>") {
		t.Errorf("snippet = %q", s)
	}

	// Notebook filter includes sub-notebooks.
	page, _ = a.ListNotes(ctx, NoteFilter{NotebookID: &nb.ID})
	if page.Total != 1 {
		t.Errorf("notebook filter total = %d", page.Total)
	}
	zero := int64(0)
	page, _ = a.ListNotes(ctx, NoteFilter{NotebookID: &zero})
	if page.Total != 2 {
		t.Errorf("no-notebook filter total = %d", page.Total)
	}

	// Public listing hides private notes, including from search.
	page, _ = a.ListNotes(ctx, NoteFilter{PublicOnly: true, Q: "channel"})
	if page.Total != 0 {
		t.Errorf("public search leaked private notes: %d", page.Total)
	}
	page, _ = a.ListNotes(ctx, NoteFilter{PublicOnly: true})
	if page.Total != 1 || page.Items[0].Title != "数据库索引" {
		t.Errorf("public list = %+v", page.Items)
	}
}

func TestPublicNoteAccess(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	priv := mustCreate(t, a, NoteInput{Title: "私有", ContentMD: "秘密"})
	p1 := mustCreate(t, a, NoteInput{Title: "公开1", ContentMD: "a", Visibility: VisibilityPublic})
	p2 := mustCreate(t, a, NoteInput{Title: "公开2", ContentMD: "b", Visibility: VisibilityPublic})

	if _, err := a.GetPublicNote(ctx, priv.Slug); !errors.Is(err, ErrNotFound) {
		t.Errorf("private note via public API: %v", err)
	}
	pn, err := a.GetPublicNote(ctx, p2.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(pn); strings.Contains(string(b), "contentMd") || strings.Contains(string(b), "manualSummary") {
		t.Errorf("public note JSON exposes source: %s", b)
	}
	if b, _ := json.Marshal(priv); !strings.Contains(string(b), `"contentMd":"秘密"`) {
		t.Errorf("admin note JSON lacks contentMd: %s", b)
	}
	if pn.Prev == nil || pn.Prev.Slug != p1.Slug || pn.Next != nil {
		t.Errorf("prev/next = %+v %+v", pn.Prev, pn.Next)
	}
}

func TestAuth(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	if err := a.CreateAdmin(ctx, "admin", "short"); err == nil {
		t.Error("short password accepted")
	}
	if err := a.CreateAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateAdmin(ctx, "other", "password123"); !errors.Is(err, ErrConflict) {
		t.Errorf("second admin: %v", err)
	}
	if _, _, err := a.Login(ctx, "admin", "wrong-password"); !errors.Is(err, ErrBadCredentials) {
		t.Errorf("bad login: %v", err)
	}
	token, s, err := a.Login(ctx, "admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.SessionByToken(ctx, token)
	if err != nil || got.CSRFToken != s.CSRFToken || !got.CheckCSRF(s.CSRFToken) || got.CheckCSRF("x") {
		t.Fatalf("session lookup: %+v %v", got, err)
	}
	if err := a.ResetPassword(ctx, "admin", "newpassword1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SessionByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Error("session survived password reset")
	}
}

func TestUpload(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	png := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 100)
	f1, err := a.SaveUpload(ctx, strings.NewReader(png), "../../图片.png")
	if err != nil {
		t.Fatal(err)
	}
	if f1.Mime != "image/png" || f1.Filename != "图片.png" || !f1.IsImage {
		t.Errorf("file = %+v", f1)
	}
	f2, err := a.SaveUpload(ctx, strings.NewReader(png), "copy.png")
	if err != nil || f2.ID != f1.ID {
		t.Errorf("dedupe failed: %+v %v", f2, err)
	}
	if _, err := a.SaveUpload(ctx, strings.NewReader("<svg xmlns='http://www.w3.org/2000/svg'><script/></svg>"), "x.svg"); err == nil {
		t.Error("svg accepted")
	}
	big := "\x89PNG\r\n\x1a\n" + strings.Repeat("\x00", 2<<20)
	if _, err := a.SaveUpload(ctx, strings.NewReader(big), "big.png"); err == nil {
		t.Error("oversize accepted")
	}

	n := mustCreate(t, a, NoteInput{Title: "图", ContentMD: "![](/files/" + f1.Hash + ")"})
	if pub, _ := a.FileIsPublic(ctx, f1.ID); pub {
		t.Error("file of private note is public")
	}
	_, err = a.UpdateNote(ctx, n.ID, NoteInput{Title: "图", ContentMD: n.ContentMD, Visibility: VisibilityPublic, Version: n.Version})
	if err != nil {
		t.Fatal(err)
	}
	if pub, _ := a.FileIsPublic(ctx, f1.ID); !pub {
		t.Error("file of public note is not public")
	}
}
