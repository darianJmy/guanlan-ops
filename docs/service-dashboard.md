# 服务仪表盘

登录观澜后，从侧栏进入「仪表盘」，打开「服务仪表盘」。服务按目录组织，可以按名称、来源或状态筛选。HTTP URL 直连只需启动观澜，目标应用负责登录。

## 运行服务代理

HTTP URL 代理与 Kubernetes Service 转发使用独立进程。用 `make service-portal` 构建。项目 Docker 镜像也包含此程序，可用 `--entrypoint /usr/local/bin/service-portal` 启动代理容器。

| 进程 | 环境变量 | 用途 |
| --- | --- | --- |
| 观澜 | `SERVICE_PORTAL_URL` | 代理管理接口的 HTTP(S) 根地址。同机部署可用 `http://127.0.0.1:8081` |
| 观澜与代理 | `SERVICE_PORTAL_INTERNAL_TOKEN` | 由部署环境向两个进程注入同一个随机密钥 |
| 代理 | `PORTAL_GUANLAN_URL` | 观澜的 HTTP(S) 根地址 |
| 代理 | `PORTAL_BIND` | 管理接口绑定地址，默认 `127.0.0.1:8081` |
| 代理 | `PORTAL_PUBLIC_URL` | 客户端使用的协议和主机，格式为 `http(s)://主机:端口`。启动地址使用对应服务的业务端口 |
| 代理 | `PORTAL_MIN_PORT` / `PORTAL_MAX_PORT` | 业务端口范围，默认 `18000–18999` |
| 代理 | `PORTAL_TCP_GRANT_TTL` | TCP 新建连接按来源 IP 放行的时长，默认 `15m` |

管理端口由观澜访问，业务端口由客户端访问。跨主机部署时通过可信网络或 HTTPS 传递内部认证请求。为 HTTP 代理使用 HTTPS 时，在业务入口终止 TLS 并支持 WebSocket Upgrade，同时把 `PORTAL_PUBLIC_URL` 设置为 HTTPS。代理业务域名使用独立于观澜登录域名的主机。

## 添加服务

- **HTTP URL** 支持浏览器直连和 HTTP/WebSocket 代理。程序可在 `X-Guanlan-Authorization` 中提供观澜 Bearer Token，同时在 `Authorization` 中提供目标应用自己的凭据。
- **Kubernetes Service** 复用接入中心已启用数据源的 Bearer Token 或客户端证书及 TLS 设置。代理需直连 API Server。发现和转发需要读取 Service、Pod，并拥有目标 Pod 的 `pods/portforward` 创建权限。已配置命名空间时，服务发现使用该命名空间。留空时需要列出集群的命名空间。ExternalName 可配合 HTTP 代理使用。
- **Kubernetes TCP** 按来源 IP 临时放行，适用于目标应用提供 HTTPS 的场景。观澜接收访问请求时看到的连接来源 IP 须与代理看到的 TCP 来源 IP 一致。业务端口直接透传，目标应用负责 TLS 和用户认证。

代理启动后会恢复已启用的服务。按需模式在访问时建立连接，空闲后释放。停用服务会关闭转发通道。

## 目录权限

管理员角色默认获得菜单、查看和管理权限。给其他角色分配「仪表盘」菜单与 `service-dashboard:list` 可允许进入页面。配置服务及目录还需要 `service-dashboard:manage` 和对应目录权限。

将供其他角色使用的服务放进目录，通过继承权限或限定角色配置查看、编辑、管理权限。根目录中的服务默认由拥有服务管理权限的用户访问。直连服务的目录权限控制入口的可见性，目标应用负责自身访问控制。
