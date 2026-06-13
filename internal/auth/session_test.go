package auth_test

import (
	"strings"
	"testing"

	"geokeep/internal/auth"
)

func TestSession_RoundTrip(t *testing.T) {
	s := auth.NewSigner("0123456789abcdef0123456789abcdef")
	tok, err := s.Issue(42)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("校验失败: %v", err)
	}
	if sess.UserID != 42 {
		t.Fatalf("UserID 错: %d", sess.UserID)
	}
}

func TestSession_BadSignature(t *testing.T) {
	s := auth.NewSigner("0123456789abcdef0123456789abcdef")
	tok, _ := s.Issue(1)
	// 翻转最后一位（'0'<->'1' 等），保证一定造成签名失配
	last := tok[len(tok)-1]
	flip := byte('a')
	if last == flip {
		flip = 'b'
	}
	bad := tok[:len(tok)-1] + string(flip)
	if _, err := s.Verify(bad); err != auth.ErrSessionInvalid {
		t.Fatalf("期望 ErrSessionInvalid，实际 %v", err)
	}
}

func TestSession_DifferentSecret(t *testing.T) {
	a := auth.NewSigner("0123456789abcdef0123456789abcdef")
	b := auth.NewSigner("ffffffffffffffffffffffffffffffff")
	tok, _ := a.Issue(7)
	if _, err := b.Verify(tok); err != auth.ErrSessionInvalid {
		t.Fatalf("期望签名失配，实际 %v", err)
	}
}

func TestSession_Malformed(t *testing.T) {
	s := auth.NewSigner("secret-secret-secret-secret-secret")
	cases := []string{"", "noseparator", strings.Repeat("a", 8) + "." + strings.Repeat("b", 8)}
	for _, c := range cases {
		if _, err := s.Verify(c); err == nil {
			t.Fatalf("畸形输入应失败: %q", c)
		}
	}
}
