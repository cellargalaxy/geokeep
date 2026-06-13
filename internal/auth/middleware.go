package auth

import (
	"context"
	"net/http"
	"time"
)

// CtxKey 是 context key 的私有类型，避免外部冲突。
type CtxKey string

const (
	// CtxKeyUserID 通过 context 携带已鉴权的 user_id。
	CtxKeyUserID CtxKey = "geokeep.user_id"
	// CtxKeyAdmin 标记当前会话是否为 admin。
	CtxKeyAdmin CtxKey = "geokeep.admin"
)

// UserIDFromCtx 提取 user_id；handler 拿到 0 应视为越权或鉴权缺失，直接 401。
func UserIDFromCtx(ctx context.Context) uint {
	v, _ := ctx.Value(CtxKeyUserID).(uint)
	return v
}

// IsAdminFromCtx 是否为 admin。
func IsAdminFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(CtxKeyAdmin).(bool)
	return v
}

// CookieMaker 根据 BasePath / Dev 等生成统一的 Cookie 模板。
type CookieMaker struct {
	Path string
	Dev  bool
}

// Build 生成 Cookie。
func (m CookieMaker) Build(name, value string, maxAge time.Duration) *http.Cookie {
	path := m.Path
	if path == "" {
		path = "/"
	}
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		HttpOnly: true,
		Secure:   !m.Dev,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(maxAge),
		MaxAge:   int(maxAge.Seconds()),
	}
}

// Clear 生成清除 cookie 的删除指令。
func (m CookieMaker) Clear(name string) *http.Cookie {
	path := m.Path
	if path == "" {
		path = "/"
	}
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		HttpOnly: true,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}
