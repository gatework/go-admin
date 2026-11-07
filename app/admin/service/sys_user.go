package service

import (
	"errors"
	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	"strings"

	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"

	"go-admin/common/actions"
	cDto "go-admin/common/dto"
)

type SysUser struct {
	service.Service
}

// GetPage 获取SysUser列表
func (e *SysUser) GetPage(c *dto.SysUserGetPageReq, p *actions.DataPermission, list *[]models.SysUser, count *int64) error {
	var err error
	var data models.SysUser

	err = e.Orm.Debug().Preload("Dept").
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
			actions.Permission(data.TableName(), p),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// GetSelf 获取调用者自己的 SysUser 对象，不套数据权限
//
// The data scope answers "whose rows may this user see"; the caller here is
// reading their own, and the id comes from the token, so there is nothing left
// for a scope to restrict. Applying one is not a stricter version of this
// query - it is a broken one. DataScopeSelf matches on create_by, and a user
// account is created by whoever added it, so a scoped self-read would fail for
// every user who did not create their own account.
//
// GetProfile has always read the same row this way, with no scope at all.
func (e *SysUser) GetSelf(d *dto.SysUserById, model *models.SysUser) error {
	err := e.Orm.First(model, d.GetId()).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Get 获取SysUser对象
func (e *SysUser) Get(d *dto.SysUserById, p *actions.DataPermission, model *models.SysUser) error {
	var data models.SysUser

	err := e.Orm.Model(&data).Debug().
		Scopes(
			actions.Permission(data.TableName(), p),
		).
		First(model, d.GetId()).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Insert 创建SysUser对象
func (e *SysUser) Insert(c *dto.SysUserInsertReq) error {
	var err error
	var data models.SysUser
	var i int64
	err = e.Orm.Model(&data).Where("username = ?", c.Username).Count(&i).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if i > 0 {
		err := errors.New("用户名已存在！")
		e.Log.Errorf("db error: %s", err)
		return err
	}
	c.Generate(&data)
	err = e.Orm.Create(&data).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Update 修改SysUser对象
//
// callerId is who is asking, not who SetUpdateBy recorded - that field only
// says who to blame, it never constrained who could be edited. When the
// target is the caller themselves, roleId/deptId/status are kept at whatever
// the database already has no matter what the request body carries: this is
// the personal-center screen's route (see CasbinExclude in settings.go, and
// the check in the API handler ahead of this call), and letting a caller
// grant themselves a different role or department through it would be a
// privilege escalation the exclusion was never meant to open.
func (e *SysUser) Update(c *dto.SysUserUpdateReq, p *actions.DataPermission, callerId int) error {
	var err error
	var model models.SysUser
	if callerId <= 0 || c.UserId <= 0 {
		return errors.New("invalid user identity")
	}
	query := e.Orm
	if c.UserId != callerId {
		query = query.Scopes(actions.Permission(model.TableName(), p))
	}
	db := query.First(&model, c.GetId())
	if err = db.Error; err != nil {
		e.Log.Errorf("Service UpdateSysUser error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")

	}
	if model.UserId == callerId {
		c.RoleId = model.RoleId
		c.DeptId = model.DeptId
		c.Status = model.Status
	}
	c.Generate(&model)
	model.UpdateBy = callerId
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		omit := []string{"password", "salt", "auth_version", "create_by", "created_at", "deleted_at", "user_id"}
		self := model.UserId == callerId
		if self {
			// A concurrent administrator edit must not be undone by a self-profile save.
			omit = append(omit, "role_id", "dept_id", "status")
		} else if model.Status != "" && model.Status != "1" && model.Status != "2" {
			return errors.New("invalid user status")
		}
		scope := func(db *gorm.DB) *gorm.DB {
			if self {
				return db
			}
			return db.Scopes(actions.Permission(model.TableName(), p))
		}
		// Compare effective struct updates against the current row, not the earlier
		// read. A value restored later must never restore an old credential stamp.
		var changed []string
		var values []any
		add := func(column string, value any) {
			changed = append(changed, "("+column+" <> ? OR "+column+" IS NULL)")
			values = append(values, value)
		}
		if model.Username != "" {
			add("username", model.Username)
		}
		if !self {
			if model.RoleId != 0 {
				add("role_id", model.RoleId)
			}
			if model.DeptId != 0 {
				add("dept_id", model.DeptId)
			}
			if model.Status != "" {
				add("status", model.Status)
			}
		}
		if len(changed) != 0 {
			if err := scope(tx.Table(model.TableName())).Where("user_id = ? AND deleted_at = 0", model.UserId).
				Where(strings.Join(changed, " OR "), values...).UpdateColumn("auth_version", gorm.Expr("auth_version + ?", 1)).Error; err != nil {
				return err
			}
		}
		update := scope(tx.Model(&model)).Where("user_id = ?", model.UserId).Omit(omit...).Updates(&model)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return errors.New("update userinfo error")
		}
		return nil
	})
}

// UpdateAvatar updates the authenticated caller's avatar. The API supplies the
// token identity; account ownership does not depend on who created that user.
// The permission argument is retained for source compatibility.
func (e *SysUser) UpdateAvatar(c *dto.UpdateSysUserAvatarReq, _ *actions.DataPermission) error {
	if c.UserId <= 0 {
		return errors.New("invalid user identity")
	}
	result := e.Orm.Model(&models.SysUser{}).Where("user_id = ?", c.UserId).
		Updates(map[string]any{"avatar": c.Avatar, "update_by": c.UserId})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	return nil
}

// UpdateStatus 更新用户状态
func (e *SysUser) UpdateStatus(c *dto.UpdateSysUserStatusReq, p *actions.DataPermission) error {
	if c.UserId <= 0 || (c.Status != "1" && c.Status != "2") {
		return errors.New("invalid user status")
	}
	var model models.SysUser
	result := e.Orm.Table(model.TableName()).Scopes(actions.Permission(model.TableName(), p)).
		Where("user_id = ? AND deleted_at = 0", c.UserId).
		Updates(map[string]interface{}{
			"auth_version": gorm.Expr("CASE WHEN status <> ? THEN auth_version + 1 ELSE auth_version END", c.Status),
			"status":       c.Status, "update_by": c.UpdateBy,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL may report zero changed rows for an idempotent status request.
		var count int64
		if err := e.Orm.Model(&models.SysUser{}).Scopes(actions.Permission(model.TableName(), p)).Where("user_id = ?", c.UserId).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("无权更新该数据")
		}
	}
	return nil
}

// ResetPwd 重置用户密码
func (e *SysUser) ResetPwd(c *dto.ResetSysUserPwdReq, p *actions.DataPermission) error {
	var err error
	var model models.SysUser
	db := e.Orm.Scopes(
		actions.Permission(model.TableName(), p),
	).First(&model, c.GetId())
	if err = db.Error; err != nil {
		e.Log.Errorf("At Service ResetSysUserPwd error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	c.Generate(&model)
	model.UpdateBy = c.UpdateBy
	result := e.Orm.Model(&model).Scopes(actions.Permission(model.TableName(), p)).
		Where("user_id = ?", model.UserId).Select("Password", "Salt", "UpdateBy").Updates(&model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("无权更新该数据")
	}

	return nil
}

// Remove 删除SysUser
func (e *SysUser) Remove(c *dto.SysUserById, p *actions.DataPermission) error {
	var err error
	var data models.SysUser

	db := e.Orm.Model(&data).
		Scopes(
			actions.Permission(data.TableName(), p),
		).Delete(&data, c.GetId())
	if err = db.Error; err != nil {
		e.Log.Errorf("Error found in  RemoveSysUser : %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权删除该数据")
	}
	return nil
}

// UpdatePwd 修改SysUser对象密码
// id must be the authenticated caller's identity, not a request-body user ID.
// The permission argument is retained for source compatibility.
func (e *SysUser) UpdatePwd(id int, oldPassword, newPassword string, _ *actions.DataPermission) error {
	var err error

	if id <= 0 || oldPassword == "" || newPassword == "" {
		return errors.New("user identity and passwords are required")
	}
	c := &models.SysUser{}

	err = e.Orm.Model(c).
		Select("UserId", "Password", "Salt").
		First(c, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("无权更新该数据")
		}
		e.Log.Errorf("db error: %s", err)
		return err
	}
	var ok bool
	ok, err = pkg.CompareHashAndPassword(c.Password, oldPassword)
	if err != nil {
		e.Log.Errorf("CompareHashAndPassword error, %s", err.Error())
		return err
	}
	if !ok {
		err = errors.New("incorrect Password")
		e.Log.Warnf("user[%d] %s", id, err.Error())
		return err
	}
	c.Password = newPassword
	db := e.Orm.Model(c).Where("user_id = ?", id).
		Select("Password", "Salt").
		Updates(c)
	if err = db.Error; err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		err = errors.New("set password error")
		log.Warnf("db update error")
		return err
	}
	return nil
}

func (e *SysUser) GetProfile(c *dto.SysUserById, user *models.SysUser, roles *[]models.SysRole, posts *[]models.SysPost) error {
	err := e.Orm.Preload("Dept").First(user, c.GetId()).Error
	if err != nil {
		return err
	}
	err = e.Orm.Find(roles, user.RoleId).Error
	if err != nil {
		return err
	}
	err = e.Orm.Find(posts, user.PostIds).Error
	if err != nil {
		return err
	}

	return nil
}
