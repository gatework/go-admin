package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	service "github.com/gatework/motor/app/net/service/inventory"
	"github.com/gatework/motor/common/request"
)

// DeviceGroup shares inventory's safe errors/initialization, while all group
// persistence and hierarchy checks remain in the service layer.
type DeviceGroup struct{ Device }

// Tree returns shared taxonomy with counts restricted to the caller's inventory.
// @Summary 查询设备分组树
// @Tags 设备分组
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-device-group [get]
// @Security Bearer
func (e DeviceGroup) Tree(c *gin.Context) {
	s, ok := prepareDeviceGroup(&e, c)
	if !ok {
		return
	}
	tree, err := s.Tree(c.Request.Context(), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(tree, "查询成功")
}

// Create adds a group under an existing parent.
// @Summary 新增设备分组
// @Tags 设备分组
// @Accept json
// @Produce json
// @Param data body dto.DeviceGroupReq true "分组资料"
// @Success 200 {object} response.Response
// @Router /api/v1/network-device-group [post]
// @Security Bearer
func (e DeviceGroup) Create(c *gin.Context) {
	s, ok := prepareDeviceGroup(&e, c)
	if !ok {
		return
	}
	var req dto.DeviceGroupReq
	if !decodeDeviceBody(c, &req) {
		e.respond(inventory.InvalidInputErr)
		return
	}
	row, err := s.Create(c.Request.Context(), &req, user.ID(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "分组已创建")
}

// Update checks version and prevents hierarchy cycles.
// @Summary 编辑设备分组
// @Tags 设备分组
// @Accept json
// @Produce json
// @Param id path int true "分组 ID"
// @Param data body dto.DeviceGroupReq true "分组资料与版本"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device-group/{id} [put]
// @Security Bearer
func (e DeviceGroup) Update(c *gin.Context) {
	s, ok := prepareDeviceGroup(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	var req dto.DeviceGroupReq
	if !decodeDeviceBody(c, &req) {
		e.respond(inventory.InvalidInputErr)
		return
	}
	row, err := s.Update(c.Request.Context(), id, &req, user.ID(c))
	if err != nil {
		e.respond(err)
		return
	}
	tree, err := s.Tree(c.Request.Context(), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	var find func([]dto.DeviceGroupNode) bool
	find = func(nodes []dto.DeviceGroupNode) bool {
		for _, node := range nodes {
			if node.ID == id {
				row = node
				return true
			}
			if find(node.Children) {
				return true
			}
		}
		return false
	}
	find(tree.Items)
	e.OK(row, "分组已更新")
}

// Delete removes only a current, empty group.
// @Summary 删除空设备分组
// @Tags 设备分组
// @Accept json
// @Produce json
// @Param id path int true "分组 ID"
// @Param data body dto.DeviceGroupDeleteReq true "当前版本"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device-group/{id} [delete]
// @Security Bearer
func (e DeviceGroup) Delete(c *gin.Context) {
	s, ok := prepareDeviceGroup(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	var req dto.DeviceGroupDeleteReq
	if !decodeDeviceBody(c, &req) {
		e.respond(inventory.InvalidInputErr)
		return
	}
	if err := s.Delete(c.Request.Context(), id, req.Version, user.ID(c)); err != nil {
		e.respond(err)
		return
	}
	e.OK(nil, "分组已删除")
}

func prepareDeviceGroup(e *DeviceGroup, c *gin.Context) (*service.DeviceGroup, bool) {
	s := &service.DeviceGroup{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.respond(err)
		return nil, false
	}
	return s, true
}
