package firewall

import (
	"strings"

	"gorm.io/gorm"

	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/paging"
)

// Page carries page state for this module.
type Page struct {
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Keyword   string `form:"keyword"`
	GroupID   *int   `form:"groupId"`
	DomainID  int    `form:"domainId"`
}

// Validate provides validate for this module.
func (p *Page) Validate() error {
	if !paging.Normalize(&p.PageIndex, &p.PageSize) || len(p.Keyword) > 512 || p.DomainID < 0 || p.GroupID != nil && *p.GroupID < 0 {
		return InvalidErr
	}
	return nil
}

func pageQuery(db *gorm.DB, p Page) *gorm.DB {
	return db.Scopes(coredto.Paginate(p.PageSize, p.PageIndex))
}

func keyword(db *gorm.DB, column, value string) *gorm.DB {
	if value != "" {
		return db.Where("LOWER("+column+") LIKE ? "+database.LikeEscape, database.LikeContains(strings.ToLower(strings.TrimSpace(value))))
	}
	return db
}

func pValue(p *Page) Page { return *p }
