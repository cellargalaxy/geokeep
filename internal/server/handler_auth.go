package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"geokeep/internal/auth"
	"geokeep/internal/model"
	"geokeep/internal/repo"
)

// 登录失败保护参数
const (
	loginMaxFails  = 5
	loginLockSecs  = 15 * 60
	loginLockReset = time.Hour
)

type setupReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type setupResp struct {
	UserID uint   `json:"user_id"`
	APIKey string `json:"api_key"`
}

// handleSetup 仅在「未初始化（无任何用户）」时允许创建首用户。
// 一次性返回 api_key，前端必须落库存储；后续只显示尾 4 位。
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	n, err := s.repo.UserCount(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if n > 0 {
		http.Error(w, "already initialized", http.StatusGone)
		return
	}
	var req setupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		http.Error(w, "邮箱格式错误", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "密码不合规（≥10字符）", http.StatusBadRequest)
		return
	}
	apiKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	u := &model.User{Email: email, PasswordHash: hash, APIKey: apiKey, Admin: true, Settings: "{}"}
	if err := s.repo.CreateUser(r.Context(), u); err != nil {
		http.Error(w, "创建失败: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, setupResp{UserID: u.ID, APIKey: apiKey})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	u, err := s.repo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// 防枚举：错误不区分 "用户不存在" 与 "密码错误"
		http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
		return
	}
	// 锁定检查
	if u.LockedAt != nil && time.Now().Unix() < *u.LockedAt+loginLockSecs {
		http.Error(w, "账号已被临时锁定，请稍后再试", http.StatusTooManyRequests)
		return
	}
	if err := auth.VerifyPassword(u.PasswordHash, req.Password); err != nil {
		_ = s.repo.IncrLoginFailure(r.Context(), u.ID)
		if u.FailedAttempts+1 >= loginMaxFails {
			now := time.Now().Unix()
			_ = s.repo.LockUser(r.Context(), u.ID, now)
		}
		http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
		return
	}
	_ = s.repo.ResetLoginFailure(r.Context(), u.ID)
	tok, err := s.signer.Issue(u.ID)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, s.cookie.Build(auth.SessionCookieName, tok, auth.SessionTTL))
	writeJSON(w, http.StatusOK, map[string]any{"user_id": u.ID, "email": u.Email, "admin": u.Admin})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, s.cookie.Clear(auth.SessionCookieName))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleBootstrap 给前端返回初始化状态与运行时配置（BasePath 等），
// 前端启动时无需任何 hardcoded 路径。
func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	n, err := s.repo.UserCount(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": n > 0,
		"base_path":   s.cfg.BasePath,
		"osm_tile":    s.cfg.OSMTileURL,
	})
}

// handleMe 返回当前登录用户信息。api_key 只暴露尾 4 位。
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	u, err := s.repo.GetUserByID(r.Context(), uid)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":        u.ID,
		"email":          u.Email,
		"admin":          u.Admin,
		"api_key_suffix": apiKeySuffix(u.APIKey),
	})
}

func (s *Server) handleRotateAPIKey(w http.ResponseWriter, r *http.Request) {
	uid := auth.UserIDFromCtx(r.Context())
	newKey, err := auth.GenerateAPIKey()
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if err := s.repo.UpdateAPIKey(r.Context(), uid, newKey); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": newKey})
}

func apiKeySuffix(k string) string {
	if len(k) <= 4 {
		return k
	}
	return "****" + k[len(k)-4:]
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
