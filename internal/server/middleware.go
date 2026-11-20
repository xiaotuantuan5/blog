package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
)

const sessionCookie = "blog_session"

type ctxKey int

const sessionKey ctxKey = 1

func sessionFrom(r *http.Request) *core.Session {
	s, _ := r.Context().Value(sessionKey).(*core.Session)
	return s
}

// statusWriter records the response status for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic", "err", rec, "stack", string(debug.Stack()))
				if sw.status == 0 {
					writeMsg(sw, http.StatusInternalServerError, "服务器内部错误")
				}
			}
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				return
			}
			slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status,
				"dur", time.Since(start).Round(time.Millisecond))
		}()
		next.ServeHTTP(sw, r)
	})
}

// The frontend loads nothing from other origins, so the CSP can be strict.
// 'unsafe-inline' styles are needed by KaTeX, Mermaid and CodeMirror.
const csp = "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; " +
	"font-src 'self' data:; script-src 'self'; connect-src 'self'; object-src 'none'; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// loadSession attaches the logged-in session (if any) to the request.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/files/") {
			if c, err := r.Cookie(sessionCookie); err == nil {
				if sess, err := s.app.SessionByToken(r.Context(), c.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), sessionKey, sess))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// admin requires a session, and a CSRF token for state-changing requests.
func admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r)
		if sess == nil {
			writeMsg(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sess.CheckCSRF(r.Header.Get("X-CSRF-Token")) {
			writeMsg(w, http.StatusForbidden, "CSRF 校验失败，请刷新页面后重试")
			return
		}
		h(w, r)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) isHTTPS(r *http.Request) bool {
	return r.TLS != nil || (s.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https")
}

// loginLimiter allows a fixed number of failed logins per IP per window.
type loginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	return &loginLimiter{max: max, window: window, fails: map[string][]time.Time{}}
}

func (l *loginLimiter) prune(ip string, now time.Time) []time.Time {
	list := l.fails[ip]
	i := 0
	for i < len(list) && now.Sub(list[i]) > l.window {
		i++
	}
	list = list[i:]
	if len(list) == 0 {
		delete(l.fails, ip)
	} else {
		l.fails[ip] = list
	}
	return list
}

func (l *loginLimiter) Allowed(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(ip, time.Now())) < l.max
}

func (l *loginLimiter) Fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.fails[ip] = append(l.prune(ip, now), now)
	// Bound memory if many IPs hammer the endpoint.
	if len(l.fails) > 10000 {
		for k := range l.fails {
			l.prune(k, now)
		}
	}
}

func (l *loginLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}
