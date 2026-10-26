package account

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"time"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Enrollment struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
	QR     string `json:"qr"`
}

func (s Service) Setup(id int, password string) (Enrollment, error) {
	u, e := s.passwordUser(id, password)
	if e != nil {
		return Enrollment{}, e
	}
	key, e := totp.Generate(totp.GenerateOpts{Issuer: "motor", AccountName: u.Username})
	if e != nil {
		return Enrollment{}, e
	}
	secret, e := s.seal(key.Secret())
	if e != nil {
		return Enrollment{}, e
	}
	if e = s.ensureState(id); e != nil {
		return Enrollment{}, e
	}
	e = s.DB.Transaction(func(tx *gorm.DB) error {
		current, err := s.lockUser(tx, id)
		if err != nil || current.Password != u.Password {
			return ErrInvalid
		}
		r := tx.Model(&State{}).Where("user_id = ? AND enabled = ?", id, false).Updates(map[string]any{"secret": secret, "pending_until": s.now().Add(10 * time.Minute).Unix(), "pending_credential": digest(u.Password), "last_step": 0})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrInvalid
		}
		return nil
	})
	if e != nil {
		return Enrollment{}, e
	}
	img, e := key.Image(240, 240)
	if e != nil {
		return Enrollment{}, e
	}
	var buffer bytes.Buffer
	if e = png.Encode(&buffer, img); e != nil {
		return Enrollment{}, e
	}
	return Enrollment{Secret: key.Secret(), URI: key.URL(), QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())}, nil
}
func (s Service) CancelSetup(id int) error {
	return s.DB.Model(&State{}).Where("user_id = ? AND enabled = ?", id, false).Updates(map[string]any{"secret": "", "pending_until": 0}).Error
}
func (s Service) newRecovery(tx *gorm.DB, id int) ([]string, error) {
	if e := tx.Where("user_id = ?", id).Delete(&Recovery{}).Error; e != nil {
		return nil, e
	}
	codes := make([]string, 10)
	for i := range codes {
		raw, e := randomToken()
		if e != nil {
			return nil, e
		}
		codes[i] = raw[:16]
		if e = tx.Create(&Recovery{Hash: digest(codes[i]), UserID: id}).Error; e != nil {
			return nil, e
		}
	}
	return codes, nil
}
func (s Service) Enable(id int, code string) (codes []string, err error) {
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		current, e := s.lockUser(tx, id)
		if e != nil {
			return ErrInvalid
		}
		var state State
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, id).Error; e != nil {
			return ErrInvalid
		}
		if state.Enabled || state.PendingUntil <= s.now().Unix() || state.PendingCredential != digest(current.Password) {
			return ErrInvalid
		}
		secret, e := s.open(state.Secret)
		if e != nil {
			return e
		}
		step := verifyStep(secret, code, s.now(), state.LastStep)
		if step == 0 {
			return ErrInvalid
		}
		if e = tx.Model(&state).Updates(map[string]any{"enabled": true, "pending_until": 0, "pending_credential": "", "last_step": step, "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
		codes, e = s.newRecovery(tx, id)
		return e
	})
	return
}

// consumeMFA executes under the state row lock; the conditional update/delete also guards replay.
func (s Service) consumeMFA(tx *gorm.DB, state State, code, recovery string) error {
	if !state.Enabled || (code == "") == (recovery == "") {
		return ErrInvalid
	}
	if recovery != "" {
		r := tx.Where("user_id = ? AND hash = ?", state.UserID, digest(recovery)).Delete(&Recovery{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrInvalid
		}
		return nil
	}
	secret, e := s.open(state.Secret)
	if e != nil {
		return e
	}
	step := verifyStep(secret, code, s.now(), state.LastStep)
	if step == 0 {
		return ErrInvalid
	}
	r := tx.Model(&State{}).Where("user_id = ? AND last_step = ?", state.UserID, state.LastStep).Update("last_step", step)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrInvalid
	}
	return nil
}
func (s Service) ManageMFA(id int, password, code, recovery string, disable bool) (codes []string, err error) {
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		current, e := s.lockUser(tx, id)
		if e != nil || bcrypt.CompareHashAndPassword([]byte(current.Password), []byte(password)) != nil {
			return ErrInvalid
		}
		var state State
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, id).Error; e != nil {
			return ErrInvalid
		}
		if e := s.consumeMFA(tx, state, code, recovery); e != nil {
			return e
		}
		if disable {
			if e := tx.Where("user_id = ?", id).Delete(&Recovery{}).Error; e != nil {
				return e
			}
			return tx.Model(&state).Updates(map[string]any{"enabled": false, "secret": "", "last_step": 0, "pending_until": 0, "version": gorm.Expr("version + 1")}).Error
		}
		if e = tx.Model(&state).UpdateColumn("version", gorm.Expr("version + 1")).Error; e != nil {
			return e
		}
		codes, e = s.newRecovery(tx, id)
		return e
	})
	return
}
func (s Service) LoginChallenge(u User, state State) (string, error) {
	token, e := randomToken()
	if e != nil {
		return "", e
	}
	e = s.DB.Create(&Challenge{Hash: digest(token), Kind: "mfa", UserID: u.UserId, Credential: Stamp(s.Key, u.Password, state.Version), ExpiresAt: s.now().Add(5 * time.Minute).Unix()}).Error
	return token, e
}
func (s Service) CompleteLogin(token, code, recovery string) (u User, err error) {
	if len(s.Key) != 32 {
		return u, ErrUnavailable
	}
	if err = s.attempt(token, "mfa"); err != nil {
		return
	}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		var c Challenge
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("hash = ? AND kind = ? AND used = ? AND expires_at > ?", digest(token), "mfa", false, s.now().Unix()).First(&c).Error; e != nil {
			return ErrInvalid
		}
		var userErr error
		u, userErr = s.lockUser(tx, c.UserID)
		if userErr != nil {
			return ErrInvalid
		}
		var state State
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, c.UserID).Error; e != nil {
			return ErrInvalid
		}
		u.SessionVersion = state.Version
		if c.Credential != Stamp(s.Key, u.Password, state.Version) {
			return ErrInvalid
		}
		if e := s.consumeMFA(tx, state, code, recovery); e != nil {
			return e
		}
		r := tx.Model(&Challenge{}).Where("hash = ? AND used = ?", c.Hash, false).Update("used", true)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrInvalid
		}
		return nil
	})
	return
}
