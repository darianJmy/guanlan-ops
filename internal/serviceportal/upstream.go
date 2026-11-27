package serviceportal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
)

type upstreamConnection struct {
	url        *url.URL
	done       <-chan error
	direct     bool
	invalidate func()
}

func (c upstreamConnection) invalidateTarget() {
	if c.invalidate != nil {
		c.invalidate()
	}
}

func (a *app) openUpstream(ctx context.Context, resource store.Resource, cluster store.Cluster, cancel context.CancelFunc) (upstreamConnection, error) {
	if resource.Type != targetconfig.HTTP {
		release, err := a.kubernetes.acquire(ctx, cluster.ID)
		if err != nil {
			return upstreamConnection{}, err
		}
		defer release()
	}
	if err := ctx.Err(); err != nil {
		return upstreamConnection{}, err
	}

	// Start the handshake timeout once a slot is available.
	timeout := time.AfterFunc(30*time.Second, cancel)
	defer timeout.Stop()
	connection, err := a.connectUpstream(ctx, resource, cluster)
	if err != nil {
		return upstreamConnection{}, err
	}
	if !timeout.Stop() || ctx.Err() != nil {
		return upstreamConnection{}, errors.New("服务连接超时")
	}
	return connection, nil
}

func newUpstreamProxy(connection upstreamConnection, cancel context.CancelFunc, onDemand bool) (*httputil.ReverseProxy, func()) {
	proxy := httputil.NewSingleHostReverseProxy(connection.url)
	closeIdle := func() {}
	if connection.direct || onDemand {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		proxy.Transport = transport
		closeIdle = transport.CloseIdleConnections
	}
	if connection.direct {
		director := proxy.Director
		proxy.Director = func(r *http.Request) {
			director(r)
			r.Host = connection.url.Host
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, _ error) {
		http.Error(w, "后端服务暂时不可用", http.StatusBadGateway)
		if connection.direct && r.Context().Err() == nil {
			connection.invalidateTarget()
			cancel()
		}
	}
	return proxy, closeIdle
}

// connectUpstream hides the different target resolution and connection paths.
// The caller owns cancellation, including any Pod port-forward it establishes.
func (a *app) connectUpstream(ctx context.Context, resource store.Resource, cluster store.Cluster) (upstreamConnection, error) {
	connection := upstreamConnection{url: &url.URL{Scheme: "http"}}
	label := "ExternalName"
	if resource.Type == targetconfig.HTTP {
		target, err := targetconfig.ParseHTTPURL(resource.TargetURL)
		if err != nil {
			return upstreamConnection{}, err
		}
		// The complete target URL selects the initial page. Keeping only the
		// origin here preserves absolute assets, links and application routes.
		connection.url.Scheme, connection.url.Host = target.Scheme, target.Host
		connection.direct, label = true, "HTTP URL"
	} else {
		client, err := a.kubernetes.client(cluster)
		if err != nil {
			return upstreamConnection{}, err
		}
		key := targetKey{namespace: resource.Namespace, service: resource.Service, port: resource.Port}
		target, err := client.resolve(ctx, key)
		if err != nil {
			return upstreamConnection{}, err
		}
		connection.invalidate = func() { client.invalidate(key, target) }
		if target.Host != "" {
			if resource.Type == targetconfig.KubernetesTCP {
				return upstreamConnection{}, errors.New("TCP 转发需要匹配 Pod 的 Kubernetes Service")
			}
			connection.url.Host = net.JoinHostPort(target.Host, strconv.Itoa(target.Port))
			connection.direct = true
		} else {
			local, done, err := client.Forward(ctx, resource.Namespace, target.Pod, target.Port)
			if err != nil {
				connection.invalidateTarget()
				return upstreamConnection{}, err
			}
			connection.url.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(local))
			connection.done = done
		}
	}
	if connection.direct {
		port := connection.url.Port()
		if port == "" {
			port = "80"
			if connection.url.Scheme == "https" {
				port = "443"
			}
		}
		address := net.JoinHostPort(connection.url.Hostname(), port)
		conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
		if err != nil {
			connection.invalidateTarget()
			return upstreamConnection{}, fmt.Errorf("无法连接 %s 目标（请检查 DNS、网络和端口）：%w", label, err)
		}
		_ = conn.Close()
	}
	return connection, nil
}
