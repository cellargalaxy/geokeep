package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"geokeep/internal/auth"
)

func TestExtractAPIKey_Query(t *testing.T) {
	r := httptest.NewRequest("POST", "/x?api_key=abc", nil)
	k, err := auth.ExtractAPIKey(r)
	if err != nil || k != "abc" {
		t.Fatalf("got %q,%v", k, err)
	}
}

func TestExtractAPIKey_Bearer(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	r.Header.Set("Authorization", "Bearer xyz123")
	k, err := auth.ExtractAPIKey(r)
	if err != nil || k != "xyz123" {
		t.Fatalf("got %q,%v", k, err)
	}
}

func TestExtractAPIKey_Basic(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	r.SetBasicAuth("user", "pwtoken")
	k, err := auth.ExtractAPIKey(r)
	if err != nil || k != "pwtoken" {
		t.Fatalf("got %q,%v", k, err)
	}
}

func TestExtractAPIKey_Missing(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	if _, err := auth.ExtractAPIKey(r); err != auth.ErrAPIKeyMissing {
		t.Fatalf("期望 ErrAPIKeyMissing，实际 %v", err)
	}
}

// XLimit 头仅作辅助识别，不应被识别为 api_key。
func TestExtractAPIKey_XLimitNotUsed(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	r.Header.Set("X-Limit-U", "u")
	r.Header.Set("X-Limit-D", "d")
	if _, err := auth.ExtractAPIKey(r); err != auth.ErrAPIKeyMissing {
		t.Fatalf("X-Limit-* 不应通过鉴权: %v", err)
	}
}

func TestGenerateAPIKey_Distinct(t *testing.T) {
	a, _ := auth.GenerateAPIKey()
	b, _ := auth.GenerateAPIKey()
	if a == b || len(a) < 40 {
		t.Fatalf("生成的 key 异常: a=%q b=%q", a, b)
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !auth.ConstantTimeEqual("abc", "abc") || auth.ConstantTimeEqual("abc", "abd") || auth.ConstantTimeEqual("abc", "ab") {
		t.Fatal("ConstantTimeEqual 行为错误")
	}
}

// 防御：构造 X-Forwarded-For 或其它无关 header 不应影响提取
func TestExtractAPIKey_Noise(t *testing.T) {
	r := httptest.NewRequest("POST", "/x?api_key=k1", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("Cookie", "session=xxx")
	k, err := auth.ExtractAPIKey(r)
	if err != nil || k != "k1" {
		t.Fatalf("got %q,%v", k, err)
	}
}

// 防御：query 与 Authorization 同时存在，优先 query（OwnTracks 默认走 query）
func TestExtractAPIKey_QueryPrecedence(t *testing.T) {
	r := httptest.NewRequest("POST", "/x?api_key=fromq", nil)
	r.Header.Set("Authorization", "Bearer fromh")
	k, _ := auth.ExtractAPIKey(r)
	if k != "fromq" {
		t.Fatalf("query 应优先: %q", k)
	}
}

// http.Request 类型断言占位，避免 unused
var _ = http.MethodPost
