package handler

import (
	"errors"
	"net/http"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/account"
)

// AccountPublic serves an anonymous registration or recovery operation.
func AccountPublic(name string, op service.AccountOperation) gin.HandlerFunc {
	return accountAction(name, false, op)
}

// AccountMFA serves an operation on the authenticated caller's second factor.
func AccountMFA(name string, op service.AccountOperation) gin.HandlerFunc {
	return accountAction(name, false, op)
}

// AccountPolicy serves a registration policy operation; administrators only.
func AccountPolicy(name string, op service.AccountOperation) gin.HandlerFunc {
	return accountAction(name, true, op)
}

func accountAction(name string, admin bool, op service.AccountOperation) gin.HandlerFunc {
	return func(c *gin.Context) {
		e, err := request.New(c)
		s := service.Account{Service: e.Service()}
		if err != nil {
			e.Error(500, nil, "account service unavailable")
			return
		}
		if admin && !account.IsAdmin(user.RoleName(c)) {
			e.Error(403, nil, "administrator required")
			return
		}
		var req dto.AccountRequest
		if c.Request.Method != http.MethodGet {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
			if err := c.ShouldBindJSON(&req); err != nil {
				e.Error(400, nil, "invalid request")
				return
			}
		}
		result, err := s.Run(c.Request.Context(), name, user.ID(c), c.ClientIP(), req, op)
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
			e.Error(code, nil, msg)
			return
		}
		e.OK(result, "success")
	}
}
