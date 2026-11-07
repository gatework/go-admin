package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	"go-admin/common/global"
)

func TestSysApiTypeFilterUsesRequestDialect(t *testing.T) {
	previousDriver, previousDP := global.Driver, config.ApplicationConfig.EnableDP
	global.Driver, config.ApplicationConfig.EnableDP = "mysql", false
	t.Cleanup(func() { global.Driver, config.ApplicationConfig.EnableDP = previousDriver, previousDP })
	for _, filter := range []string{"BUS", "暂无"} {
		t.Run(filter, func(t *testing.T) {
			base, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := base.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { pool.Close() })
			db, err := gorm.Open(sqlserver.New(sqlserver.Config{Conn: pool}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			stopped := errors.New("captured query")
			var sql string
			var vars []interface{}
			if err := db.Callback().Query().After("gorm:query").Register("api:capture", func(tx *gorm.DB) {
				sql, vars = tx.Statement.SQL.String(), tx.Statement.Vars
				tx.AddError(stopped)
			}); err != nil {
				t.Fatal(err)
			}
			s := SysApi{}
			s.Orm, s.Log = db, logger.NewHelper(logger.DefaultLogger)
			var list []models.SysApi
			var count int64
			if err := s.GetPage(&dto.SysApiGetPageReq{Type: filter}, nil, &list, &count); !errors.Is(err, stopped) {
				t.Fatalf("GetPage error: %v", err)
			}
			if strings.Contains(sql, "`") || !strings.Contains(sql, `"type" = @p`) {
				t.Fatalf("filter used the process driver instead of the request dialect: %s", sql)
			}
			want := filter
			if filter == "暂无" {
				want = ""
			}
			if len(vars) == 0 || vars[0] != want {
				t.Fatalf("filter binding = %#v, want %q", vars, want)
			}
		})
	}
}
