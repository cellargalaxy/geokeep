package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SessionCookieName HTTP Cookie 字段名。
const SessionCookieName = "geokeep_sess"

// SessionTTL Session 有效期。
const SessionTTL = 7 * 24 * time.Hour

// ErrSessionInvalid Session 解析或签名校验失败。
var ErrSessionInvalid = errors.New("session 无效")

// ErrSessionExpired Session 过期。
var ErrSessionExpired = errors.New("session 过期")

// Session 是 Cookie 载荷结构。
type Session struct {
	UserID   uint
	IssuedAt int64
	Nonce    string
}

// Signer 提供签名/校验能力；secret 来自 cfg.Secret。
type Signer struct {
	secret []byte
}

// NewSigner 构造签名器。
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// Issue 为指定用户生成 Cookie 载荷字符串。
// 载荷格式：base64url(uid|issued|nonce) + "." + base64url(hmac)
func (s *Signer) Issue(userID uint) (string, error) {
	nonce, err := randNonce()
	if err != nil {
		return "", err
	}
	sess := Session{UserID: userID, IssuedAt: time.Now().Unix(), Nonce: nonce}
	return s.encode(sess), nil
}

// Verify 解析并校验 Cookie 字符串。
func (s *Signer) Verify(token string) (Session, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return Session{}, ErrSessionInvalid
	}
	payload, sig := parts[0], parts[1]
	expectSig := s.sign(payload)
	if !hmac.Equal([]byte(sig), []byte(expectSig)) {
		return Session{}, ErrSessionInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return Session{}, ErrSessionInvalid
	}
	pieces := strings.SplitN(string(raw), "|", 3)
	if len(pieces) != 3 {
		return Session{}, ErrSessionInvalid
	}
	uid, err := strconv.ParseUint(pieces[0], 10, 64)
	if err != nil {
		return Session{}, ErrSessionInvalid
	}
	issued, err := strconv.ParseInt(pieces[1], 10, 64)
	if err != nil {
		return Session{}, ErrSessionInvalid
	}
	if time.Now().Unix()-issued > int64(SessionTTL/time.Second) {
		return Session{}, ErrSessionExpired
	}
	return Session{UserID: uint(uid), IssuedAt: issued, Nonce: pieces[2]}, nil
}

func (s *Signer) encode(sess Session) string {
	body := fmt.Sprintf("%d|%d|%s", sess.UserID, sess.IssuedAt, sess.Nonce)
	payload := base64.RawURLEncoding.EncodeToString([]byte(body))
	return payload + "." + s.sign(payload)
}

func (s *Signer) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randNonce() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
