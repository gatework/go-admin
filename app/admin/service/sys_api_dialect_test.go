package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/internal/testconfig"
)

func TestSysAPITypeFilterUsesRequestDialect(t *testing.T) {
	testconfig.Apply(t, func(settings *config.Config) { settings.Application.EnableDP = false })
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
			var vars []any
			if err := db.Callback().Query().After("gorm:query").Register("api:capture", func(tx *gorm.DB) {
				sql, vars = tx.Statement.SQL.String(), tx.Statement.Vars
				tx.AddError(stopped)
			}); err != nil {
				t.Fatal(err)
			}
			s := SysAPI{}
			s.DB, s.Logger = db, logger.NewHelper(logger.Default())
			var list []models.SysAPI
			var count int64
			if err := s.GetPage(&dto.SysAPIGetPageReq{Type: filter}, nil, &list, &count); !errors.Is(err, stopped) {
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
