package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/contract/actions"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	service "github.com/gatework/motor/app/net/service/fabric"
)

// Collections returns collection history.
// @Summary Fabric 采集历史
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/collections [get]
// @Security Bearer
func (e Fabric) Collections(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	rows, count, err := s.Collections(c.Request.Context(), id, r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// CreateCollection queues an idempotent collection.
// @Summary Fabric 开始采集
// @Tags Fabric
// @Accept json
// @Param id path int true "Fabric ID"
// @Param data body dto.CollectionCreateReq true "采集请求"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/collections [post]
// @Security Bearer
func (e Fabric) CreateCollection(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.CollectionCreateReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidInputErr)
		return
	}
	row, err := s.CreateCollection(c.Request.Context(), id, &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "采集已排队")
}

// Collection returns a persisted collection.
// @Summary Fabric 采集详情
// @Tags Fabric
// @Param collectionId path int true "采集 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/collections/{collectionId} [get]
// @Security Bearer
func (e Fabric) Collection(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "collectionId")
	if !ok {
		return
	}
	row, err := s.GetCollection(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "查询成功")
}

// Changes returns controlled change history.
// @Summary Fabric 变更历史
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/changes [get]
// @Security Bearer
func (e Fabric) Changes(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	rows, count, err := s.Changes(c.Request.Context(), id, r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// PreviewChange derives an immutable structured plan from source evidence.
// @Summary Fabric 预览增量变更
// @Tags Fabric
// @Accept json
// @Param id path int true "Fabric ID"
// @Param data body dto.ChangeCreateReq true "标准源与目标"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/changes [post]
// @Security Bearer
func (e Fabric) PreviewChange(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.ChangeCreateReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidInputErr)
		return
	}
	row, err := s.PreviewChange(c.Request.Context(), id, &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "预览已生成")
}

// Change returns immutable plan and target receipts.
// @Summary Fabric 变更详情
// @Tags Fabric
// @Param changeId path int true "变更 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/changes/{changeId} [get]
// @Security Bearer
func (e Fabric) Change(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "changeId")
	if !ok {
		return
	}
	row, err := s.GetChange(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "查询成功")
}

// ExecuteChange validates versions and queues execution.
// @Summary Fabric 执行变更
// @Tags Fabric
// @Accept json
// @Param changeId path int true "变更 ID"
// @Param data body dto.VersionReq true "版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/changes/{changeId}/execute [post]
// @Security Bearer
func (e Fabric) ExecuteChange(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "changeId")
	if !ok {
		return
	}
	var r dto.VersionReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidInputErr)
		return
	}
	row, err := s.ExecuteChange(c.Request.Context(), id, &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "变更已排队")
}

// CancelChange cancels an unstarted change with version protection.
// @Summary Fabric 取消变更
// @Tags Fabric
// @Accept json
// @Param changeId path int true "变更 ID"
// @Param data body dto.VersionReq true "版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/changes/{changeId}/cancel [post]
// @Security Bearer
func (e Fabric) CancelChange(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "changeId")
	if !ok {
		return
	}
	var r dto.VersionReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidInputErr)
		return
	}
	row, err := s.CancelChange(c.Request.Context(), id, &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "变更已取消")
}
