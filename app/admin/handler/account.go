package handler

import (
	"errors"
	"net/http"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/common/account"
)

// AccountPublic handles public registration and password recovery.
// @Summary Account registration, recovery and MFA
// @Tags Account
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/account/settings [get]
// @Router /api/v1/account/registration/email [post]
// @Router /api/v1/account/register [post]
// @Router /api/v1/account/password/forgot [post]
// @Router /api/v1/account/password/reset [post]
func AccountPublic(operation string) gin.HandlerFunc { return accountAction(operation, false) }

// AccountMFA manages the authenticated caller's second factor.
// @Summary Manage own MFA
// @Tags Account
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Security Bearer
// @Router /api/v1/account/mfa [get]
// @Router /api/v1/account/mfa/setup [post]
// @Router /api/v1/account/mfa/cancel [post]
// @Router /api/v1/account/mfa/enable [post]
// @Router /api/v1/account/mfa/disable [post]
// @Router /api/v1/account/mfa/regenerate [post]
func AccountMFA(operation string) gin.HandlerFunc { return accountAction(operation, false) }

// AccountPolicy restricts registration policy changes to administrators.
// @Summary Registration policy
// @Tags Account
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Security Bearer
// @Router /api/v1/account/registration-policy [get]
// @Router /api/v1/account/registration-policy [put]
func AccountPolicy(operation string) gin.HandlerFunc { return accountAction(operation, true) }

func accountAction(operation string, admin bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		e, err := request.New(c)
		s := service.Account{
			Service: e.Service()}
		if err != nil {
			e.Error(500, err, "account service unavailable")
			return
		}
		if admin && jwt.ExtractClaims(c)[jwt.RoleKey] != "admin" {
			e.Error(403, account.InvalidErr, "administrator required")
			return
		}
		var req service.AccountRequest
		if c.Request.Method != http.MethodGet {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
			if err := c.ShouldBindJSON(&req); err != nil {
				e.Error(400, err, "invalid request")
				return
			}
		}
		result, err := s.Execute(c.Request.Context(), operation, user.ID(c), account.PeerIP(c.Request.RemoteAddr), req)
		if err != nil {
			code := 400
			switch {
			case errors.Is(err, account.LimitedErr):
				code = 429
			case errors.Is(err, account.DisabledErr):
				code = 403
			case errors.Is(err, account.UnavailableErr):
				code = 503
			}
			// Never expose driver, encryption or SMTP errors to a public caller.
			msg := "account operation failed"
			for _, known := range []error{account.InvalidErr, account.LimitedErr, account.DisabledErr, account.UnavailableErr, account.PasswordErr, account.IdentityErr} {
				if errors.Is(err, known) {
					msg = known.Error()
				}
			}
			e.Error(code, err, msg)
			return
		}
		e.OK(result, "success")
	}
}
