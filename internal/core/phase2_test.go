package core

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xiaotuantuan5/blog/internal/db"
)

func TestWikilinksAndBacklinks(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	target := mustCreate(t, a, NoteInput{Title: "Go 并发", ContentMD: "## Channel\n\n内容"})

	source := mustCreate(t, a, NoteInput{
		Title:     "学习笔记",
		ContentMD: "参见 [[Go 并发]] 和 [[不存在的笔记]]。",
		Version:   1,
	})
	if !strings.Contains(source.ContentHTML, `data-note="`+strconv.FormatInt(target.ID, 10)+`"`) {
		t.Fatalf("html = %s", source.ContentHTML)
	}
	if strings.Contains(source.ContentHTML, "不存在的笔记</a>") {
		t.Error("missing target should not be an anchor")
	}
	if !strings.Contains(source.ContentHTML, `wikilink-missing`) {
		t.Fatalf("html = %s", source.ContentHTML)
	}

	// Backlinks appear on the target.
	got, err := a.GetNote(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Backlinks) != 1 || got.Backlinks[0].Title != "学习笔记" {
		t.Fatalf("backlinks = %+v", got.Backlinks)
	}

	// A private note is not publicly readable.
	if _, err := a.GetPublicNote(ctx, source.Slug); err != ErrNotFound {
		t.Fatalf("private note should 404, err = %v", err)
	}
	source2, err := a.UpdateNote(ctx, source.ID, NoteInput{
		Title: "学习笔记", ContentMD: "参见 [[Go 并发]]。", Visibility: VisibilityPublic, Version: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	pn, err := a.GetPublicNote(ctx, source2.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pn.ContentHTML, "data-note") || !strings.Contains(pn.ContentHTML, "Go 并发") {
		t.Fatalf("public html should hide private links: %s", pn.ContentHTML)
	}
	// Making the target public turns it into a /n/ link.
	target2, err := a.UpdateNote(ctx, target.ID, NoteInput{
		Title: "Go 并发", ContentMD: "## Channel", Visibility: VisibilityPublic, Version: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	pn, err = a.GetPublicNote(ctx, source2.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pn.ContentHTML, `href="/n/`+target2.Slug+`"`) {
		t.Fatalf("public html = %s", pn.ContentHTML)
	}

	// Preview resolves too.
	res, err := a.RenderPreview(ctx, "[[Go 并发]]")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `data-note="`+strconv.FormatInt(target.ID, 10)+`"`) {
		t.Fatalf("preview html = %s", res.HTML)
	}
}

func TestTrashLifecycle(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	n := mustCreate(t, a, NoteInput{Title: "待删除", ContentMD: "内容"})

	if err := a.DeleteNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	// Lists and search exclude trashed notes.
	page, err := a.ListNotes(ctx, NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("total = %d", page.Total)
	}
	// Trash lists it.
	trash, err := a.ListTrash(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(trash) != 1 || trash[0].ID != n.ID || trash[0].DeletedAt.IsZero() {
		t.Fatalf("trash = %+v", trash)
	}
	// Slug still taken while in trash.
	if err := checkSlugFree(ctx, a.DB, n.Slug, 0); err == nil {
		t.Error("slug should be taken by trashed note")
	}

	if err := a.RestoreNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	page, _ = a.ListNotes(ctx, NoteFilter{})
	if page.Total != 1 {
		t.Fatalf("total after restore = %d", page.Total)
	}

	// Purge removes for good; second purge 404s.
	if err := a.DeleteNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.PurgeNote(ctx, n.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := a.PurgeNote(ctx, n.ID, false); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}

	// Retention cleanup: a deleted note past the window is purged.
	old := mustCreate(t, a, NoteInput{Title: "过期", ContentMD: "x"})
	if err := a.DeleteNote(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-31 * 24 * time.Hour).Format(db.TimeLayout)
	if _, err := a.DB.ExecContext(ctx, `UPDATE notes SET deleted_at = ? WHERE id = ?`, past, old.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.PurgeExpiredTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetNote(ctx, old.ID); err != ErrNotFound {
		t.Fatalf("expired note should be gone, err = %v", err)
	}
}

func TestRevisions(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	n := mustCreate(t, a, NoteInput{Title: "标题", ContentMD: "v1"})

	// Autosave-style updates within 10 minutes only snapshot once.
	for i := 0; i < 3; i++ {
		if _, err := a.UpdateNote(ctx, n.ID, NoteInput{
			Title: "标题", ContentMD: "v2", Version: 1 + i,
		}); err != nil {
			t.Fatal(err)
		}
	}
	revs, err := a.ListRevisions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0].ContentMD != "v1" {
		t.Fatalf("revisions = %+v", revs)
	}

	// Manual saves always snapshot.
	cur, err := a.GetNote(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateNote(ctx, n.ID, NoteInput{
		Title: "标题", ContentMD: "v3", Version: cur.Version, Manual: true,
	}); err != nil {
		t.Fatal(err)
	}
	revs, _ = a.ListRevisions(ctx, n.ID)
	if len(revs) != 2 || revs[0].ContentMD != "v2" {
		t.Fatalf("revisions = %+v", revs)
	}

	// Restore the first snapshot: content comes back as a new version.
	restored, err := a.RestoreRevision(ctx, n.ID, revs[len(revs)-1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ContentMD != "v1" || restored.Title != "标题" || restored.Version != cur.Version+2 {
		t.Fatalf("restored = %+v", restored)
	}
	// Restoring snapshots the pre-restore state too.
	revs, _ = a.ListRevisions(ctx, n.ID)
	if len(revs) != 3 || revs[0].ContentMD != "v3" {
		t.Fatalf("revisions after restore = %+v", revs)
	}

	// Restoring another note's revision fails.
	other := mustCreate(t, a, NoteInput{Title: "其他", ContentMD: "x"})
	if _, err := a.RestoreRevision(ctx, other.ID, revs[0].ID); err == nil {
		t.Error("should not restore cross-note revision")
	}
}

func TestDailyNote(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)

	n, err := a.GetOrCreateDaily(ctx, "2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "2026-11-20" || !strings.Contains(n.ContentHTML, "<h1") {
		t.Fatalf("daily = %+v", n)
	}
	// Same date returns the same note.
	n2, err := a.GetOrCreateDaily(ctx, "2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	if n2.ID != n.ID {
		t.Fatalf("daily ids differ: %d vs %d", n.ID, n2.ID)
	}
	// The 日记 notebook exists with one note.
	nbs, err := a.ListNotebooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, nb := range nbs {
		if nb.Name == "日记" && nb.NoteCount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("notebooks = %+v", nbs)
	}
	if _, err := a.GetOrCreateDaily(ctx, "not-a-date"); err == nil {
		t.Error("invalid date should fail")
	}
}

func TestHeatmap(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	n1 := mustCreate(t, a, NoteInput{Title: "a", ContentMD: "x"})
	n2 := mustCreate(t, a, NoteInput{Title: "b", ContentMD: "x"})
	if _, err := a.UpdateNote(ctx, n1.ID, NoteInput{Title: "a", ContentMD: "y", Version: 1}); err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(cst).Format("2006-01-02")
	days, err := a.Heatmap(ctx, today, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].Count != 2 || days[0].Day != today {
		t.Fatalf("days = %+v", days)
	}
	_ = n2
}

func TestAnnotations(t *testing.T) {
	ctx := context.Background()
	a := newTestApp(t)
	n := mustCreate(t, a, NoteInput{Title: "文章", ContentMD: "第一段。第二段。"})

	in := AnnotationInput{
		Exact: "第一段", Prefix: "前文", Suffix: "。第二段",
		StartOffset: 0, EndOffset: 3, Color: "yellow", CommentMD: "**重点**",
	}
	an, err := a.CreateAnnotation(ctx, n.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if an.CommentHTML == "" || an.Orphaned {
		t.Fatalf("annotation = %+v", an)
	}
	// Context is capped at 32 runes.
	if len(an.Prefix) != len("前文") || len([]rune(an.Suffix)) != 4 {
		t.Fatalf("ctx not trimmed: %q %q", an.Prefix, an.Suffix)
	}

	// Per-note list, document order.
	if _, err := a.CreateAnnotation(ctx, n.ID, AnnotationInput{
		Exact: "第二段", StartOffset: 4, EndOffset: 7, Color: "red",
	}); err != nil {
		t.Fatal(err)
	}
	list, err := a.ListNoteAnnotations(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Exact != "第一段" {
		t.Fatalf("list = %+v", list)
	}

	// Update: recolor and clear the orphaned flag.
	in.Color = "blue"
	in.Orphaned = true
	up, err := a.UpdateAnnotation(ctx, an.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if up.Color != "blue" || !up.Orphaned {
		t.Fatalf("updated = %+v", up)
	}

	// Overview: filter by color and by text.
	page, err := a.ListAnnotations(ctx, AnnotationFilter{Color: "red"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].NoteTitle != "文章" {
		t.Fatalf("page = %+v", page)
	}
	page, err = a.ListAnnotations(ctx, AnnotationFilter{Q: "第一段"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("search page = %+v", page)
	}
	// Trigram FTS on longer queries too.
	if _, err := a.CreateAnnotation(ctx, n.ID, AnnotationInput{
		Exact: "这是较长的一段高亮文本", Color: "green", CommentMD: "**间隔**重复",
	}); err != nil {
		t.Fatal(err)
	}
	page, err = a.ListAnnotations(ctx, AnnotationFilter{Q: "间隔重复"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("fts page = %+v", page)
	}

	// Deleting the note cascades.
	if err := a.PurgeNote(ctx, n.ID, true); err != nil {
		t.Fatal(err)
	}
	list, err = a.ListNoteAnnotations(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("list after purge = %+v", list)
	}
	// Annotations on trashed notes are hidden from the overview.
	n3 := mustCreate(t, a, NoteInput{Title: "另一篇", ContentMD: "x"})
	if _, err := a.CreateAnnotation(ctx, n3.ID, AnnotationInput{Exact: "任意", Color: "yellow"}); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteNote(ctx, n3.ID); err != nil {
		t.Fatal(err)
	}
	page, _ = a.ListAnnotations(ctx, AnnotationFilter{})
	if page.Total != 0 {
		t.Fatalf("trashed note annotations should be hidden: %+v", page)
	}
}
