package core

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/xiaotuantuan5/blog/internal/db"
	"github.com/xiaotuantuan5/blog/internal/markdown"
)

type File struct {
	ID        int64   `json:"id"`
	Hash      string  `json:"hash"`
	Filename  string  `json:"filename"`
	Mime      string  `json:"mime"`
	Size      int64   `json:"size"`
	URL       string  `json:"url"`
	IsImage   bool    `json:"isImage"`
	CreatedAt db.Time `json:"createdAt"`
}

// Allowed upload types, detected from file content (not the extension).
var allowedMimes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"application/zip": true,
	"text/plain":      true,
}

// Plain text is only accepted for these extensions, so markup such as SVG
// (which content sniffing reports as text/plain) is rejected.
var textExts = map[string]bool{".txt": true, ".md": true, ".markdown": true, ".log": true, ".csv": true, ".json": true}

var hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func summarize(textPrefix string) string { return markdown.Summary(textPrefix, summaryLen) }

func detectMime(head []byte) string {
	m := http.DetectContentType(head)
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	return m
}

func cleanFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`"<>|:*?`, r) {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	for utf8.RuneCountInString(name) > 120 {
		r := []rune(name)
		name = string(r[len(r)-120:])
	}
	return name
}

// SaveUpload stores an uploaded file, deduplicating by content hash.
func (a *App) SaveUpload(ctx context.Context, r io.Reader, filename string) (*File, error) {
	if err := os.MkdirAll(a.filesDir(), 0o750); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(a.filesDir(), ".upload-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	h := sha256.New()
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	head = head[:n]
	if n == 0 {
		return nil, invalid("文件为空")
	}
	mime := detectMime(head)
	if !allowedMimes[mime] || (mime == "text/plain" && !textExts[strings.ToLower(filepath.Ext(filename))]) {
		return nil, invalid(fmt.Sprintf("不支持的文件类型（%s）。支持 PNG、JPEG、GIF、WebP、PDF、ZIP 和 .txt/.md 等纯文本", mime))
	}
	limited := io.LimitReader(io.MultiReader(bytes.NewReader(head), r), a.MaxUpload+1)
	size, err := io.Copy(io.MultiWriter(tmp, h), limited)
	if err != nil {
		return nil, err
	}
	if size > a.MaxUpload {
		return nil, invalid(fmt.Sprintf("文件太大（最大 %d MB）", a.MaxUpload>>20))
	}
	if err := tmp.Sync(); err != nil {
		return nil, err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	dest := a.filePath(hash)
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return nil, err
	}
	if _, err := os.Stat(dest); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(tmp.Name(), dest); err != nil {
			return nil, err
		}
	}

	filename = cleanFilename(filename)
	_, err = a.DB.ExecContext(ctx, `
		INSERT INTO files (hash, filename, mime, size, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(hash) DO NOTHING`, hash, filename, mime, size, db.Now())
	if err != nil {
		return nil, err
	}
	f, err := a.GetFile(ctx, hash)
	if err != nil {
		return nil, err
	}
	// Report the name the user uploaded this time, even if deduplicated.
	f.Filename = filename
	return f, nil
}

func (a *App) filePath(hash string) string {
	return filepath.Join(a.filesDir(), hash[:2], hash)
}

func (a *App) GetFile(ctx context.Context, hash string) (*File, error) {
	if !hashRe.MatchString(hash) {
		return nil, ErrNotFound
	}
	var f File
	err := a.DB.QueryRowContext(ctx, `SELECT id, hash, filename, mime, size, created_at FROM files WHERE hash = ?`, hash).
		Scan(&f.ID, &f.Hash, &f.Filename, &f.Mime, &f.Size, &f.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	f.URL = "/files/" + f.Hash
	f.IsImage = strings.HasPrefix(f.Mime, "image/")
	return &f, nil
}

// FileIsPublic reports whether a public note references the file.
func (a *App) FileIsPublic(ctx context.Context, fileID int64) (bool, error) {
	var n int
	err := a.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM note_files nf JOIN notes n ON n.id = nf.note_id
		WHERE nf.file_id = ? AND n.visibility = 'public'`, fileID).Scan(&n)
	return n > 0, err
}

// OpenFile opens the stored content of a file.
func (a *App) OpenFile(f *File) (*os.File, error) {
	return os.Open(a.filePath(f.Hash))
}
