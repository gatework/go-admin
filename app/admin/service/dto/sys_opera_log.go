package dto

import (
	"go-admin/common/dto"
	"go-admin/common/global"
)

// Deprecated: use global.OperaStatusEnabled / global.OperaStatusDisabled.
// These two names are kept - misspelling and all - because forks import them;
// the values moved to common/global so common/middleware no longer has to
// import this package. See docs/contract.md.
const (
	OperaStatusEnabel  = global.OperaStatusEnabled  // 状态-正常
	OperaStatusDisable = global.OperaStatusDisabled // 状态-关闭
)

type SysOperaLogGetPageReq struct {
	dto.Pagination `search:"-"`
	Title          string `form:"title" search:"type:contains;column:title;table:sys_opera_log" comment:"操作模块"`
	Method         string `form:"method" search:"type:contains;column:method;table:sys_opera_log" comment:"函数"`
	RequestMethod  string `form:"requestMethod" search:"type:contains;column:request_method;table:sys_opera_log" comment:"请求方式: GET POST PUT DELETE"`
	OperUrl        string `form:"operUrl" search:"type:contains;column:oper_url;table:sys_opera_log" comment:"访问地址"`
	OperIp         string `form:"operIp" search:"type:exact;column:oper_ip;table:sys_opera_log" comment:"客户端ip"`
	Status         int    `form:"status" search:"type:exact;column:status;table:sys_opera_log" comment:"状态 1:正常 2:关闭"`
	BeginTime      string `form:"beginTime" search:"type:gte;column:created_at;table:sys_opera_log" comment:"创建时间"`
	EndTime        string `form:"endTime" search:"type:lte;column:created_at;table:sys_opera_log" comment:"更新时间"`
	SysOperaLogOrder
}

type SysOperaLogOrder struct {
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_opera_log" form:"createdAtOrder"`
}

func (m *SysOperaLogGetPageReq) GetNeedSearch() interface{} {
	return *m
}

type SysOperaLogGetReq struct {
	Id int `uri:"id"`
}

func (s *SysOperaLogGetReq) GetId() interface{} {
	return s.Id
}

// SysOperaLogDeleteReq 功能删除请求参数
type SysOperaLogDeleteReq struct {
	Ids []int `json:"ids"`
}

func (s *SysOperaLogDeleteReq) GetId() interface{} {
	return s.Ids
}
