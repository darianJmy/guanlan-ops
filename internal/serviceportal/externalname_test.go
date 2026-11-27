package serviceportal

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/store"
)

func TestExternalNameWithoutServicePortsConnectsAndProxies(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "localhost:"+r.URL.Query().Get("port") || r.URL.Path != "/nacos/" {
			t.Errorf("unexpected upstream request: host=%q path=%q", r.Host, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" || strings.Contains(r.Header.Get("Cookie"), "portal_access_") {
			t.Error("platform credentials leaked to ExternalName target")
		}
		io.WriteString(w, "nacos console")
	}))
	t.Cleanup(upstream.Close)
	u, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	var unexpectedKubernetesCalls atomic.Int32
	kubernetes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/namespaces/iot-middleware/services/nacos-cs" {
			unexpectedKubernetesCalls.Add(1)
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":"nacos-cs","namespace":"iot-middleware","creationTimestamp":"2025-04-16T02:39:48Z","resourceVersion":"5582745484","uid":"afd62a88-5e49-4c55-a4d3-282816c7fef3"},"spec":{"externalName":"localhost","sessionAffinity":"None","type":"ExternalName"},"status":{"loadBalancer":{}}}`)
	}))
	t.Cleanup(kubernetes.Close)
	a, _ := integrationApp(t)
	a.runs = map[string]*run{}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	publicPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	a.cfg.MinPort, a.cfg.MaxPort = 1, 65535
	a.cfg.PublicURL = "http://127.0.0.1:8081"
	raw := fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"users":[{"name":"test","user":{}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}],"current-context":"test"}`, kubernetes.URL)
	a.db.Load(store.State{
		Clusters:  []store.Cluster{{ID: "cluster", Context: "test", Kubeconfig: raw}},
		Resources: []store.Resource{{ID: "nacos", Name: "Nacos", ClusterID: "cluster", Namespace: "iot-middleware", Service: "nacos-cs", Port: u.Port(), PublicPort: publicPort, Path: "/nacos/", Desired: true}},
	})
	t.Cleanup(func() {
		a.operations.Lock()
		defer a.operations.Unlock()
		a.stopLocked("nacos")
	})
	if err := a.startDesired("nacos"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var proxy http.Handler
	for time.Now().Before(deadline) {
		resource, _ := a.db.Snapshot().Resource("nacos")
		if resource.Status == "retrying" || resource.Status == "failed" {
			t.Fatalf("ExternalName service could not connect: %s", resource.LastError)
		}
		a.mu.Lock()
		if rr := a.runs["nacos"]; rr != nil && rr.status == "running" {
			proxy = rr.proxy
		}
		a.mu.Unlock()
		if proxy != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if proxy == nil {
		resource, _ := a.db.Snapshot().Resource("nacos")
		data, _ := json.Marshal(resource)
		t.Fatalf("ExternalName connection did not become running: %s", data)
	}
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	page := "http://127.0.0.1:" + strconv.Itoa(publicPort) + "/nacos/?port=" + u.Port()
	response, err := client.Get(page)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("unauthenticated ExternalName access = %d", response.StatusCode)
	}
	launch := apiCall(a, http.MethodPost, "/api/resources/nacos/access", "admin-token", "")
	var access struct {
		URL string `json:"url"`
	}
	if launch.Code != http.StatusOK || json.Unmarshal(launch.Body.Bytes(), &access) != nil {
		t.Fatalf("launch = %d %s", launch.Code, launch.Body)
	}
	response, err = client.Get(access.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusSeeOther || len(response.Cookies()) == 0 {
		t.Fatalf("ticket exchange = %d", response.StatusCode)
	}
	request, _ := http.NewRequest(http.MethodGet, page, nil)
	for _, cookie := range response.Cookies() {
		request.AddCookie(cookie)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(body) != "nacos console" {
		t.Fatalf("proxy response = %d %s (read: %v)", response.StatusCode, body, err)
	}
	if unexpectedKubernetesCalls.Load() != 0 {
		t.Fatalf("ExternalName attempted %d Pod/port-forward requests", unexpectedKubernetesCalls.Load())
	}
	upstream.Close()
	response, err = client.Do(request.Clone(request.Context()))
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusBadGateway {
		t.Fatalf("unavailable ExternalName target response = %d (read: %v)", response.StatusCode, err)
	}
	awaitLifecycle(t, func() bool {
		resource, _ := a.db.Snapshot().Resource("nacos")
		return resource.Desired && resource.Status == "retrying"
	}, "lost ExternalName target did not enter automatic retry")
	if err := a.startDesired("nacos"); err != nil {
		t.Fatal(err)
	}
	awaitLifecycle(t, func() bool {
		resource, _ := a.db.Snapshot().Resource("nacos")
		return resource.Status == "retrying" && strings.Contains(resource.LastError, "无法连接 ExternalName")
	}, "unreachable ExternalName target was published as running")
	a.operations.Lock()
	a.stopLocked("nacos")
	a.operations.Unlock()
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(publicPort)), time.Second); err == nil {
		conn.Close()
		t.Fatal("stopped ExternalName service still has an open public listener")
	}
}
