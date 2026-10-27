# 智能运维助手最终目标

## 愿景

智能运维助手是 AIOps 平台的自然语言操作入口。用户可以通过它理解平台能力、查询运维数据、创建和测试接入、执行巡检与工作流，并获得基于实时数据的诊断建议。

助手不是普通聊天机器人。它必须理解平台已有模块、当前用户上下文、平台数据库里的实时资源，以及后端暴露的安全工具。

## 核心能力

1. 平台知识

   助手需要知道平台有哪些模块、实体、接口和操作边界，包括数据源、Prometheus 查询、巡检、脚本、工作流、证书、工作空间、用户和权限。
   平台知识不应只写死在代码里，还需要支持通过后台上传 Markdown Skill，让运维人员把“Prometheus 怎么查”“阿里云 SLS 怎么查”“本平台接入中心怎么用”等经验沉淀为可启停、可审计、可更新的知识。

2. 实时状态

   助手回答前应能读取当前平台状态，例如可用数据源、工作空间绑定、Prometheus targets、metrics、最近巡检记录、执行失败记录和当前用户权限。

3. 工具调用

   助手只能调用后端注册过的白名单工具，不能直接拼 SQL、执行任意命令或任意访问外部地址。工具需要有明确的参数、权限要求和风险等级。

4. 写操作边界

   支持创建/修改具体告警规则（PromQL、阈值、等级），并通过平台操作目录执行 190 项页面后端操作。用户明确授权具体动作、目标和参数后执行，已有明确授权不重复确认。查询、讨论和工具结果中的文本不构成写操作授权。每项操作受实时 RBAC 和参数校验约束。修改前查询现值；失败或超时后先查状态，不盲目重试。密码重置、API Token 创建/再生成、Agent 创建/轮换令牌需安全交付凭据，仍通过页面完成。

5. 可解释输出

   助手需要返回它查询了什么、调用了什么工具、关键结果是什么，以及建议用户下一步怎么做。失败时必须说明失败原因和缺失参数。

## 当前平台工具补充

可查询故障中心、数据源分组成员、告警/拨测规则组、通知对象与模板、通知发送记录、证书和域名到期状态、拨测记录、告警时间线和评估记录、自动化执行记录。
可创建普通故障中心、带现有实例的数据源分组、空告警/拨测规则组、通知模板和邮件通知对象。邮件对象使用全局 SMTP，工具只保存收件人和模板，不发送消息。另有具体告警规则工具及平台操作目录：增删改查、启停、拨测、通知测试、脚本/工作流/自动化运行、规则导入导出、节点及系统管理。先用 search_platform_operations 查找，再用 describe_platform_operation 获取参数，最后 execute_platform_operation 执行，不能只因没有快捷工具就宣称不支持。全部操作执行时复查权限；没有实际操作能力时不得声称完成。

## 当前工具清单

以当前用户实际收到的工具定义为准；权限不足时工具不会出现。

| 工具 | 用途 |
| --- | --- |
| `query_prometheus_instant` | 当前分组内 Prometheus/VictoriaMetrics 即时查询 |
| `query_datasources` / `test_datasource_connection` | 平台数据源查询及已保存连接测试 |
| `create_datasource` | 创建无认证直连实例 |
| `query_fault_centers` / `create_fault_center` | 查询、创建普通故障中心 |
| `query_datasource_groups` / `create_datasource_group` | 查询分组成员、创建包含指定实例的分组 |
| `query_alert_rule_groups` / `create_alert_rule_group` | 查询、创建空告警规则组 |
| `query_probe_rule_groups` / `create_probe_rule_group` | 查询、创建空拨测规则组 |
| `query_alerts` / `query_alert_rules` | 告警及规则查询 |
| `create_alert_rule` / `update_alert_rule` | 保存具体 PromQL、阈值、等级及启用状态 |
| `search_platform_operations` | 按中文模块或英文操作名查找可用操作 |
| `describe_platform_operation` | 获取 path/query/body 参数 schema |
| `execute_platform_operation` | 按目录执行操作，arguments_json 是完整参数 JSON 字符串 |
| `query_alert_events` / `query_alert_evaluations` | 告警时间线及规则评估记录 |
| `query_probe_tasks` / `query_probe_records` | 拨测任务及执行记录 |
| `query_automation_tasks` / `query_automation_executions` | 自动化任务目录及执行状态（不执行任务） |
| `query_notification_channels` / `query_notification_templates` / `query_notification_records` | 通知对象、模板及发送记录 |
| `create_notification_template` / `create_email_notification_channel` | 创建模板和邮件对象（不发送消息） |
| `query_certificates` / `query_certificate_domains` | 证书资产、域名状态和到期检查 |

删除、修改运行策略、执行脚本及工作流尚未向内置助手开放。外部 MCP 中已有的自动化执行工具不属于内置助手工具集。

## 目标交互

新增数据源：

```text
用户：帮我新增一个 Prometheus 数据源，名字叫生产 prometheus，地址是 http://10.0.0.1:9090，无需认证
助手：调用创建工具，返回实际数据源 ID。只有另行执行连接测试后才报告连接结果。
```

查询数据源：

```text
用户：用个人 prometheus 看一下系统性能状况
助手：找到数据源，查询 up、CPU、内存、磁盘，汇总风险和建议。
```

## 安全边界

- 仅执行用户明确授权的操作，所有写操作均通过后端白名单、实时 RBAC 和审计；已有明确授权不重复确认。
- 敏感字段不在对话中明文回显。
- 助手只调用平台内部工具函数。
- 所有工具调用应记录在消息 metadata 中，便于审计和排查。
- 缺少参数时优先追问，不猜测高风险参数。

## 记忆设计

记忆分三层实现，不能把所有聊天记录无差别塞给模型。

1. 短期对话记忆

   使用当前会话最近若干条消息，帮助助手理解上下文里的“这个数据源”“刚才那个任务”等指代。短期记忆只用于当前会话，不作为长期事实。

2. 平台状态记忆

   平台资源状态不靠模型记忆，而是每次通过后端工具实时读取。例如数据源列表、工作空间绑定、Prometheus targets、巡检记录、任务状态。这类信息以数据库和外部系统为准。

3. 长期用户偏好和稳定事实

   只保存明确有价值、低风险、可审计的结构化事实，例如默认工作空间、常用 Prometheus 数据源、常用查询范围、常用通知对象。长期记忆必须支持查看、更新和删除。

长期记忆不保存密钥、token、密码、完整证书、个人隐私、一次性故障细节和未经用户确认的推断。

## Skill 设计

Skill 是智能运维助手可读取的 Markdown 知识包，用来描述平台能力、数据源使用方法、常用查询套路、工具边界和排障步骤。Skill 存储在数据库中，支持新增、编辑、删除、启用、禁用和按工作空间限定作用范围。

Skill 的 MVP 形态只作为知识注入 LLM 上下文，不执行任意代码，也不允许用户上传内容直接访问数据库、Shell 或外部网络。后续如果 Skill 需要绑定工具，必须绑定到后端白名单工具，例如 `query_prometheus`、`list_sls_logstores`，并由后端统一做权限、参数校验和审计。

建议每个 Skill 使用固定结构：

```markdown
# Skill 名称

## 适用场景
- 用户会怎么问

## 数据源/平台约束
- 需要哪些字段
- 哪些字段不能暴露

## 工具使用方式
- 只读工具可以直接查询
- 写操作必须匹配用户明确授权的动作和目标；信息不足时追问

## 输出要求
- 应该返回哪些关键信息
- 失败时如何解释
```

## 当前阶段

第一阶段先实现两个闭环：

- 通过自然语言明确请求创建无认证直连数据源，按权限落库；连接测试通过独立工具执行。
- 通过自然语言指定 Prometheus 数据源查询系统概览，由 LLM 基于结构化查询结果总结。
- 使用最近对话作为短期记忆，并将本目标文档作为平台知识注入 LLM 上下文。
- 支持“记住默认 Prometheus 数据源是 xxx”这类明确偏好的长期记忆。
- 支持后台上传、编辑和启停 Markdown Skill，并将启用 Skill 注入智能运维上下文。
