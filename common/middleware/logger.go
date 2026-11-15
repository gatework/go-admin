package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/storage"

	"github.com/gatework/motor/common/auditlog"
)

// AccessLog 日志记录到文件
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		log := api.GetRequestLogger(c)
		// 开始时间
		startTime := time.Now()
		// 处理请求
		//
		// The body is only read when it has a destination. operParam below is
		// the only consumer, and it is written when logger.enableddb is on -
		// off in the shipped configuration, where reading the body was a copy
		// of every request made and discarded.
		var body string
		if config.Current().Settings.Logger.EnabledDB && !sensitiveAccountPath(c.Request.URL.Path) {
			body = readOperParam(c, log)
		}

		c.Next()
		if skipsOperationLog(c.Request.URL.Path) {
			return
		}
		// 结束时间
		endTime := time.Now()
		if c.Request.Method == http.MethodOptions {
			return
		}

		rt, bl := c.Get("result")
		var result = ""
		if bl && !sensitiveAccountPath(c.Request.URL.Path) {
			rb, err := json.Marshal(rt)
			if err != nil {
				log.Warnf("json Marshal result error, %s", err.Error())
			} else {
				result = string(rb)
			}
		}

		st, bl := c.Get("status")
		var statusBus = 0
		if bl {
			statusBus, _ = st.(int)
		}

		// 请求方式
		reqMethod := c.Request.Method
		// 请求路由
		reqURI := redactLogURL(c.Request.URL)
		// 状态码
		statusCode := c.Writer.Status()
		// 请求IP
		clientIP := c.ClientIP()
		// 执行时间
		latencyTime := endTime.Sub(startTime)
		// 日志格式
		logData := map[string]any{
			"statusCode":  statusCode,
			"latencyTime": latencyTime,
			"clientIP":    clientIP,
			"method":      reqMethod,
			"uri":         reqURI,
		}
		log.WithFields(logData).Info()
		if c.Request.Method != http.MethodOptions && config.Current().Settings.Logger.EnabledDB && statusCode != 404 {
			SetDBOperLog(c, clientIP, statusCode, reqURI, reqMethod, latencyTime, body, result, statusBus)
		}
	}
}

// operParamLimit caps what is copied out of a request body for the operation
// log. A file upload is a POST like any other and reaches this middleware
// before any handler, so without a limit the whole upload is held in memory to
// write a log row - a 16MB upload allocated about 67MB. The limit also keeps
// the value inside the column, which is TEXT.
const operParamLimit = 32 << 10

// readOperParam copies the start of the request body for the operation log and
// leaves the request readable by the handler.
//
// The body is not buffered whole: the handler reads the part copied here from
// memory and the rest straight from the connection, so what this holds is
// bounded by operParamLimit however large the request is.
func readOperParam(c *gin.Context, log *logger.Helper) string {
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodGet, http.MethodDelete:
	default:
		return ""
	}
	if c.Request.Body == nil {
		return ""
	}

	rest := c.Request.Body
	head := make([]byte, operParamLimit)
	n, err := io.ReadFull(rest, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		log.Warnf("read body for the operation log: %s", err)
	}
	head = head[:n]

	c.Request.Body = readCloser{
		Reader: io.MultiReader(bytes.NewReader(head), rest),
		Closer: rest,
	}
	if n == operParamLimit {
		return "[body omitted: size limit]"
	}
	switch strings.ToLower(strings.Split(c.GetHeader("Content-Type"), ";")[0]) {
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(head))
		if err != nil {
			return "[body omitted]"
		}
		for key := range values {
			if sensitiveLogKey(key) {
				values.Set(key, "[redacted]")
			}
		}
		return values.Encode()
	case "application/json", "":
		return redactLogJSON(string(head))
	default:
		return "[body omitted]"
	}
}

type readCloser struct {
	io.Reader
	io.Closer
}

// operaLogFields builds the message written to the operation log queue.
//
// Split out of SetDBOperLog so the field set can be asserted in a test: the
// consumer on the other end of the queue reads these keys by name, so a
// dropped or renamed key costs a column in sys_opera_log and reports nothing.
func operaLogFields(c *gin.Context, clientIP string, statusCode int, reqURI string, reqMethod string, latencyTime time.Duration, body string, result string, status int) map[string]any {
	l := make(map[string]any)
	l["_fullPath"] = c.FullPath()
	l["operUrl"] = reqURI
	l["operIp"] = clientIP
	l["operLocation"] = ""
	l["operName"] = user.Name(c)
	l["requestMethod"] = reqMethod
	l["operParam"] = body
	l["operTime"] = time.Now()
	l["jsonResult"] = redactLogJSON(result)
	l["latencyTime"] = latencyTime.String()
	l["statusCode"] = statusCode
	l["userAgent"] = c.Request.UserAgent()
	l["createBy"] = user.ID(c)
	l["updateBy"] = user.ID(c)
	if status == http.StatusOK {
		l["status"] = auditlog.StatusSucceeded
	} else {
		l["status"] = auditlog.StatusFailed
	}
	return l
}

// SetDBOperLog 写入操作日志表 fixme 该方法后续即将弃用
func SetDBOperLog(c *gin.Context, clientIP string, statusCode int, reqURI string, reqMethod string, latencyTime time.Duration, body string, result string, status int) {

	log := api.GetRequestLogger(c)
	l := operaLogFields(c, clientIP, statusCode, reqURI, reqMethod, latencyTime, body, result, status)
	payload, err := json.Marshal(l)
	if err != nil {
		log.Errorf("encode log message: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	message := storage.Message{Topic: auditlog.OperationTopic, Values: map[string]any{"payload": string(payload)}}
	if err := sdk.Runtime.QueueForTenant(c.Request.Host).Publish(ctx, message); err != nil {
		log.Errorf("publish log message: %v", err)
	}
}

// Account secrets, firewall configurations, delivery scripts and device output must never reach operation-log storage.
// skipsOperationLog reports the session routes whose record is the login log,
// not the operation log. It matches routes exactly: matching any path that
// contains "login" also dropped every change made through /sys-login-log, so
// deleting login records left no audit trail.
func skipsOperationLog(path string) bool {
	return path == "/api/v1/login" || path == "/api/v1/logout" || strings.HasPrefix(path, "/wslogout/")
}

func sensitiveAccountPath(path string) bool {
	return strings.HasPrefix(path, "/api/v1/network-firewall/") || path == "/api/v1/network-firewall" || strings.HasPrefix(path, "/api/v1/network-wireless/") || path == "/api/v1/network-wireless" || strings.HasPrefix(path, "/api/v1/network-fabrics/") || path == "/api/v1/network-fabrics" || strings.HasPrefix(path, "/api/v1/network-delivery/") || path == "/api/v1/network-delivery" || strings.HasPrefix(path, "/api/v1/account/") || path == "/api/v1/login" || strings.HasPrefix(path, "/api/v1/user/pwd/")
}

// Invalid/truncated JSON and arbitrary upload bodies are never retained: they
// cannot be reliably scrubbed by field name.
func redactLogJSON(input string) string {
	if input == "" {
		return ""
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return "[body omitted]"
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return "[body omitted]"
	}
	redactLogValue(value)
	result, err := json.Marshal(value)
	if err != nil {
		return "[body omitted]"
	}
	return string(result)
}

func sensitiveLogKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	for _, fragment := range []string{"password", "passwd", "pwd", "token", "secret", "authorization", "cookie", "privatekey", "apikey"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func redactLogValue(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if sensitiveLogKey(key) {
				v[key] = "[redacted]"
			} else {
				redactLogValue(child)
			}
		}
	case []any:
		for _, child := range v {
			redactLogValue(child)
		}
	}
}

func redactLogURL(input *url.URL) string {
	safe := *input
	query, err := url.ParseQuery(input.RawQuery)
	if err != nil {
		safe.RawQuery = ""
		return safe.RequestURI()
	}
	for key := range query {
		if sensitiveLogKey(key) {
			query.Set(key, "[redacted]")
		}
	}
	safe.RawQuery = query.Encode()
	return safe.RequestURI()
}
