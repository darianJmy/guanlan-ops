package serviceportal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"guanlan/internal/serviceportal/store"
	"guanlan/internal/serviceportal/targetconfig"
)

// Tokens remain in memory and are never placed in launch URLs, responses.
type accessTicket struct {
	ResourceID, Authorization string
	Expires                   time.Time
}
type accessSession struct {
	ResourceID, Authorization string
	Expires                   time.Time
}

var identityClient = &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}

type identityError struct {
	status  int
	message string
}

func (e *identityError) Error() string { return e.message }
func identityStatus(err error) int {
	var failure *identityError
	if errors.As(err, &failure) {
		return failure.status
	}
	return http.StatusServiceUnavailable
}

func (a *app) platformGet(ctx context.Context, authorization, endpoint string, dst any) error {
	if !strings.HasPrefix(authorization, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")) == "" {
		return &identityError{http.StatusUnauthorized, "需要 观澜 登录"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.GuanlanURL+endpoint, nil)
	if err != nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份校验地址无效"}
	}
	req.Header.Set("Authorization", authorization)
	if a.cfg.InternalToken != "" {
		req.Header.Set("X-Service-Portal-Internal", a.cfg.InternalToken)
	}
	resp, err := identityClient.Do(req)
	if err != nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份校验暂不可用，请稍后重试"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return &identityError{401, "观澜 登录无效或已过期"}
		}
		if resp.StatusCode == http.StatusForbidden {
			return &identityError{403, "没有服务仪表盘权限"}
		}
		return &identityError{http.StatusServiceUnavailable, "观澜 身份校验暂不可用，请稍后重试"}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份响应无效"}
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份响应无效"}
	}
	if len(envelope.Data) == 0 || (envelope.Code == 0 && string(envelope.Data) == "null") {
		// Private runtime endpoints use a bare JSON body to keep this contract
		// independent from the browser API envelope.
		if err := json.Unmarshal(body, dst); err != nil {
			return &identityError{http.StatusServiceUnavailable, "观澜 身份响应无效"}
		}
		return nil
	}
	if envelope.Code == 401 || envelope.Code == 403 {
		return &identityError{envelope.Code, "观澜 身份或权限无效"}
	}
	if envelope.Code != 0 {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份校验暂不可用"}
	}
	if len(envelope.Data) == 0 {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份响应无效"}
	}
	if err := json.Unmarshal(envelope.Data, dst); err != nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 身份响应无效"}
	}
	return nil
}

func (a *app) platformIdentity(ctx context.Context, authorization string) (store.User, error) {
	var identity struct {
		User store.User `json:"user"`
	}
	if err := a.platformGet(ctx, authorization, "/api/service-access/identity", &identity); err != nil {
		return store.User{}, err
	}
	if !strings.HasPrefix(identity.User.ID, "guanlan:") || !identity.User.Enabled {
		return store.User{}, &identityError{403, "观澜 账号无效或已停用"}
	}
	return identity.User, nil
}

// refreshState refreshes configuration while checking a user's access. Desired
// is owned by 观澜; only listener status is overlaid from process memory.
func (a *app) refreshState(ctx context.Context, authorization string) error {
	// Run() rejects a missing token. Keeping this guard makes manually
	// constructed in-memory test applications independent of 观澜 state.
	if a.cfg.InternalToken == "" {
		return nil
	}
	sequence := a.stateFetch.Add(1)
	var next store.State
	if err := a.platformGet(ctx, authorization, "/api/service-access/portal/state", &next); err != nil {
		return err
	}
	if next.Resources == nil || next.Clusters == nil {
		return &identityError{http.StatusServiceUnavailable, "观澜 运行配置响应无效"}
	}
	a.operations.Lock()
	defer a.operations.Unlock()
	a.applyFetchedStateLocked(next, sequence)
	return nil
}

func (a *app) integrationRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/auth/") || strings.HasPrefix(r.URL.Path, "/api/users/") {
			jsonErr(w, http.StatusNotFound, errors.New("账号认证由 观澜 管理"))
			return
		}
		if r.URL.Path != "/healthz" && !strings.HasPrefix(r.URL.Path, "/api/") {
			jsonErr(w, http.StatusNotFound, errors.New("请从 观澜 服务仪表盘访问"))
			return
		}
		if r.URL.Path == "/api/me" && !methodAllowed(w, r, http.MethodGet) {
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
		}
		next.ServeHTTP(w, r)
	})
}

func methodAllowed(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if method == r.Method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	jsonErr(w, http.StatusMethodNotAllowed, errors.New("不支持的请求方法"))
	return false
}

func (a *app) requireAdmin(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := a.require(w, r)
	if !ok {
		return u, false
	}
	if !u.Admin {
		jsonErr(w, http.StatusForbidden, errors.New("没有服务管理权限"))
		return u, false
	}
	return u, true
}

func (a *app) requireFolderAccess(w http.ResponseWriter, u store.User, folderID uint, minimum string) bool {
	if folderID == 0 {
		return true
	}
	if store.AccessRank(u.FolderAccess[folderID]) < store.AccessRank(minimum) {
		jsonErr(w, http.StatusForbidden, errors.New("没有该服务目录的操作权限"))
		return false
	}
	return true
}

var resourceName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$`)
var portName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var resourceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func validateResource(x store.Resource) error {
	if !resourceID.MatchString(x.ID) {
		return errors.New("资源标识无效")
	}
	if strings.TrimSpace(x.Name) == "" || len(x.Name) > 128 {
		return errors.New("请填写有效的服务名称")
	}
	kind, err := targetconfig.Type(x.Type)
	if err != nil {
		return err
	}
	if kind == targetconfig.HTTPDirect {
		return errors.New("直连服务不支持代理运行")
	}
	if kind == targetconfig.HTTP {
		_, err := targetconfig.ParseHTTPURL(x.TargetURL)
		return err
	}
	if x.ClusterID == "" || len(x.Namespace) > 63 || !resourceName.MatchString(x.Namespace) || len(x.Service) > 253 || !resourceName.MatchString(x.Service) {
		return errors.New("请填写有效的名称、集群、命名空间和 Service")
	}
	port, err := strconv.Atoi(x.Port)
	if err == nil {
		if port < 1 || port > 65535 {
			return errors.New("Service 端口无效")
		}
	} else if len(x.Port) > 15 || !portName.MatchString(x.Port) {
		return errors.New("Service 端口名称无效")
	}
	u, err := url.Parse(x.Path)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(x.Path, "/") || strings.HasPrefix(x.Path, "//") || strings.ContainsAny(x.Path, "\\\r\n") || u.Fragment != "" || strings.HasPrefix(u.Path, "/_portal/") {
		return errors.New("打开路径必须是站内绝对路径，例如 /nacos/")
	}
	return nil
}

func (a *app) issueAccess(w http.ResponseWriter, r *http.Request, u store.User, id string) {
	st := a.db.Snapshot()
	resource, exists := st.Resource(id)
	if !exists {
		jsonErr(w, 404, errors.New("资源不存在"))
		return
	}
	if !st.Allowed(u, id) {
		jsonErr(w, 403, errors.New("没有该服务的访问权限"))
		return
	}
	a.mu.Lock()
	rr := a.runs[id]
	ready := rr != nil && (rr.status == "running" || (rr.resource.OnDemand && rr.listener != nil))
	if ready && resource.Type == targetconfig.KubernetesTCP {
		ready = rr.resource.Type == targetconfig.KubernetesTCP && rr.port == resource.PublicPort && rr.listener != nil
	}
	a.mu.Unlock()
	if !ready {
		jsonErr(w, 409, errors.New("服务尚未启动"))
		return
	}
	base, err := url.Parse(a.cfg.PublicURL)
	if err != nil || base.Hostname() == "" {
		jsonErr(w, 500, errors.New("服务访问地址未配置"))
		return
	}
	base.Host = net.JoinHostPort(base.Hostname(), strconv.Itoa(resource.PublicPort))
	if resource.Type == targetconfig.KubernetesTCP {
		if err := validateResource(resource); err != nil {
			jsonErr(w, 400, err)
			return
		}
		expires, status, err := a.grantTCPAccess(r, u, resource, rr)
		if err != nil {
			jsonErr(w, status, err)
			return
		}
		path, _ := url.Parse(resource.Path)
		base.Scheme = "https"
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]string{"url": base.ResolveReference(path).String(), "address": base.Host, "expiresAt": expires.UTC().Format(time.RFC3339Nano)})
		return
	}
	ticket := store.ID()
	a.mu.Lock()
	a.cleanupAccessLocked()
	if len(a.tickets) >= 4096 {
		a.mu.Unlock()
		jsonErr(w, 503, errors.New("访问请求繁忙，请稍后重试"))
		return
	}
	a.tickets[ticket] = accessTicket{ResourceID: id, Authorization: r.Header.Get("Authorization"), Expires: time.Now().Add(time.Minute)}
	a.mu.Unlock()
	base.Path = "/_portal/access"
	base.RawQuery = url.Values{"ticket": {ticket}}.Encode()
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{"url": base.String()})
}

func (a *app) cleanupAccessLocked() {
	if a.tickets == nil {
		a.tickets = map[string]accessTicket{}
	}
	if a.access == nil {
		a.access = map[string]accessSession{}
	}
	now := time.Now()
	for id, ticket := range a.tickets {
		if !now.Before(ticket.Expires) {
			delete(a.tickets, id)
		}
	}
	for id, session := range a.access {
		if !now.Before(session.Expires) {
			delete(a.access, id)
		}
	}
}

func resourceCookie(id string) string { return "portal_access_" + id }

func (a *app) exchangeAccess(w http.ResponseWriter, r *http.Request, id string) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	token := r.URL.Query().Get("ticket")
	a.mu.Lock()
	a.cleanupAccessLocked()
	ticket, ok := a.tickets[token]
	if ok {
		delete(a.tickets, token)
	}
	a.mu.Unlock()
	if !ok || ticket.ResourceID != id {
		jsonErr(w, 401, errors.New("访问票据已过期或已使用，请从服务仪表盘重新打开"))
		return
	}
	u, err := a.platformIdentity(r.Context(), ticket.Authorization)
	if err == nil {
		err = a.refreshState(r.Context(), ticket.Authorization)
	}
	st := a.db.Snapshot()
	resource, exists := st.Resource(id)
	if err != nil {
		jsonErr(w, identityStatus(err), err)
		return
	}
	if !exists || !st.Allowed(u, id) {
		jsonErr(w, 403, errors.New("服务访问权限已失效"))
		return
	}
	sid := store.ID()
	a.mu.Lock()
	if len(a.access) >= 8192 {
		a.mu.Unlock()
		jsonErr(w, 503, errors.New("访问会话繁忙，请稍后重试"))
		return
	}
	a.access[sid] = accessSession{ResourceID: id, Authorization: ticket.Authorization, Expires: time.Now().Add(12 * time.Hour)}
	a.mu.Unlock()
	// Business listeners use plain HTTP; TLS can be terminated by an external proxy.
	http.SetCookie(w, &http.Cookie{Name: resourceCookie(id), Value: sid, Path: "/", HttpOnly: true, Secure: strings.HasPrefix(a.cfg.PublicURL, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 43200})
	destination := resource.Path
	if validateResource(resource) != nil {
		destination = "/"
	} else if resource.Type == targetconfig.HTTP {
		target, _ := targetconfig.ParseHTTPURL(resource.TargetURL)
		destination = targetconfig.OpenPath(target)
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

func (a *app) resourceHandler(id string, proxy http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u store.User
		var good bool
		var platformAuthorization string
		var identityFailure error
		var bearerAuthentication bool
		platformHeaders, explicitPlatformHeader := r.Header[http.CanonicalHeaderKey("X-Guanlan-Authorization")]
		if r.URL.Path == "/_portal/access" {
			a.exchangeAccess(w, r, id)
			return
		}
		cookie, err := r.Cookie(resourceCookie(id))
		if err == nil && !explicitPlatformHeader {
			a.mu.Lock()
			session, exists := a.access[cookie.Value]
			a.mu.Unlock()
			if exists && session.ResourceID == id && time.Now().Before(session.Expires) {
				platformAuthorization = session.Authorization
				u, err = a.platformIdentity(r.Context(), session.Authorization)
				if err == nil {
					err = a.refreshState(r.Context(), session.Authorization)
				}
				identityFailure = err
				good = err == nil
			}
		}
		// The explicit platform header takes precedence over browser sessions.
		// Keep Authorization available for the application; accept it as a
		// legacy platform credential only when neither header nor session exists.
		if explicitPlatformHeader || (platformAuthorization == "" && r.Header.Get("Authorization") != "") {
			bearerAuthentication = true
			platformAuthorization = r.Header.Get("Authorization")
			if explicitPlatformHeader {
				if len(platformHeaders) != 1 {
					jsonErr(w, http.StatusUnauthorized, errors.New("观澜 认证请求头无效"))
					return
				}
				platformAuthorization = platformHeaders[0]
			}
			u, err = a.platformIdentity(r.Context(), platformAuthorization)
			if err == nil {
				err = a.refreshState(r.Context(), platformAuthorization)
			}
			identityFailure, good = err, err == nil
		}
		if identityFailure != nil && (bearerAuthentication || identityStatus(identityFailure) == http.StatusServiceUnavailable) {
			jsonErr(w, identityStatus(identityFailure), identityFailure)
			return
		}
		if !good || !a.db.Snapshot().Allowed(u, id) {
			http.Error(w, "需要资源权限，请从服务仪表盘重新打开", 403)
			return
		}
		request := r.Clone(r.Context())
		request.Header = r.Header.Clone()
		request.Header.Del("X-Guanlan-Authorization")
		request.Header.Del("X-Service-Portal-Internal")
		request.Header.Del("X-Service-Portal-Client-IP")
		// Keep the target application's own Basic/Bearer credentials functional.
		// The platform token is used only for the identity request above.
		if !explicitPlatformHeader && platformAuthorization != "" && request.Header.Get("Authorization") == platformAuthorization {
			request.Header.Del("Authorization")
		}
		request.Header.Del("Cookie")
		for _, cookie := range r.Cookies() {
			if cookie.Name != "portal_session" && !strings.HasPrefix(cookie.Name, "portal_access_") {
				request.AddCookie(cookie)
			}
		}
		proxy.ServeHTTP(w, request)
	})
}
