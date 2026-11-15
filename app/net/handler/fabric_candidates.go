package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/contract/actions"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	service "github.com/gatework/motor/app/net/service/fabric"
)

// ChangeCandidates checks only actual missing compatible device increments.
// @Summary 检查 Fabric 精准变更目标
// @Tags Fabric
// @Accept json
// @Produce json
// @Param id path int true "Fabric ID"
// @Param data body dto.CandidatesReq true "标准配置与问题范围"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/change-candidates [post]
// @Security Bearer
func (e Fabric) ChangeCandidates(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.CandidatesReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	out, err := s.ChangeCandidates(c.Request.Context(), id, &r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "检查完成")
}
