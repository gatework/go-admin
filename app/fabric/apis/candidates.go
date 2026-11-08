package apis

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/fabric/service"
	"go-admin/app/fabric/service/dto"
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
	s, ok := e.prepare(c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.CandidatesReq
	if !decode(c, &r) {
		e.respond(service.ErrInvalidInput)
		return
	}
	out, err := s.ChangeCandidates(c.Request.Context(), id, &r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "检查完成")
}
