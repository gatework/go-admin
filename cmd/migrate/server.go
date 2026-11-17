package migrate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/spf13/cobra"

	"github.com/gatework/motor-core/config/source/file"
	"github.com/gatework/motor-core/sdk/contract/app"

	"github.com/gatework/motor/cmd/migrate/migration"
	_ "github.com/gatework/motor/cmd/migrate/migration/version"       // Registers the distributed migrations.
	_ "github.com/gatework/motor/cmd/migrate/migration/version-local" // Registers local migration extensions.
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/models"
	"github.com/gatework/motor/common/settings"
)

var (
	configYml string
	generate  bool
	framework bool
	host      string
	appCode   string
	dryRun    bool
)

// NewCommand builds the `migrate` command and its status, install and
// uninstall subcommands.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "migrate",
		Short:   "Initialize the database",
		Example: "motor migrate -c config/settings.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context())
		},
	}

	cmd.PersistentFlags().StringVarP(&configYml, "config", "c", "config/settings.yml", "Start server with provided configuration file")
	cmd.PersistentFlags().BoolVarP(&generate, "generate", "g", false, "generate migration file")
	cmd.PersistentFlags().BoolVarP(&framework, "framework", "a", false, "with -g, write the generated file to version/ instead of version-local/ (does not affect which migrations run)")
	cmd.PersistentFlags().StringVarP(&host, "domain", "d", "*", "select tenant host")

	// --app is deliberately long-only. -a already means "generate into
	// version/ rather than version-local/", which is about writing a template
	// file, not about which migrations run; giving the two the same letter
	// would be a trap.
	cmd.PersistentFlags().StringVar(&appCode, "app", "", "limit to the migrations of one app (\""+migration.FrameworkAppCode+"\" for the framework's own)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list what would be applied, in order, and write nothing")

	cmd.AddCommand(newStatusCommand(), newInstallCommand(), newUninstallCommand())
	return cmd
}

// newStatusCommand, newInstallCommand and newUninstallCommand build the
// migrate subcommands; NewCommand calls them, so each parent gets its own.
func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "List applied and pending migrations, grouped by app",
		Example: "motor migrate status -c config/settings.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.Context())
		},
	}
}

func newInstallCommand() *cobra.Command {
	// Under migrate rather than under the existing `app` command, which
	// already means "generate the skeleton of a new app" - a directory that
	// does not exist yet, not an application already compiled into this
	// binary. Installing an application is running its migrations, which is
	// what this command is; --app, --domain and resolveDB are all already
	// here, including the guard that refuses a mistyped code instead of
	// reporting a successful no-op.
	return &cobra.Command{
		Use:     "install <code>",
		Short:   "Install one application: run its migrations and record it in sys_app",
		Example: "motor migrate install order -c config/settings.yml",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstall(cmd.Context(), args[0])
		},
	}
}

func newUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall <code>",
		Short:   "Remove one application's menus, apis and permission grants; its own tables are left alone",
		Example: "motor migrate uninstall order -c config/settings.yml",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUninstall(cmd.Context(), args[0])
		},
	}
}

// run migrates, or with -g writes a new migration file. Its error is the
// command's: the root command reports it and exits non-zero, after every
// deferred close here has run.
func run(ctx context.Context) error {
	if generate {
		fmt.Println(`generate migration file`)
		return genFile()
	}
	fmt.Println(`start init`)
	return withDatabase(ctx, initDB)
}

// resolveDB picks the tenant database and hands it to the registry.
//
// It creates and alters nothing, which is what lets status and --dry-run share
// it: those two must be able to run against a production database without
// leaving a trace.
func resolveDB() (*gorm.DB, error) {
	if host == "" {
		host = "*"
	}
	db := sdk.Runtime.DB(host)
	if db == nil {
		if len(sdk.Runtime.Databases()) == 1 && host == "*" {
			for k, v := range sdk.Runtime.Databases() {
				db = v
				host = k
				break
			}
		}
	}
	if db == nil {
		return nil, fmt.Errorf("未找到数据库配置")
	}
	return withTableOptions(db, config.Current().Settings.Databases[host].Driver), nil
}

// withTableOptions makes tables created by migrations on MySQL use InnoDB and
// utf8mb4. Set returns a new handle rather than changing its receiver, so the
// result must be the handle that is returned; Session then makes it a reusable
// base again, with the setting carried into every statement started from it.
func withTableOptions(db *gorm.DB, driver string) *gorm.DB {
	if driver != "mysql" {
		return db
	}
	return db.Set("gorm:table_options", "ENGINE=InnoDB CHARSET=utf8mb4").Session(&gorm.Session{})
}

// appRegistrationError refuses an --app that names no registered app, before
// any database work. Every path used to take a typo as "nothing matched" and
// report success - which is what an up-to-date database says too - so
// `motor migrate --app crmm && deploy` deployed. Nil means --app was either
// empty or names a registered app.
func appRegistrationError() error {
	if appCode == "" {
		return nil
	}
	want := migration.DisplayAppCode(migration.AppFilter(appCode))
	registered := migration.Migrate.AppCodes()
	if slices.Contains(registered, want) {
		return nil
	}
	return fmt.Errorf("no migrations are registered for app %q; registered: %s",
		want, strings.Join(registered, ", "))
}

func migrateModel() error {
	db, err := resolveDB()
	if err != nil {
		return err
	}
	// sys_migration is the one table that never goes through a versioned
	// migration - it is the table that records them. AutoMigrate realigns it
	// on every run, which is how the app_code column reaches an existing
	// database without anyone writing a migration for it.
	if err = db.Debug().AutoMigrate(&models.Migration{}); err != nil {
		return err
	}
	migration.Migrate.SetDB(db.Debug())
	if appCode != "" {
		return migration.Migrate.MigrateApp(appCode)
	}
	return migration.Migrate.Migrate()
}

func initDB() error {
	// Before the database is touched, so a typo cannot get as far as looking
	// like a successful no-op on either path below.
	if err := appRegistrationError(); err != nil {
		return err
	}
	if dryRun {
		db, err := resolveDB()
		if err != nil {
			return err
		}
		migration.Migrate.SetDB(db)
		entries, err := migration.Migrate.Status()
		if err != nil {
			return err
		}
		return printPending(os.Stdout, entries, appCode)
	}
	fmt.Println("数据库迁移开始")
	if err := migrateModel(); err != nil {
		return err
	}
	fmt.Println(`数据库基础数据初始化成功`)
	return nil
}

func runStatus(ctx context.Context) error {
	return withDatabase(ctx, func() error {
		if err := appRegistrationError(); err != nil {
			return err
		}
		db, err := resolveDB()
		if err != nil {
			return err
		}
		migration.Migrate.SetDB(db)
		entries, err := migration.Migrate.Status()
		if err != nil {
			return err
		}
		// Which applications exist is a different question from which
		// migrations ran, and an install that stopped partway is only
		// visible in the answer to the first.
		apps, err := loadApps(db)
		if err != nil {
			return err
		}
		return printStatus(os.Stdout, entries, apps, appCode)
	})
}

func runInstall(ctx context.Context, code string) error {
	return withDatabase(ctx, func() error {
		db, err := resolveDB()
		if err != nil {
			return err
		}
		registered := app.Snapshot()
		m, err := manifestFor(registered, code)
		if err != nil {
			return err
		}
		// Over every registered manifest, not just this one's closure: a
		// cycle between two other applications is still an authoring
		// mistake, and the day somebody installs into it is the worse
		// time to find out.
		if err := refuseOnDependencyCycle(registered); err != nil {
			return err
		}
		rep, err := install(db, migration.Migrate, m)
		if err != nil {
			return err
		}
		reportInstall(os.Stdout, rep)
		return nil
	})
}

func runUninstall(ctx context.Context, code string) error {
	return withDatabase(ctx, func() error {
		db, err := resolveDB()
		if err != nil {
			return err
		}
		// No manifest lookup. An application whose code has already been
		// taken out of the binary registers nothing, and that is exactly
		// when somebody needs to clear its rows out of the database.
		rep, err := uninstall(db, code)
		if err != nil {
			return err
		}
		reportUninstall(os.Stdout, rep)
		return nil
	})
}

func genFile() error {
	t1, err := template.ParseFiles("template/migrate.template")
	if err != nil {
		return err
	}
	m := map[string]string{}
	m["GenerateTime"] = strconv.FormatInt(time.Now().UnixNano()/1e6, 10)
	// The package name of cmd/migrate/migration/version-local, not its directory
	// name: a generated file in any other package does not compile there.
	m["Package"] = "versionlocal"
	if framework {
		m["Package"] = "version"
	}
	var b1 bytes.Buffer
	if err = t1.Execute(&b1, m); err != nil {
		return err
	}
	if framework {
		return pkg.FileCreate(b1, "./cmd/migrate/migration/version/"+m["GenerateTime"]+"_migrate.go")
	}
	return pkg.FileCreate(b1, "./cmd/migrate/migration/version-local/"+m["GenerateTime"]+"_migrate.go")
}

// withDatabase loads the configuration and tenant databases, runs action, and
// closes both before returning action's error.
func withDatabase(ctx context.Context, action func() error) error {
	watcher, err := settings.Load(ctx, sdk.Runtime, file.NewSource(file.WithPath(configYml)), database.Setup)
	if err != nil {
		return err
	}
	defer watcher.Close()
	defer func() {
		if err := settings.CloseLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "close logger: %v\n", err)
		}
	}()
	return action()
}
