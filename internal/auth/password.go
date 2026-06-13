package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLen 最小密码长度（含），用于注册/重置时校验。
	MinPasswordLen = 10
	// bcryptCost 略高于默认（10），单用户场景下计算开销可忽略。
	bcryptCost = 12
)

// ErrPasswordTooShort 密码长度不足。
var ErrPasswordTooShort = errors.New("密码长度不足")

// HashPassword 生成 bcrypt 哈希；明文长度不足直接返回错误。
func HashPassword(plain string) (string, error) {
	if len([]rune(plain)) < MinPasswordLen {
		return "", ErrPasswordTooShort
	}
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword 比对哈希与明文。
func VerifyPassword(hash, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}
