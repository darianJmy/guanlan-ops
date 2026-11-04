---
title: guanlan-ops 文档
permalink: /
---

# guanlan-ops 文档

**观澜运维平台**将监控接入、告警治理、拨测和自动化放进一个运维工作台。项目采用 Go + Vue 3，前端嵌入服务端二进制，业务数据使用 SQLite。

## 从这里开始

1. 阅读 [快速开始](getting-started.md)，构建并启动服务。
2. [接入 Prometheus / VictoriaMetrics](architecture/prometheus-monitoring.md)，验证指标查询。
3. 配置通知和告警，了解 [告警生命周期](alert-lifecycle-management.md)。
4. 按需接入 [远程 Agent](agent-connection.md)、[脚本拨测](architecture/script-probes.md) 和 [AI 助手](architecture/ai-assistant-integration.md)。

## 部署与维护

| 文档 | 内容 |
| --- | --- |
| [快速开始](getting-started.md) | 源码构建、容器与 Kubernetes 部署入口 |
| [数据库说明](database.md) | SQLite 初始化、存储与开发边界 |
| [链路追踪](tracing.md) | OpenTelemetry 配置、日志关联和覆盖范围 |
| [文档站维护](documentation.md) | GitHub Pages 发布和文档组织 |

## 功能与开发

- [Kafka 接入](kafka.md)
- [Redis 接入](redis.md)
- [MySQL 数据库接入](mysql.md)
- [Kubernetes 集群接入](kubernetes.md)
- [日志与链路查询接入](log-query.md)
- [自动化任务执行](architecture/automation-task-execution.md) · [工作流编排](architecture/workflow-dag-execution.md)
- [AI 工具目录](architecture/platform-operation-tools.md) · [故障中心 AI 分析](architecture/alert-ai-analysis-design.md)
- [项目目录](project-layout.md)

## 设计与规划

以下文档包含历史设计或未来规划，能力是否已经交付请以当前功能文档和代码为准。

- [AI 助手阶段规划](architecture/guanlan-ai-roadmap.md)
- [工作空间分析设计](architecture/workspace-scoped-analysis.md)
- [Tool Server 设计](architecture/aiops-tool-server-design.md)
- [Function Calling 历史设计](aiops-function-calling-architecture.md)
- [智能运维助手目标](aiops-agent-goal.md)
- [证书生命周期规划](certificate-lifecycle-platform-plan.md)
