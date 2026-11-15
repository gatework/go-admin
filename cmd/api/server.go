package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/gatework/motor-core/config/source/file"
	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/runtime"
	corestorage "github.com/gatework/motor-core/storage"
	"github.com/gatework/motor/common/settings"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/router"
	adminservice "github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/jobs"
	"github.com/gatework/motor/common/auditlog"
	"github.com/gatework/motor/common/buildinfo"
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/storage"
	ext "github.com/gatework/motor/config"
)

var (
	configYml string
	apiCheck  bool
	// StartCmd exposes this command to the host CLI.
	StartCmd *cobra.Command
)

// AppRouters holds the configured approuters value.
var AppRouters = make([]func(), 0)

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	// StartCmd exposes this command to the host CLI.
	StartCmd = &cobra.Command{
		Use:          "server",
		Short:        "Start API server",
		Example:      "motor server -c config/settings.yml",
		SilenceUsage: true,
		PreRunE:      func(cmd *cobra.Command, args []string) error { return setup() },
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}

	StartCmd.PersistentFlags().StringVarP(&configYml, "config", "c", "config/settings.yml", "Start server with provided configuration file")
	StartCmd.PersistentFlags().BoolVarP(&apiCheck, "api", "a", false, "Start server with check api data")

	//注册路由 fixme 其他应用的路由，在本目录新建文件放在init方法
	AppRouters = append(AppRouters, router.InitRouter)
}

func setup() error {
	resetApplicationLifetime()
	// 注入配置扩展项

	// Registered before the configuration is read. SetupConfig announces
	// AfterResource as soon as the callbacks that build the resources have
	// run, so a hook added after that call would miss the first round and the
	// queue would have no consumers until somebody edited the config file.

	// On AfterListen rather than on a bare goroutine from run(). Two reasons:
	// the phase runs behind core's panic guard, which does not reach across a
	// goroutine boundary - a panic while loading jobs used to take the whole
	// process down with a stack that named this file - and the jobs it starts
	// can call the API, which is only true once the socket is accepting.
	sdk.Runtime.SetPhase(runtime.AfterListen, startCronJobs)

	// Registered before the configuration is read, because it registers a
	// callback rather than reading anything: the check runs per request and
	// asks the databases that exist then.
	registerSchemaCheck()

	//1. 读取配置
	var err error
	configurationWatcher, err = settings.Load(lifecycleContext(), sdk.Runtime, file.NewSource(file.WithPath(configYml)), database.Setup, func(candidate *config.Settings) error {
		return storage.Setup(lifecycleContext(), candidate.Settings, subscribeQueue)
	})
	if err != nil {
		stopApplication()
		return err
	}

	usageStr := `starting api server...`
	log.NewHelper(log.Default()).Info(usageStr)
	return nil
}

// startCronJobs registers the job implementations and starts a scheduler for
// every tenant database.
//
// It is synchronous, like the phase that runs it. jobs.Setup returns now that
// the `select {}` at the end of its per-tenant setup is gone, which is what
// makes that possible; while it was there this could only be a goroutine, and
// a goroutine is outside the panic guard.
func startCronJobs() {
	jobs.InitJob()
	jobs.Setup(sdk.Runtime.Databases())
}

var (
	applicationContext   = context.Background()
	stopApplication      = context.CancelFunc(func() {})
	configurationWatcher *settings.Watcher
)

func lifecycleContext() context.Context { return applicationContext }

// subscribeQueue prepares every topic before storage publishes the queue.
func subscribeQueue(ctx context.Context, q corestorage.Queue) error {
	for _, topic := range []struct {
		name    string
		handler corestorage.Handler
	}{
		{auditlog.LoginTopic, models.SaveLoginLog}, {auditlog.OperationTopic, models.SaveOperaLog}, {adminservice.APIRegistryTopic, adminservice.SyncAPIs},
	} {
		if err := q.Subscribe(ctx, topic.name, topic.handler); err != nil {
			return err
		}
	}
	return nil
}

func run() error {
	defer stopApplication()
	defer func() {
		if err := settings.CloseLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "close logger: %v\n", err)
		}
	}()
	if configurationWatcher != nil {
		defer configurationWatcher.Close()
	}
	// Resolved first, and used both for the line it prints and for the
	// shutdown that spends it. Reading the configuration again at signal time
	// would let the two disagree, and the sum that gets printed is the whole
	// point of printing it.
	//
	// Refused rather than corrected, and refused before anything is built: a
	// budget that cannot be spent as written is a configuration error, and the
	// moment to say so is while nothing depends on this process yet.
	seconds, err := ext.Ext().Shutdown.Budget()
	if err != nil {
		return err
	}
	reportShutdownBudget(seconds)

	if config.Current().Settings.Application.Mode == pkg.ModeProd.String() {
		gin.SetMode(gin.ReleaseMode)
	}
	if err := buildRouter(); err != nil {
		return err
	}
	reportGeneratorWriteRoutes()

	srv := newHTTPServer(config.Current().Settings.Application, sdk.Runtime.Engine())

	if apiCheck {
		routes, err := json.Marshal(sdk.Runtime.Routers())
		if err != nil {
			return fmt.Errorf("encode api registry: %w", err)
		}
		if err = sdk.Runtime.QueueForTenant("").Publish(lifecycleContext(), corestorage.Message{Topic: adminservice.APIRegistryTopic, Values: map[string]any{"routes": string(routes)}}); err != nil {
			return fmt.Errorf("publish api registry: %w", err)
		}
	}

	// Armed before the server starts serving, and well before the readiness
	// banner: a signal arriving between "the process is up" and "the process
	// is listening for signals" reaches the default handler and kills it
	// without any of the shutdown below. That window is the whole reason
	// arming is separate from waiting.
	quit, disarmStopSignals := armStopSignals()

	if err := startServing(srv, config.Current().Settings.SSL.Enable, config.Current().Settings.SSL.Pem, config.Current().Settings.SSL.KeyStr); err != nil {
		return err
	}

	fmt.Println(pkg.Red(banner))
	tip()
	scheme, port := "http", config.Current().Settings.Application.Port
	if config.Current().Settings.SSL.Enable {
		scheme = "https"
	}
	fmt.Println(pkg.Green("Server run at:"))
	fmt.Printf("-  Local:   %s://localhost:%d/ \r\n", scheme, port)
	fmt.Printf("-  Network: %s://%s:%d/ \r\n", scheme, pkg.GetLocalHost(), port)
	fmt.Println(pkg.Green("Swagger run at:"))
	fmt.Printf("-  Local:   %s://localhost:%d/swagger/admin/index.html \r\n", scheme, port)
	fmt.Printf("-  Network: %s://%s:%d/swagger/admin/index.html \r\n", scheme, pkg.GetLocalHost(), port)
	fmt.Printf("%s Enter Control + C Shutdown Server \r\n", pkg.GetCurrentTimeStr())

	<-quit
	serverErr, cleanupErr := gracefulShutdown(srv, quit, disarmStopSignals, budgetFrom(seconds))
	if serverErr != nil {
		// Not log.Fatal: that is an unconditional os.Exit(1), and Shutdown
		// reports an error exactly when connections were still in flight -
		// which is when the cleanup that ran after it mattered most.
		log.NewHelper(log.Default()).Error("Server Shutdown: ", serverErr)
	}
	if cleanupErr != nil {
		log.NewHelper(log.Default()).Error("Cleanup: ", cleanupErr)
	}

	return nil
}

func tip() {
	usageStr := `欢迎使用 ` + pkg.Green(`motor `+buildinfo.Version) + ` 可以使用 ` + pkg.Red(`-h`) + ` 查看命令`
	fmt.Printf("%s \n\n", usageStr)
}

// banner identifies the backend in startup output.
const banner = "\nmotor\n"

func applicationLifetime(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

//nolint:fatcontext // The host explicitly replaces the process lifecycle at command startup.
func resetApplicationLifetime() {
	applicationContext, stopApplication = applicationLifetime(StartCmd.Context())
}
