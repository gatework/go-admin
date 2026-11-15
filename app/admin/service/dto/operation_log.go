package dto

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/common/dto"
)

// OperationLogSearch filters the operation log.
type OperationLogSearch struct {
	dto.Pagination `search:"-"`
	OperationLogOrder
	Title         string `form:"title" search:"type:contains;column:title;table:sys_opera_log"`
	Method        string `form:"method" search:"type:contains;column:method;table:sys_opera_log"`
	RequestMethod string `form:"requestMethod" search:"type:contains;column:request_method;table:sys_opera_log"`
	OperURL       string `form:"operUrl" search:"type:contains;column:oper_url;table:sys_opera_log"`
	OperIP        string `form:"operIp" search:"type:exact;column:oper_ip;table:sys_opera_log"`
	Status        int    `form:"status" search:"type:exact;column:status;table:sys_opera_log"`
	BeginTime     string `form:"beginTime" search:"type:gte;column:created_at;table:sys_opera_log"`
	EndTime       string `form:"endTime" search:"type:lte;column:created_at;table:sys_opera_log"`
}

// OperationLogOrder sorts the operation log.
type OperationLogOrder struct {
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_opera_log" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (s *OperationLogSearch) GetNeedSearch() any { return *s }

// Bind decodes the query.
func (s *OperationLogSearch) Bind(c *gin.Context) error { return c.ShouldBind(s) }

// OperationLogByID names one operation record in the URI, or several in a
// DELETE body.
type OperationLogByID struct {
	dto.ObjectByID
}
