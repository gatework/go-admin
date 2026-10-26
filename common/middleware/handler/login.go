package handler

import (
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"gorm.io/gorm"
)

type Login struct {
	ChallengeToken string `json:"challengeToken"`
	TOTPCode       string `json:"totpCode"`
	RecoveryCode   string `json:"recoveryCode"`
	Username       string `form:"UserName" json:"username"`
	Password       string `form:"Password" json:"password"`
	Code           string `form:"Code" json:"code"`
	UUID           string `form:"UUID" json:"uuid"`
}

func (u *Login) GetUser(tx *gorm.DB) (user SysUser, role SysRole, err error) {
	err = tx.Table("sys_user").Where("username = ? AND status = '2' AND deleted_at = 0", u.Username).First(&user).Error
	if err != nil {
		log.Errorf("get user error, %s", err.Error())
		return
	}
	_, err = pkg.CompareHashAndPassword(user.Password, u.Password)
	if err != nil {
		log.Errorf("user login error, %s", err.Error())
		return
	}
	err = tx.Table("sys_role").Where("role_id = ? AND status = '2' AND deleted_at = 0", user.RoleId).First(&role).Error
	if err != nil {
		log.Errorf("get role error, %s", err.Error())
		return
	}
	return
}
