# MySQL 数据库接入

在 **接入中心 → 新增实例 → 数据库 → MySQL** 中配置连接。接入后，可在 **即时查询** 中选择 MySQL 实例执行只读 SQL。

## 接入配置

| 字段 | 示例 / 说明 |
| --- | --- |
| 接入地址 | `mysql://mysql.example.com:3306`，省略端口时使用 3306；IPv6 使用 `mysql://[::1]:3306` |
| 用户名、密码 | 独立填写，不放在地址中；编辑时密码留空保留原值 |
| 数据库 | 要使用的默认数据库，例如 `business` |
| TLS | 按数据库服务配置启用；启用后校验服务端证书及主机名，不自动回退到明文连接 |
| CA 证书 | 可选 PEM 内容；TLS 启用且 CA 留空时使用系统信任证书 |

MySQL 使用原生 TCP 协议，不使用 HTTP 地址或 HTTP 请求头。可以通过现有接入 API 的 `tunnel_id` 使用 SSH 隧道，连接目标仍取自 MySQL 地址，TLS 仍按原数据库主机名验证。

连接测试会连接目标数据库，在只读事务中执行 `SELECT 1`，测试后回滚并关闭连接。密码不会出现在接入列表或详情响应中；认证失败、库不存在、权限不足和 TLS 失败会分别提示。

## 查询账号

建议为观澜使用仅有目标数据库 `SELECT` 权限的账号。例如由数据库管理员按实际来源地址创建：

```sql
CREATE USER 'guanlan_reader'@'10.0.0.%' IDENTIFIED BY 'replace-with-a-strong-password';
GRANT SELECT ON business.* TO 'guanlan_reader'@'10.0.0.%';
```

平台的只读事务和 SQL 检查不能替代数据库账号权限；跨库查询范围、视图和函数访问能力仍由该账号授权决定。

## 即时 SQL 查询

角色需要具有 `query:sql` 权限；只有启用的 MySQL 实例会出现在 SQL 数据源选择器中。MySQL 不会被当作 PromQL 或日志查询数据源。

```sql
SELECT id, status, created_at
FROM orders
WHERE created_at >= '2026-11-01 00:00:00'
ORDER BY created_at DESC
```

- 支持单条 `SELECT`，MySQL 8.0 及以上可使用 `WITH`。语句末尾不要添加分号。
- 查询在只读事务中执行；不支持多语句、写操作、文件输出、锁操作、可执行注释或用户优化器提示。
- 最多返回 5000 行、15 MB 结果，查询上下文最长 25 秒；页面可选择行数上限。
- 时间过滤直接写进 SQL，MySQL 不使用 ClickHouse 的 `{from:Int64}` / `{to:Int64}` 参数。
- DECIMAL 和超出 JavaScript 安全整数范围的整数按字符串返回，避免精度丢失；二进制值显示为 `0x` 十六进制，NULL 保持为空值。
- 列名需要唯一，联表查询出现重名时使用 `AS` 指定不同别名。

平台自身业务数据仍保存在 SQLite 中，MySQL 在这里作为外部查询数据源使用。

驱动与事务语义参考 [Go MySQL Driver](https://github.com/go-sql-driver/mysql) 和 [MySQL 只读事务](https://dev.mysql.com/doc/refman/8.4/en/set-transaction.html)。
