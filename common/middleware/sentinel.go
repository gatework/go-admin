package middleware

import (
	"net/http"

	"github.com/alibaba/sentinel-golang/core/system"
	sentinel "github.com/alibaba/sentinel-golang/pkg/adapters/gin"
	"github.com/gin-gonic/gin"

	log "github.com/gatework/motor-core/logger"

	"github.com/gatework/motor/config"
)

// Sentinel 限流
//
// The threshold comes from extend.ratelimit.inboundqps; see config.RateLimit
// for the values it accepts.
func Sentinel() gin.HandlerFunc {
	qps := config.Ext().RateLimit.Threshold()
	if qps <= 0 {
		log.NewHelper(log.Default()).Info("rate limit disabled by extend.ratelimit.inboundqps")
		return func(c *gin.Context) { c.Next() }
	}

	if _, err := system.LoadRules([]*system.Rule{
		{
			MetricType:   system.InboundQPS,
			TriggerCount: qps,
			// InboundQPS is compared against TriggerCount directly - the
			// adaptive strategy is only consulted for Load and CpuUsage. BBR
			// stood here and read as if the limit adapted to the machine, which
			// it never did.
			Strategy: system.NoAdaptive,
		},
	}); err != nil {
		log.NewHelper(log.Default()).Fatalf("Unexpected error: %+v", err)
	}

	log.NewHelper(log.Default()).Infof("rate limit: %.0f inbound req/s", qps)

	return sentinel.SentinelMiddleware(
		sentinel.WithBlockFallback(func(ctx *gin.Context) {
			// 429, not 200. Everything that reads the status line rather than
			// the body counts a 200 as served: load balancers, metrics,
			// client-side retry, and load tests - a benchmark against the old
			// behaviour reported the limiter's own rejections as successful
			// traffic and overstated throughput by more than tenfold.
			ctx.AbortWithStatusJSON(http.StatusTooManyRequests, map[string]any{
				"msg":  "too many request; the quota used up!",
				"code": http.StatusTooManyRequests,
			})
		}),
	)
}
