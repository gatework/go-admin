package account

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IssueEmail returns a registration challenge, but never returns a password-reset token.
func (s Service) IssueEmail(ctx context.Context, kind, email string) (string, error) {
	if kind != "register" && kind != "reset" {
		return "", ErrInvalid
	}
	email, e := NormalizeEmail(email)
	if e != nil {
		return "", e
	}
	if !s.configured() {
		return "", ErrUnavailable
	}
	if kind == "register" {
		p, e := s.Policy()
		if e != nil {
			return "", e
		}
		if !p.Enabled {
			return "", ErrDisabled
		}
	}
	if e = s.TakeLimit("email:"+kind+":"+email, 1, time.Minute); e != nil {
		return "", e
	}
	token, e := randomToken()
	if e != nil {
		return "", e
	}
	var users []User
	if e = s.DB.Where("LOWER(email) = ? AND deleted_at = 0", email).Limit(2).Find(&users).Error; e != nil {
		return "", e
	}
	if kind == "register" && len(users) > 0 {
		return token, nil
	}
	if kind == "reset" && (len(users) != 1 || users[0].Status != "2") {
		return "", nil
	}
	n, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return "", e
	}
	code := fmt.Sprintf("%06d", n.Int64())
	c := Challenge{Hash: digest(token), Kind: kind, Email: email, CodeHash: keyed(s.Key, token+":"+code), ExpiresAt: s.now().Add(10 * time.Minute).Unix()}
	title := "motor · Email verification"
	body := fmt.Sprintf("Your verification code is %s. It expires in 10 minutes.\nIf you did not request this, ignore this email.\n", code)
	if kind == "reset" {
		c.UserID = users[0].UserId
		c.Credential = digest(users[0].Password)
		title = "motor · Password reset"
		body = fmt.Sprintf("Reset your password: %s/#/reset-password?token=%s&email=%s\nVerification code: %s\nThis link and code expire in 10 minutes and can be used once. If you did not request this, ignore this email.\n", strings.TrimRight(s.BaseURL, "/"), url.QueryEscape(token), url.QueryEscape(email), code)
	}
	if e = s.DB.Create(&c).Error; e != nil {
		return "", e
	}
	if e = s.Mail.Send(ctx, email, title, body); e != nil {
		s.DB.Where("hash = ?", c.Hash).Delete(&Challenge{})
		return "", e
	}
	// A successfully delivered replacement invalidates earlier messages.
	if e = s.DB.Where("kind = ? AND email = ? AND hash <> ?", kind, email, c.Hash).Delete(&Challenge{}).Error; e != nil {
		return "", e
	}
	if kind == "reset" {
		return "", nil
	}
	return token, nil
}

type Registration struct {
	Username             string `json:"username"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
	Token                string `json:"token"`
	Code                 string `json:"code"`
}

func (s Service) Register(r Registration) error {
	var err error
	r.Username = strings.TrimSpace(r.Username)
	r.Email, err = NormalizeEmail(r.Email)
	if err != nil || !usernamePattern.MatchString(r.Username) || len(r.Name) > 128 || r.Name == "" || r.Password != r.PasswordConfirmation {
		return ErrInvalid
	}
	if err = Password(r.Password); err != nil {
		return err
	}
	if err = s.attempt(r.Token, "register"); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var p Policy
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; e != nil {
			return e
		}
		if !p.Enabled {
			return ErrDisabled
		}
		var count int64
		if e := tx.Model(&User{}).Where("LOWER(username) = ? OR LOWER(email) = ?", strings.ToLower(r.Username), r.Email).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			return ErrIdentity
		}
		if _, e := s.consumeEmail(tx, r.Token, r.Code, "register", r.Email); e != nil {
			return e
		}
		var role Role
		if e := tx.Where("role_key = ? AND deleted_at = 0", "registered").First(&role).Error; e == gorm.ErrRecordNotFound {
			role = Role{RoleKey: "registered", RoleName: "注册用户", Status: "2", DataScope: "5"}
			if e = tx.Create(&role).Error; e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
		if role.Admin || role.RoleKey == "admin" || role.Status != "2" {
			return ErrDisabled
		}
		// Never bind privileged fields from the request. This role starts with no administrative grants.
		u := User{Username: r.Username, NickName: r.Name, Email: r.Email, Password: string(hash), Status: "2", RoleId: role.RoleId, DeptId: 1, PostId: 1}
		return tx.Create(&u).Error
	})
}

type Reset struct {
	Token                string `json:"token"`
	Code                 string `json:"code"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
}

func (s Service) ResetPassword(r Reset) error {
	email, e := NormalizeEmail(r.Email)
	if e != nil || r.Password != r.PasswordConfirmation {
		return ErrInvalid
	}
	if e = Password(r.Password); e != nil {
		return e
	}
	if e = s.attempt(r.Token, "reset"); e != nil {
		return e
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	return s.DB.Transaction(func(tx *gorm.DB) error {
		c, e := s.consumeEmail(tx, r.Token, r.Code, "reset", email)
		if e != nil {
			return e
		}
		var u User
		if e = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND LOWER(email) = ? AND deleted_at = 0 AND status = ?", c.UserID, email, "2").First(&u).Error; e != nil {
			return ErrInvalid
		}
		if digest(u.Password) != c.Credential {
			return ErrInvalid
		}
		return tx.Model(&User{}).Where("user_id = ?", u.UserId).Update("password", string(hash)).Error
	})
}
