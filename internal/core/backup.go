package core

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/xiaotuantuan5/blog/internal/db"
)

// Backup and restore keep the site alive across instance resets (ZCode Sites
// storage is per-instance): a sync agent downloads backups over the admin
// API and pushes the latest one back when it detects a fresh instance.

// BackupManifest describes a backup archive's contents.
type BackupManifest struct {
	InstanceID    string `json:"instanceId"`
	GeneratedAt   string `json:"generatedAt"`
	SchemaVersion int    `json:"schemaVersion"`
	Notes         int    `json:"notes"`
	Files         int    `json:"files"`
	Bytes         int64  `json:"bytes"`
}

// backupMu serializes restores against backup preparation.
var backupMu sync.RWMutex

// Caps on extraction, so a hostile archive cannot fill the disk.
const (
	maxRestoreBytes  = 400 << 20
	maxRestoreEntry  = 100 << 20
	maxRestoreCounts = 50000
)

// InstanceID returns this instance's identity, minted on first use. A fresh
// database (fresh deploy, wiped storage) gets a new one — the signal for the
// sync agent to restore the latest backup.
func (a *App) InstanceID(ctx context.Context) (string, error) {
	var id string
	err := a.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'instance_id'`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err = a.DB.ExecContext(ctx,
			`INSERT INTO settings (key, value) VALUES ('instance_id', ?) ON CONFLICT(key) DO NOTHING`,
			randomString(24)); err != nil {
			return "", err
		}
		err = a.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'instance_id'`).Scan(&id)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

// PreparedBackup is a complete archive on disk, ready to stream.
type PreparedBackup struct {
	Dir      string // holds backup.tar.gz; remove after use
	Manifest BackupManifest
}

// PrepareBackup snapshots the database (VACUUM INTO) and packs it with all
// attachments into a gzip tar.
func (a *App) PrepareBackup(ctx context.Context) (*PreparedBackup, error) {
	backupMu.RLock()
	defer backupMu.RUnlock()

	id, err := a.InstanceID(ctx)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp(a.DataDir, ".backup-*")
	if err != nil {
		return nil, err
	}
	p := &PreparedBackup{Dir: dir}
	archive := filepath.Join(dir, "backup.tar.gz")
	fail := func(err error) (*PreparedBackup, error) {
		os.RemoveAll(dir)
		return nil, err
	}

	// VACUUM INTO refuses to overwrite an existing target, and the file name
	// cannot be a bound parameter, so interpolate a server-generated path.
	snap := filepath.Join(dir, "snapshot.db")
	if _, err := a.DB.ExecContext(ctx, `VACUUM INTO '`+strings.ReplaceAll(snap, "'", "''")+`'`); err != nil {
		return fail(err)
	}

	m := BackupManifest{InstanceID: id, GeneratedAt: db.Now().Format(db.TimeLayout)}
	_ = a.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&m.SchemaVersion)
	_ = a.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes`).Scan(&m.Notes)

	f, err := os.Create(archive)
	if err != nil {
		return fail(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	writeErr := func(e error) (*PreparedBackup, error) {
		tw.Close()
		gz.Close()
		f.Close()
		return fail(e)
	}

	// Attachment blobs first, then the database, then the manifest.
	walkFiles := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || strings.HasPrefix(d.Name(), ".upload-") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(a.DataDir, path)
		if err != nil {
			return err
		}
		if err := tarHeader(tw, filepath.ToSlash(rel), info.Size()); err != nil {
			return err
		}
		if err := copyFileTo(tw, path); err != nil {
			return err
		}
		m.Files++
		m.Bytes += info.Size()
		return nil
	}
	if _, err := os.Stat(a.filesDir()); err == nil {
		if err := filepath.WalkDir(a.filesDir(), walkFiles); err != nil {
			return writeErr(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return writeErr(err)
	}

	info, err := os.Stat(snap)
	if err != nil {
		return writeErr(err)
	}
	if err := tarHeader(tw, "blog.db", info.Size()); err != nil {
		return writeErr(err)
	}
	if err := copyFileTo(tw, snap); err != nil {
		return writeErr(err)
	}

	body, err := json.Marshal(m)
	if err != nil {
		return writeErr(err)
	}
	if err := tarHeader(tw, "manifest.json", int64(len(body))); err != nil {
		return writeErr(err)
	}
	if _, err := tw.Write(body); err != nil {
		return writeErr(err)
	}

	if err := tw.Close(); err != nil {
		f.Close()
		return fail(err)
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return fail(err)
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	os.Remove(snap)
	p.Manifest = m
	return p, nil
}

// Size is the archive size in bytes.
func (p *PreparedBackup) Size() (int64, error) {
	fi, err := os.Stat(filepath.Join(p.Dir, "backup.tar.gz"))
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// WriteTo streams the archive contents.
func (p *PreparedBackup) WriteTo(w io.Writer) (int64, error) {
	f, err := os.Open(filepath.Join(p.Dir, "backup.tar.gz"))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return io.Copy(w, f)
}

// Remove deletes the temporary archive.
func (p *PreparedBackup) Remove() { os.RemoveAll(p.Dir) }

func tarHeader(tw *tar.Writer, name string, size int64) error {
	return tw.WriteHeader(&tar.Header{
		Name:     name,
		Mode:     0o640,
		Size:     size,
		Typeflag: tar.TypeReg,
		ModTime:  db.Now().Time,
	})
}

func copyFileTo(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// RestoreBackup validates a WriteBackup archive and swaps the database and
// attachment files into place. Existing handles keep serving the old state;
// the caller must ReexecSelf right after so the restored files are opened.
func (a *App) RestoreBackup(ctx context.Context, r io.Reader) (BackupManifest, error) {
	backupMu.Lock()
	defer backupMu.Unlock()

	staging := filepath.Join(a.DataDir, ".restore-"+randomString(8))
	if err := os.MkdirAll(staging, 0o750); err != nil {
		return BackupManifest{}, err
	}
	done := false
	defer func() {
		if !done {
			os.RemoveAll(staging)
		}
	}()

	if err := extractBackup(r, staging); err != nil {
		return BackupManifest{}, err
	}

	dbFile := filepath.Join(staging, "blog.db")
	if _, err := os.Stat(dbFile); err != nil {
		return BackupManifest{}, invalid("备份缺少数据库文件")
	}
	schema, instID, err := db.CheckBackup(ctx, dbFile)
	if err != nil {
		return BackupManifest{}, invalid("备份校验未通过：" + err.Error())
	}
	if schema > db.LatestVersion() {
		return BackupManifest{}, invalid("备份来自更新版本的应用，请先升级再恢复")
	}

	var m BackupManifest
	if b, err := os.ReadFile(filepath.Join(staging, "manifest.json")); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	m.InstanceID = instID
	m.SchemaVersion = schema

	// Keep the replaced state one directory deep as a safety net.
	prev := filepath.Join(a.DataDir, ".restore-prev")
	if err := os.RemoveAll(prev); err != nil {
		return BackupManifest{}, err
	}
	if err := os.MkdirAll(prev, 0o750); err != nil {
		return BackupManifest{}, err
	}
	for _, name := range []string{"blog.db", "blog.db-wal", "blog.db-shm"} {
		if err := moveIfExists(filepath.Join(a.DataDir, name), filepath.Join(prev, name)); err != nil {
			return BackupManifest{}, err
		}
	}
	if err := moveIfExists(a.filesDir(), filepath.Join(prev, "files")); err != nil {
		return BackupManifest{}, err
	}
	if err := os.Rename(dbFile, a.dbPath()); err != nil {
		return BackupManifest{}, err
	}
	// The live files dir was moved aside above, so these renames never
	// overwrite an existing directory (overlayfs refuses that).
	stFiles := filepath.Join(staging, "files")
	if _, err := os.Stat(stFiles); err == nil {
		if err := os.Rename(stFiles, a.filesDir()); err != nil {
			return BackupManifest{}, err
		}
	} else if err := os.MkdirAll(a.filesDir(), 0o750); err != nil {
		// An empty attachment set archives no files/ entries at all.
		return BackupManifest{}, err
	}

	done = true
	os.RemoveAll(staging)
	slog.Info("备份已恢复，等待重启加载", "instance", instID, "notes", m.Notes)
	return m, nil
}

func (a *App) dbPath() string { return filepath.Join(a.DataDir, "blog.db") }

func moveIfExists(src, dst string) error {
	if _, err := os.Stat(src); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func extractBackup(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return invalid("不是有效的备份档案")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var total int64
	entries := 0
	for {
		hd, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("备份档案损坏")
		}
		if hd.Typeflag != tar.TypeReg {
			return invalid("备份包含不支持的条目：" + hd.Name)
		}
		name, err := secureEntry(hd.Name)
		if err != nil {
			return err
		}
		if name != "blog.db" && name != "manifest.json" && !strings.HasPrefix(name, "files/") {
			return invalid("备份包含意外文件：" + hd.Name)
		}
		if hd.Size > maxRestoreEntry || total+hd.Size > maxRestoreBytes {
			return invalid("备份内容过大")
		}
		if entries++; entries > maxRestoreCounts {
			return invalid("备份条目过多")
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := writeCapped(tr, hd.Size, target); err != nil {
			return invalid("备份档案损坏")
		}
		total += hd.Size
	}
	return nil
}

// secureEntry validates an archive member name and returns its clean,
// slash-separated relative path.
func secureEntry(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\\') {
		return "", invalid("备份包含非法路径：" + name)
	}
	p := filepath.ToSlash(filepath.Clean("/" + name))
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") {
		return "", invalid("备份包含非法路径：" + name)
	}
	return p, nil
}

// writeCapped copies exactly size bytes (and nothing more) to target.
func writeCapped(r io.Reader, size int64, target string) error {
	f, err := os.Create(target)
	if err != nil {
		return err
	}
	n, err := io.CopyN(f, r, size)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(target)
		return err
	}
	if n != size {
		os.Remove(target)
		return io.ErrUnexpectedEOF
	}
	return nil
}

// ReexecSelf replaces the process image with a fresh start of the same
// binary and arguments, so restored files are opened cleanly. It only
// returns on failure.
func ReexecSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("定位可执行文件：%w", err)
	}
	slog.Info("重启进程以加载恢复的数据", "exe", exe)
	return syscall.Exec(exe, os.Args, os.Environ())
}
