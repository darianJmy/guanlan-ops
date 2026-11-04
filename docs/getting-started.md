# 快速开始

## 从源码构建

构建需要 Go 1.25 或更高版本（以 `go.mod` 为准）、Node.js 22、npm 和 Make。在仓库根目录执行：

```bash
make build
./guanlan
```

`make build` 安装前端依赖、构建 Vue 页面，然后将页面嵌入 `guanlan` 二进制。

打开 **http://127.0.0.1:8080**。首次登录账号为 `admin`，初始密码为 `admin123`，登录后修改密码。

## 配置与数据

默认数据库为工作目录下的 `data/aiops.db`，首次启动自动初始化。外部 YAML 可通过 `./guanlan --conf /path/to/config.yaml` 加载，也可用环境变量覆盖：

```bash
APP_SERVER_ADDR=:8080 \
APP_AUTH_JWT_SECRET=replace-with-your-own-random-secret \
APP_DATABASE_DSN=data/aiops.db \
./guanlan
```

对外部署前设置独立的 JWT Secret，并保持该值稳定。数据库备份、恢复和初始化边界见 [数据库说明](database.md)。目前使用单实例部署。

## 容器与 Kubernetes

```bash
make image-build
# 默认镜像名称为 guanlan:<Git 短 commit SHA>
make image-build IMAGE_NAME=registry.example.com/ops/guanlan
```

服务端 Dockerfile 位于仓库根目录，Agent Dockerfile 位于 `build/agent/Dockerfile`。Kubernetes 的 Deployment、Service 和 PVC 在同一个 YAML 中，以 `---` 分隔。

具体的镜像推送、JWT Secret、存储和访问命令见 [Kubernetes 部署说明](../deploy/kubernetes/README.md)。

## 下一步

- [接入指标数据源](architecture/prometheus-monitoring.md)
- [接入日志和链路查询](log-query.md)
- [连接远程 Agent](agent-connection.md)
- [启用服务端链路追踪](tracing.md)

完整配置、开发命令及功能介绍见 [仓库 README](../README.md)。
