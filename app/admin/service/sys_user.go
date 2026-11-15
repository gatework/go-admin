package service

import (
	"context"
	"errors"
	"strings"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"

	"gorm.io/gorm"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/common/actions"
	cDto "github.com/gatework/motor/common/dto"
)

// User coordinates administration queries and mutations.
type User struct {
	service.Service
}

// List 获取SysUser列表
func (e *User) List(ctx context.Context, c *dto.UserSearch, p *actions.DataPermission, list *[]models.SysUser, count *int64) error {
	var err error
	var data models.SysUser

	err = e.DB.WithContext(ctx).Preload("Dept").
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
			actions.Permission(data.TableName(), p),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Self 获取调用者自己的 SysUser 对象，不套数据权限
//
// The data scope answers "whose rows may this user see"; the caller here is
// reading their own, and the id comes from the token, so there is nothing left
// for a scope to restrict. Applying one is not a stricter version of this
// query - it is a broken one. DataScopeSelf matches on create_by, and a user
// account is created by whoever added it, so a scoped self-read would fail for
// every user who did not create their own account.
//
// GetProfile has always read the same row this way, with no scope at all.
func (e *User) Self(ctx context.Context, d *dto.UserByID, model *models.SysUser) error {
	err := e.DB.WithContext(ctx).First(model, d.GetID()).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Get 获取SysUser对象
func (e *User) Get(ctx context.Context, d *dto.UserByID, p *actions.DataPermission, model *models.SysUser) error {
	var data models.SysUser

	err := e.DB.WithContext(ctx).Model(&data).
		Scopes(
			actions.Permission(data.TableName(), p),
		).
		First(model, d.GetID()).Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Create 创建SysUser对象
func (e *User) Create(ctx context.Context, c *dto.UserCreate) error {
	var err error
	var data models.SysUser
	var i int64
	err = e.DB.WithContext(ctx).Model(&data).Where("username = ?", c.Username).Count(&i).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	if i > 0 {
		err := errors.New("用户名已存在！")
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	c.Generate(&data)
	err = e.DB.WithContext(ctx).Create(&data).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
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
func (e *User) Update(ctx context.Context, c *dto.UserUpdate, p *actions.DataPermission, callerID int) error {
	var err error
	var model models.SysUser
	if callerID <= 0 || c.UserID <= 0 {
		return errors.New("invalid user identity")
	}
	query := e.DB.WithContext(ctx)
	if c.UserID != callerID {
		query = query.Scopes(actions.Permission(model.TableName(), p))
	}
	db := query.First(&model, c.GetID())
	if err = db.Error; err != nil {
		e.Logger.Errorf("updating user: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")

	}
	if model.UserID == callerID {
		c.RoleID = model.RoleID
		c.DeptID = model.DeptID
		c.Status = model.Status
	}
	c.Generate(&model)
	model.UpdateBy = callerID
	return e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return e.persistUserUpdate(tx, model, p, model.UserID == callerID)
	})
}

// UpdateAvatar updates the authenticated caller's avatar. The API supplies the
// token identity; account ownership does not depend on who created that user.
// The permission argument is retained for source compatibility.
func (e *User) UpdateAvatar(ctx context.Context, c *dto.AvatarUpdate, _ *actions.DataPermission) error {
	if c.UserID <= 0 {
		return errors.New("invalid user identity")
	}
	result := e.DB.WithContext(ctx).Model(&models.SysUser{}).Where("user_id = ?", c.UserID).
		Updates(map[string]any{"avatar": c.Avatar, "update_by": c.UserID})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	return nil
}

// UpdateStatus 更新用户状态
func (e *User) UpdateStatus(ctx context.Context, c *dto.UserStatusUpdate, p *actions.DataPermission) error {
	if c.UserID <= 0 || (c.Status != "1" && c.Status != "2") {
		return errors.New("invalid user status")
	}
	var model models.SysUser
	result := e.DB.WithContext(ctx).Table(model.TableName()).Scopes(actions.Permission(model.TableName(), p)).
		Where("user_id = ? AND deleted_at = 0", c.UserID).
		Updates(map[string]any{
			"auth_version": gorm.Expr("CASE WHEN status <> ? THEN auth_version + 1 ELSE auth_version END", c.Status),
			"status":       c.Status, "update_by": c.UpdateBy,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL may report zero changed rows for an idempotent status request.
		var count int64
		if err := e.DB.WithContext(ctx).Model(&models.SysUser{}).Scopes(actions.Permission(model.TableName(), p)).Where("user_id = ?", c.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("无权更新该数据")
		}
	}
	return nil
}

// ResetPassword 重置用户密码
func (e *User) ResetPassword(ctx context.Context, c *dto.PasswordReset, p *actions.DataPermission) error {
	var err error
	var model models.SysUser
	db := e.DB.WithContext(ctx).Scopes(
		actions.Permission(model.TableName(), p),
	).First(&model, c.GetID())
	if err = db.Error; err != nil {
		e.Logger.Errorf("resetting password: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	c.Generate(&model)
	model.UpdateBy = c.UpdateBy
	result := e.DB.WithContext(ctx).Model(&model).Scopes(actions.Permission(model.TableName(), p)).
		Where("user_id = ?", model.UserID).Select("Password", "Salt", "UpdateBy").Updates(&model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("无权更新该数据")
	}

	return nil
}

// Delete 删除SysUser
func (e *User) Delete(ctx context.Context, c *dto.UserByID, p *actions.DataPermission) error {
	var err error
	var data models.SysUser

	db := e.DB.WithContext(ctx).Model(&data).
		Scopes(
			actions.Permission(data.TableName(), p),
		).Delete(&data, c.GetID())
	if err = db.Error; err != nil {
		e.Logger.Errorf("Error found in  RemoveSysUser : %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		return errors.New("无权删除该数据")
	}
	return nil
}

// ChangePassword 修改SysUser对象密码
// id must be the authenticated caller's identity, not a request-body user ID.
// The permission argument is retained for source compatibility.
func (e *User) ChangePassword(ctx context.Context, id int, oldPassword, newPassword string, _ *actions.DataPermission) error {
	var err error

	if id <= 0 || oldPassword == "" || newPassword == "" {
		return errors.New("user identity and passwords are required")
	}
	c := &models.SysUser{}

	err = e.DB.WithContext(ctx).Model(c).
		Select("user_id", "password", "salt").
		First(c, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("无权更新该数据")
		}
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	var ok bool
	ok, err = pkg.CompareHashAndPassword(c.Password, oldPassword)
	if err != nil {
		e.Logger.Errorf("CompareHashAndPassword error, %s", err.Error())
		return err
	}
	if !ok {
		err = errors.New("incorrect Password")
		e.Logger.Warnf("user[%d] %s", id, err.Error())
		return err
	}
	c.Password = newPassword
	db := e.DB.WithContext(ctx).Model(c).Where("user_id = ?", id).
		Select("Password", "Salt").
		Updates(c)
	if err = db.Error; err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		err = errors.New("set password error")
		log.NewHelper(log.Default()).Warnf("db update error")
		return err
	}
	return nil
}

// Profile loads the authenticated user's profile.
func (e *User) Profile(ctx context.Context, c *dto.UserByID, user *models.SysUser, roles *[]models.SysRole, posts *[]models.SysPost) error {
	err := e.DB.WithContext(ctx).Preload("Dept").First(user, c.GetID()).Error
	if err != nil {
		return err
	}
	err = e.DB.WithContext(ctx).Find(roles, user.RoleID).Error
	if err != nil {
		return err
	}
	err = e.DB.WithContext(ctx).Find(posts, user.PostIDs).Error
	if err != nil {
		return err
	}

	return nil
}

func credentialChanges(model models.SysUser, self bool) ([]string, []any) {
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
		if model.RoleID != 0 {
			add("role_id", model.RoleID)
		}
		if model.DeptID != 0 {
			add("dept_id", model.DeptID)
		}
		if model.Status != "" {
			add("status", model.Status)
		}
	}
	return changed, values
}

func (e *User) persistUserUpdate(tx *gorm.DB, model models.SysUser, p *actions.DataPermission, isSelf bool) error {
	omit := []string{"password", "salt", "auth_version", "create_by", "created_at", "deleted_at", "user_id"}
	self := isSelf
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
	changed, values := credentialChanges(model, self)
	if len(changed) != 0 {
		if err := scope(tx.Table(model.TableName())).Where("user_id = ? AND deleted_at = 0", model.UserID).
			Where(strings.Join(changed, " OR "), values...).UpdateColumn("auth_version", gorm.Expr("auth_version + ?", 1)).Error; err != nil {
			return err
		}
	}
	update := scope(tx.Model(&model)).Where("user_id = ?", model.UserID).Omit(omit...).Updates(&model)
	if update.Error != nil {
		return update.Error
	}
	if update.RowsAffected == 0 {
		return errors.New("update userinfo error")
	}
	return nil
}
