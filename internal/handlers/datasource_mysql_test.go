package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/middleware"
	"guanlan/internal/models"
)

func mysqlTestDatasource() models.Datasource {
	return models.Datasource{Name: "mysql-prod", Type: "mysql", URL: "mysql://db.example.com:3306", AuthType: "basic", AuthConfig: `{"username":"reader","password":"p@ss:/?&secret"}`, Config: `{"database":"business"}`, Status: 1}
}

func TestMySQLConnectionConfiguration(t *testing.T) {
	ds := mysqlTestDatasource()
	cfg, err := mysqlDriverConfig(ds)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.User != "reader" || cfg.Passwd != "p@ss:/?&secret" || cfg.DBName != "business" || cfg.Addr != "db.example.com:3306" || cfg.MultiStatements || cfg.AllowAllFiles || cfg.AllowFallbackToPlaintext {
		t.Fatalf("driver configuration mismatch")
	}
	ds.URL = "mysql://[::1]"
	cfg, err = mysqlDriverConfig(ds)
	if err != nil || cfg.Addr != "[::1]:3306" {
		t.Fatalf("IPv6/default port: %v", err)
	}
	ds.Config = `{"database":"business","tls":{"enabled":true}}`
	cfg, err = mysqlDriverConfig(ds)
	if err != nil || cfg.TLS == nil || cfg.TLS.InsecureSkipVerify || cfg.TLS.ServerName != "::1" {
		t.Fatal("TLS verification not configured")
	}
	for _, tc := range []struct{ url, auth, settings string }{
		{url: "http://db.example.com:3306"}, {url: "mysql://reader:secret@db.example.com:3306"}, {url: "mysql://db.example.com:0"}, {url: "mysql://db.example.com:65536"}, {url: "mysql://db.example.com/business"}, {url: "mysql://db.example.com?multiStatements=true"}, {url: "mysql:///tmp/mysql.sock"},
		{auth: `{"username":"reader"}`}, {auth: `{"password":"secret"}`},
		{settings: `{}`}, {settings: `{"database":"bad/db"}`}, {settings: `{"database":"business","tls":{"enabled":true,"ca_certificate":"not-pem"}}`}, {settings: `{"database":"business","tls":{"ca_certificate":"not-pem"}}`},
	} {
		invalid := mysqlTestDatasource()
		if tc.url != "" {
			invalid.URL = tc.url
		}
		if tc.auth != "" {
			invalid.AuthConfig = tc.auth
		}
		if tc.settings != "" {
			invalid.Config = tc.settings
		}
		if err := validateDatasourceConnection(invalid); err == nil {
			t.Fatalf("invalid config accepted: %+v", tc)
		}
	}
}

func TestMySQLSQLValidation(t *testing.T) {
	for _, query := range []string{"SELECT 1", "WITH cte AS (SELECT 1 AS n) SELECT n FROM cte", "SELECT '/*! not executable */' AS value", "SELECT 'INTO OUTFILE; DROP' AS value", "SELECT 1 # UPDATE is only a comment", "SELECT `update` FROM `orders`"} {
		if err := validateExploreSQLDialect(query, true); err != nil {
			t.Fatalf("valid query %q: %v", query, err)
		}
	}
	for _, query := range []string{"SELECT 1; SELECT 2", "DELETE FROM orders", "SELECT 1 INTO OUTFILE '/tmp/x'", "SELECT 1 /*! INTO OUTFILE '/tmp/x' */", "SELECT /*+ MAX_EXECUTION_TIME(0) */ 1", "SELECT /*M! 100100 SQL_NO_CACHE */ 1", "SELECT SLEEP(10)", "SELECT LOAD_FILE('/etc/passwd')", "SELECT GET_LOCK('x',10)", "SELECT * FROM orders FOR UPDATE", "WITH c AS (SELECT 1) DELETE FROM orders"} {
		if err := validateExploreSQLDialect(query, true); err == nil {
			t.Fatalf("unsafe query accepted: %q", query)
		}
	}
}

func TestMySQLResultConversion(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	columns := []*sqlmock.Column{
		sqlmock.NewColumn("id").OfType("BIGINT", int64(0)),
		sqlmock.NewColumn("amount").OfType("DECIMAL", ""),
		sqlmock.NewColumn("body").OfType("TEXT", ""),
		sqlmock.NewColumn("binary").OfType("BLOB", []byte{}),
		sqlmock.NewColumn("nullable").OfType("VARCHAR", ""),
	}
	rows := sqlmock.NewRowsWithColumnDefinition(columns...).AddRow(int64(9007199254740993), []byte("123456789012345.6789"), []byte("观澜"), []byte{0, 255}, nil)
	mock.ExpectQuery("SELECT /*+ MAX_EXECUTION_TIME(25000) */ * FROM ( SELECT id, amount, body, binary, nullable FROM orders ) AS guanlan_result LIMIT 100").WillReturnRows(rows).RowsWillBeClosed()
	mock.ExpectRollback()
	result, err := readMySQLRows(context.Background(), db, SQLQueryRequest{Query: "SELECT id, amount, body, binary, nullable FROM orders", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Data[0]
	if row["id"] != "9007199254740993" || row["amount"] != "123456789012345.6789" || row["body"] != "观澜" || row["binary"] != "0x00ff" || row["nullable"] != nil {
		t.Fatalf("bad conversion: %+v", row)
	}
	if result.Meta[0]["name"] != "id" || result.Meta[0]["type"] != "BIGINT" {
		t.Fatal("missing column metadata")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLQueryFailureHandling(t *testing.T) {
	for _, name := range []string{"query error", "row error", "oversized result", "empty result", "duplicate column"} {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectBegin()
			expected := mock.ExpectQuery("SELECT")
			switch name {
			case "query error":
				expected.WillReturnError(&mysql.MySQLError{Number: 1142, Message: "secret-password"})
			case "row error":
				expected.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(1).RowError(0, errors.New("secret-password"))).RowsWillBeClosed()
			case "oversized result":
				expected.WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(strings.Repeat("a", 15_000_001))).RowsWillBeClosed()
			case "empty result":
				expected.WillReturnRows(sqlmock.NewRows([]string{"value"})).RowsWillBeClosed()
			case "duplicate column":
				expected.WillReturnRows(sqlmock.NewRows([]string{"value", "value"})).RowsWillBeClosed()
			}
			mock.ExpectRollback()
			result, err := readMySQLRows(context.Background(), db, SQLQueryRequest{Query: "SELECT value FROM orders", Limit: 1})
			if name == "empty result" {
				if err != nil || result.Data == nil || len(result.Data) != 0 {
					t.Fatalf("bad empty result: %+v %v", result, err)
				}
			} else if err == nil || strings.Contains(err.Error(), "secret-password") {
				t.Fatalf("missing or unredacted error: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMySQLDatasourceLifecycleAndPermissions(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "mysql.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	userID := uint(1)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.GET("/sources/:id", GetDatasourceAuth)
	router.PUT("/sources/:id", UpdateDatasourceWithTunnel)
	router.DELETE("/sources/:id", DeleteDatasource)
	router.GET("/explore", ExploreDatasources)
	router.POST("/sql", middleware.RequirePermission("query:sql"), QuerySQL)
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		return w
	}
	ds := mysqlTestDatasource()
	if w := call("POST", "/sources", ds); w.Code != 200 || !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatal(w.Body.String())
	}
	var saved models.Datasource
	db.First(&saved)
	if saved.Type != "mysql" || saved.Category != "database" || saved.Capabilities != "sql" {
		t.Fatalf("bad datasource definition")
	}
	path := fmt.Sprintf("/sources/%d", saved.ID)
	for _, url := range []string{"/sources", path, "/explore"} {
		w := call("GET", url, nil)
		if w.Code != 200 || strings.Contains(w.Body.String(), "p@ss") {
			t.Fatal(w.Body.String())
		}
		if url == "/explore" && !strings.Contains(w.Body.String(), `"modes":["sql"]`) {
			t.Fatal("SQL datasource not visible")
		}
	}
	ds.AuthConfig = ""
	ds.Name = "updated"
	if w := call("PUT", path, ds); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	db.First(&saved, saved.ID)
	if !strings.Contains(saved.AuthConfig, "p@ss") {
		t.Fatal("password lost")
	}
	ds.AuthConfig = `{"username":"another-reader","password":""}`
	if w := call("PUT", path, ds); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	db.First(&saved, saved.ID)
	if !strings.Contains(saved.AuthConfig, "p@ss") || !strings.Contains(saved.AuthConfig, "another-reader") {
		t.Fatal("username update lost saved password")
	}
	query := map[string]any{"datasource_id": saved.ID, "query": "SELECT 1 /*! INTO OUTFILE '/tmp/a' */", "start": "2026-11-04T00:00:00Z", "end": "2026-11-04T01:00:00Z"}
	if w := call("POST", "/sql", query); w.Code != 400 {
		t.Fatal("executable comment reached database", w.Body.String())
	}
	userID = 999
	if w := call("POST", "/sql", query); w.Code != 403 {
		t.Fatal("SQL permission bypass")
	}
	if w := call("GET", "/explore", nil); strings.Contains(w.Body.String(), `"type":"mysql"`) {
		t.Fatal("unauthorized source exposed")
	}
	userID = 1
	query["query"] = "SELECT 1"
	db.Model(&saved).Update("status", 0)
	if w := call("POST", "/sql", query); w.Code != 400 {
		t.Fatal("disabled MySQL queried")
	}
	if w := call("DELETE", path, nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
}
