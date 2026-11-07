package router_test

import (
	"errors"
	"go-admin/app/admin/models"
	"gorm.io/gorm"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/gorilla/websocket"
	"go-admin/app/admin/router"
	"go-admin/common/middleware"
)

func TestWebsocketRejectsForeignOrigin(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "*", "websocket-admin")
	token := tokenFor(t, mw, db, "websocket-admin", "*")
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("temp/logs/job", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("temp/logs/job/db-20200820.log", nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(middleware.WithContextDb)
	router.InitSysRouter(r, mw)
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/test-connection/log?token="+token, http.Header{"Origin": []string{"https://foreign.example"}})
	if conn != nil {
		conn.Close()
		t.Fatal("foreign origin upgraded")
	}
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wanted forbidden handshake: response=%v error=%v", resp, err)
	}
}

func TestWebsocketOwnedConnectionAndActualMessages(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "*", "stream-admin")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	second := models.SysUser{UserId: 2, Username: "second-admin", Password: "Audit-password-1", RoleId: 1, Status: "2"}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	firstToken := tokenFor(t, mw, db, "stream-admin", "*")
	secondToken := tokenFor(t, mw, db, "second-admin", "*")
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("temp/logs/job", 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile("temp/logs/job/db-20200820.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	capture, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer capture.Close()
	old := os.Stdout
	os.Stdout = capture
	defer func() { os.Stdout = old }()
	r := gin.New()
	r.Use(middleware.WithContextDb)
	router.InitSysRouter(r, mw)
	srv := httptest.NewServer(r)
	defer srv.Close()
	dial := func(token string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/opaque-uuid/log?token="+token, http.Header{"Origin": []string{srv.URL}})
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	first := dial(firstToken)
	defer first.Close()
	read := func(conn *websocket.Conn, want string) {
		t.Helper()
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil || !strings.Contains(string(message), want) {
			t.Fatalf("message=%s error=%v want=%s", message, err, want)
		}
	}
	if err := first.WriteMessage(websocket.TextMessage, []byte("echo-fixture")); err != nil {
		t.Fatal(err)
	}
	read(first, "echo-fixture")
	if _, err := file.WriteString("\"stream-line\"\n"); err != nil {
		t.Fatal(err)
	}
	read(first, "stream-line")
	closePath := func(token string) {
		t.Helper()
		req, err := http.NewRequest("GET", srv.URL+"/wslogout/opaque-uuid/log", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("disconnect status=%d", response.StatusCode)
		}
	}
	closePath(secondToken)
	if err := first.WriteMessage(websocket.TextMessage, []byte("still-owned")); err != nil {
		t.Fatal(err)
	}
	read(first, "still-owned")
	secondConn := dial(secondToken)
	defer secondConn.Close()
	closePath(secondToken)
	secondConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := secondConn.ReadMessage(); err == nil {
		t.Fatal("own disconnect left connection open")
	}
	if err := first.WriteMessage(websocket.TextMessage, []byte("other-user-safe")); err != nil {
		t.Fatal(err)
	}
	read(first, "other-user-safe")
	req, err := http.NewRequest("POST", srv.URL+"/api/v1/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+firstToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	first.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("logout did not close own websocket")
	}
	os.Stdout = old
	if _, err := capture.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	logged, err := io.ReadAll(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), firstToken) || strings.Contains(string(logged), "echo-fixture") {
		t.Fatal("websocket printed token or message payload")
	}
}

func TestWebsocketRejectsNamedTenantAndUnsupportedChannel(t *testing.T) {
	for _, tc := range []struct{ name, tenant, channel string }{
		{"named tenant", "isolated.test", "log"},
		{"unsupported channel", "*", "private-channel"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := auth(t)
			db := dbFor(t, tc.tenant, "denied-admin")
			token := tokenFor(t, mw, db, "denied-admin", tc.tenant)
			r := gin.New()
			r.Use(middleware.WithContextDb)
			router.InitSysRouter(r, mw)
			req := httptest.NewRequest("GET", "/ws/uuid/"+tc.channel, nil)
			req.Host = tc.tenant
			req.Header.Set("Authorization", "Bearer "+token)
			out := httptest.NewRecorder()
			r.ServeHTTP(out, req)
			if out.Code != 403 {
				t.Fatalf("policy did not reject: %d %s", out.Code, out.Body.String())
			}
		})
	}
}

func TestWebsocketRechecksRevokedSessions(t *testing.T) {
	mw := auth(t)
	db := dbFor(t, "*", "revocable-stream-admin")
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	token := tokenFor(t, mw, db, "revocable-stream-admin", "*")
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("temp/logs/job", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("temp/logs/job/db-20200820.log", nil, 0600); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(middleware.WithContextDb)
	router.InitSysRouter(r, mw)
	srv := httptest.NewServer(r)
	defer srv.Close()
	endpoint := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/revocation-uuid/log?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	duplicate, resp, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if duplicate != nil {
		duplicate.Close()
		t.Fatal("duplicate replaced live connection")
	}
	if err == nil || resp == nil || resp.StatusCode != 409 {
		t.Fatalf("duplicate handshake: %v %v", resp, err)
	}
	if err := db.Table("sys_user").Where("user_id=1").UpdateColumn("auth_version", gorm.Expr("auth_version+1")).Error; err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	_, _, err = conn.ReadMessage()
	var networkError net.Error
	if err == nil || (errors.As(err, &networkError) && networkError.Timeout()) {
		t.Fatalf("remote revocation did not close websocket: %v", err)
	}
	replay, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if replay != nil {
		replay.Close()
		t.Fatal("revoked token reopened websocket")
	}
	if err == nil {
		t.Fatal("revoked handshake accepted")
	}
}

func TestWebsocketDevelopmentLoopbackOrigins(t *testing.T) {
	for _, tc := range []struct {
		name, mode, origin string
		allowed            bool
	}{
		{"development localhost cross-port", "dev", "http://localhost:9527", true},
		{"development IPv4 cross-port", "dev", "http://127.0.0.1:9527", true},
		{"production localhost cross-port", "prod", "http://localhost:9527", false},
		{"development foreign host", "dev", "http://foreign.example:9527", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mw := auth(t)
			config.ApplicationConfig.Mode = tc.mode
			db := dbFor(t, "*", "development-admin")
			token := tokenFor(t, mw, db, "development-admin", "*")
			t.Chdir(t.TempDir())
			if err := os.MkdirAll("temp/logs/job", 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("temp/logs/job/db-20200820.log", nil, 0600); err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.Use(middleware.WithContextDb)
			router.InitSysRouter(r, mw)
			srv := httptest.NewServer(r)
			defer srv.Close()
			conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/development-uuid/log?token="+token, http.Header{"Origin": []string{tc.origin}})
			if !tc.allowed {
				if conn != nil {
					conn.Close()
					t.Fatal("disallowed origin upgraded")
				}
				if err == nil || resp == nil || resp.StatusCode != 403 {
					t.Fatalf("expected 403: response=%v err=%v", resp, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.WriteMessage(websocket.TextMessage, []byte("development echo")); err != nil {
				t.Fatal(err)
			}
			conn.SetReadDeadline(time.Now().Add(time.Second))
			_, message, err := conn.ReadMessage()
			if err != nil || string(message) != "development echo" {
				t.Fatalf("message=%s err=%v", message, err)
			}
		})
	}
}
