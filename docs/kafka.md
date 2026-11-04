# Kafka 接入

在 **接入中心 → 新增实例 → 消息队列 → Apache Kafka** 中添加 Kafka 集群连接。卡片只显示 Logo 和名称，实例支持分组、标签、启停、编辑和删除。

## 连接配置

| 字段 | 说明 |
| --- | --- |
| 接入地址 | 首个引导 Broker，例如 `kafka://broker-1.example.com:9092`；省略端口时使用 9092 |
| 备用 Broker | 每行一个 `host:port`，也可用逗号分隔；最多 9 个，IPv6 使用 `[地址]:端口` |
| 认证方式 | 无认证、SASL/PLAIN、SCRAM-SHA-256、SCRAM-SHA-512 |
| 用户名 / 密码 | 与 Broker 的 SASL 配置一致，编辑时密码留空保留原值 |
| TLS | 与 Kafka 监听器保持一致；开启后验证证书及每个 Broker 的主机名 |
| CA 证书 | 可选 PEM 内容；TLS 启用且 CA 留空时使用系统信任证书 |

所有引导 Broker 共用认证与 TLS 配置，需指向同一集群、同一类监听器。不要填写 Kafka UI 的 HTTP 地址，也不要将密码、Topic 或查询参数写在 Broker 地址中。SASL/PLAIN 的密码由 TLS 保护，应搭配 TLS 使用。

切换认证方式需要重新填写对应凭据；选择无认证并保存会移除已保存的认证信息。列表和详情接口不返回密码原文。

## 连接测试

依次尝试首个和备用 Broker。成功建立 Kafka 协议连接、完成所选 SASL / TLS 握手并读取有效 Broker 元数据后，显示 Broker 数量。单个 Broker 尝试最长 3 秒，整个测试预算 10 秒；请求取消时关闭连接，包括正在等待的认证握手。

测试仅使用 API 版本协商、SASL 握手和不指定 Topic 的 Metadata 请求，不创建 Topic、不生产或消费消息，也不提交消费位点。认证方式不匹配、密码错误、证书问题和超时会分别提示。

成功表示至少一个引导连接可用且能读取 Broker 元数据，**不代表所有 advertised.listeners 地址均可达，也不代表账号具备 Topic 读写或管理权限**。实际网络中应保证服务端能访问 Kafka 对外公布的 Broker 地址。

通过现有接入 API 的 `tunnel_id` 可以使用 SSH 隧道；引导地址由隧道节点访问，TLS 仍验证目标 Broker 主机名。

## 当前范围

当前提供集群连接管理与验证，尚未提供 Topic 管理、消息查询、消费组监控或 Kafka 命令工作台。暂不支持 Kerberos、OAuth 和双向 TLS 客户端证书认证。

客户端实现与协议参考 [kafka-go](https://github.com/segmentio/kafka-go) 和 [Kafka 协议](https://kafka.apache.org/protocol.html)。
