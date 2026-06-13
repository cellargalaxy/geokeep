package auth_test

import (
	"strings"
	"testing"

	"geokeep/internal/auth"
)

func TestHashPassword_TooShort(t *testing.T) {
	if _, err := auth.HashPassword("short"); err == nil {
		t.Fatal("期望短密码报错，实际成功")
	}
}

func TestHashAndVerify(t *testing.T) {
	pw := strings.Repeat("a", 10)
	h, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := auth.VerifyPassword(h, pw); err != nil {
		t.Fatalf("正确密码验证失败: %v", err)
	}
	if err := auth.VerifyPassword(h, pw+"x"); err == nil {
		t.Fatal("错误密码却验证通过")
	}
}
