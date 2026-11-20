package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/xiaotuantuan5/blog/internal/core"
)

const maxJSONBody = 8 << 20

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeMsg(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeError maps domain errors to HTTP responses.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		ve *core.ValidationError
		ce *core.ConflictError
		mb *http.MaxBytesError
	)
	switch {
	case errors.As(err, &ve):
		writeMsg(w, http.StatusBadRequest, ve.Msg)
	case errors.As(err, &ce):
		writeMsg(w, http.StatusConflict, ce.Msg)
	case errors.Is(err, core.ErrNotFound):
		writeMsg(w, http.StatusNotFound, "未找到")
	case errors.Is(err, core.ErrBadCredentials):
		writeMsg(w, http.StatusUnauthorized, core.ErrBadCredentials.Error())
	case errors.As(err, &mb):
		writeMsg(w, http.StatusRequestEntityTooLarge, "请求内容太大")
	default:
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeMsg(w, http.StatusInternalServerError, "服务器内部错误")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			return err
		}
		return &core.ValidationError{Msg: "请求格式错误"}
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, core.ErrNotFound
	}
	return id, nil
}

func queryInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil {
		return v
	}
	return def
}
