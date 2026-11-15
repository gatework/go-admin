package dto

import (
	"github.com/gatework/motor/common/dto"
)

// SysOperaLogGetPageReq carries validated administration request parameters.
type SysOperaLogGetPageReq struct {
	dto.Pagination `search:"-"`
	SysOperaLogOrder
	Title         string `form:"title" search:"type:contains;column:title;table:sys_opera_log" comment:"操作模块"`
	Method        string `form:"method" search:"type:contains;column:method;table:sys_opera_log" comment:"函数"`
	RequestMethod string `form:"requestMethod" search:"type:contains;column:request_method;table:sys_opera_log" comment:"请求方式: GET POST PUT DELETE"`
	OperURL       string `form:"operUrl" search:"type:contains;column:oper_url;table:sys_opera_log" comment:"访问地址"`
	OperIP        string `form:"operIp" search:"type:exact;column:oper_ip;table:sys_opera_log" comment:"客户端ip"`
	Status        int    `form:"status" search:"type:exact;column:status;table:sys_opera_log" comment:"状态 1:正常 2:关闭"`
	BeginTime     string `form:"beginTime" search:"type:gte;column:created_at;table:sys_opera_log" comment:"创建时间"`
	EndTime       string `form:"endTime" search:"type:lte;column:created_at;table:sys_opera_log" comment:"更新时间"`
}

// SysOperaLogOrder carries validated administration request parameters.
type SysOperaLogOrder struct {
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_opera_log" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysOperaLogGetPageReq) GetNeedSearch() any {
	return *m
}

// SysOperaLogGetReq carries validated administration request parameters.
type SysOperaLogGetReq struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *SysOperaLogGetReq) GetID() any {
	return s.ID
}

// SysOperaLogDeleteReq 功能删除请求参数
type SysOperaLogDeleteReq struct {
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *SysOperaLogDeleteReq) GetID() any {
	return s.IDs
}
