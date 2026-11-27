package serviceportal

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"time"

	"guanlan/internal/serviceportal/store"
)

func (a *app) grantTCPAccess(r *http.Request, user store.User, resource store.Resource, rr *run) (time.Time, int, error) {
	token := r.Header.Values("X-Service-Portal-Internal")
	if a.cfg.InternalToken == "" || len(token) != 1 || subtle.ConstantTimeCompare([]byte(token[0]), []byte(a.cfg.InternalToken)) != 1 {
		return time.Time{}, http.StatusForbidden, errors.New("请从 观澜 服务仪表盘打开 TCP 服务")
	}
	addresses := r.Header.Values("X-Service-Portal-Client-IP")
	if len(addresses) != 1 {
		return time.Time{}, http.StatusBadRequest, errors.New("无法获取 TCP 访问来源 IP")
	}
	ip, err := netip.ParseAddr(addresses[0])
	ip = ip.Unmap()
	if err != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
		return time.Time{}, http.StatusBadRequest, errors.New("TCP 访问来源 IP 无效")
	}
	a.mu.Lock()
	if a.runs[resource.ID] != rr || !sameTarget(rr.resource, resource) || rr.listener == nil {
		a.mu.Unlock()
		return time.Time{}, http.StatusConflict, errors.New("服务连接已变更，请重试")
	}
	now := time.Now()
	if rr.tcpGrants == nil {
		rr.tcpGrants = make(map[netip.Addr]time.Time)
	}
	for source, expiry := range rr.tcpGrants {
		if !now.Before(expiry) {
			delete(rr.tcpGrants, source)
		}
	}
	if _, exists := rr.tcpGrants[ip]; !exists && len(rr.tcpGrants) >= 4096 {
		a.mu.Unlock()
		return time.Time{}, http.StatusServiceUnavailable, errors.New("TCP 访问授权繁忙，请稍后重试")
	}
	expires := now.Add(a.cfg.TCPGrantTTL)
	rr.tcpGrants[ip] = expires
	a.mu.Unlock()
	log.Printf("tcp access granted user=%q resource=%q source=%q expires=%s", user.ID, resource.ID, ip, expires.UTC().Format(time.RFC3339Nano))
	return expires, http.StatusOK, nil
}

func (a *app) serveTCP(ctx context.Context, rr *run, listener net.Listener, upstream upstreamConnection) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	defer listener.Close()

	forwardClosed := make(chan struct{})
	go func() {
		select {
		case <-upstream.done:
			close(forwardClosed)
			listener.Close()
		case <-ctx.Done():
		}
	}()
	for {
		client, err := listener.Accept()
		if err != nil {
			select {
			case <-forwardClosed:
				if ctx.Err() == nil {
					upstream.invalidateTarget()
				}
				return errors.New("端口转发已断开")
			default:
				return err
			}
		}
		if !a.tcpSourceAllowed(rr, client.RemoteAddr(), time.Now()) {
			client.Close()
			continue
		}
		if rr.resource.OnDemand {
			go a.relayDemandTCP(ctx, rr, client.(*net.TCPConn))
		} else {
			go relayTCP(ctx, client.(*net.TCPConn), upstream.url.Host)
		}
	}
}

func (a *app) relayDemandTCP(ctx context.Context, rr *run, client *net.TCPConn) {
	defer client.Close()
	waitCtx, cancel := context.WithTimeout(ctx, onDemandRequestWait)
	defer cancel()

	demand := a.waitForTCPDemand(ctx, waitCtx, rr)
	if demand == nil {
		return
	}
	defer a.releaseDemand(rr, demand)
	relayTCP(demand.ctx, client, demand.address)
}

func (a *app) waitForTCPDemand(ctx, waitCtx context.Context, rr *run) *demandConnection {
	for {
		demand, retryAfter := a.takeDemand(ctx, rr, waitCtx)
		if demand == nil {
			if retryAfter <= 0 {
				return nil
			}
			timer := time.NewTimer(retryAfter)
			select {
			case <-timer.C:
				continue
			case <-waitCtx.Done():
				timer.Stop()
				return nil
			}
		}
		select {
		case <-demand.ready:
			if demand.err == nil && demand.ctx.Err() == nil && waitCtx.Err() == nil {
				return demand
			}
		case <-demand.ctx.Done():
		case <-waitCtx.Done():
		}
		a.releaseDemand(rr, demand)
	}
}

func (a *app) tcpSourceAllowed(rr *run, address net.Addr, now time.Time) bool {
	peer, ok := address.(*net.TCPAddr)
	if !ok {
		return false
	}
	ip := peer.AddrPort().Addr().Unmap().WithZone("")
	a.mu.Lock()
	defer a.mu.Unlock()
	expires := rr.tcpGrants[ip]
	if a.runs[rr.resource.ID] == rr && now.Before(expires) {
		return true
	}
	delete(rr.tcpGrants, ip)
	return false
}

func relayTCP(ctx context.Context, client *net.TCPConn, address string) {
	defer client.Close()
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return
	}
	upstream := conn.(*net.TCPConn)
	defer upstream.Close()
	stop := context.AfterFunc(ctx, func() {
		client.Close()
		upstream.Close()
	})
	defer stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(upstream, client)
		_ = upstream.CloseWrite()
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.CloseWrite()
	<-done
}
