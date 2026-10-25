# Agent 连接验证

所有源码和构建命令均在项目根目录执行。

Agent 用于让远程机器主动连接平台。当前 Agent 版本为 `0.2.0`，支持心跳、远程命令和自动化任务 Git 制品执行。

## 平台侧

在“节点管理 / Agent 节点”里创建 Agent。Token 只显示一次，平台只保存 SHA256。

## 二进制一键安装

页面创建 Agent 后会生成一键安装命令：

```bash
curl -fsSL 'https://平台地址/api/agent/install.sh?token=...' | sh
```

脚本会按远程机器架构下载平台内置的 `aiops-agent` 二进制，安装到 `/usr/local/bin/aiops-agent`，配置写入 `/etc/aiops-agent/agent.env`。
如果远程机器支持 systemd，脚本会创建并启动 `aiops-agent.service`；否则会用后台进程启动，日志写入 `/var/log/aiops-agent.log`。

当前平台需要预置以下二进制：

```text
data/agents/aiops-agent-linux-amd64
data/agents/aiops-agent-linux-arm64
```

## 源码方式运行

本地调试也可以源码方式运行：

```bash
AIOPS_SERVER='ws://平台地址/api/agent/connect' \
AIOPS_AGENT_TOKEN='创建时显示的 Token' \
AIOPS_AGENT_REGION='新加坡' \
go run ./cmd/aiops-agent
```

## Docker 方式

在节点管理中创建“Docker 节点”后，平台会直接生成包含节点 Token、平台地址和镜像名称的 `docker run` 命令。Docker 节点仍使用 Agent 通信协议，但脚本运行在容器环境中。

```bash
docker build -f Dockerfile.agent -t aiops-agent:latest .
docker run -d --name aiops-agent \
  --restart unless-stopped \
  --init \
  --read-only \
  --tmpfs /tmp:rw,exec,size=512m \
  -e AIOPS_SERVER='wss://平台地址/api/agent/connect' \
  -e AIOPS_AGENT_TOKEN='创建时显示的 Token' \
  -e AIOPS_AGENT_REGION='新加坡' \
  -e AIOPS_AGENT_IMAGE='aiops-agent:latest' \
  aiops-agent:latest
```

基础镜像内置 Bash、Python 3、pip、curl 和常用拨测工具。不同 Python 依赖应通过派生镜像隔离，例如：

```dockerfile
FROM aiops-agent:latest
USER root
RUN pip install --no-cache-dir requests pyyaml pandas
USER aiops
```

每个派生镜像使用单独的 Docker 节点 Token 注册，自动化任务选择对应节点即可进入相应依赖环境。

生产环境建议使用 `wss://`，由网关或负载均衡提供 TLS。

## Git 制品执行

自动化任务使用 Git 仓库时，平台负责拉取代码并生成带 SHA-256 的压缩制品。Agent 通过加密 WebSocket 接收 `artifact_exec_v1` 消息，完成摘要校验、安全解压、运行环境注入、命令执行和临时目录清理。Agent 不需要访问 Git，也不会收到 Git Token 或 Deploy Key。

Git 制品执行要求 Agent `0.2.0` 及以上；旧节点需要在节点管理中复制安装命令重新部署。
