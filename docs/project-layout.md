# 项目目录

本项目参照 `golang-standards/project-layout` 中的常用目录约定，保留实际使用的目录。

- `.air.toml`：本地开发热重载配置，通过 `make dev` 使用。
- `main.go`：服务端入口和前端静态资源嵌入声明；发布后运行 `./guanlan`。
- `cmd/aiops-agent`：独立 Agent 入口。
- `internal/app`：应用生命周期与 Wire 依赖装配。生成代码同目录保存。
- `internal/conf`：配置解析及 `conf/config.yaml` 内嵌默认值。
- `internal/auth`：登录 JWT、平台 API Token 规则，以及读取应用密钥的加解密适配。
- `pkg/logger`：通用结构化日志，使用独立 `logger.Config` 配置。
- `pkg/database`：SQLite 连接、AutoMigrate、事务初始化、版本校验和 GORM 日志适配。通过模型列表和回调接入业务。
- `pkg/resp`：统一 API 业务码、JSON 响应结构和成功/失败返回方法；HTTP 状态码使用标准库 `net/http` 常量，JSON-RPC 保持独立协议。
- `pkg/middleware`：通用 Gin 访问日志、异常恢复和可配置 CORS。
- `pkg/cryptoutil`：显式传入密钥的 AES-GCM 加解密，不读取应用配置、不清理明文。
- `pkg/shellutil`：Shell 参数转义、命令拼接、变量名校验和换行规范化。
- `pkg/textutil`：按 Unicode 字符截断，可选择省略号或直接裁剪。
- `pkg/syncbuffer`：并发输出缓冲，可在使用前设置字节上限；服务端与 Agent 共用。
- `pkg/archiveutil`：tar.gz 制品解压及路径、链接类型校验；服务端与 Agent 共用。Git 仓库打包、大小限制和制品记录属于内部业务。
- `pkg/sshclient`：显式认证和主机密钥策略的 SSH 连接、命令执行与退出码处理。
- `pkg/httpclient`：HTTP 认证和 Header Transport、超时配置、连接清理；不读取数据源模型。
- `pkg/mailclient`：SMTP 配置校验、MIME 编码及 TLS/STARTTLS 邮件发送。
- `pkg/certutil`：PEM 解析、公私钥匹配、证书元信息与 TLS 证书获取。
- `internal/database`：业务表清单、版本号和种子数据，调用公共数据库组件。
- `internal/middleware`：依赖用户、角色和审计模型的业务鉴权与审计。
- 其他 `internal` 包：API、业务模型、鉴权、AI、Agent 等私有实现。
- `web`：Vue 项目；根目录 `main.go` 将 `web/dist` 嵌入 Go 二进制。
- `configs`：外部配置示例，默认运行无需复制该目录。
- `Dockerfile.agent`：Agent 容器构建定义；Docker 构建上下文为仓库根目录。
- `docs/database.md`：SQLite 数据库初始化与开发环境结构调整说明。
- `web/src/assets`：前端图片和图标。
- `docs`：设计与使用文档。
- `test/smoke`：可复用的冒烟测试脚本。
- `test/artifacts`：本地浏览器验证截图与辅助测试产物，不提交 Git。

包测试保留在对应包中。公共包不引用 `internal`；配置与业务模型由应用层提供。没有脚本或部署清单的目录不创建空占位文件夹。

构建与验证在仓库根目录执行：

```bash
make build
./guanlan
# 本地开发：Air 热重载，缺少前端构建资源时自动准备
make dev
make wire
make test
```

最终发布只需要 `guanlan` 二进制。默认配置、前端资源和首次数据库初始化代码均包含在内；SQLite 数据和可选日志文件在运行时创建。

公共包不导入 `guanlan/internal/...`。业务层负责配置、数据库查询、权限、默认值与状态转换，再将独立参数传给公共包。已有密文格式、文本截断策略及接口返回保持兼容。

Agent 的 Docker 构建只复制自身依赖的 `pkg/archiveutil` 和 `pkg/syncbuffer`，避免引入服务器的 Gin、数据库及云 SDK 依赖；增加 Agent 公共依赖时同步更新根目录 `Dockerfile.agent`。
