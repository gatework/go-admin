package account

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/pquerna/otp/totp"
)

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func digest(value string) string { s := sha256.Sum256([]byte(value)); return hex.EncodeToString(s[:]) }
func keyed(key, value string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}

// Stamp binds a JWT to the current password and MFA version, never exposing a password hash.
func Stamp(key, password string, version int64) string {
	return keyed(key, fmt.Sprintf("session:%s:%d", password, version))
}
func (s Service) aead() (cipher.AEAD, error) {
	if len(s.Key) != 32 {
		return nil, ErrUnavailable
	}
	b, e := aes.NewCipher([]byte(s.Key))
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(b)
}
func (s Service) seal(value string) (string, error) {
	a, e := s.aead()
	if e != nil {
		return "", e
	}
	n := make([]byte, a.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(a.Seal(n, n, []byte(value), []byte("motor-account-v1"))), nil
}
func (s Service) open(value string) (string, error) {
	a, e := s.aead()
	if e != nil {
		return "", e
	}
	b, e := base64.RawStdEncoding.DecodeString(value)
	if e != nil || len(b) < a.NonceSize() {
		return "", errors.New("invalid encrypted secret")
	}
	p, e := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte("motor-account-v1"))
	return string(p), e
}
func otpCode(secret string, now time.Time) (string, error) { return totp.GenerateCode(secret, now) }
func verifyStep(secret, code string, now time.Time, last int64) int64 {
	if len(code) != 6 {
		return 0
	}
	for _, offset := range []int64{0, -1, 1} {
		step := now.Unix()/30 + offset
		if step <= last {
			continue
		}
		want, e := otpCode(secret, time.Unix(step*30, 0))
		if e == nil && hmac.Equal([]byte(want), []byte(code)) {
			return step
		}
	}
	return 0
}
