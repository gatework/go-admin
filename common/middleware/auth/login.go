package auth

import (
	"gorm.io/gorm"

	"github.com/gatework/motor/common/account"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
)

// Login defines the login contract.
type Login struct {
	ChallengeToken string `json:"challengeToken"`
	TOTPCode       string `json:"totpCode"`
	RecoveryCode   string `json:"recoveryCode"`
	Username       string `form:"UserName" json:"username"`
	Password       string `form:"Password" json:"password"`
	Code           string `form:"Code" json:"code"`
	UUID           string `form:"UUID" json:"uuid"`
}

// GetUser returns the selected data.
func (u *Login) GetUser(tx *gorm.DB) (user account.User, role account.Role, err error) {
	err = tx.Table("sys_user").Where("username = ? AND status = '2' AND deleted_at = 0", u.Username).First(&user).Error
	if err != nil {
		log.NewHelper(log.Default()).Errorf("get user error, %s", err.Error())
		return
	}
	_, err = pkg.CompareHashAndPassword(user.Password, u.Password)
	if err != nil {
		log.NewHelper(log.Default()).Errorf("user login error, %s", err.Error())
		return
	}
	err = tx.Table("sys_role").Where("role_id = ? AND status = '2' AND deleted_at = 0", user.RoleID).First(&role).Error
	if err != nil {
		log.NewHelper(log.Default()).Errorf("get role error, %s", err.Error())
		return
	}
	return
}
