package auth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gatework/motor/common/account"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
)

// The shipped UI consumes this legacy process-wide log, not a tenant log.
// Until the source itself is tenant-aware, only the single-tenant administrator
// can subscribe. Connection UUIDs remain opaque client-generated identifiers.
const websocketLogPath = "temp/logs/job/db-20200820.log"

type socketKey struct {
	tenant      string
	user        int
	id, channel string
}
type logSocket struct {
	key     socketKey
	conn    *websocket.Conn
	cancel  context.CancelFunc
	writeMu sync.Mutex
}

var logSockets = struct {
	sync.Mutex
	entries map[socketKey]*logSocket
}{entries: make(map[socketKey]*logSocket)}

// WebsocketHandlers preserves /ws/:id/log and /wslogout/:id/log, with owned
// connections and the actual log/echo protocol used by the bundled UI. It does
// not expose core's unrestricted global pubsub manager or log token/message data.
func WebsocketHandlers(mw *jwt.GinJWTMiddleware) (gin.HandlerFunc, gin.HandlerFunc) {
	return func(c *gin.Context) { connectWebsocket(c, mw) }, disconnectWebsocket
}

func websocketOwner(c *gin.Context) (socketKey, bool) {
	key := socketKey{tenant: Tenant(c.Request.Host), user: user.ID(c), id: c.Param("id"), channel: c.Param("channel")}
	if key.user <= 0 || key.tenant != "*" || !account.IsAdmin(user.RoleName(c)) || key.channel != "log" || key.id == "" || len(key.id) > 128 || !sameWebsocketOrigin(c.Request) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 403, "msg": "无权访问日志流"})
		return socketKey{}, false
	}
	return key, true
}

func sameWebsocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	// The bundled development UI runs on a separate loopback port. Public
	// origins never receive this exception, including when development is set.
	requestURL, err := url.Parse("http://" + r.Host)
	return config.Current().Settings.Application.Mode == pkg.ModeDev.String() && err == nil &&
		loopbackWebsocketHost(parsed.Hostname()) && loopbackWebsocketHost(requestURL.Hostname())
}

func loopbackWebsocketHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func closeUserWebsockets(tenant string, id int) {
	logSockets.Lock()
	var sessions []*logSocket
	for key, session := range logSockets.entries {
		if key.tenant == tenant && key.user == id && session != nil {
			sessions = append(sessions, session)
		}
	}
	logSockets.Unlock()
	for _, session := range sessions {
		session.close()
	}
}

func watchWebsocketSession(ctx context.Context, c *gin.Context, mw *jwt.GinJWTMiddleware, session *logSocket) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			token, err := mw.ParseTokenString(c.GetString("JWT_TOKEN"))
			if err != nil || !token.Valid {
				session.close()
				return
			}
			db, err := pkg.GetORM(c)
			if err != nil {
				session.close()
				return
			}
			check, cancel := context.WithTimeout(ctx, 2*time.Second)
			c.Set("db", db.WithContext(check))
			allowed := mw.Authorizator(IdentityHandler(c), c)
			cancel()
			if !allowed {
				session.close()
				return
			}
		}
	}
}

func tailWebsocketLog(ctx context.Context, file *os.File, session *logSocket) {
	reader := bufio.NewReaderSize(file, 64<<10)
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	pending := make([]byte, 0, 4096)
	for {
		if ctx.Err() != nil {
			return
		}
		fragment, err := reader.ReadSlice('\n')
		if len(pending)+len(fragment) > 64<<10 {
			session.close()
			return
		}
		pending = append(pending, fragment...)
		if err == nil {
			var data any = string(pending)
			if json.Valid(pending) {
				data = json.RawMessage(pending)
			}
			message, marshalErr := json.Marshal(gin.H{"code": 200, "data": data})
			if marshalErr != nil || session.write(message) != nil {
				session.close()
				return
			}
			pending = pending[:0]
			continue
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
			session.close()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (s *logSocket) close() { s.cancel(); _ = s.conn.Close() }

func (s *logSocket) write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

func connectWebsocket(c *gin.Context, mw *jwt.GinJWTMiddleware) {

	key, ok := websocketOwner(c)
	if !ok {
		return
	}
	file, err := os.Open(websocketLogPath)
	if err != nil {
		c.AbortWithStatusJSON(503, gin.H{"code": 503, "msg": "日志流暂不可用"})
		return
	}
	defer file.Close()
	if _, err = file.Seek(0, io.SeekEnd); err != nil {
		c.AbortWithStatusJSON(503, gin.H{"code": 503, "msg": "日志流暂不可用"})
		return
	}
	// Reserve before upgrading: a duplicate UUID must not replace a live socket.
	logSockets.Lock()
	if _, exists := logSockets.entries[key]; exists {
		logSockets.Unlock()
		c.AbortWithStatusJSON(409, gin.H{"code": 409, "msg": "连接已存在"})
		return
	}
	logSockets.entries[key] = nil
	logSockets.Unlock()
	var owned *logSocket
	defer func() {
		logSockets.Lock()
		if logSockets.entries[key] == owned {
			delete(logSockets.entries, key)
		}
		logSockets.Unlock()
	}()
	upgrader := websocket.Upgrader{CheckOrigin: sameWebsocketOrigin}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := &logSocket{key: key, conn: conn, cancel: cancel}
	owned = session
	defer session.close()
	conn.SetReadLimit(64 << 10)
	logSockets.Lock()
	logSockets.entries[key] = session
	logSockets.Unlock()
	serveWebsocket(ctx, c, file, session, mw)
}
func disconnectWebsocket(c *gin.Context) {

	key, ok := websocketOwner(c)
	if !ok {
		return
	}
	logSockets.Lock()
	session := logSockets.entries[key]
	if session != nil {
		delete(logSockets.entries, key)
	}
	logSockets.Unlock()
	if session != nil {
		session.close()
	}
	c.JSON(200, gin.H{"code": 200, "data": "ws close success", "msg": "success"})
}
func serveWebsocket(ctx context.Context, c *gin.Context, file *os.File, session *logSocket, mw *jwt.GinJWTMiddleware) {

	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); tailWebsocketLog(ctx, file, session) }()
	validation := c.Copy()
	go func() { defer workers.Done(); watchWebsocketSession(ctx, validation, mw, session) }()
	defer func() { session.close(); workers.Wait() }()
	for {
		_, message, err := session.conn.ReadMessage()
		if err != nil {
			return
		}
		if err := session.write(message); err != nil {
			return
		}
	}
}
