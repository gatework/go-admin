package handler

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/gatework/motor/common/account"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// User handles HTTP requests for its administration resource.
type User struct {
	api.API
}

// List lists records matching the request and data permissions.
// @Summary 列表用户信息数据
// @Description 获取JSON
// @Tags 用户
// @Param username query string false "username"
// @Success 200 {string} {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-user [get]
// @Security Bearer
func (e User) List(c *gin.Context) {
	s := service.User{}
	req := dto.UserSearch{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	//数据权限检查
	p := actions.GetPermissionFromContext(c)

	list := make([]models.SysUser, 0)
	var count int64

	err = s.List(c.Request.Context(), &req, p, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get loads the requested record within its data permissions.
// @Summary 获取用户
// @Description 获取JSON
// @Tags 用户
// @Param id path int true "用户编码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-user/{id} [get]
// @Security Bearer
func (e User) Get(c *gin.Context) {
	s := service.User{}
	req := dto.UserByID{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	var object models.SysUser
	//数据权限检查
	p := actions.GetPermissionFromContext(c)
	err = s.Get(c.Request.Context(), &req, p, &object)
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, "查询失败")
		return
	}
	e.OK(object, "查询成功")
}

// Create creates a record from the validated request.
// @Summary 创建用户
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body dto.UserCreate true "用户数据"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-user [post]
// @Security Bearer
func (e User) Create(c *gin.Context) {
	s := service.User{}
	req := dto.UserCreate{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	// 设置创建人
	req.SetCreateBy(user.ID(c))
	err = s.Create(c.Request.Context(), &req)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	e.OK(req.GetID(), "创建成功")
}

// Update updates an existing record within its data permissions.
// @Summary 修改用户数据
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body dto.UserUpdate true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-user [put]
// @Security Bearer
func (e User) Update(c *gin.Context) {
	s := service.User{}
	req := dto.UserUpdate{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	callerID := user.ID(c)

	// This route is in CasbinExclude so the personal-center screen can edit
	// the caller's own record without a policy grant (see settings.go). That
	// exclusion covers the whole route, not just the caller's own record, and
	// the request carries the target userId in the body - so without this
	// check here, any authenticated caller could edit any other user, up to
	// and including their roleId. When the target is someone else, ask Casbin
	// directly for the permission AuthCheckRole skipped.
	if req.UserID != callerID {
		allowed, err := middleware.EnforceRoleFor(c, c.Request.URL.Path, c.Request.Method)
		if err != nil {
			e.Logger.Error(err)
			e.Error(500, err, err.Error())
			return
		}
		if !allowed {
			e.Error(http.StatusForbidden, errors.New("无权更新其他用户数据"), "对不起，您没有该接口访问权限，请联系管理员")
			return
		}
	}

	req.SetUpdateBy(callerID)

	//数据权限检查
	p := actions.GetPermissionFromContext(c)

	err = s.Update(c.Request.Context(), &req, p, callerID)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "操作失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除用户数据
// @Description 删除数据
// @Tags 用户
// @Param data body dto.UserByID true "ids"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-user [delete]
// @Security Bearer
func (e User) Delete(c *gin.Context) {
	s := service.User{}
	req := dto.UserByID{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	// 设置编辑人
	req.SetUpdateBy(user.ID(c))

	// 数据权限检查
	p := actions.GetPermissionFromContext(c)

	err = s.Delete(c.Request.Context(), &req, p)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "操作失败")
		return
	}
	e.OK(req.GetID(), "删除成功")
}

// UpdateAvatar performs the inset avatar administration operation.
// @Summary 修改头像
// @Description 获取JSON
// @Tags 个人中心
// @Accept multipart/form-data
// @Param file formData file true "file"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/user/avatar [post]
// @Security Bearer
func (e User) UpdateAvatar(c *gin.Context) {
	s := service.User{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, nil, "数据库连接获取失败")
		return
	}
	userID := user.ID(c)
	if userID <= 0 {
		e.Error(http.StatusUnauthorized, nil, "登录失效")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAvatarBytes)
	form, err := c.MultipartForm()
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, nil, "头像上传参数错误")
		return
	}
	files := form.File["upload[]"]
	if len(files) != 1 {
		e.Error(http.StatusUnprocessableEntity, nil, "请选择一张头像")
		return
	}
	image, err := readUpload(files[0])
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, nil, "头像上传参数错误")
		return
	}
	path, err := s.SetAvatar(c.Request.Context(), userID, image)
	if errors.Is(err, service.InvalidAvatarErr) {
		e.Error(http.StatusUnprocessableEntity, nil, "头像须为 PNG、JPEG、GIF 或 WebP 图片")
		return
	}
	if err != nil {
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, nil, "操作失败")
		return
	}
	e.OK(strings.TrimPrefix(path, "/"), "修改成功")
}

// maxAvatarBytes bounds the whole avatar request.
const maxAvatarBytes = 2 << 20

func readUpload(header *multipart.FileHeader) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxAvatarBytes))
}

// UpdateStatus 修改用户状态
// @Summary 修改用户状态
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body dto.UserStatusUpdate true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/user/status [put]
// @Security Bearer
func (e User) UpdateStatus(c *gin.Context) {
	s := service.User{}
	req := dto.UserStatusUpdate{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	req.SetUpdateBy(user.ID(c))

	//数据权限检查
	p := actions.GetPermissionFromContext(c)

	err = s.UpdateStatus(c.Request.Context(), &req, p)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "操作失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// ResetPassword 重置用户密码
// @Summary 重置用户密码
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body dto.PasswordReset true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/user/pwd/reset [put]
// @Security Bearer
func (e User) ResetPassword(c *gin.Context) {
	s := service.User{}
	req := dto.PasswordReset{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	req.SetUpdateBy(user.ID(c))

	//数据权限检查
	p := actions.GetPermissionFromContext(c)

	err = s.ResetPassword(c.Request.Context(), &req, p)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "操作失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// ChangePassword performs the update pwd administration operation.
// @Summary 修改密码
// @Description 获取JSON
// @Tags 用户
// @Accept  application/json
// @Product application/json
// @Param data body dto.PasswordChange true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/user/pwd/set [put]
// @Security Bearer
func (e User) ChangePassword(c *gin.Context) {
	s := service.User{}
	req := dto.PasswordChange{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	err = s.ChangePassword(c.Request.Context(), user.ID(c), req.OldPassword, req.NewPassword, nil)
	if errors.Is(err, account.PasswordErr) {
		e.Error(http.StatusBadRequest, nil, "新密码需为 12–72 字节")
		return
	}
	if err != nil {
		e.Logger.Error(err)
		e.Error(http.StatusForbidden, nil, "密码修改失败")
		return
	}

	e.OK(nil, "密码修改成功")
}

// Profile loads the authenticated user's profile.
// @Summary 获取个人中心用户
// @Description 获取JSON
// @Tags 个人中心
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/user/profile [get]
// @Security Bearer
func (e User) Profile(c *gin.Context) {
	s := service.User{}
	req := dto.UserByID{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	req.ID = user.ID(c)

	sysUser := models.SysUser{}
	roles := make([]models.SysRole, 0)
	posts := make([]models.SysPost, 0)
	err = s.Profile(c.Request.Context(), &req, &sysUser, &roles, &posts)
	if err != nil {
		e.Logger.Errorf("get user profile error, %s", err.Error())
		e.Error(500, err, "获取用户信息失败")
		return
	}
	e.OK(gin.H{
		"user":  sysUser,
		"roles": roles,
		"posts": posts,
	}, "查询成功")
}

// Current performs the get info administration operation.
// @Summary 获取个人信息
// @Description 获取JSON
// @Tags 个人中心
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/getinfo [get]
// @Security Bearer
func (e User) Current(c *gin.Context) {
	req := dto.UserByID{}
	s := service.User{}
	r := service.Role{}
	var err error
	e.API, err = request.New(c)
	r.Service = e.Service()
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	var roles = make([]string, 1)
	roles[0] = user.RoleName(c)
	var permissions = make([]string, 1)
	permissions[0] = "*:*:*"
	var buttons = make([]string, 1)
	buttons[0] = "*:*:*"

	var mp = make(map[string]any)
	mp["roles"] = roles
	if account.IsAdmin(user.RoleName(c)) {
		mp["permissions"] = permissions
		mp["buttons"] = buttons
	} else {
		list, err := r.Permissions(c.Request.Context(), user.RoleID(c))
		if err != nil {
			e.Logger.Errorf("loading permissions for role %d: %v", user.RoleID(c), err)
			e.Error(http.StatusInternalServerError, nil, "权限加载失败")
			return
		}
		mp["permissions"] = list
		mp["buttons"] = list
	}
	sysUser := models.SysUser{}
	req.ID = user.ID(c)
	// Unscoped on purpose: the id is the caller's own, taken from the token.
	// This used to go through Get with whatever GetPermissionFromContext
	// returned - and this route installs no PermissionAction, so that was the
	// zero value. An unset scope is not a recognised one, so once unknown
	// scopes started failing closed rather than silently matching everything,
	// every login on a deployment with enabledp: true ended here with a 401
	// and the browser went straight back to the login page.
	err = s.Self(c.Request.Context(), &req, &sysUser)
	if err != nil {
		e.Error(http.StatusUnauthorized, nil, "登录失败")
		return
	}
	// No default avatar URL: the product runs offline, and the UI draws the
	// user's initials when there is none. It used to point at a public CDN.
	mp["introduction"] = ""
	mp["avatar"] = sysUser.Avatar
	mp["userName"] = sysUser.Username
	mp["userId"] = sysUser.UserID
	mp["deptId"] = sysUser.DeptID
	mp["name"] = sysUser.NickName
	e.OK(mp, "")
}
