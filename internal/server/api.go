package server

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
	"github.com/xiaotuantuan5/blog/internal/markdown"
)

// ---- Public ----

type publicSite struct {
	SiteTitle       string `json:"siteTitle"`
	SiteDescription string `json:"siteDescription"`
	Author          string `json:"author"`
	Avatar          string `json:"avatar"`
	AboutHTML       string `json:"aboutHtml"`
}

func (s *Server) handleSite(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, publicSite{st.SiteTitle, st.SiteDescription, st.Author, st.Avatar, st.AboutHTML})
}

func (s *Server) handlePublicNotes(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	q := r.URL.Query()
	page, err := s.app.ListNotes(r.Context(), core.NoteFilter{
		PublicOnly: true,
		Tag:        q.Get("tag"),
		Q:          q.Get("q"),
		Page:       queryInt(r, "page", 1),
		PageSize:   st.PageSize,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handlePublicNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.GetPublicNote(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handlePublicTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.app.ListTags(r.Context(), true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleArchive(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.Archive(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ---- Auth ----

type meResponse struct {
	LoggedIn  bool   `json:"loggedIn"`
	Username  string `json:"username,omitempty"`
	CSRFToken string `json:"csrfToken,omitempty"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	if !s.limiter.Allowed(ip) {
		writeMsg(w, http.StatusTooManyRequests, "登录失败次数过多，请 15 分钟后再试")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	token, sess, err := s.app.Login(r.Context(), strings.TrimSpace(in.Username), in.Password)
	if errors.Is(err, core.ErrBadCredentials) {
		s.limiter.Fail(ip)
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.limiter.Reset(ip)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   s.isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, meResponse{true, sess.Username, sess.CSRFToken})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.app.Logout(r.Context(), c.Value); err != nil {
			writeError(w, r, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.isHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, meResponse{})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	if sess == nil {
		writeJSON(w, http.StatusOK, meResponse{})
		return
	}
	writeJSON(w, http.StatusOK, meResponse{true, sess.Username, sess.CSRFToken})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	c, _ := r.Cookie(sessionCookie)
	if err := s.app.ChangePassword(r.Context(), sessionFrom(r), in.OldPassword, in.NewPassword, c.Value); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Notes ----

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := core.NoteFilter{
		Tag:        q.Get("tag"),
		Q:          q.Get("q"),
		Visibility: q.Get("visibility"),
		Favorite:   q.Get("favorite") == "1",
		Page:       queryInt(r, "page", 1),
		PageSize:   queryInt(r, "pageSize", 30),
	}
	if v := q.Get("notebook"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeMsg(w, http.StatusBadRequest, "无效的笔记本")
			return
		}
		f.NotebookID = &id
	}
	page, err := s.app.ListNotes(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	var in core.NoteInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	n, err := s.app.CreateNote(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	n, err := s.app.GetNote(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in core.NoteInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	n, err := s.app.UpdateNote(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handlePatchNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in core.NoteFlags
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.SetNoteFlags(r.Context(), id, in); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.DeleteNote(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleRestoreNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.RestoreNote(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePurgeNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// Only trashed notes can be purged; force is not exposed over HTTP.
	if err := s.app.PurgeNote(r.Context(), id, false); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	items, err := s.app.ListTrash(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) handleListRevisions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	list, err := s.app.ListRevisions(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleRestoreRevision(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	rid, err := strconv.ParseInt(r.PathValue("rid"), 10, 64)
	if err != nil || rid <= 0 {
		writeMsg(w, http.StatusBadRequest, "无效的版本号")
		return
	}
	n, err := s.app.RestoreRevision(r.Context(), id, rid)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDailyNote(w http.ResponseWriter, r *http.Request) {
	n, err := s.app.GetOrCreateDaily(r.Context(), r.PathValue("date"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleHeatmap(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeMsg(w, http.StatusBadRequest, "缺少 from / to 参数")
		return
	}
	days, err := s.app.Heatmap(r.Context(), from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, days)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ContentMD string `json:"contentMd"`
		Format    string `json:"format"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	res, err := s.app.RenderPreview(r.Context(), in.Format, in.ContentMD)
	if err != nil {
		writeError(w, r, err)
		return
	}
	toc := res.TOC
	if toc == nil {
		toc = []markdown.Heading{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"html": res.HTML, "toc": toc})
}

// ---- Annotations ----

func (s *Server) handleListNoteAnnotations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	list, err := s.app.ListNoteAnnotations(r.Context(), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in core.AnnotationInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	an, err := s.app.CreateAnnotation(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, an)
}

func (s *Server) handleUpdateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in core.AnnotationInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	an, err := s.app.UpdateAnnotation(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, an)
}

func (s *Server) handleDeleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.DeleteAnnotation(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListAnnotations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := core.AnnotationFilter{
		Color:    q.Get("color"),
		Q:        q.Get("q"),
		Page:     queryInt(r, "page", 1),
		PageSize: queryInt(r, "pageSize", 50),
	}
	if v := q.Get("notebook"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeMsg(w, http.StatusBadRequest, "无效的笔记本")
			return
		}
		f.NotebookID = &id
	}
	page, err := s.app.ListAnnotations(r.Context(), f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

// ---- Notebooks & tags ----

func (s *Server) handleListNotebooks(w http.ResponseWriter, r *http.Request) {
	list, err := s.app.ListNotebooks(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleCreateNotebook(w http.ResponseWriter, r *http.Request) {
	var in core.NotebookInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	nb, err := s.app.CreateNotebook(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, nb)
}

func (s *Server) handleUpdateNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in core.NotebookInput
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	nb, err := s.app.UpdateNotebook(r.Context(), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, nb)
}

func (s *Server) handleDeleteNotebook(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.DeleteNotebook(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.app.ListTags(r.Context(), false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleRenameTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.RenameTag(r.Context(), id, in.Name); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.app.DeleteTag(r.Context(), id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Settings & stats ----

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var in core.Settings
	if err := decodeJSON(w, r, &in); err != nil {
		writeError(w, r, err)
		return
	}
	st, err := s.app.UpdateSettings(r.Context(), in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.Stats(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// ---- Files ----

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, s.app.MaxUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeMsg(w, http.StatusBadRequest, "请使用 multipart/form-data 上传")
		return
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			writeMsg(w, http.StatusBadRequest, "缺少 file 字段")
			return
		}
		if err != nil {
			writeError(w, r, err)
			return
		}
		if part.FormName() != "file" {
			part.Close()
			continue
		}
		f, err := s.app.SaveUpload(r.Context(), part, part.FileName())
		part.Close()
		if err != nil {
			writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, f)
		return
	}
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.app.GetFile(r.Context(), r.PathValue("hash"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	cache := "private, max-age=86400"
	if sessionFrom(r) == nil {
		public, err := s.app.FileIsPublic(r.Context(), f.ID)
		if err != nil {
			writeError(w, r, err)
			return
		}
		if !public {
			http.NotFound(w, r)
			return
		}
		cache = "public, max-age=3600"
	}
	fh, err := s.app.OpenFile(f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer fh.Close()

	disposition := "attachment"
	if f.IsImage || f.Mime == "application/pdf" || f.Mime == "text/plain" {
		disposition = "inline"
	}
	h := w.Header()
	ct := f.Mime
	if ct == "text/plain" {
		ct += "; charset=utf-8"
	}
	h.Set("Content-Type", ct)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename}))
	h.Set("Cache-Control", cache)
	h.Set("ETag", `"`+f.Hash+`"`)
	http.ServeContent(w, r, "", f.CreatedAt.Time, fh)
}
