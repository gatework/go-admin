package dto

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	commondto "github.com/gatework/motor/common/dto"

	"github.com/gatework/motor/app/admin/models"
)

// The role and dictionary sort parameters were declared but never embedded in
// their search requests, so every sort the client asked for was dropped; the
// role sort also read the wrong query key.
func TestRoleAndDictionarySearchesHonourTheirSortParameters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	roles := RoleSearch{RoleSortOrder: "desc"}
	sql := db.Model(&models.SysRole{}).Scopes(commondto.MakeCondition(roles.GetNeedSearch())).Find(&[]models.SysRole{}).Statement.SQL.String()
	if !strings.Contains(sql[strings.Index(sql, "ORDER BY")+1:], "role_sort") || !strings.Contains(sql, "ORDER BY") {
		t.Fatalf("role sort dropped: %s", sql)
	}
	dictionaries := DictionarySearch{DictIDOrder: "desc"}
	sql = db.Model(&models.SysDictType{}).Scopes(commondto.MakeCondition(dictionaries.GetNeedSearch())).Find(&[]models.SysDictType{}).Statement.SQL.String()
	if !strings.Contains(sql, "ORDER BY") || !strings.Contains(sql[strings.Index(sql, "ORDER BY"):], "dict_id") {
		t.Fatalf("dictionary sort dropped: %s", sql)
	}
}
