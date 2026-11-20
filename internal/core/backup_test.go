package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiaotuantuan5/blog/internal/db"
)

// newBackupApp opens an app on <dir>/blog.db, matching production layout —
// restore swaps exactly that file.
func newBackupApp(t *testing.T, dir string) *App {
	t.Helper()
	conn, err := db.Open(context.Background(), filepath.Join(dir, "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn, dir, 1<<20)
}

func tgz(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o640, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	a := newBackupApp(t, t.TempDir())

	// A file, a public note and the instance identity.
	upload, err := a.SaveUpload(ctx, strings.NewReader("hello 备份"), "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	note := mustCreate(t, a, NoteInput{Title: "备份测试", ContentMD: "内容 [[闪卡]]", Visibility: VisibilityPublic})
	inst, err := a.InstanceID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	p, err := a.PrepareBackup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Remove()
	m := p.Manifest
	if m.InstanceID != inst || m.Notes != 1 || m.Files != 1 || m.SchemaVersion != db.LatestVersion() || m.GeneratedAt == "" {
		t.Fatalf("manifest = %+v", m)
	}
	var buf bytes.Buffer
	if _, err := p.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}

	// A fresh instance (wiped storage) restores the archive.
	dir2 := t.TempDir()
	b := newBackupApp(t, dir2)
	if _, err := b.CreateNote(ctx, NoteInput{Title: "重置后的临时内容"}); err != nil {
		t.Fatal(err)
	}
	m2, err := b.RestoreBackup(ctx, bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if m2.InstanceID != inst {
		t.Fatalf("restored instance = %q, want %q", m2.InstanceID, inst)
	}

	// Reopen (what ReexecSelf would do) and verify content came back.
	b.DB.Close()
	c := newBackupApp(t, dir2)
	got, err := c.ListNotes(ctx, NoteFilter{PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].ID != note.ID || got.Items[0].Title != "备份测试" {
		t.Fatalf("restored notes = %+v", got)
	}
	f, err := c.GetFile(ctx, upload.Hash)
	if err != nil {
		t.Fatal(err)
	}
	rf, err := c.OpenFile(f)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()
	body, _ := io.ReadAll(rf)
	if string(body) != "hello 备份" {
		t.Fatalf("restored attachment = %q", body)
	}
	id2, err := c.InstanceID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if id2 != inst {
		t.Fatalf("instance id changed across restore: %q vs %q", id2, inst)
	}
	// The replaced state is kept aside.
	if _, err := os.Stat(filepath.Join(dir2, ".restore-prev")); err != nil {
		t.Fatalf("previous state not kept: %v", err)
	}
}

func TestPrepareBackupWithoutAttachments(t *testing.T) {
	// A fresh instance has no files/ directory at all; backup must still work.
	a := newBackupApp(t, t.TempDir())
	p, err := a.PrepareBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Remove()
	if p.Manifest.Files != 0 || p.Manifest.SchemaVersion != db.LatestVersion() {
		t.Fatalf("manifest = %+v", p.Manifest)
	}
	var buf bytes.Buffer
	if _, err := p.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	names := map[string]bool{}
	for {
		hd, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[hd.Name] = true
	}
	if !names["blog.db"] || !names["manifest.json"] {
		t.Fatalf("archive entries = %v", names)
	}
}

func TestRestoreRejectsBadArchives(t *testing.T) {
	ctx := context.Background()
	fresh := func(t *testing.T) *App { return newBackupApp(t, t.TempDir()) }

	if _, err := fresh(t).RestoreBackup(ctx, strings.NewReader("这不是 gzip")); err == nil {
		t.Fatal("garbage input should fail")
	}
	if _, err := fresh(t).RestoreBackup(ctx, bytes.NewReader(tgz(t, map[string]string{"../evil.txt": "x"}))); err == nil {
		t.Fatal("path traversal should fail")
	}
	if _, err := fresh(t).RestoreBackup(ctx, bytes.NewReader(tgz(t, map[string]string{"blog.db": "不是数据库"}))); err == nil {
		t.Fatal("non-database payload should fail")
	}
	if _, err := fresh(t).RestoreBackup(ctx, bytes.NewReader(tgz(t, map[string]string{"other.txt": "x"}))); err == nil {
		t.Fatal("unexpected entry should fail")
	}
}

func TestRestoreRejectsNewerSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := db.Open(ctx, filepath.Join(dir, "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (9999, '2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	// Fold the WAL back in so the copied file carries the fake version.
	if _, err := conn.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	body, err := os.ReadFile(filepath.Join(dir, "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	archive := tgz(t, map[string]string{"blog.db": string(body)})
	if _, err := newBackupApp(t, t.TempDir()).RestoreBackup(ctx, bytes.NewReader(archive)); err == nil {
		t.Fatal("backup from a newer app should be rejected")
	}
}
