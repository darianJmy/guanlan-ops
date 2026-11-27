package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func serviceClient(t *testing.T, service *v1.Service, pods ...v1.Pod) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		base := "/api/v1/namespaces/" + service.Namespace
		switch r.URL.Path {
		case base + "/services":
			json.NewEncoder(w).Encode(v1.ServiceList{Items: []v1.Service{*service}})
		case base + "/services/" + service.Name:
			json.NewEncoder(w).Encode(service)
		case base + "/pods":
			json.NewEncoder(w).Encode(v1.PodList{Items: pods})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	api, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return &Client{API: api}
}

func TestExternalNameDiscoveryIncludesTarget(t *testing.T) {
	s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "nacos-cs", Namespace: "iot-middleware"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: "dreame-iot-prod-nacos-internal.dreame.tech"}}
	c := serviceClient(t, s)
	services, err := c.Services(context.Background(), s.Namespace)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(services)
	if err != nil {
		t.Fatal(err)
	}
	var result []struct {
		Type         string `json:"type"`
		ExternalName string `json:"externalName"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Type != "ExternalName" || result[0].ExternalName != s.Spec.ExternalName {
		t.Fatalf("ExternalName target missing from service discovery: %s", data)
	}
}

func TestExternalNameWithoutDeclaredPortsResolvesExplicitPort(t *testing.T) {
	s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "nacos-cs", Namespace: "iot-middleware"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: "dreame-iot-prod-nacos-internal.dreame.tech"}}
	c := serviceClient(t, s)
	target, err := c.ResolveTarget(context.Background(), s.Namespace, s.Name, "8848")
	if err != nil || target.Host != s.Spec.ExternalName || target.Port != 8848 || target.Pod != "" {
		t.Fatalf("ExternalName with an explicit port cannot connect: target=%+v err=%v", target, err)
	}
}

func TestExternalNamePortSelection(t *testing.T) {
	for _, test := range []struct {
		name     string
		selected string
		ports    []v1.ServicePort
		want     int
		wantErr  string
	}{
		{name: "minimum explicit port", selected: "1", want: 1},
		{name: "maximum explicit port", selected: "65535", want: 65535},
		{name: "missing explicit port", wantErr: "请填写 ExternalName 目标端口"},
		{name: "missing explicit port never defaults to unnamed port", ports: []v1.ServicePort{{Port: 8848}}, wantErr: "请填写 ExternalName 目标端口"},
		{name: "undeclared name", selected: "http", wantErr: "请填写 ExternalName 目标端口"},
		{name: "zero", selected: "0", wantErr: "1–65535"},
		{name: "negative", selected: "-1", wantErr: "1–65535"},
		{name: "too large", selected: "65536", wantErr: "1–65535"},
		{name: "declared name uses service port", selected: "http", ports: []v1.ServicePort{{Name: "http", Port: 8848, TargetPort: intstr.FromInt(8080)}}, want: 8848},
		{name: "declared number ignores named target port", selected: "8848", ports: []v1.ServicePort{{Name: "http", Port: 8848, TargetPort: intstr.FromString("web")}}, want: 8848},
		{name: "explicit undeclared number", selected: "8080", ports: []v1.ServicePort{{Name: "http", Port: 8848}}, want: 8080},
		{name: "UDP name", selected: "dns", ports: []v1.ServicePort{{Name: "dns", Port: 53, Protocol: v1.ProtocolUDP}}, wantErr: "仅支持 TCP"},
		{name: "UDP number", selected: "53", ports: []v1.ServicePort{{Name: "dns", Port: 53, Protocol: v1.ProtocolUDP}}, wantErr: "仅支持 TCP"},
		{name: "UDP alternative number spelling", selected: "053", ports: []v1.ServicePort{{Name: "dns", Port: 53, Protocol: v1.ProtocolUDP}}, wantErr: "仅支持 TCP"},
		{name: "UDP before TCP on same numeric port", selected: "53", ports: []v1.ServicePort{{Name: "dns-udp", Port: 53, Protocol: v1.ProtocolUDP}, {Name: "dns-tcp", Port: 53, Protocol: v1.ProtocolTCP}}, want: 53},
		{name: "TCP before UDP on same numeric port", selected: "53", ports: []v1.ServicePort{{Name: "dns-tcp", Port: 53, Protocol: v1.ProtocolTCP}, {Name: "dns-udp", Port: 53, Protocol: v1.ProtocolUDP}}, want: 53},
		{name: "UDP name remains rejected when same numeric TCP exists", selected: "dns-udp", ports: []v1.ServicePort{{Name: "dns-udp", Port: 53, Protocol: v1.ProtocolUDP}, {Name: "dns-tcp", Port: 53, Protocol: v1.ProtocolTCP}}, wantErr: "仅支持 TCP"},
		{name: "SCTP number", selected: "80", ports: []v1.ServicePort{{Port: 80, Protocol: v1.ProtocolSCTP}}, wantErr: "仅支持 TCP"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: "example.test.", Ports: test.ports}}
			target, err := serviceClient(t, s).ResolveTarget(context.Background(), s.Namespace, s.Name, test.selected)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("target=%+v error=%v, want error containing %q", target, err, test.wantErr)
				}
				return
			}
			if err != nil || target.Host != "example.test." || target.Pod != "" || target.Port != test.want {
				t.Fatalf("target=%+v error=%v, want DNS target on port %d", target, err, test.want)
			}
		})
	}
}

func TestExternalNameRejectsInvalidDNSNames(t *testing.T) {
	for _, name := range []string{"", "https://example.test", "example.test:80", "example.test/path", "example.test..", "-example.test"} {
		t.Run(name, func(t *testing.T) {
			s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: name}}
			if _, err := serviceClient(t, s).ResolveTarget(context.Background(), s.Namespace, s.Name, "80"); err == nil || !strings.Contains(err.Error(), "DNS") {
				t.Fatalf("invalid ExternalName %q accepted: %v", name, err)
			}
		})
	}
}

func TestExternalNameCannotBeUsedAsPodForPortForward(t *testing.T) {
	s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "default"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: "example.test"}}
	if _, _, err := serviceClient(t, s).Target(context.Background(), s.Namespace, s.Name, "80"); err == nil || !strings.Contains(err.Error(), "没有可端口转发的 Pod") {
		t.Fatalf("ExternalName incorrectly used as Pod: %v", err)
	}
}

func TestResolveTargetStillRequiresSelectorForPodServices(t *testing.T) {
	s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "no-selector", Namespace: "default"}, Spec: v1.ServiceSpec{Type: v1.ServiceTypeClusterIP}}
	if _, err := serviceClient(t, s).ResolveTarget(context.Background(), s.Namespace, s.Name, "80"); err == nil || !strings.Contains(err.Error(), "没有 selector") {
		t.Fatalf("Service without selector accepted: %v", err)
	}
}

func TestResolveTargetStillSelectsReadyPodAndResolvesTargetPort(t *testing.T) {
	s := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "nacos", Namespace: "default"}, Spec: v1.ServiceSpec{Selector: map[string]string{"app": "nacos"}, Ports: []v1.ServicePort{{Name: "http", Port: 8848, TargetPort: intstr.FromString("web")}}}}
	ready := v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "ready"}, Spec: v1.PodSpec{Containers: []v1.Container{{Ports: []v1.ContainerPort{{Name: "web", ContainerPort: 8080}}}}}, Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}}
	notReady := ready.DeepCopy()
	notReady.Name = "a-not-ready"
	notReady.Status.Conditions[0].Status = v1.ConditionFalse
	c := serviceClient(t, s, *notReady, ready)
	target, err := c.ResolveTarget(context.Background(), s.Namespace, s.Name, "http")
	if err != nil || target.Pod != "ready" || target.Host != "" || target.Port != 8080 {
		t.Fatalf("Pod target resolution changed: target=%+v error=%v", target, err)
	}
	pod, port, err := c.Target(context.Background(), s.Namespace, s.Name, "8848")
	if err != nil || pod != "ready" || port != 8080 {
		t.Fatalf("legacy Pod target resolution changed: pod=%q port=%d error=%v", pod, port, err)
	}
}

func TestResolvePortByNameAndNumber(t *testing.T) {
	s := &v1.Service{Spec: v1.ServiceSpec{Ports: []v1.ServicePort{{Name: "http", Port: 8848, TargetPort: intstr.FromString("web")}}}}
	p := &v1.Pod{Spec: v1.PodSpec{Containers: []v1.Container{{Ports: []v1.ContainerPort{{Name: "web", ContainerPort: 8080}}}}}}
	for _, selected := range []string{"http", "8848"} {
		if got, err := ResolvePort(s, p, selected); err != nil || got != 8080 {
			t.Fatalf("selected %s: got %d, %v", selected, got, err)
		}
	}
}

func TestReady(t *testing.T) {
	p := &v1.Pod{Status: v1.PodStatus{Phase: v1.PodRunning, Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}}}}
	if !Ready(p) {
		t.Fatal("ready pod reported not ready")
	}
}

func TestForwardCancellationClosesStalledUpgrade(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(closed)
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/namespaces/default/pods/target/portforward" ||
			r.Header.Get("Authorization") != "Bearer test-token" ||
			r.Header.Get("Upgrade") != "SPDY/3.1" {
			t.Errorf("unexpected port-forward upgrade method, path or headers")
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	client, err := New([]byte(fmt.Sprintf(`{
		"apiVersion":"v1","kind":"Config",
		"clusters":[{"name":"test","cluster":{"server":%q,"insecure-skip-tls-verify":true}}],
		"users":[{"name":"test","user":{"token":"test-token"}}],
		"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}]
	}`, server.URL)), "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := client.Forward(ctx, "default", "target", 8848)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upgrade request did not reach the API server")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("forward returned %v after cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("forward did not return after cancellation")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled upgrade remained open at the API server")
	}
}

func TestForwardTunnelsPlaintextAPIThroughHTTPProxy(t *testing.T) {
	const apiAddress = "kubernetes.test:8080"
	request := make(chan error, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != apiAddress || r.Header.Get("Proxy-Authorization") != "Basic cHJveHk6dGVzdA==" {
			request <- fmt.Errorf("unexpected proxy CONNECT method, host or authorization")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			request <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			request <- err
			return
		}
		upgrade, err := http.ReadRequest(buffered.Reader)
		if err == nil && (upgrade.Method != http.MethodPost || upgrade.URL.IsAbs() || upgrade.URL.Path != "/api/v1/namespaces/default/pods/target/portforward" ||
			upgrade.Header.Get("Upgrade") != "SPDY/3.1" || upgrade.Header.Get("Proxy-Authorization") != "") {
			err = fmt.Errorf("unexpected tunneled SPDY request: method=%q URI=%q upgrade=%q proxy authorization set=%t", upgrade.Method, upgrade.RequestURI, upgrade.Header.Get("Upgrade"), upgrade.Header.Get("Proxy-Authorization") != "")
		}
		request <- err
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL.User = url.UserPassword("proxy", "test")
	client, err := New([]byte(fmt.Sprintf(`{
		"apiVersion":"v1","kind":"Config",
		"clusters":[{"name":"test","cluster":{"server":%q,"proxy-url":%q}}],
		"contexts":[{"name":"test","context":{"cluster":"test"}}]
	}`, "http://"+apiAddress, proxyURL.String())), "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, _ = client.Forward(ctx, "default", "target", 8848)
	select {
	case err := <-request:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("SPDY upgrade did not reach the API server through CONNECT")
	}
}
