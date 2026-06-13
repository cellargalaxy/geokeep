package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"geokeep/internal/auth"
	"geokeep/internal/repo"
)

// chain 顺序应用一组中间件，最先列出的最外层。
func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// withRecover 兜底捕获 panic，避免单请求拖垮整个服务。
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "path", r.URL.Path, "err", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withLogger 结构化访问日志。
func withLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		slog.Info("http", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// withBodyLimit 用 http.MaxBytesReader 防御超大上传。
func (s *Server) withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes())
		next.ServeHTTP(w, r)
	})
}

// withCSRF 防御 CSRF：要求所有非 GET/HEAD 请求带 Origin 头且与本机匹配，
// 或带 Sec-Fetch-Site=same-origin。上报通道（api_key 路径）跳过本中间件。
func (s *Server) withCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg.Dev {
			next.ServeHTTP(w, r)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site == "same-origin" || site == "same-site" || site == "none" {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			http.Error(w, "missing Origin header", http.StatusForbidden)
			return
		}
		if !sameOrigin(origin, r) {
			http.Error(w, "csrf: origin mismatch", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin string, r *http.Request) bool {
	host := r.Host
	return strings.HasSuffix(origin, "//"+host)
}

// requireSession 校验 Cookie，把 user_id / admin 写入 context。
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.SessionCookieName)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sess, err := s.signer.Verify(c.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := s.repo.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), auth.CtxKeyUserID, u.ID)
		ctx = context.WithValue(ctx, auth.CtxKeyAdmin, u.Admin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAdmin 必须经 requireSession 之后调用。
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.IsAdminFromCtx(r.Context()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAPIKey 上报通道鉴权；从 query/Bearer/Basic 提取 api_key 后查用户。
func (s *Server) requireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, err := auth.ExtractAPIKey(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		u, err := s.repo.GetUserByAPIKey(r.Context(), key)
		if err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		ctx := context.WithValue(r.Context(), auth.CtxKeyUserID, u.ID)
		ctx = context.WithValue(ctx, auth.CtxKeyAdmin, u.Admin)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
