// Command firewall-browser-fixture runs a disposable, loopback-only integration
// backend. It uses the production router and worker with SQLite and synthetic
// inventory. It has no network-device transport and is not a production server.
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	fm "github.com/gatework/firemon"
	fc "github.com/gatework/firemon/config"
	native "github.com/gatework/firemon/device"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	delivery "go-admin/app/delivery/models"
	"go-admin/app/firewall/models"
	firewallrouter "go-admin/app/firewall/router"
	"go-admin/app/firewall/service"
	"go-admin/app/firewall/storage"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The same native Hillstone configuration is exercised by
// app/firewall/service/worker_test.go; all addresses are documentation prefixes.
const configuration = `zone inside
exit
zone outside
exit
interface eth0
 zone inside
 ip address 192.0.2.1/24
exit
interface eth1
 zone outside
 ip address 198.51.100.1/24
exit
policy from inside to outside
 rule id 1
  src-addr any
  dst-addr any
  service any
  action permit
  log session-end
 exit
exit
`

// This reader cannot open a socket. Browser tests exercise the production
// queue, collector, publication and SQL projection with explicit synthetic I/O.
type fixtureReader struct {
	instance      models.Instance
	reads, closes *atomic.Int64
}

func (r *fixtureReader) Read(ctx context.Context, view native.View) (native.Capture, error) {
	r.reads.Add(1)
	return native.Capture{Source: fc.Source{Text: configuration, Complete: true}, Metadata: fm.Metadata{Device: storage.DeviceKey(r.instance.ID), Vendor: r.instance.Vendor, Context: r.instance.NativeContext, CapturedAt: time.Now().UTC()}}, ctx.Err()
}
func (r *fixtureReader) Close() error { r.closes.Add(1); return nil }

func main() {
	addr := flag.String("addr", "127.0.0.1:18039", "loopback HTTP address")
	origin := flag.String("origin", "http://localhost:9539", "browser origin")
	flag.Parse()
	host, _, err := net.SplitHostPort(*addr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		log.Fatal("fixture must bind a numeric loopback address")
	}
	directory, err := os.MkdirTemp("", "go-admin-firewall-browser-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err = os.Chmod(directory, 0700); err != nil {
		log.Fatal(err)
	}
	if err = run(*addr, *origin, directory); err != nil {
		_ = os.RemoveAll(directory)
		log.Print(err)
		os.Exit(1)
	}
}

func run(addr, origin, directory string) error {
	gin.SetMode(gin.TestMode)
	config.ApplicationConfig.EnableDP = true
	db, err := gorm.Open(sqlite.Open(filepath.Join(directory, "fixture.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	connection, err := db.DB()
	if err != nil {
		return err
	}
	connection.SetMaxOpenConns(1)
	defer connection.Close()
	if err = db.AutoMigrate(append(models.Schema(), &network.Device{}, &network.DeviceGroup{}, &delivery.Connection{}, &delivery.DeviceLease{})...); err != nil {
		return err
	}
	if err = db.Create(&models.Domain{ID: 1, Code: "default", Name: "Fixture domain", Version: 1}).Error; err != nil {
		return err
	}
	device := network.Device{SourceFacts: network.SourceFacts{Name: "Hillstone fixture", IP: "192.0.2.254", Vendor: "hillstone", SourceTags: []string{}}, DeviceType: "firewall", DeviceRole: "firewall", Status: "pending", Tags: []string{}}
	device.CreateBy = 7
	if err = db.Create(&device).Error; err != nil {
		return err
	}
	if err = os.Setenv("FIREWALL_SNAPSHOT_DIR", filepath.Join(directory, "snapshots")); err != nil {
		return err
	}
	root := service.RegisterRoot(db, "browser-fixture")
	svc := &service.Firewall{Service: base.Service{Orm: db}, Actor: 7, Root: root}
	instance, err := svc.CreateInstance(context.Background(), service.InstanceInput{DeviceID: device.Id, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, &actions.DataPermission{UserId: 7, DataScope: actions.DataScopeAll})
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "firewall-browser-fixture", Key: key, Timeout: time.Hour, PayloadFunc: func(value interface{}) jwt.MapClaims { return value.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		return err
	}
	token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": "admin", "roleid": 1, "deptid": 1, "datascope": actions.DataScopeAll})
	if err != nil {
		return err
	}
	worker := service.NewWorker(svc)
	worker.Start()
	for name, value := range map[string]string{"NC_HILLSTONE_USERNAME": "fixture-only", "NC_HILLSTONE_PASSWORD": "fixture-only", "NC_HOST_KEY_POLICY": "strict", "NC_KNOWN_HOSTS": ""} {
		if err = os.Setenv(name, value); err != nil {
			return err
		}
	}
	var opens, reads, closes atomic.Int64
	collector := service.NewCollectionWorker(svc, func(ctx context.Context, source service.CollectionSource) (native.Reader, error) {
		if source.Device.Id != device.Id {
			return nil, service.ErrInvalid
		}
		opens.Add(1)
		return &fixtureReader{instance: source.Instance, reads: &reads, closes: &closes}, nil
	})
	collector.Start()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		worker.Close(ctx)
		collector.Close(ctx)
	}()
	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(func(c *gin.Context) {
		if c.GetHeader("Origin") == origin {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "X-Firewall-Fixture")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Header("X-Firewall-Fixture", "real-go-sqlite")
		c.Set("db", db.WithContext(c.Request.Context()))
		c.Next()
	})
	engine.GET("/__fixture", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"token": token, "instanceId": instance.ID, "configuration": configuration})
	})
	engine.GET("/__fixture/evidence", func(c *gin.Context) {
		counts := map[string]int64{}
		for name, model := range map[string]interface{}{"imports": &models.Import{}, "snapshots": &models.Snapshot{}, "requests": &models.Request{}, "rows": &models.RequestRow{}, "analyses": &models.Analysis{}, "drafts": &models.Draft{}} {
			var count int64
			if e := db.Model(model).Count(&count).Error; e != nil {
				c.JSON(500, gin.H{"error": e.Error()})
				return
			}
			counts[name] = count
		}
		var current models.Instance
		if e := db.First(&current, instance.ID).Error; e != nil {
			c.JSON(500, gin.H{"error": e.Error()})
			return
		}
		c.JSON(200, gin.H{"counts": counts, "currentRevision": current.CurrentRevision, "settingsVersion": current.SettingsVersion, "collection": gin.H{"opens": opens.Load(), "reads": reads.Load(), "closes": closes.Load()}})
	})
	firewallrouter.InitBusinessRouter(engine, auth)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: engine, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	fmt.Printf("firewall browser fixture ready at %s (temporary SQLite and snapshots)\n", addr)
	select {
	case err = <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
