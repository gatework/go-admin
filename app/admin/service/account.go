package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/common/account"
)

type Account struct{ service.Service }
type AccountRequest struct {
	Username             string `json:"username"`
	Name                 string `json:"name"`
	Email                string `json:"email"`
	Password             string `json:"password"`
	PasswordConfirmation string `json:"passwordConfirmation"`
	Token                string `json:"token"`
	Code                 string `json:"code"`
	RecoveryCode         string `json:"recoveryCode"`
	Enabled              *bool  `json:"enabled"`
}

func (e *Account) Execute(ctx context.Context, operation string, id int, ip string, r AccountRequest) (any, error) {
	s := account.New(e.Orm.WithContext(ctx))
	if operation != "settings" && operation != "status" && operation != "policy-get" {
		if err := s.TakeLimit("request:"+operation+":"+ip, 10, 5*time.Minute); err != nil {
			return nil, err
		}
	}
	if id > 0 && operation != "status" && operation != "policy-get" {
		if err := s.TakeLimit("account:"+fmt.Sprint(id), 20, 5*time.Minute); err != nil {
			return nil, err
		}
	}
	switch operation {
	case "settings", "policy-get":
		p, err := s.Policy()
		return map[string]any{"enabled": p.Enabled, "emailConfigured": s.Mail != nil && len(s.Key) == 32 && s.BaseURL != "", "mfaConfigured": len(s.Key) == 32}, err
	case "policy-set":
		if r.Enabled == nil {
			return nil, account.ErrInvalid
		}
		return nil, s.SetPolicy(*r.Enabled)
	case "registration-email":
		token, err := s.IssueEmail(ctx, "register", r.Email)
		return map[string]any{"token": token, "expiresIn": 600, "resendAfter": 60}, err
	case "register":
		return nil, s.Register(account.Registration{Username: r.Username, Name: r.Name, Email: r.Email, Password: r.Password, PasswordConfirmation: r.PasswordConfirmation, Token: r.Token, Code: r.Code})
	case "forgot":
		// The response does not reveal whether an address exists or SMTP delivery succeeded.
		if s.Mail == nil || len(s.Key) != 32 || s.BaseURL == "" {
			return nil, account.ErrUnavailable
		}
		_, err := s.IssueEmail(ctx, "reset", r.Email)
		if errors.Is(err, account.ErrLimited) || errors.Is(err, account.ErrInvalid) {
			return nil, err
		}
		if err != nil && e.Log != nil {
			e.Log.Warn("password recovery delivery failed")
		}
		return map[string]any{"expiresIn": 600, "resendAfter": 60}, nil
	case "reset":
		return nil, s.ResetPassword(account.Reset{Email: r.Email, Token: r.Token, Code: r.Code, Password: r.Password, PasswordConfirmation: r.PasswordConfirmation})
	case "status":
		state, err := s.State(id)
		if err != nil {
			return nil, err
		}
		var count int64
		err = e.Orm.Model(&account.Recovery{}).Where("user_id = ?", id).Count(&count).Error
		return map[string]any{"enabled": state.Enabled, "recoveryCodesRemaining": count, "configured": len(s.Key) == 32}, err
	case "setup":
		return s.Setup(id, r.Password)
	case "cancel":
		return nil, s.CancelSetup(id)
	case "enable":
		codes, err := s.Enable(id, r.Code)
		return map[string]any{"recoveryCodes": codes}, err
	case "disable", "regenerate":
		codes, err := s.ManageMFA(id, r.Password, r.Code, r.RecoveryCode, operation == "disable")
		return map[string]any{"recoveryCodes": codes}, err
	}
	return nil, account.ErrInvalid
}
