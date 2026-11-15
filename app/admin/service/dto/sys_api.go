package dto

import (
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
	commonmodels "github.com/gatework/motor/common/models"
)

// RouteSearch carries validated administration request parameters.
type RouteSearch struct {
	dto.Pagination `search:"-"`
	RouteOrder
	Title    string `form:"title"  search:"type:contains;column:title;table:sys_api" comment:"标题"`
	Path     string `form:"path"  search:"type:contains;column:path;table:sys_api" comment:"地址"`
	Action   string `form:"action"  search:"type:exact;column:action;table:sys_api" comment:"请求方式"`
	ParentID string `form:"parentId"  search:"type:exact;column:parent_id;table:sys_api" comment:"按钮id"`
	Type     string `form:"type" search:"-" comment:"类型"`
}

// RouteOrder carries validated administration request parameters.
type RouteOrder struct {
	TitleOrder     string `search:"type:order;column:title;table:sys_api" form:"titleOrder"`
	PathOrder      string `search:"type:order;column:path;table:sys_api" form:"pathOrder"`
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_api" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *RouteSearch) GetNeedSearch() any {
	return *m
}

// RouteUpdate carries validated administration request parameters.
type RouteUpdate struct {
	commonmodels.ControlBy
	ID     int    `uri:"id" comment:"编码"` // 编码
	Handle string `json:"handle" comment:"handle"`
	Title  string `json:"title" comment:"标题"`
	Path   string `json:"path" comment:"地址"`
	Type   string `json:"type" comment:""`
	Action string `json:"action" comment:"类型"`
}

// Generate copies request fields into a new request or supplied model.
func (s *RouteUpdate) Generate(model *models.SysAPI) {
	if s.ID != 0 {
		model.ID = s.ID
	}
	model.Handle = s.Handle
	model.Title = s.Title
	model.Path = s.Path
	model.Type = s.Type
	model.Action = s.Action
}

// GetID returns the selected resource identity.
func (s *RouteUpdate) GetID() any {
	return s.ID
}

// RouteByID carries validated administration request parameters.
type RouteByID struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *RouteByID) GetID() any {
	return s.ID
}
