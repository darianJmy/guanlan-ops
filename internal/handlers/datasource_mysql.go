package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-sql-driver/mysql"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

type mysqlConnectionConfig struct {
	Database string `json:"database"`
	TLS      struct {
		Enabled       bool   `json:"enabled"`
		CACertificate string `json:"ca_certificate"`
	} `json:"tls"`
}

func mysqlDriverConfig(ds models.Datasource) (*mysql.Config, error) {
	endpoint, err := url.Parse(ds.URL)
	if err != nil || endpoint.Scheme != "mysql" || endpoint.Hostname() == "" || endpoint.User != nil || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.ContainsAny(ds.URL, "\r\n\t ") {
		return nil, fmt.Errorf("请填写 mysql://主机:端口，数据库和认证信息请单独配置")
	}
	port := endpoint.Port()
	if port == "" {
		port = "3306"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return nil, fmt.Errorf("MySQL 端口必须在 1–65535 之间")
	}
	if ds.AuthType != "basic" {
		return nil, fmt.Errorf("MySQL 使用用户名和密码认证")
	}
	var auth DatasourceAuthConfig
	if json.Unmarshal([]byte(ds.AuthConfig), &auth) != nil || strings.TrimSpace(auth.Username) == "" || auth.Password == "" {
		return nil, fmt.Errorf("请填写 MySQL 用户名和密码")
	}
	var settings mysqlConnectionConfig
	if json.Unmarshal([]byte(ds.Config), &settings) != nil {
		return nil, fmt.Errorf("MySQL 连接配置格式不正确")
	}
	if strings.TrimSpace(settings.Database) == "" || len([]rune(settings.Database)) > 64 || strings.ContainsAny(settings.Database, "/\\.\x00") || strings.IndexFunc(settings.Database, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("请填写有效的 MySQL 数据库名称（最多 64 个字符）")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = auth.Username, auth.Password, "tcp", net.JoinHostPort(endpoint.Hostname(), port), settings.Database
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 10*time.Second
	cfg.MaxAllowedPacket = 16 << 20
	// Do not permit multi-statements, LOCAL INFILE, or unencrypted TLS fallback.
	cfg.MultiStatements, cfg.AllowAllFiles, cfg.AllowFallbackToPlaintext = false, false, false
	if settings.TLS.Enabled {
		cfg.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: endpoint.Hostname()}
	}
	if len(settings.TLS.CACertificate) > 64<<10 {
		return nil, fmt.Errorf("MySQL CA 证书不能超过 64 KiB")
	}
	if settings.TLS.CACertificate != "" {
		if !settings.TLS.Enabled {
			return nil, fmt.Errorf("配置 CA 证书时请开启 TLS")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(settings.TLS.CACertificate)) {
			return nil, fmt.Errorf("MySQL CA 证书必须为有效的 PEM 格式")
		}
		cfg.TLS.RootCAs = pool
	}
	return cfg, nil
}

func openMySQL(ds models.Datasource) (*sql.DB, func(), error) {
	cfg, err := mysqlDriverConfig(ds)
	if err != nil {
		return nil, func() {}, err
	}
	closeTunnel := func() {}
	if ds.TunnelID != 0 {
		var tunnel models.Tunnel
		if database.DB.First(&tunnel, ds.TunnelID).Error != nil || tunnel.Status != 1 || (tunnel.Type != "" && tunnel.Type != "ssh") {
			return nil, closeTunnel, fmt.Errorf("SSH 隧道不存在、已禁用或类型不受支持")
		}
		connection, err := openSSHClient(tunnel)
		if err != nil {
			return nil, closeTunnel, fmt.Errorf("连接 SSH 隧道失败")
		}
		cfg.DialFunc = connection.DialContext
		closeTunnel = func() { _ = connection.Close() }
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		closeTunnel()
		return nil, func() {}, fmt.Errorf("MySQL 连接参数无效")
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	return db, func() { _ = db.Close(); closeTunnel() }, nil
}

func mysqlError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("MySQL 查询超时，请缩小查询范围")
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("MySQL 查询已取消")
	}
	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return fmt.Errorf("MySQL TLS 证书校验失败，请检查 CA 和主机名")
	}
	var serverError *mysql.MySQLError
	if errors.As(err, &serverError) {
		switch serverError.Number {
		case 1045:
			return fmt.Errorf("MySQL 认证失败，请检查用户名、密码和账号来源限制")
		case 1044, 1142, 1143:
			return fmt.Errorf("MySQL 权限不足，请检查目标数据库的 SELECT 权限")
		case 1049:
			return fmt.Errorf("MySQL 数据库不存在")
		case 1064:
			return fmt.Errorf("MySQL SQL 语法错误，请检查语句及数据库版本")
		case 1792:
			return fmt.Errorf("MySQL 拒绝在只读事务中执行该操作")
		default:
			return fmt.Errorf("MySQL 执行失败（错误码 %d）", serverError.Number)
		}
	}
	return fmt.Errorf("MySQL 连接或查询失败，请检查网络、端口、认证和 TLS 配置")
}

func queryMySQLSQL(ctx context.Context, ds models.Datasource, req SQLQueryRequest) (sqlResult, error) {
	if err := validateExploreSQLDialect(req.Query, true); err != nil {
		return sqlResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	db, cleanup, err := openMySQL(ds)
	if err != nil {
		return sqlResult{}, err
	}
	defer cleanup()
	return readMySQLRows(ctx, db, req)
}

func readMySQLRows(ctx context.Context, db *sql.DB, req SQLQueryRequest) (sqlResult, error) {
	result := sqlResult{Meta: []map[string]string{}, Data: []map[string]any{}}
	if req.Limit < 1 || req.Limit > 5000 {
		return result, fmt.Errorf("MySQL 返回行数必须在 1–5000 之间")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, mysqlError(err)
	}
	defer tx.Rollback()
	query := fmt.Sprintf("SELECT /*+ MAX_EXECUTION_TIME(25000) */ * FROM (\n%s\n) AS guanlan_result LIMIT %d", req.Query, req.Limit)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return result, mysqlError(err)
	}
	defer rows.Close()
	columns, err := rows.ColumnTypes()
	if err != nil {
		return result, mysqlError(err)
	}
	seen := map[string]bool{}
	for _, column := range columns {
		if seen[column.Name()] {
			return result, fmt.Errorf("查询结果列名重复，请使用不同的 AS 别名")
		}
		seen[column.Name()] = true
		result.Meta = append(result.Meta, map[string]string{"name": column.Name(), "type": column.DatabaseTypeName()})
	}
	size := 0
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return sqlResult{}, mysqlError(err)
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			value := values[i]
			switch v := value.(type) {
			case []byte:
				switch column.DatabaseTypeName() {
				case "BINARY", "VARBINARY", "BLOB", "MEDIUMBLOB", "LONGBLOB", "TINYBLOB", "BIT":
					value = "0x" + hex.EncodeToString(v)
				default:
					value = string(v)
				}
			case int64:
				if v > 9007199254740991 || v < -9007199254740991 {
					value = strconv.FormatInt(v, 10)
				}
			case uint64:
				if v > 9007199254740991 {
					value = strconv.FormatUint(v, 10)
				}
			}
			row[column.Name()] = value
		}
		encoded, err := json.Marshal(row)
		if err != nil {
			return sqlResult{}, fmt.Errorf("MySQL 返回了无法编码的查询结果")
		}
		size += len(encoded)
		if size > 15_000_000 {
			return sqlResult{}, fmt.Errorf("MySQL 查询结果超过 15 MB，请减少列或返回行数")
		}
		result.Data = append(result.Data, row)
	}
	if err := rows.Err(); err != nil {
		return sqlResult{}, mysqlError(err)
	}
	return result, nil
}

func testMySQL(ds models.Datasource) (bool, string) {
	return testMySQLContext(context.Background(), ds)
}
func testMySQLContext(ctx context.Context, ds models.Datasource) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	result, err := queryMySQLSQL(ctx, ds, SQLQueryRequest{Query: "SELECT 1 AS connected", Limit: 1})
	if err != nil {
		return false, err.Error()
	}
	if len(result.Data) != 1 {
		return false, "MySQL 未返回有效查询结果"
	}
	return true, "MySQL 连接成功，数据库只读查询可用"
}

func maskPasswordDatasourceAuth(ds *models.Datasource) {
	if ds.Type != "mysql" && ds.Type != "redis" && ds.Type != "kafka" {
		return
	}
	var auth DatasourceAuthConfig
	_ = json.Unmarshal([]byte(ds.AuthConfig), &auth)
	masked, _ := json.Marshal(DatasourceAuthConfig{Username: auth.Username, HasPassword: auth.Password != ""})
	ds.AuthConfig = string(masked)
}
