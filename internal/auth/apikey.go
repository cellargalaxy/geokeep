package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

// APIKeyLen 上报 token 的字节长度（基于 base64 url-safe 编码后约 43 字符）。
const APIKeyLen = 32

// ErrAPIKeyMissing 当请求中找不到任何形式的 api_key 时返回。
var ErrAPIKeyMissing = errors.New("api_key 缺失")

// GenerateAPIKey 生成 url-safe 随机 token。
func GenerateAPIKey() (string, error) {
	b := make([]byte, APIKeyLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ExtractAPIKey 按 OwnTracks/Overland 兼容的三种官方路径提取 token：
//  1. Query string ?api_key=
//  2. Authorization: Bearer <token>
//  3. HTTP Basic 的密码段
//
// 注：X-Limit-U / X-Limit-D 仅作设备/用户名辅助识别，不参与鉴权（OwnTracks 文档）。
func ExtractAPIKey(r *http.Request) (string, error) {
	if v := r.URL.Query().Get("api_key"); v != "" {
		return v, nil
	}
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimSpace(strings.TrimPrefix(h, "Bearer ")), nil
		}
		if strings.HasPrefix(h, "Basic ") {
			if _, pw, ok := r.BasicAuth(); ok && pw != "" {
				return pw, nil
			}
		}
	}
	return "", ErrAPIKeyMissing
}

// ConstantTimeEqual 用常量时间比对避免时序泄露。
func ConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
