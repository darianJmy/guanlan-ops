# Kubernetes 部署

在仓库根目录执行以下命令。需要 Docker、kubectl、可用的 Kubernetes 集群及支持 SQLite 文件锁的持久化存储，推荐块存储；PVC 默认申请 10Gi，使用集群默认 StorageClass。没有默认 StorageClass 时先修改 `deployment.yaml` 中的 PersistentVolumeClaim 配置。

## 1. 构建镜像

将下面地址改成自己的镜像仓库，构建架构需与集群节点一致：

```bash
make image-build IMAGE_NAME=registry.example.com/ops/guanlan
docker push "registry.example.com/ops/guanlan:$(git rev-parse --short HEAD)"
```

`make image-build` 默认使用当前 Git 短 commit SHA 作为标签，可通过 `IMAGE_TAG` 覆盖。Dockerfile 自动构建前端并嵌入服务端，运行镜像包含本地脚本、Git 任务及拨测使用的基础命令。脚本需要的其他依赖可自行扩展镜像。

将 `deployment.yaml` 中的 `image: guanlan:local` 替换成刚推送的镜像地址。私有仓库还需在 Pod 的 `spec.imagePullSecrets` 配置已有的拉取凭据。本地 kind/minikube 可以导入构建的 `guanlan:<commit>` 镜像，并同步修改清单中的镜像标签。

## 2. 创建命名空间和密钥

```bash
kubectl create namespace guanlan
kubectl -n guanlan create secret generic guanlan-auth \
  --from-literal=jwt-secret="$(openssl rand -hex 32)"
```

密钥只需首次创建，升级时保留。已有数据库迁入时必须使用原来的 `auth.jwt_secret`，否则需要重新保存模型 API Key。不要把真实密钥提交到仓库。

## 3. 部署和访问

Deployment、Service 和 PVC 统一放在 `deployment.yaml` 中，以 `---` 分隔。

```bash
kubectl -n guanlan apply -f deploy/kubernetes/deployment.yaml
kubectl -n guanlan rollout status deployment/guanlan
kubectl -n guanlan port-forward service/guanlan 8080:8080
```

浏览器访问 http://127.0.0.1:8080，首次登录使用 `admin / admin123` 并修改密码。Service 默认仅集群内可达；需要对外访问时，在现有 Ingress/Gateway 中转发到 `guanlan:8080`，并支持 WebSocket 连接。

链路追踪可通过添加 `OTEL_*` 环境变量启用，详见 [分布式链路追踪](../../docs/tracing.md)。

## 配置与维护

- 配置通过 Deployment 的 `env` 覆盖内嵌默认值，变量名称见根目录 README。SMTP 密码等敏感值使用 Secret 引用。
- 数据库位于 PVC 的 `/app/data/aiops.db`，Pod 重建后保留。存储驱动需要支持 `fsGroup: 10001` 或预先赋予 UID/GID 10001 写入权限。
- 保持 `replicas: 1` 和 `strategy.type: Recreate`，不要启用 HPA；SQLite 和进程内调度器不支持多实例。升级期间会短暂中断服务。
- 使用新镜像标签更新：`kubectl -n guanlan set image deployment/guanlan guanlan=registry.example.com/ops/guanlan:v2`，然后检查 rollout 状态。
- 备份前将 Deployment 缩容到 0，确认 Pod 已停止，再使用存储快照或临时挂载 PVC 备份完整数据目录；完成后恢复到 1 副本。JWT Secret 也应单独备份。
- 删除 PVC 或命名空间可能删除持久化数据，卸载服务时可仅删除 Deployment 和 Service。
- 探针检查 TCP 端口可达性，不代表外部数据源健康。ICMP 拨测还取决于集群的网络及 ping 权限设置。
