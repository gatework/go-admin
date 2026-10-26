package account

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalid     = errors.New("invalid or expired credentials")
	ErrLimited     = errors.New("too many attempts; try later")
	ErrDisabled    = errors.New("registration is disabled")
	ErrUnavailable = errors.New("account delivery or encryption is not configured")
	ErrPassword    = errors.New("password must contain 12 to 72 bytes")
	ErrIdentity    = errors.New("username or email is unavailable")
)

type Mailer interface {
	Send(context.Context, string, string, string) error
}
type Service struct {
	DB      *gorm.DB
	Key     string
	Mail    Mailer
	BaseURL string
	Now     func() time.Time
}

func New(db *gorm.DB) Service {
	key, _ := base64.StdEncoding.DecodeString(os.Getenv("ACCOUNT_SECURITY_KEY"))
	return Service{DB: db, Key: string(key), Mail: SMTPFromEnv(), BaseURL: os.Getenv("ACCOUNT_PUBLIC_URL")}
}
func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s Service) configured() bool { return len(s.Key) == 32 && s.Mail != nil && s.BaseURL != "" }
func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	a, e := mail.ParseAddress(value)
	if e != nil || a.Address != value || len(value) > 128 {
		return "", ErrInvalid
	}
	return value, nil
}
func Password(value string) error {
	if len(value) < 12 || len(value) > 72 {
		return ErrPassword
	}
	return nil
}
func (s Service) Policy() (Policy, error) {
	var p Policy
	err := s.DB.First(&p, 1).Error
	return p, err
}
func (s Service) SetPolicy(enabled bool) error {
	return s.DB.Model(&Policy{}).Where("id = 1").Update("enabled", enabled).Error
}
func (s Service) State(id int) (State, error) {
	var v State
	err := s.DB.Where("user_id = ?", id).Find(&v).Error
	v.UserID = id
	return v, err
}
func (s Service) User(id int) (User, error) {
	var u User
	e := s.DB.Where("user_id = ? AND deleted_at = 0 AND status = ?", id, "2").First(&u).Error
	return u, e
}
func (s Service) ensureState(id int) error {
	return s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&State{UserID: id}).Error
}

// Limit is shared across server processes and uses a conditional atomic increment.
func (s Service) TakeLimit(scope string, max int, window time.Duration) error {
	now := s.now().Unix()
	key := digest(scope)
	row := Limit{Key: key, ExpiresAt: now + int64(window.Seconds())}
	if e := s.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
		return e
	}
	if e := s.DB.Model(&Limit{}).Where("key = ? AND expires_at <= ?", key, now).Updates(map[string]any{"count": 0, "expires_at": row.ExpiresAt}).Error; e != nil {
		return e
	}
	result := s.DB.Model(&Limit{}).Where("key = ? AND count < ?", key, max).UpdateColumn("count", gorm.Expr("count + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLimited
	}
	return nil
}
func (s Service) passwordUser(id int, password string) (User, error) {
	u, e := s.User(id)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) != nil {
		return User{}, ErrInvalid
	}
	return u, nil
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$`)

func (s Service) consumeEmail(tx *gorm.DB, token, code, kind, email string) (Challenge, error) {
	var c Challenge
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("hash = ? AND kind = ? AND email = ?", digest(token), kind, email).First(&c).Error
	if err != nil || c.Used || c.ExpiresAt <= s.now().Unix() || c.Attempts > 5 {
		return c, ErrInvalid
	}
	if keyed(s.Key, token+":"+code) != c.CodeHash {
		return c, ErrInvalid
	}
	result := tx.Model(&Challenge{}).Where("hash = ? AND used = ?", c.Hash, false).Update("used", true)
	if result.Error != nil {
		return c, result.Error
	}
	if result.RowsAffected != 1 {
		return c, ErrInvalid
	}
	return c, nil
}

// Failed verification attempts persist even when the following business transaction rolls back.
func (s Service) attempt(token, kind string) error {
	r := s.DB.Model(&Challenge{}).Where("hash = ? AND kind = ? AND used = ? AND attempts < ? AND expires_at > ?", digest(token), kind, false, 5, s.now().Unix()).UpdateColumn("attempts", gorm.Expr("attempts + 1"))
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrInvalid
	}
	return nil
}

func (s Service) lockUser(tx *gorm.DB, id int) (User, error) {
	var u User
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND status = ? AND deleted_at = 0", id, "2").First(&u).Error
	return u, e
}

// PeerIP ignores forwarded headers unless a deployment configures a separate trusted edge limiter.
func PeerIP(remote string) string {
	host, _, e := net.SplitHostPort(remote)
	if e != nil {
		return remote
	}
	return host
}
