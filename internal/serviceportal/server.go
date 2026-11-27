package serviceportal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"guanlan/internal/serviceportal/config"
	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
)

type app struct {
	cfg          config.Config
	db           *store.Store
	mu           sync.Mutex
	kubernetes   kubernetesConnections
	operations   sync.Mutex
	runs         map[string]*run
	tickets      map[string]accessTicket
	access       map[string]accessSession
	retries      map[string]retryAttempt // guarded by operations
	stateFetch   atomic.Uint64
	stateApplied uint64 // guarded by operations
}

type run struct {
	cancel      context.CancelFunc
	listener    net.Listener
	port        int
	status      string
	lastError   string
	proxy       *httputil.ReverseProxy
	resource    store.Resource
	cluster     store.Cluster
	demand      *demandConnection        // guarded by app.mu
	demandRetry retryAttempt             // guarded by app.mu
	tcpGrants   map[netip.Addr]time.Time // guarded by app.mu
}
type apiErr struct {
	Error string `json:"error"`
}

// Run starts the service access backend using 观澜 as its identity authority.
func Run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	a := &app{cfg: c, db: store.New(), runs: map[string]*run{}, tickets: map[string]accessTicket{}, access: map[string]accessSession{}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.reconcileLoop(ctx)
	log.Printf("service portal listening on %s", c.Bind)
	return http.ListenAndServe(c.Bind, a.routes())
}
func (a *app) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	m.HandleFunc("/api/me", a.me)
	m.HandleFunc("/api/clusters", func(w http.ResponseWriter, r *http.Request) {
		jsonErr(w, http.StatusNotFound, errors.New("路径不存在"))
	})
	m.HandleFunc("/api/clusters/", a.clusterQuery)
	m.HandleFunc("/api/resources", a.resources)
	m.HandleFunc("/api/resources/", a.resourceAction)
	return a.recover(a.integrationRoutes(m))
}
func (a *app) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic: %v", e)
				http.Error(w, "内部错误", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func jsonErr(w http.ResponseWriter, code int, e error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(apiErr{e.Error()})
}
func (a *app) require(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, err := a.platformIdentity(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		jsonErr(w, identityStatus(err), err)
		return store.User{}, false
	}
	if err := a.refreshState(r.Context(), r.Header.Get("Authorization")); err != nil {
		jsonErr(w, identityStatus(err), err)
		return store.User{}, false
	}
	return u, true
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	u, ok := a.require(w, r)
	if !ok {
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"user": publicUser(u), "minPort": a.cfg.MinPort, "maxPort": a.cfg.MaxPort})
}

func publicUser(u store.User) map[string]any {
	result := map[string]any{"id": u.ID, "name": u.Name, "admin": u.Admin, "enabled": u.Enabled}
	if u.FolderAccess != nil {
		result["folderAccess"] = u.FolderAccess
	}
	return result
}
func (a *app) clusterQuery(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/"), "/")
	if len(parts) != 3 || parts[0] != "clusters" || !methodAllowed(w, r, http.MethodGet) {
		if len(parts) != 3 || parts[0] != "clusters" {
			jsonErr(w, http.StatusNotFound, errors.New("路径不存在"))
		}
		return
	}
	clusterID := parts[1]
	st := a.db.Snapshot()
	cluster, ok := st.Cluster(clusterID)
	if !ok {
		jsonErr(w, http.StatusNotFound, errors.New("Kubernetes 接入不存在"))
		return
	}
	if cluster.Kubeconfig == "" {
		jsonErr(w, http.StatusServiceUnavailable, errors.New("Kubernetes 接入凭据不可用"))
		return
	}
	kc, err := a.kubernetes.client(cluster)
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	switch parts[2] {
	case "namespaces":
		values, err := kc.Namespaces(ctx)
		if err != nil {
			jsonErr(w, http.StatusBadGateway, err)
			return
		}
		json.NewEncoder(w).Encode(values)
	case "services":
		values, err := kc.Services(ctx, r.URL.Query().Get("namespace"))
		if err != nil {
			jsonErr(w, http.StatusBadGateway, err)
			return
		}
		json.NewEncoder(w).Encode(values)
	default:
		jsonErr(w, http.StatusNotFound, errors.New("路径不存在"))
	}
}

func (a *app) resources(w http.ResponseWriter, r *http.Request) {
	u, ok := a.require(w, r)
	if !ok {
		return
	}
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	st := a.db.Snapshot()
	out := []map[string]any{}
	for _, item := range st.Resources {
		if !st.Allowed(u, item.ID) {
			continue
		}
		status := item.Status
		if status == "" {
			status = "stopped"
		}
		result := map[string]any{"resource": item, "status": status, "error": item.LastError, "access_level": st.AccessLevel(u, item.ID)}
		a.mu.Lock()
		run := a.runs[item.ID]
		if run != nil {
			result["status"] = run.status
			result["error"] = run.lastError
		}
		a.mu.Unlock()
		if cluster, exists := st.Cluster(item.ClusterID); exists {
			result["clusterName"] = cluster.Name
		}
		out = append(out, result)
	}
	json.NewEncoder(w).Encode(out)
}
func (a *app) resourceAction(w http.ResponseWriter, r *http.Request) {
	u, ok := a.require(w, r)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[1] != "resources" || (parts[3] != "access" && parts[3] != "start" && parts[3] != "stop") {
		jsonErr(w, http.StatusNotFound, errors.New("资源路径不存在"))
		return
	}
	if parts[3] == "access" {
		if !methodAllowed(w, r, http.MethodPost) {
			return
		}
		a.issueAccess(w, r, u, parts[2])
		return
	}
	if !u.Admin {
		jsonErr(w, http.StatusForbidden, errors.New("没有服务管理权限"))
		return
	}
	if !methodAllowed(w, r, http.MethodPost) {
		return
	}
	a.operations.Lock()
	defer a.operations.Unlock()
	resource, exists := a.db.Snapshot().Resource(parts[2])
	if !exists {
		jsonErr(w, http.StatusNotFound, errors.New("资源不存在"))
		return
	}
	if !a.requireFolderAccess(w, u, resource.FolderID, "edit") {
		return
	}
	var err error
	if parts[3] == "start" {
		err = a.startLocked(parts[2], false)
	} else {
		a.stopLocked(parts[2])
	}
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *app) startDesired(id string) error { return a.startWithDesiredCheck(id, true) }

func (a *app) startWithDesiredCheck(id string, desiredOnly bool) (err error) {
	a.operations.Lock()
	defer a.operations.Unlock()
	return a.startLocked(id, desiredOnly)
}

func (a *app) startLocked(id string, desiredOnly bool) (err error) {
	if desiredOnly {
		resource, exists := a.db.Snapshot().Resource(id)
		if !exists || !resource.Desired {
			return nil
		}
	}
	a.mu.Lock()
	if _, ok := a.runs[id]; ok {
		a.mu.Unlock()
		return nil
	}
	a.mu.Unlock()
	a.setResourceStatus(id, "connecting", "")
	defer func() {
		if err != nil {
			a.setResourceStatus(id, "failed", err.Error())
		}
	}()
	st := a.db.Snapshot()
	res, ok := st.Resource(id)
	if !ok {
		return errors.New("资源不存在")
	}
	if err := validateResource(res); err != nil {
		return err
	}
	if res.PublicPort < a.cfg.MinPort || res.PublicPort > a.cfg.MaxPort {
		return errors.New("对外端口超出配置范围")
	}
	var cl store.Cluster
	if res.Type != targetconfig.HTTP {
		cl, ok = st.Cluster(res.ClusterID)
		if !ok {
			return errors.New("集群不存在")
		}
		if len(cl.Kubeconfig) == 0 {
			return errors.New("已保存的集群凭据不可用")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	rr := &run{cancel: cancel, status: "connecting", resource: res, cluster: cl}
	a.mu.Lock()
	a.runs[id] = rr
	a.mu.Unlock()
	_ = a.db.Update(func(s *store.State) error {
		for i := range s.Resources {
			if s.Resources[i].ID == id {
				s.Resources[i].Desired = true
			}
		}
		return nil
	})
	go a.connect(ctx, id, rr)
	return nil
}

// Connecting happens outside operations so one unavailable cluster cannot
// block other resources, state refreshes, or an administrator's stop request.
func (a *app) connect(ctx context.Context, id string, rr *run) {
	var connectionError error
	defer func() { a.connectionClosed(id, rr, connectionError) }()
	cancel := rr.cancel
	res := rr.resource
	var connection upstreamConnection
	var proxy *httputil.ReverseProxy
	status := "idle"
	if !res.OnDemand {
		var err error
		connection, err = a.openUpstream(ctx, res, rr.cluster, cancel)
		if err != nil {
			connectionError = err
			return
		}
		if res.Type != targetconfig.KubernetesTCP {
			var closeIdle func()
			proxy, closeIdle = newUpstreamProxy(connection, cancel, false)
			defer closeIdle()
		}
		status = "running"
	}
	ln, e := net.Listen("tcp", fmt.Sprintf("%s:%d", "0.0.0.0", res.PublicPort))
	if e != nil {
		connectionError = fmt.Errorf("对外端口不可用：%w", e)
		return
	}
	defer ln.Close()
	a.operations.Lock()
	a.mu.Lock()
	if a.runs[id] != rr || ctx.Err() != nil {
		a.mu.Unlock()
		a.operations.Unlock()
		return
	}
	rr.listener, rr.port, rr.status, rr.proxy = ln, res.PublicPort, status, proxy
	a.mu.Unlock()
	a.setResourceStatus(id, status, "")
	delete(a.retries, id)
	a.operations.Unlock()
	if res.Type == targetconfig.KubernetesTCP {
		connectionError = a.serveTCP(ctx, rr, ln, connection)
		return
	}
	var upstream http.Handler
	if res.OnDemand {
		upstream = a.onDemandHandler(ctx, rr)
	} else {
		upstream = proxy
	}
	srv := &http.Server{Handler: a.resourceHandler(id, upstream), ReadHeaderTimeout: 10 * time.Second}
	defer func() {
		if connection.direct {
			// A proxy failure cancels this run from inside the request handler.
			// Let its 502 response finish before closing active connections.
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdown)
		}
		_ = srv.Close()
	}()
	go func() {
		_ = srv.Serve(ln)
		cancel()
	}()
	select {
	case <-connection.done:
		// Even a clean forwarder exit must be recovered while enabled.
		if ctx.Err() == nil {
			connection.invalidateTarget()
		}
		connectionError = errors.New("端口转发已断开")
	case <-ctx.Done():
		connectionError = errors.New("服务访问连接已关闭")
	}
}
func (a *app) setResourceStatus(id, status, lastError string) {
	_ = a.db.Update(func(s *store.State) error {
		for i := range s.Resources {
			if s.Resources[i].ID == id {
				s.Resources[i].Status = status
				s.Resources[i].LastError = lastError
				if status == "running" {
					s.Resources[i].Recovered = time.Now().UTC().Format(time.RFC3339)
				}
			}
		}
		return nil
	})
}

// The caller owns operations for the entire stop + mutation transaction.
func (a *app) stopLocked(id string) {
	a.mu.Lock()
	if x := a.runs[id]; x != nil {
		x.cancel()
		if x.listener != nil {
			x.listener.Close()
		}
		delete(a.runs, id)
	}
	a.mu.Unlock()
	delete(a.retries, id)
	_ = a.db.Update(func(s *store.State) error {
		for i := range s.Resources {
			if s.Resources[i].ID == id {
				s.Resources[i].Desired = false
				s.Resources[i].Status = "stopped"
				s.Resources[i].LastError = ""
			}
		}
		return nil
	})
}
