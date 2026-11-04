package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func redisTestDatasource(address string) models.Datasource {
	return models.Datasource{Name: "cache", Type: "redis", URL: "redis://" + address, AuthType: "password", AuthConfig: `{"password":"redis-secret"}`, Config: `{"database":3}`, Status: 1}
}

func TestRedisAuthenticationAndDatabaseSelection(t *testing.T) {
	for _, authType := range []string{"none", "password", "basic"} {
		t.Run(authType, func(t *testing.T) {
			server := miniredis.RunT(t)
			ds := redisTestDatasource(server.Addr())
			ds.AuthType = authType
			switch authType {
			case "none":
				ds.AuthConfig = ""
			case "password":
				server.RequireAuth("redis-secret")
			case "basic":
				server.RequireUserAuth("reader", "redis-secret")
				ds.AuthConfig = `{"username":"reader","password":"redis-secret"}`
			}
			server.Select(0)
			server.Set("marker", "db0")
			server.Select(3)
			server.Set("marker", "db3")
			ok, msg := testConnectionForDatasource(ds)
			if !ok || !strings.Contains(msg, "数据库 3") {
				t.Fatalf("test connection: %v %s", ok, msg)
			}
			// Verify the actual driver selected the requested DB, without changing application data.
			client, cleanup, err := openRedis(ds)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			value, err := client.Get(context.Background(), "marker").Result()
			if err != nil || value != "db3" {
				t.Fatalf("database selection failed: %s %v", value, err)
			}
			if len(server.DB(0).Keys()) != 1 || len(server.DB(3).Keys()) != 1 {
				t.Fatal("connection test changed keys")
			}
		})
	}
}

func TestRedisValidation(t *testing.T) {
	good := redisTestDatasource("cache.example.com:6379")
	cfg, err := redisClientOptions(good)
	if err != nil || cfg.DB != 3 || cfg.Password != "redis-secret" || cfg.TLSConfig != nil {
		t.Fatal("bad Redis configuration", err)
	}
	ipv6 := good
	ipv6.URL = "rediss://[::1]"
	ipv6.Config = ""
	cfg, err = redisClientOptions(ipv6)
	if err != nil || cfg.Addr != "[::1]:6379" || cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify {
		t.Fatal("IPv6/TLS defaults", err)
	}
	for _, tc := range []struct{ url, authType, auth, settings string }{
		{url: "http://cache:6379"}, {url: "redis://:secret@cache:6379"}, {url: "redis://cache:0"}, {url: "redis://cache:65536"}, {url: "redis://cache/3"}, {url: "redis://cache?db=3"},
		{authType: "bearer"}, {authType: "basic", auth: `{"password":"secret"}`}, {auth: `{"password":""}`},
		{settings: `{"database":-1}`}, {settings: `{"database":65536}`}, {settings: `{"database":1.5}`}, {settings: `{"database":"3"}`},
		{settings: `{"tls":{"ca_certificate":"bad"}}`}, {url: "rediss://cache", settings: `{"tls":{"ca_certificate":"bad"}}`},
	} {
		ds := good
		if tc.url != "" {
			ds.URL = tc.url
		}
		if tc.authType != "" {
			ds.AuthType = tc.authType
		}
		if tc.auth != "" {
			ds.AuthConfig = tc.auth
		}
		if tc.settings != "" {
			ds.Config = tc.settings
		}
		if err := validateDatasourceConnection(ds); err == nil {
			t.Fatalf("invalid configuration accepted: %+v", tc)
		}
	}
}

func TestRedisConnectionErrorsAndCancellation(t *testing.T) {
	server := miniredis.RunT(t)
	server.RequireAuth("redis-secret")
	ds := redisTestDatasource(server.Addr())
	ds.AuthConfig = `{"password":"wrong-secret"}`
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "认证失败") || strings.Contains(msg, "wrong-secret") {
		t.Fatalf("authentication error: %v %s", ok, msg)
	}
	ds.AuthType = "none"
	ds.AuthConfig = ""
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "认证失败") {
		t.Fatalf("missing authentication: %v %s", ok, msg)
	}
	server.RequireAuth("")
	for _, tc := range []struct{ raw, message string }{{"NOPERM redis-secret not allowed", "权限不足"}, {"ERR DB index is out of range redis-secret", "数据库编号无效"}} {
		server.SetError(tc.raw)
		if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, tc.message) || strings.Contains(msg, "redis-secret") {
			t.Fatalf("error mapping: %v %s", ok, msg)
		}
	}
	server.SetError("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok, _ := testConnectionForDatasourceContext(ctx, ds); ok {
		t.Fatal("cancelled test succeeded")
	}
}

func TestRedisTLS(t *testing.T) {
	reference := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := reference.TLS.Certificates[0]
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: reference.Certificate().Raw}))
	reference.Close()
	server := miniredis.NewMiniRedis()
	if err := server.StartTLS(&tls.Config{Certificates: []tls.Certificate{certificate}}); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	server.RequireAuth("redis-secret")
	ds := redisTestDatasource(server.Addr())
	ds.URL = "rediss://" + server.Addr()
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "证书校验失败") {
		t.Fatalf("untrusted server accepted: %v %s", ok, msg)
	}
	raw, _ := json.Marshal(map[string]any{"database": 3, "tls": map[string]any{"ca_certificate": ca}})
	ds.Config = string(raw)
	if ok, msg := testConnectionForDatasource(ds); !ok {
		t.Fatal(msg)
	}
}

func TestRedisDatasourceLifecycle(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "redis.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	server := miniredis.RunT(t)
	server.RequireAuth("redis-secret")
	router := gin.New()
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.GET("/sources/:id", GetDatasourceAuth)
	router.PUT("/sources/:id", UpdateDatasourceWithTunnel)
	router.POST("/sources/:id/test", TestSavedDatasourceConnection)
	router.DELETE("/sources/:id", DeleteDatasource)
	call := func(method, path string, body any) string {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || response.Code != 0 {
			t.Fatalf("%s %s: %s", method, path, w.Body.String())
		}
		return w.Body.String()
	}
	ds := redisTestDatasource(server.Addr())
	call("POST", "/sources", ds)
	var saved models.Datasource
	db.First(&saved)
	if saved.Category != "database" || saved.Capabilities != "redis" || isMetricDatasourceType(saved.Type) {
		t.Fatal("wrong Redis type definition")
	}
	path := fmt.Sprintf("/sources/%d", saved.ID)
	for _, url := range []string{"/sources", path} {
		if body := call("GET", url, nil); strings.Contains(body, "redis-secret") || !strings.Contains(body, "has_password") {
			t.Fatal("password exposed or configured indicator missing")
		}
	}
	ds.AuthConfig = ""
	ds.Name = "updated cache"
	call("PUT", path, ds)
	call("POST", path+"/test", ds)
	db.First(&saved, saved.ID)
	if !strings.Contains(saved.AuthConfig, "redis-secret") {
		t.Fatal("password lost")
	}
	// Switching to no authentication explicitly removes stored credentials.
	ds.AuthType = "none"
	call("PUT", path, ds)
	db.First(&saved, saved.ID)
	if saved.AuthConfig != "" {
		t.Fatal("credentials retained after disabling authentication")
	}
	server.RequireAuth("")
	call("POST", path+"/test", ds)
	call("DELETE", path, nil)
	var count int64
	db.Model(&models.Datasource{}).Count(&count)
	if count != 0 {
		t.Fatal("datasource not deleted")
	}
}
