// Package server exposes the HTTP API, attachments, feeds and the SPA.
package server

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
)

type Config struct {
	// TrustProxy honours X-Forwarded-For / X-Forwarded-Proto from a reverse proxy.
	TrustProxy bool
}

type Server struct {
	app     *core.App
	cfg     Config
	limiter *loginLimiter
	spa     *spa
}

// New builds the HTTP handler. dist is the built frontend (may be empty).
func New(app *core.App, cfg Config, dist fs.FS) http.Handler {
	s := &Server{
		app:     app,
		cfg:     cfg,
		limiter: newLoginLimiter(10, 15*time.Minute),
		spa:     newSPA(dist),
	}
	mux := http.NewServeMux()

	// Public API
	mux.HandleFunc("GET /api/site", s.handleSite)
	mux.HandleFunc("GET /api/public/notes", s.handlePublicNotes)
	mux.HandleFunc("GET /api/public/notes/{slug}", s.handlePublicNote)
	mux.HandleFunc("GET /api/public/tags", s.handlePublicTags)
	mux.HandleFunc("GET /api/public/archive", s.handleArchive)

	// Auth
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", admin(s.handleLogout))
	mux.HandleFunc("GET /api/auth/me", s.handleMe)
	mux.HandleFunc("PUT /api/auth/password", admin(s.handleChangePassword))

	// Admin API
	mux.HandleFunc("GET /api/admin/notes", admin(s.handleListNotes))
	mux.HandleFunc("POST /api/admin/notes", admin(s.handleCreateNote))
	mux.HandleFunc("GET /api/admin/notes/{id}", admin(s.handleGetNote))
	mux.HandleFunc("PUT /api/admin/notes/{id}", admin(s.handleUpdateNote))
	mux.HandleFunc("PATCH /api/admin/notes/{id}", admin(s.handlePatchNote))
	mux.HandleFunc("DELETE /api/admin/notes/{id}", admin(s.handleDeleteNote))
	mux.HandleFunc("POST /api/admin/notes/{id}/restore", admin(s.handleRestoreNote))
	mux.HandleFunc("DELETE /api/admin/notes/{id}/purge", admin(s.handlePurgeNote))
	mux.HandleFunc("GET /api/admin/notes/{id}/revisions", admin(s.handleListRevisions))
	mux.HandleFunc("POST /api/admin/notes/{id}/revisions/{rid}/restore", admin(s.handleRestoreRevision))
	mux.HandleFunc("GET /api/admin/notes/{id}/annotations", admin(s.handleListNoteAnnotations))
	mux.HandleFunc("POST /api/admin/notes/{id}/annotations", admin(s.handleCreateAnnotation))
	mux.HandleFunc("GET /api/admin/annotations", admin(s.handleListAnnotations))
	mux.HandleFunc("PUT /api/admin/annotations/{id}", admin(s.handleUpdateAnnotation))
	mux.HandleFunc("DELETE /api/admin/annotations/{id}", admin(s.handleDeleteAnnotation))
	mux.HandleFunc("GET /api/admin/daily/{date}", admin(s.handleDailyNote))
	mux.HandleFunc("GET /api/admin/trash", admin(s.handleTrash))
	mux.HandleFunc("GET /api/admin/stats/heatmap", admin(s.handleHeatmap))
	mux.HandleFunc("POST /api/admin/preview", admin(s.handlePreview))
	mux.HandleFunc("GET /api/admin/notebooks", admin(s.handleListNotebooks))
	mux.HandleFunc("POST /api/admin/notebooks", admin(s.handleCreateNotebook))
	mux.HandleFunc("PUT /api/admin/notebooks/{id}", admin(s.handleUpdateNotebook))
	mux.HandleFunc("DELETE /api/admin/notebooks/{id}", admin(s.handleDeleteNotebook))
	mux.HandleFunc("GET /api/admin/tags", admin(s.handleListTags))
	mux.HandleFunc("PUT /api/admin/tags/{id}", admin(s.handleRenameTag))
	mux.HandleFunc("DELETE /api/admin/tags/{id}", admin(s.handleDeleteTag))
	mux.HandleFunc("POST /api/admin/files", admin(s.handleUpload))
	mux.HandleFunc("GET /api/admin/settings", admin(s.handleGetSettings))
	mux.HandleFunc("PUT /api/admin/settings", admin(s.handleUpdateSettings))
	mux.HandleFunc("GET /api/admin/stats", admin(s.handleStats))

	// Attachments, feeds, SPA
	mux.HandleFunc("GET /files/{hash}", s.handleFile)
	mux.HandleFunc("GET /rss.xml", s.handleRSS)
	mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	mux.HandleFunc("GET /robots.txt", s.handleRobots)
	mux.HandleFunc("GET /n/{slug}", s.handleNotePage)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			writeMsg(w, http.StatusNotFound, "接口不存在")
		case r.Method != http.MethodGet && r.Method != http.MethodHead:
			w.Header().Set("Allow", "GET, HEAD")
			writeMsg(w, http.StatusMethodNotAllowed, "不支持的请求方法")
		default:
			s.handleSPA(w, r)
		}
	})

	return logRequests(securityHeaders(s.loadSession(mux)))
}
