package server

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
)

// Backup sync API for an external agent (see docs/DESIGN.md 数据持久化):
// it logs in, downloads backups and pushes the latest one back after an
// instance reset.

func (s *Server) handleBackupDownload(w http.ResponseWriter, r *http.Request) {
	p, err := s.app.PrepareBackup(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer p.Remove()

	h := w.Header()
	h.Set("Content-Type", "application/gzip")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Disposition", `attachment; filename="blog-backup.tar.gz"`)
	h.Set("X-Backup-Instance", p.Manifest.InstanceID)
	h.Set("X-Backup-Generated-At", p.Manifest.GeneratedAt)
	h.Set("X-Backup-Schema-Version", strconv.Itoa(p.Manifest.SchemaVersion))
	h.Set("X-Backup-Notes", strconv.Itoa(p.Manifest.Notes))
	if size, err := p.Size(); err == nil {
		h.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if _, err := p.WriteTo(w); err != nil {
		slog.Warn("备份下载中断", "err", err)
	}
}

// The restore payload carries the whole data directory.
const maxRestoreBody = 400 << 20

func (s *Server) handleRestoreUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRestoreBody)
	m, err := s.app.RestoreBackup(r.Context(), r.Body)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status":     "restarting",
		"instanceId": m.InstanceID,
	})
	go func() {
		time.Sleep(500 * time.Millisecond) // let the response flush
		if err := core.ReexecSelf(); err != nil {
			slog.Error("恢复后重启进程失败，请手动重启", "err", err)
			os.Exit(1)
		}
	}()
}
