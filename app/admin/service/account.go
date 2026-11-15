package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/account"
)

// Account runs self-service registration, password recovery and second-factor
// management on behalf of the caller.
type Account struct{ service.Service }

// AccountOperation is one account action as a route invokes it: the caller's
// user ID (0 when anonymous) and the request body.
type AccountOperation func(ctx context.Context, e *Account, s account.Service, userID int, r dto.AccountRequest) (any, error)

// Run applies the rate limits for operation, then runs it. Every operation
// that sends mail, checks a credential or changes state is limited per client
// address; an authenticated one also per account. Reads are not limited.
func (e *Account) Run(ctx context.Context, name string, userID int, clientIP string, r dto.AccountRequest, op AccountOperation) (any, error) {
	s := account.New(e.DB.WithContext(ctx))
	if limited := name != "settings" && name != "status" && name != "policy-get"; limited {
		if err := s.TakeLimit("request:"+name+":"+clientIP, 10, 5*time.Minute); err != nil {
			return nil, err
		}
		if userID > 0 {
			if err := s.TakeLimit("account:"+fmt.Sprint(userID), 20, 5*time.Minute); err != nil {
				return nil, err
			}
		}
	}
	return op(ctx, e, s, userID, r)
}

// AccountSettings reports which self-service features are available.
func AccountSettings(_ context.Context, _ *Account, s account.Service, _ int, _ dto.AccountRequest) (any, error) {
	p, err := s.Policy()
	return map[string]any{"enabled": p.Enabled, "emailConfigured": s.Mail != nil && len(s.Key) == 32 && s.BaseURL != "", "mfaConfigured": len(s.Key) == 32}, err
}

// SetRegistrationPolicy turns self-registration on or off.
func SetRegistrationPolicy(_ context.Context, _ *Account, s account.Service, _ int, r dto.AccountRequest) (any, error) {
	if r.Enabled == nil {
		return nil, account.InvalidErr
	}
	return nil, s.SetPolicy(*r.Enabled)
}

// SendRegistrationEmail mails a registration code.
func SendRegistrationEmail(ctx context.Context, _ *Account, s account.Service, _ int, r dto.AccountRequest) (any, error) {
	token, err := s.IssueEmail(ctx, "register", r.Email)
	return map[string]any{"token": token, "expiresIn": 600, "resendAfter": 60}, err
}

// Register creates an account from a verified registration code.
func Register(_ context.Context, _ *Account, s account.Service, _ int, r dto.AccountRequest) (any, error) {
	return nil, s.Register(account.Registration{Username: r.Username, Name: r.Name, Email: r.Email, Password: r.Password, PasswordConfirmation: r.PasswordConfirmation, Token: r.Token, Code: r.Code})
}

// SendRecoveryEmail mails a password-reset code. The answer does not reveal
// whether the address exists or whether delivery succeeded.
func SendRecoveryEmail(ctx context.Context, e *Account, s account.Service, _ int, r dto.AccountRequest) (any, error) {
	if s.Mail == nil || len(s.Key) != 32 || s.BaseURL == "" {
		return nil, account.UnavailableErr
	}
	_, err := s.IssueEmail(ctx, "reset", r.Email)
	if errors.Is(err, account.LimitedErr) || errors.Is(err, account.InvalidErr) {
		return nil, err
	}
	if err != nil && e.Logger != nil {
		e.Logger.Warn("password recovery delivery failed")
	}
	return map[string]any{"expiresIn": 600, "resendAfter": 60}, nil
}

// ResetPassword sets a new password from a recovery code.
func ResetPassword(_ context.Context, _ *Account, s account.Service, _ int, r dto.AccountRequest) (any, error) {
	return nil, s.ResetPassword(account.Reset{Email: r.Email, Token: r.Token, Code: r.Code, Password: r.Password, PasswordConfirmation: r.PasswordConfirmation})
}

// MFAStatus reports the caller's second-factor state.
func MFAStatus(ctx context.Context, e *Account, s account.Service, userID int, _ dto.AccountRequest) (any, error) {
	state, err := s.State(userID)
	if err != nil {
		return nil, err
	}
	var count int64
	err = e.DB.WithContext(ctx).Model(&account.Recovery{}).Where("user_id = ?", userID).Count(&count).Error
	return map[string]any{"enabled": state.Enabled, "recoveryCodesRemaining": count, "configured": len(s.Key) == 32}, err
}

// SetupMFA starts enrolling a second factor.
func SetupMFA(_ context.Context, _ *Account, s account.Service, userID int, r dto.AccountRequest) (any, error) {
	return s.Setup(userID, r.Password)
}

// CancelMFASetup abandons an enrolment in progress.
func CancelMFASetup(_ context.Context, _ *Account, s account.Service, userID int, _ dto.AccountRequest) (any, error) {
	return nil, s.CancelSetup(userID)
}

// EnableMFA confirms an enrolment and returns recovery codes.
func EnableMFA(_ context.Context, _ *Account, s account.Service, userID int, r dto.AccountRequest) (any, error) {
	codes, err := s.Enable(userID, r.Code)
	return map[string]any{"recoveryCodes": codes}, err
}

// DisableMFA removes the second factor.
func DisableMFA(_ context.Context, _ *Account, s account.Service, userID int, r dto.AccountRequest) (any, error) {
	codes, err := s.ManageMFA(userID, r.Password, r.Code, r.RecoveryCode, true)
	return map[string]any{"recoveryCodes": codes}, err
}

// RegenerateRecoveryCodes replaces the recovery codes.
func RegenerateRecoveryCodes(_ context.Context, _ *Account, s account.Service, userID int, r dto.AccountRequest) (any, error) {
	codes, err := s.ManageMFA(userID, r.Password, r.Code, r.RecoveryCode, false)
	return map[string]any{"recoveryCodes": codes}, err
}
