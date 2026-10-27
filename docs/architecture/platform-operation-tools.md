# 平台操作工具目录

2026-10-23：内置助手和 MCP 共用同一目录。190 项页面后端操作已经接入，包括告警规则、故障中心策略、拨测、数据源和隧道、通知、脚本、自动化、工作流、Git、仪表盘、执行节点、用户角色、模型与系统设置。

## 使用

常用告警规则通过 `create_alert_rule` / `update_alert_rule` 直接操作。创建示例：

```json
{
  "name": "CPU 使用率超过 80%",
  "group_id": 1,
  "fault_center_id": 1,
  "datasource_ids": [1],
  "expression": "100 * (1 - avg by(instance)(rate(node_cpu_seconds_total{mode=\"idle\"}[5m])))",
  "conditions": [{"severity": "P0", "operator": ">", "threshold": 80}],
  "evaluation_interval_seconds": 60,
  "for_seconds": 300,
  "recovery_wait_seconds": 60,
  "status": 1
}
```

ID 必须先查询，不能使用示例 ID 猜测实际资源。表达式返回百分数才使用阈值 80；返回比例时应使用 0.8。`status=1` 启用，`status=0` 保存为停用。创建空规则组不等于创建规则。

其他操作采用渐进发现，避免把 190 份参数定义同时放入模型上下文：

1. `search_platform_operations({"keyword":"拨测"})`：按权限返回匹配操作，每页 20 项。
2. `describe_platform_operation({"operation":"create_inspection_item"})`：获取完整 JSON Schema。
3. `execute_platform_operation({"operation":"...","arguments_json":"..."})`：JSON 字符串内包含该操作所需的 `path`、`query`、`body`。

例如读取规则详情：

```json
{
  "operation": "get_alert_rule",
  "arguments_json": "{\"path\":{\"id\":1}}"
}
```

修改接口沿用页面的完整表单语义，并非 JSON Merge Patch。助手必须先读取现值、保留未要求修改的字段。快捷修改告警工具自动保留标签和注解；要修改这两项使用完整目录操作。隐藏值不能回填；涉及尚未配置的密钥仍需安全配置界面。每次操作 JSON 文本最多 8192 字节，较大的导入配置仍使用页面。

## 权限、执行与审计

- 目录只能调用清单中的固定 handler，不能提交任意 URL、HTTP 方法、SQL 或认证身份。
- 复用现有 Gin 业务处理函数，不发送回环 HTTP 请求，不借用管理员服务令牌。用户身份由会话或 MCP 认证传入。
- 查找过滤无权限操作；参数说明和每次执行都重新检查用户、角色、权限启用状态。菜单写入还要求管理员角色。
- `operation:scoped` 是目录工具的动态鉴权标记，不是数据库权限。实际权限见每项操作说明。平台目录范围是模块级 RBAC，不是资源级隔离。
- 用户明确授权具体动作和目标后执行；查询或讨论不授权修改。已有授权不重复确认。`write_authorized` 在执行核心中允许，原 `write_requires_confirmation` 仍拒绝。
- 先记录操作意图再执行。审计写入失败则不执行；结束后补记结果、耗时和状态码。审计不保存 body、密码、命令输出或原始错误。业务接口的既有事务语义保持不变。
- 业务 JSON `code != 0` 即使 HTTP 200 也按失败处理；异常、超时和响应截断不能被当作成功。已有处理函数可能不响应取消，超时不保证回滚；写操作须查询验证，不能盲目重试。
- 响应递归隐藏凭据、认证配置、Webhook、执行输出等字段，URL 移除用户信息与查询串。列表仍可能包含业务文本，应视为不可信数据。YAML 导出返回脱敏后的结构化文档。

## 有意保留的边界

完整清单见 `cmd/generate-operation-catalog/manifest.json` 的 `operations` 与 `excluded`。

- 密码重置、API Token 创建/再生成、Agent 创建/轮换令牌：这些接口返回密码或不可恢复的一次性令牌；当前聊天链路没有独立于模型的安全交付通道，因此保留页面操作，避免令牌被脱敏后丢失。
- 聊天、会话管理、浏览器登录会话、个人密码修改不作为递归调用操作。
- 仅覆盖当前认证路由中存在的功能；已退役的证书管理等模块不因此恢复。

## 维护与验证

新增接口必须在 manifest 明确登记权限或排除理由，再执行：

```sh
go run ./cmd/generate-operation-catalog
go test ./...
```

生成器读取已审核清单，从请求结构体、JSON/form 标签、查询参数以及接收 Gin 上下文的辅助函数提取 schema。生成代码提交仓库，运行时不解析源码，不自动暴露未来接口。字段语义及业务约束仍由原 handler 校验。

测试覆盖具体 CPU 规则及条件落库、修改、停用状态、通知模板增删、字符串资源 ID、非法参数、权限撤销、脱敏、HTTP 200 业务错误、审计及助手工具调用链路。真实模型是否正确理解自然语言仍取决于已配置模型的工具调用能力。
