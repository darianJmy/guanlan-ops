# 日志查询接入

一级菜单「即时查询」提供统一查询工作台：选择数据源后自动切换查询编辑器与结果视图。日志模式支持 Loki、Elasticsearch、ClickHouse。Loki、Elasticsearch 位于接入中心 → 日志；ClickHouse 位于接入中心 → 数据库。ClickHouse 配置日志表和字段映射后，仍可在日志查询中选择。日志读取需要 `observability:logs` 权限，数据源管理权限与日志查询权限分开。

查询统一使用开始时间（含）、结束时间、返回条数和最新优先排列；Elasticsearch / ClickHouse 的结束时间不包含边界。单次最多 31 天、5000 条日志，响应上限 16 MiB。达到条数上限时页面提示缩小范围。当前没有翻页、日志实时追尾或跨数据源混合查询。

## Loki

填入查询服务地址，使用 LogQL，例如 `{app="api"} |= "error"`。支持无认证、Basic、Bearer 和自定义请求头；多租户通过 `X-Scope-OrgID` 配置。仅展示日志流结果，聚合指标查询会提示使用日志行查询。

## Elasticsearch

- 地址填写 HTTP/HTTPS 服务入口，允许反向代理路径前缀。
- 指定索引模式，例如 `logs-*` 或 `logs-*,app-logs-*`。
- 时间字段默认 `@timestamp`，要求 `date` / `date_nanos`，开启 `doc_values`；正文默认 `message`，读取需要启用 `_source`。
- 服务、级别、Trace ID 字段可选，例如 `service.name`、`log.level`、`trace.id`，支持点分隔的对象路径和字面量带点字段。
- 支持 Basic/Bearer；ApiKey 可选择无认证后在高级配置添加 `Authorization: ApiKey <encoded-key>`。
- 搜索输入支持 Elasticsearch query_string 语法，例如 `error AND service.name:api`。不指定字段的条件默认搜索日志正文字段；`*` 查询范围内全部日志。
- 时间限制、排序、条数由后端生成，不能通过查询条件移除。超时、分片失败、无匹配索引、缺少时间/正文映射均报错，不把部分结果显示成完整结果。

详情展示原始 `_source`。连接测试会查询最近一小时最多一条日志，验证索引读取权限与查询路径；若索引为空，需在有日志后确认正文和可选字段映射。

## ClickHouse

- 使用 HTTP 接口（通常 8123）或 HTTPS 接口（通常 8443），不是 Native TCP 9000。
- 配置数据库、日志表、时间和正文列。默认 `default.otel_logs`、`Timestamp`、`Body`，应按实际表结构修改。
- 时间列要求 `DateTime` / `DateTime64`。列名、数据库和表名支持字母、数字、下划线，不能以数字开头。
- 可选服务、级别、Trace ID 列，例如 OpenTelemetry 表的 `ServiceName`、`SeverityText`、`TraceId`。
- 输入为日志正文关键词（不区分大小写的包含匹配），`*` 查询范围内全部日志。这是日志视图的行为；切换到 SQL 模式后可执行 SELECT / WITH 查询及聚合分析。
- 后端生成只读、参数化 SELECT，关键词以 Base64 参数传递，时间和 LIMIT 独立绑定。引号、换行、反斜杠等输入不会成为 SQL 语法。
- 查询设置 `readonly=1`、`max_execution_time=25`、返回行数/字节数限制，且检查完整 JSON 响应，拒绝 HTTP 200 中夹带的查询异常。

日志详情展示已映射的列。连接测试执行相同的只读查询，验证表、字段类型和读取权限。账号只需目标表 SELECT 权限；若其 profile 使用 `readonly=1`，需允许以下客户端查询设置在只读模式下调整：`max_execution_time`、`max_result_rows`、`max_result_bytes`、`result_overflow_mode`、`date_time_output_format`。可由管理员对对应 settings profile 配置 `CHANGEABLE_IN_READONLY`，不需要开放写权限。

## 配置存储示例

映射保存在现有数据源 `config.log_query` 中，与 `custom_headers` 并存，不新增数据库表或自动更改已有连接。

```json
{
  "custom_headers": {},
  "log_query": {
    "database": "default",
    "table": "otel_logs",
    "timestamp_field": "Timestamp",
    "message_field": "Body",
    "service_field": "ServiceName",
    "level_field": "SeverityText",
    "trace_id_field": "TraceId"
  }
}
```

Elasticsearch 改用 `index` 配置索引，其余映射字段相同。现有数据源列表接口只向日志查询页面投影 ID、名称、类型、标签，不返回地址、认证或请求头。切换数据源会保留本次页面会话中各自的查询输入，并清空旧结果。

## 验证与后续范围

适配器测试使用模拟 HTTP 服务，覆盖认证、路径前缀、字段映射、纳秒时间、权限、禁用源、SQL 参数绑定、空结果、部分失败、畸形响应和取消/重定向。实际部署还需用目标 Elasticsearch / ClickHouse 服务完成连接测试，确认版本、字段类型与账号设置。

OpenSearch、SLS、CLS、Tempo 查询适配器尚未加入本页；现有云日志仪表盘入口不等于原生日志查询接入。

参考：[Elasticsearch Search API](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-search)、[ClickHouse HTTP API](https://clickhouse.com/docs/interfaces/http)、[ClickHouse 只读账号设置](https://grafana.com/docs/plugins/grafana-clickhouse-datasource/latest/configure/)。


## 统一即时查询工作台

新安装默认只有一个一级「即时查询」入口，地址沿用 `/monitor/query`，`/explore` 为别名。旧的 `/observability/logs` 和 `/observability/traces` 跳转到工作台并优先选中对应能力的数据源。新数据库初始化时创建 `query:sql` 权限，仅默认授权管理员。开发环境已有数据库如需更新默认菜单和权限，可按 [数据库说明](database.md) 重建无需保留数据的开发库。

顶部数据源列表按查询权限过滤。Prometheus / VictoriaMetrics 使用 `datasource:list`，日志使用 `observability:logs`，Jaeger 使用 `observability:traces`，ClickHouse SQL 使用独立的 `query:sql`。SQL 权限允许查询数据源账号可读取的所有表，应给连接配置适当的数据库侧读取权限。

ClickHouse 新连接默认无需日志表映射，保存后即可执行 SQL；需要日志视图时打开「启用日志字段映射」。SQL 支持单条 SELECT / WITH（含聚合），后台以只读模式执行并包裹返回行数上限。禁止多语句、写操作、SETTINGS、FORMAT 和文件输出。结果按列展示。时间选择器只提供 `{from:Int64}` / `{to:Int64}`（Unix 秒）参数，必须在 SQL 中引用才会生效，例如：

```sql
SELECT ServiceName, count() AS count
FROM default.otel_logs
WHERE Timestamp >= toDateTime({from:Int64})
  AND Timestamp < toDateTime({to:Int64})
GROUP BY ServiceName
ORDER BY count DESC
```

Jaeger 接入 Query HTTP 服务（默认 16686），使用其 UI JSON 接口 `/api/services` 和 `/api/traces/{traceID}`，支持 16/32 位十六进制 Trace ID、Span 列表、父子层级缩进、耗时和时间线、Span 标签与引用详情。该接口属于 Jaeger 内部 API，升级 Jaeger 需进行兼容验证；目前不提供按服务搜索 Trace 或 Tempo TraceQL。参考 [Jaeger APIs](https://www.jaegertracing.io/docs/latest/apis/)。

统一工作台已通过模拟接口测试及前端构建，尚未对接真实 ClickHouse、Elasticsearch、Jaeger 实例完成联调。

## 按数据源类型独立展示

即时查询只统一入口和顶部的数据源选择，各类型查询视图独立维护模板与局部样式：

- Prometheus / VictoriaMetrics：`InstantQuery.vue`，保留原指标查询模板、样式、多数据源选择、历史、图表和表格；顶部所选实例用于设置默认值。
- Loki：`LokiQuery.vue`，独立 LogQL 查询页面。
- Elasticsearch：`ElasticsearchQuery.vue`，独立日志搜索页面。
- ClickHouse：`SQLQuery.vue` / `ClickHouseLogQuery.vue`，SQL 和日志模式分别展示。
- Jaeger：`TraceQuery.vue`，独立调用链页面。

先前的通用 `QueryWorkbench` 已从即时查询入口撤下，其分屏、分享、通用工具栏不再挂载。外层不对查询组件使用深度样式覆盖。各类型可复用 API 与数据处理逻辑，但页面样式不统一套用。
