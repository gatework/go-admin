package dto

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/common/dto"
)

// LoginLogSearch filters the login log.
type LoginLogSearch struct {
	dto.Pagination `search:"-"`
	LoginLogOrder
	Username      string `form:"username" search:"type:exact;column:username;table:sys_login_log"`
	Status        string `form:"status" search:"type:exact;column:status;table:sys_login_log"`
	Ipaddr        string `form:"ipaddr" search:"type:exact;column:ipaddr;table:sys_login_log"`
	LoginLocation string `form:"loginLocation" search:"type:exact;column:login_location;table:sys_login_log"`
	// The range filters on created_at. They named a ctime column the table
	// does not have, so any request carrying them failed with a SQL error.
	BeginTime string `form:"beginTime" search:"type:gte;column:created_at;table:sys_login_log"`
	EndTime   string `form:"endTime" search:"type:lte;column:created_at;table:sys_login_log"`
}

// LoginLogOrder sorts the login log.
type LoginLogOrder struct {
	CreatedAtOrder string `search:"type:order;column:created_at;table:sys_login_log" form:"createdAtOrder"`
}

// GetNeedSearch returns the tagged query parameters.
func (s *LoginLogSearch) GetNeedSearch() any { return *s }

// Bind decodes the query.
func (s *LoginLogSearch) Bind(c *gin.Context) error { return c.ShouldBind(s) }

// LoginLogByID names one login record in the URI, or several in a DELETE body.
type LoginLogByID struct {
	dto.ObjectByID
}
