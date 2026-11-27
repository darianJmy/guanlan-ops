# Kubernetes 集群接入

在 **接入中心 → 实例分组 → 新增实例 → 容器 → Kubernetes** 中添加集群。支持实例分组、标签、启停、编辑、删除，以及保存前后的真实连接测试。

## 连接配置

| 字段 | 填写方式 |
| --- | --- |
| 接入地址 | API Server 的 HTTPS 地址，例如 `https://k8s.example.com:6443`，也支持网关代理前缀路径 |
| Bearer Token | ServiceAccount Token 或集群支持的有效 Bearer Token |
| 客户端证书 | 选择客户端证书认证后，填写 PEM 格式的证书与对应私钥 |
| 命名空间 | 默认 `default`；留空表示验证所有命名空间的 Pod 读取权限 |
| CA 证书 | PEM 格式的集群 CA；留空使用服务端系统信任证书 |
| 跳过服务端证书校验 | 默认关闭，只用于临时诊断证书问题 |

选择 Kubeconfig 可直接粘贴 YAML 或 JSON。填写 Context 可覆盖 current-context。导入会提取 API Server、命名空间、TLS 设置，以及内嵌的 Token 或客户端证书。保存后的实例可在表单里继续通过对应认证方式编辑。手动填写证书时使用 PEM 内容。带有 exec 插件或外部凭据文件的 Context 需要先转换为内嵌凭据。

编辑时，Token 留空可保留原值；证书与私钥均留空保留原认证，更新时需同时填写两者。列表和详情接口不返回已保存的 Kubernetes Token 或客户端私钥。

## 准备读取账号

可在目标集群应用 [读取账号示例](../configs/kubernetes-reader.example.yaml)：

```bash
kubectl apply -f configs/kubernetes-reader.example.yaml
kubectl -n default create token guanlan-reader --duration=24h
```

将输出的 Token 填到接入表单。示例允许读取集群版本及 `default` 命名空间的 Pod 列表，不授予集群管理权限。如果改用其他命名空间，应同步修改示例中的 Role、RoleBinding 和 ServiceAccount 所属命名空间，以及表单配置。

Token 的实际有效期由 API Server 决定，过期后需要重新生成并更新接入配置，目前不自动续期。若命名空间留空，需要自行授予集群范围的 Pod `list` 权限；示例 Role 不包含该权限。

客户端证书的用户或用户组也需要对应的 RBAC 授权，证书认证本身并不授予资源访问权限。

## 连接测试检查什么

1. 使用所填 CA 校验服务端证书，并携带 Token 或客户端证书。
2. 读取 `/version`，确认返回 Kubernetes 版本。
3. 对指定命名空间或所有命名空间读取一页 Pod 列表（`limit=1`），确认读取权限。空列表也视为成功。

测试只读，不创建、修改或删除集群资源。401 提示认证失败，403 提示权限不足，404 提示 API 地址或命名空间不存在；证书校验失败单独提示。不会把 API 响应正文中的敏感内容直接作为错误提示返回。

当前功能是**集群接入与连接验证**，尚未提供集群资源浏览、Pod 日志、终端或部署操作。Kubernetes 接入不作为 PromQL 数据源使用；指标查询和告警规则仍应接入 [Prometheus / VictoriaMetrics](architecture/prometheus-monitoring.md)。

认证及 Pod API 语义参考 [Kubernetes 认证文档](https://kubernetes.io/docs/reference/access-authn-authz/authentication/) 和 [Pod API](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/)。
