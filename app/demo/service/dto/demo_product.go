package dto

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"go-admin/app/demo/models"
	"go-admin/common/dto"
	common "go-admin/common/models"
)

// DemoProductSearch 列表查询条件
//
// search tag 决定 MakeCondition 拼出的 WHERE：
//
//	exact 精确匹配 / icontains 忽略大小写模糊 / gte 大于等于 …
//
// 未打 search tag 的字段不参与查询，可避免无意间开放过滤维度。
type DemoProductSearch struct {
	dto.Pagination `search:"-"`

	Name   string `form:"name" search:"type:icontains;column:name;table:demo_product"`
	Code   string `form:"code" search:"type:exact;column:code;table:demo_product"`
	Status string `form:"status" search:"type:exact;column:status;table:demo_product"`

	DemoProductOrder
}

// DemoProductOrder 排序字段单独成组，避免与查询字段混在一起
type DemoProductOrder struct {
	CreatedAtOrder string `form:"createdAtOrder" search:"type:order;column:created_at;table:demo_product"`
}

func (m *DemoProductSearch) GetNeedSearch() interface{} { return *m }

func (m *DemoProductSearch) Bind(ctx *gin.Context) error {
	return ctx.ShouldBind(m)
}

// DemoProductControl is the request body of both create and update.
//
// actions.Create / actions.Update get the row to write from ToModel, so the
// model itself is not exposed; Bind validates writable fields and records which fields were supplied.
type DemoProductControl struct {
	Id      int     `json:"id" form:"id" uri:"id" comment:"主键"`
	Name    string  `json:"name" form:"name" comment:"名称"`
	Code    string  `json:"code" form:"code" comment:"编码"`
	Price   float64 `json:"price" form:"price" comment:"单价" binding:"gte=0"`
	Status  string  `json:"status" form:"status" comment:"状态"`
	Remark  string  `json:"remark" form:"remark" comment:"备注"`
	present map[string]json.RawMessage
}

func (s *DemoProductControl) Bind(ctx *gin.Context) error {
	if err := ctx.ShouldBind(s); err != nil {
		return err
	}
	if s.present == nil {
		s.present = make(map[string]json.RawMessage)
		for key := range ctx.Request.Form {
			s.present[strings.ToLower(key)] = nil
		}
		if ctx.Request.MultipartForm != nil {
			for key := range ctx.Request.MultipartForm.Value {
				s.present[strings.ToLower(key)] = nil
			}
		}
	}
	if ctx.Param("id") != "" {
		if err := ctx.ShouldBindUri(s); err != nil {
			return err
		}
	}
	if ctx.Request.Method == http.MethodPost {
		if s.Name == "" || s.Code == "" {
			return errors.New("name and code are required")
		}
	} else {
		if s.Id <= 0 {
			return errors.New("id must be positive")
		}
		if _, ok := s.present["name"]; ok && s.Name == "" {
			return errors.New("name must not be empty")
		}
		if _, ok := s.present["code"]; ok && s.Code == "" {
			return errors.New("code must not be empty")
		}
	}
	return nil
}

// UnmarshalJSON distinguishes an omitted field from an explicit zero value.
func (s *DemoProductControl) UnmarshalJSON(data []byte) error {
	type plain DemoProductControl
	if err := json.Unmarshal(data, (*plain)(s)); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.present = make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		s.present[strings.ToLower(key)] = value
	}
	return nil
}

// UpdateFields exposes only client-writable columns that were supplied.
func (s *DemoProductControl) UpdateFields() []string {
	fields := make([]string, 0, len(s.present))
	for _, field := range []string{"name", "code", "price", "status", "remark"} {
		if _, ok := s.present[field]; ok {
			fields = append(fields, field)
		}
	}
	return fields
}

// ToModel builds the row to write. The actions set CreateBy / UpdateBy
// afterwards, so do not set them here.
func (s *DemoProductControl) ToModel() (*models.DemoProduct, error) {
	return &models.DemoProduct{
		Model:  common.Model{Id: s.Id},
		Name:   s.Name,
		Code:   s.Code,
		Price:  s.Price,
		Status: s.Status,
		Remark: s.Remark,
	}, nil
}

// DemoProductById 详情与删除共用，支持单个 id 与批量 ids
type DemoProductById struct {
	dto.ObjectById
}

// Bind 与 GetId 由内嵌的 dto.ObjectById 提供：它已处理好 uri 绑定、
// DELETE 时的批量 ids 合并与参数校验，无需在此重复实现。
