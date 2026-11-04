# 分布式链路追踪

服务端支持 OpenTelemetry，通过 **OTLP/HTTP protobuf** 将链路导出到 Collector、Tempo 或 Jaeger 的 OTLP HTTP 接收端。默认关闭导出，配置接收地址后重启生效。

## 启用

```bash
OTEL_SERVICE_NAME=guanlan \
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
OTEL_TRACES_SAMPLER=parentbased_traceidratio \
OTEL_TRACES_SAMPLER_ARG=0.1 \
./guanlan
```

地址应指向实际可访问的采集服务。程序会向通用 endpoint 追加 `/v1/traces`；也可以设置 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 指定完整 URL（例如 `https://collector.example.com/v1/traces`），该变量优先。这里使用 HTTP 接收端口，不能填 gRPC 的 4317 或查询页面的端口。

这些是 OpenTelemetry SDK 读取的**进程环境变量**，不通过应用的 `APP_*`、YAML 或 `.env` 加载。支持 `OTEL_EXPORTER_OTLP_HEADERS`（例如 `Authorization=Bearer%20token`）、SDK 的 TLS 证书配置及 `OTEL_RESOURCE_ATTRIBUTES`；敏感配置通过环境或 Kubernetes Secret 注入。协议固定为 `http/protobuf`，显式配置其他协议会在启动时报错。

未设置采样变量时，SDK 默认跟随父链路的采样决定，并采集全部新根链路。上面的配置将新根链路采样率设为 10%；联调时可使用 `OTEL_TRACES_SAMPLER=parentbased_always_on`，并确保上游请求也设置了采样标志。`OTEL_SDK_DISABLED=true` 或移除接收地址可以关闭本应用的导出初始化。

## Kubernetes

在 `deploy/kubernetes/deployment.yaml` 的主容器 `env` 中添加：

```yaml
- name: OTEL_SERVICE_NAME
  value: guanlan
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: http://otel-collector.observability.svc.cluster.local:4318
- name: OTEL_TRACES_SAMPLER
  value: parentbased_traceidratio
- name: OTEL_TRACES_SAMPLER_ARG
  value: "0.1"
```

将地址换成自己的接收服务后应用清单。本项目的部署清单不安装 Collector 或链路存储服务。

## 已覆盖的调用

- Gin HTTP 入口：接收 W3C `traceparent` / `tracestate`，创建 server span；使用路由模板命名，记录方法、状态码、耗时，5xx 标记为错误。
- 共用 `pkg/httpclient` 的数据源 HTTP 请求：创建 client span，传播 W3C 上下文，记录目标主机、方法、状态码，网络错误和 4xx/5xx 标记为错误。耗时计到响应头返回，不含读取响应体的时间。
- `logger.LogContext`：记录上下文中的真实 `trace_id` 和 `span_id`，访问及 panic 日志自动关联；响应头 `X-Trace-ID` 返回对应 trace ID。未启用时仍使用请求关联 UUID。
- SIGTERM / SIGINT：等待普通 HTTP 请求退出（最多 15 秒），随后刷新待导出 span（最多 5 秒）。导出采用异步批处理，采集端不可用不会阻塞业务请求；缓冲区满或退出超时可能丢失 span。

目前包括即时 PromQL、日志查询和使用公共 HTTP 客户端的 AI 数据源查询。不自动覆盖绕过该客户端的第三方 SDK、通知 HTTP 调用、数据库、SSH、远程 Agent 或后台任务的完整执行链；新增调用应传递原请求的 `context.Context` 并使用公共客户端。没有父上下文的后台 HTTP 请求会成为独立根链路。下游需支持 W3C 上下文并导出自己的 span，才能在界面里看到其内部执行过程。

不采集请求/响应正文、查询字符串、认证头或原始错误内容。没有请求上下文的 `logger.Info` 等日志无法自动关联到某个 HTTP 请求。

## 验证

启用采集后，通过页面执行一次指标查询，在网络响应中找到 `X-Trace-ID`，使用该 ID 在链路后端搜索，应看到观澜入口 span 和数据源调用的子 span。日志里的 trace ID 与界面保持一致；未采样请求仍有 trace ID，但不会出现在存储后端。

本地自动化测试使用临时 OTLP HTTP 接收端，验证上游 trace ID 延续、父子 span、下游请求头、日志关联、错误状态、敏感查询过滤和退出刷新：

```bash
go test ./pkg/tracing ./pkg/logger ./pkg/middleware ./pkg/httpclient
```

配置语义参考 [OpenTelemetry Go exporters](https://opentelemetry.io/docs/languages/go/exporters/) 和 [OTLP exporter 规范](https://opentelemetry.io/docs/specs/otel/protocol/exporter/)。
