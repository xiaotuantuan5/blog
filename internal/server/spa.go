package server

import (
	"bytes"
	"compress/gzip"
	"errors"
	"html"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
)

// spa serves the embedded frontend build.
type spa struct {
	dist  fs.FS
	index string // index.html, or "" if the frontend wasn't built

	mu   sync.Mutex
	gzip map[string][]byte
}

func newSPA(dist fs.FS) *spa {
	s := &spa{dist: dist, gzip: map[string][]byte{}}
	if dist != nil {
		if b, err := fs.ReadFile(dist, "index.html"); err == nil {
			s.index = string(b)
		}
	}
	return s
}

var compressible = map[string]bool{".js": true, ".css": true, ".svg": true, ".json": true, ".html": true, ".txt": true}

// gzipped returns the compressed asset, compressing it once on first use.
func (s *spa) gzipped(name string, data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if gz, ok := s.gzip[name]; ok {
		return gz
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(data)
	_ = zw.Close()
	gz := buf.Bytes()
	if len(gz) >= len(data) {
		gz = nil
	}
	s.gzip[name] = gz
	return gz
}

// serveAsset serves a file from dist; it returns false if there's no such file.
func (s *spa) serveAsset(w http.ResponseWriter, r *http.Request) bool {
	if s.dist == nil {
		return false
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "index.html" {
		return false
	}
	st, err := fs.Stat(s.dist, name)
	if err != nil || st.IsDir() {
		return false
	}
	data, err := fs.ReadFile(s.dist, name)
	if err != nil {
		return false
	}
	h := w.Header()
	ext := path.Ext(name)
	if ct := mime.TypeByExtension(ext); ct != "" {
		h.Set("Content-Type", ct)
	}
	if strings.HasPrefix(name, "assets/") {
		// Vite puts a content hash in asset file names.
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=3600")
	}
	if compressible[ext] {
		h.Add("Vary", "Accept-Encoding")
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			if gz := s.gzipped(name, data); gz != nil {
				h.Set("Content-Encoding", "gzip")
				data = gz
			}
		}
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
	return true
}

type pageMeta struct {
	Title       string
	Description string
	Type        string // "website" or "article"
	URL         string
	Published   string
}

// renderIndex injects page-specific <title> and meta tags so search engines
// and link previews see real content without running JavaScript.
func (s *spa) renderIndex(w http.ResponseWriter, r *http.Request, status int, m pageMeta) {
	if s.index == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("前端尚未构建：请运行 make build（开发时请访问 Vite 开发服务器）。\n"))
		return
	}
	esc := html.EscapeString
	var meta strings.Builder
	meta.WriteString(`<meta name="description" content="` + esc(m.Description) + `">` + "\n")
	meta.WriteString(`<meta property="og:title" content="` + esc(m.Title) + `">` + "\n")
	meta.WriteString(`<meta property="og:description" content="` + esc(m.Description) + `">` + "\n")
	meta.WriteString(`<meta property="og:type" content="` + esc(m.Type) + `">` + "\n")
	if m.URL != "" {
		meta.WriteString(`<meta property="og:url" content="` + esc(m.URL) + `">` + "\n")
		meta.WriteString(`<link rel="canonical" href="` + esc(m.URL) + `">` + "\n")
	}
	if m.Published != "" {
		meta.WriteString(`<meta property="article:published_time" content="` + esc(m.Published) + `">` + "\n")
	}
	meta.WriteString(`<link rel="alternate" type="application/rss+xml" title="RSS" href="/rss.xml">` + "\n")

	page := s.index
	if i, j := strings.Index(page, "<title>"), strings.Index(page, "</title>"); i >= 0 && j > i {
		page = page[:i] + "<title>" + esc(m.Title) + page[j:]
	}
	page = strings.Replace(page, "</head>", meta.String()+"</head>", 1)

	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(page))
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.spa.serveAsset(w, r) {
		return
	}
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	title := st.SiteTitle
	switch {
	case r.URL.Path == "/archive":
		title = "归档 - " + st.SiteTitle
	case r.URL.Path == "/about":
		title = "关于 - " + st.SiteTitle
	case strings.HasPrefix(r.URL.Path, "/tags"):
		title = "标签 - " + st.SiteTitle
	case strings.HasPrefix(r.URL.Path, "/app") || r.URL.Path == "/login":
		title = "工作台 - " + st.SiteTitle
	}
	s.spa.renderIndex(w, r, http.StatusOK, pageMeta{
		Title:       title,
		Description: st.SiteDescription,
		Type:        "website",
	})
}

func (s *Server) handleNotePage(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	n, err := s.app.GetPublicNote(r.Context(), r.PathValue("slug"))
	if errors.Is(err, core.ErrNotFound) {
		s.spa.renderIndex(w, r, http.StatusNotFound, pageMeta{Title: "页面不存在 - " + st.SiteTitle, Type: "website"})
		return
	} else if err != nil {
		writeError(w, r, err)
		return
	}
	s.spa.renderIndex(w, r, http.StatusOK, pageMeta{
		Title:       n.Title + " - " + st.SiteTitle,
		Description: n.Summary,
		Type:        "article",
		URL:         s.baseURL(r, st) + "/n/" + n.Slug,
		Published:   n.PublishedAt.UTC().Format(time.RFC3339),
	})
}
