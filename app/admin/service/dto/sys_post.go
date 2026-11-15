package dto

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/dto"
)

// PostSearch filters the post list.
type PostSearch struct {
	dto.Pagination `search:"-"`
	PostID         int    `form:"postId" search:"type:exact;column:post_id;table:sys_post"`
	PostName       string `form:"postName" search:"type:contains;column:post_name;table:sys_post"`
	PostCode       string `form:"postCode" search:"type:contains;column:post_code;table:sys_post"`
	Sort           int    `form:"sort" search:"type:exact;column:sort;table:sys_post"`
	Status         int    `form:"status" search:"type:exact;column:status;table:sys_post"`
	Remark         string `form:"remark" search:"type:exact;column:remark;table:sys_post"`
}

// GetNeedSearch returns the tagged query parameters.
func (s *PostSearch) GetNeedSearch() any { return *s }

// Bind decodes the query.
func (s *PostSearch) Bind(c *gin.Context) error { return c.ShouldBind(s) }

// PostControl is the body of creating a post and of replacing one.
type PostControl struct {
	PostID   int    `json:"postId" uri:"id"`
	PostName string `json:"postName"`
	PostCode string `json:"postCode"`
	Sort     int    `json:"sort"`
	Status   int    `json:"status"`
	Remark   string `json:"remark"`
}

// Bind decodes the body, takes the key from the URI on update, and requires a
// name and a code.
func (s *PostControl) Bind(c *gin.Context) error {
	if err := c.ShouldBindJSON(s); err != nil {
		return err
	}
	if c.Request.Method != http.MethodPost {
		if err := c.ShouldBindUri(s); err != nil {
			return err
		}
		if s.PostID <= 0 {
			return errors.New("postId must be positive")
		}
	}
	if s.PostName == "" || s.PostCode == "" {
		return errors.New("postName and postCode are required")
	}
	return nil
}

// UpdateFields makes an update a full replace of the writable columns, as the
// post form always sends all of them: a cleared remark is written as empty.
func (*PostControl) UpdateFields() []string {
	return []string{"post_name", "post_code", "sort", "status", "remark"}
}

// ToModel builds the row to write; the actions stamp creator and updater.
func (s *PostControl) ToModel() (*models.SysPost, error) {
	return &models.SysPost{
		PostID:   s.PostID,
		PostName: s.PostName,
		PostCode: s.PostCode,
		Sort:     s.Sort,
		Status:   s.Status,
		Remark:   s.Remark,
	}, nil
}

// PostByID names one post in the URI, or several in a DELETE body.
type PostByID struct {
	dto.ObjectByID
}
