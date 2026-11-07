package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/storage/cache"
	"gorm.io/gorm"

	otherrouter "go-admin/app/other/router"
	"go-admin/common/health"
	ext "go-admin/config"
)

// The seconds in the configuration and the durations the sequence waits on are
// two spellings of one budget, and only one of them is printed at start-up.
func TestBudgetFromSeconds(t *testing.T) {
	got := budgetFrom(ext.ShutdownBudget{Drain: 10, Server: 5, Cleanup: 3})
	want := budget{
		drain:   10 * time.Second,
		server:  5 * time.Second,
		cleanup: 3 * time.Second,
	}
	if got != want {
		t.Errorf("budgetFrom = %+v, want %+v", got, want)
	}
}

// The package variables and config.Default*Seconds have to say the same thing.
// They are the same default written twice - once as durations for the shutdown
// and once as seconds for the fallback - and a deployment that configures
// nothing is entitled to one answer, not two.
func TestDefaultBudgetIsTheConfiguredFallback(t *testing.T) {
	unconfigured, err := ext.Shutdown{}.Budget()
	if err != nil {
		t.Fatalf("the empty section did not resolve: %v", err)
	}
	if got, want := defaultBudget(), budgetFrom(unconfigured); got != want {
		t.Errorf("defaultBudget = %+v, want the unconfigured budget %+v", got, want)
	}
}

// The rate limiter must not answer for the probes.
//
// It is installed on the engine, so without this the probes are limited like
// any other route and answer 429 above the threshold. A liveness probe that
// collects 429s fails its threshold and the container is restarted - taking
// capacity out of a deployment that is already short of it and pushing the
// rest closer to the threshold. The limiter working exactly as designed is
// what would cause it.
//
// The stand-in rejects everything rather than being a real limiter: what is
// under test is which requests reach it, and a real one would need the traffic
// to cross a threshold before it said anything.
func TestTheProbesSkipTheRateLimiter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var reached []string
	r := gin.New()
	r.Use(exemptProbes(func(c *gin.Context) {
		reached = append(reached, c.FullPath())
		c.AbortWithStatus(http.StatusTooManyRequests)
	}))
	v1 := r.Group(otherrouter.APIPrefix)
	otherrouter.RegisterMonitorRouter(v1)
	v1.GET("/business", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, tc := range []struct {
		path    string
		limited bool
	}{
		{otherrouter.APIPrefix + otherrouter.HealthPath, false},
		{otherrouter.APIPrefix + otherrouter.ReadyPath, false},
		// Not a probe, and deliberately not exempt: the exemption is for the
		// two routes an orchestrator acts on, not for everything under
		// /api/v1 that happens to be unauthenticated.
		{otherrouter.APIPrefix + "/metrics", true},
		{otherrouter.APIPrefix + "/business", true},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if tc.limited {
				if w.Code != http.StatusTooManyRequests {
					t.Errorf("answered %d, want the middleware's 429 - it was skipped for a route that is not a probe", w.Code)
				}
				return
			}
			if w.Code == http.StatusTooManyRequests {
				t.Errorf("answered 429; a probe that can be rate-limited gets the container restarted under load")
			}
		})
	}

	// Said separately, because a probe could also answer 429 by itself: what
	// has to be true is that the middleware never saw the request.
	for _, p := range reached {
		if probePaths[p] {
			t.Errorf("the middleware ran for %s", p)
		}
	}
}

// /health has to stay 200 while draining, and it is the assertion most easily
// lost by accident: making the liveness probe follow the readiness flag reads
// like tidying up, and it turns every rolling restart into a kubelet-issued
// kill part-way through the drain.
func TestHealthStaysUpWhileDraining(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	v1 := r.Group(otherrouter.APIPrefix)
	otherrouter.RegisterMonitorRouter(v1)

	ask := func(path string) int {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}

	if got := ask(otherrouter.APIPrefix + otherrouter.HealthPath); got != http.StatusOK {
		t.Fatalf("/health answered %d before draining, want 200", got)
	}

	// Process-wide and one-way - nothing clears it - so this is the last thing
	// in this package that may run in-process and care. Everything else that
	// exercises draining does so in a child process of its own.
	health.BeginDraining()

	if got := ask(otherrouter.APIPrefix + otherrouter.HealthPath); got != http.StatusOK {
		t.Errorf("/health answered %d while draining, want 200 - liveness is "+
			"\"should I restart you\", and the answer during a drain is no", got)
	}
	if got := ask(otherrouter.APIPrefix + otherrouter.ReadyPath); got != http.StatusServiceUnavailable {
		t.Errorf("/ready answered %d while draining, want 503", got)
	}
}

func TestNamedTenantProbesBypassRequestDatabaseSelection(t *testing.T) {
	// Draining is intentionally irreversible. Exercise readiness in a fresh
	// process so this test does not depend on the order of shutdown tests.
	if os.Getenv("GO_ADMIN_PROBE_CHILD") != "1" {
		child := exec.Command(os.Args[0], "-test.run=^TestNamedTenantProbesBypassRequestDatabaseSelection$")
		child.Env = append(os.Environ(), "GO_ADMIN_PROBE_CHILD=1")
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("probe child: %v\n%s", err, output)
		}
		return
	}
	freshRuntime(t)
	savedSecret := config.JwtConfig.Secret
	config.JwtConfig.Secret = "probe-test-secret"
	t.Cleanup(func() { config.JwtConfig.Secret = savedSecret })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	t.Cleanup(func() { pool.Close() })
	sdk.Runtime.SetDbByTenant("tenant.example", db)
	cache := cache.NewMemoryWithCleanupInterval(0)
	t.Cleanup(func() { cache.Close() })
	sdk.Runtime.SetCacheAdapter(cache)
	initRouter()
	r := sdk.Runtime.GetEngine().(*gin.Engine)
	otherrouter.RegisterMonitorRouter(r.Group(otherrouter.APIPrefix))
	for _, path := range []string{"/api/v1/health", "/api/v1/ready"} {
		req := httptest.NewRequest("GET", path, nil)
		req.Host = "127.0.0.1:8000"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("probe %s failed for pod Host: %d %s", path, w.Code, w.Body.String())
		}
	}
	pool.Close()
	req := httptest.NewRequest("GET", "/api/v1/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("unhealthy tenant database accepted: %d", w.Code)
	}
}
