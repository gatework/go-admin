package firewall

import (
	"strings"

	"gorm.io/gorm"

	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	"github.com/gatework/motor/app/net/service/firewall/dto"
	"github.com/gatework/motor/common/database"
)

func pageQuery(db *gorm.DB, p dto.Page) *gorm.DB {
	return db.Scopes(coredto.Paginate(p.PageSize, p.PageIndex))
}

func keyword(db *gorm.DB, column, value string) *gorm.DB {
	if value != "" {
		return db.Where("LOWER("+column+") LIKE ? "+database.LikeEscape, database.LikeContains(strings.ToLower(strings.TrimSpace(value))))
	}
	return db
}

func pValue(p *dto.Page) dto.Page { return *p }
