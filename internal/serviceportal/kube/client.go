package kube

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
)

type Client struct {
	API              kubernetes.Interface
	forwardTransport http.RoundTripper
	namespace        string
}
type Port struct {
	Name     string      `json:"name"`
	Port     int32       `json:"port"`
	Protocol v1.Protocol `json:"protocol"`
}
type Service struct {
	Name         string         `json:"name"`
	Ports        []Port         `json:"ports"`
	Type         v1.ServiceType `json:"type"`
	ExternalName string         `json:"externalName,omitempty"`
}

// Target identifies either a Pod to port-forward or an ExternalName to dial.
type Target struct {
	Pod  string
	Host string
	Port int
}

// Only portable, non-executable, self-contained credentials may be uploaded.
func Contexts(data []byte) ([]string, error) {
	c, err := clientcmd.Load(data)
	if err != nil {
		return nil, errors.New("kubeconfig 格式无效")
	}
	if len(c.Contexts) == 0 {
		return nil, errors.New("kubeconfig 中没有 context")
	}
	for _, a := range c.AuthInfos {
		if a.Exec != nil || a.AuthProvider != nil || a.TokenFile != "" || a.ClientCertificate != "" || a.ClientKey != "" {
			return nil, errors.New("仅支持内嵌证书或 token，不支持 exec、外部文件或 auth-provider")
		}
		if a.Impersonate != "" || len(a.ImpersonateGroups) > 0 || len(a.ImpersonateUserExtra) > 0 {
			return nil, errors.New("不支持 impersonation")
		}
	}
	for _, cl := range c.Clusters {
		if cl.CertificateAuthority != "" {
			return nil, errors.New("CA 必须内嵌在 kubeconfig 中")
		}
	}
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

func New(data []byte, selected string) (*Client, error) {
	names, err := Contexts(data)
	if err != nil {
		return nil, err
	}
	found := false
	for _, n := range names {
		if n == selected {
			found = true
		}
	}
	if !found {
		return nil, errors.New("请选择 kubeconfig 中的有效 context")
	}
	cfg, err := clientcmd.NewClientConfigFromBytes(data)
	if err != nil {
		return nil, errors.New("kubeconfig 无效")
	}
	raw, err := cfg.RawConfig()
	if err != nil {
		return nil, errors.New("kubeconfig 无效")
	}
	rc, err := clientcmd.NewNonInteractiveClientConfig(raw, selected, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return nil, errors.New("context 缺少有效的集群或凭据")
	}
	rc.Timeout = 20 * time.Second
	api, err := kubernetes.NewForConfig(rc)
	if err != nil {
		return nil, errors.New("无法创建 Kubernetes 客户端")
	}
	forwardConfig := rest.CopyConfig(rc)
	forwardConfig.NextProtos = []string{"http/1.1"}
	if err := configureForwardProxy(forwardConfig); err != nil {
		return nil, errors.New("无法配置转发代理")
	}
	forwardTransport, err := rest.TransportFor(forwardConfig)
	if err != nil {
		return nil, errors.New("无法创建转发传输")
	}
	return &Client{API: api, forwardTransport: forwardTransport, namespace: raw.Contexts[selected].Namespace}, nil
}

func (c *Client) Namespaces(ctx context.Context) ([]string, error) {
	if c.namespace != "" {
		return []string{c.namespace}, nil
	}
	list, err := c.API.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, SafeError(err)
	}
	names := []string{}
	for _, n := range list.Items {
		names = append(names, n.Name)
	}
	sort.Strings(names)
	return names, nil
}
func (c *Client) Services(ctx context.Context, ns string) ([]Service, error) {
	if c.namespace != "" && ns != c.namespace {
		return nil, errors.New("请选择接入配置中的命名空间")
	}
	list, err := c.API.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, SafeError(err)
	}
	result := []Service{}
	for _, s := range list.Items {
		ports := []Port{}
		for _, p := range s.Spec.Ports {
			ports = append(ports, Port{p.Name, p.Port, p.Protocol})
		}
		result = append(result, Service{Name: s.Name, Ports: ports, Type: s.Spec.Type, ExternalName: s.Spec.ExternalName})
	}
	return result, nil
}
func SafeError(err error) error {
	// Kubernetes transport errors can embed endpoints and credential-bearing URLs.
	if err == nil {
		return nil
	}
	return fmt.Errorf("Kubernetes 请求失败（请检查网络、凭据及 RBAC 权限）：%T", err)
}

func ResolvePort(s *v1.Service, p *v1.Pod, selected string) (int, error) {
	for _, sp := range s.Spec.Ports {
		if selected != sp.Name && selected != strconv.Itoa(int(sp.Port)) {
			continue
		}
		if sp.Protocol != v1.ProtocolTCP && sp.Protocol != "" {
			return 0, errors.New("仅支持 TCP Service 端口")
		}
		if sp.TargetPort.Type == intstr.String {
			for _, ct := range p.Spec.Containers {
				for _, cp := range ct.Ports {
					if cp.Name == sp.TargetPort.StrVal && (cp.Protocol == v1.ProtocolTCP || cp.Protocol == "") {
						return int(cp.ContainerPort), nil
					}
				}
			}
			return 0, errors.New("Pod 中没有对应的命名 targetPort")
		}
		if sp.TargetPort.IntVal == 0 {
			return int(sp.Port), nil
		}
		return int(sp.TargetPort.IntVal), nil
	}
	return 0, errors.New("Service 中没有所选端口")
}
func Ready(p *v1.Pod) bool {
	if p.DeletionTimestamp != nil || p.Status.Phase != v1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == v1.PodReady && c.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
func (c *Client) Target(ctx context.Context, ns, name, port string) (string, int, error) {
	target, err := c.ResolveTarget(ctx, ns, name, port)
	if err != nil {
		return "", 0, err
	}
	if target.Pod == "" {
		return "", 0, errors.New("ExternalName Service 没有可端口转发的 Pod")
	}
	return target.Pod, target.Port, nil
}

func (c *Client) ResolveTarget(ctx context.Context, ns, name, port string) (Target, error) {
	s, err := c.API.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Target{}, SafeError(err)
	}
	if s.Spec.Type == v1.ServiceTypeExternalName {
		if len(validation.IsDNS1123Subdomain(strings.TrimSuffix(s.Spec.ExternalName, "."))) > 0 {
			return Target{}, errors.New("ExternalName 必须是有效的 DNS 名称")
		}
		n, err := externalPort(s, port)
		if err != nil {
			return Target{}, err
		}
		return Target{Host: s.Spec.ExternalName, Port: n}, nil
	}
	if len(s.Spec.Selector) == 0 {
		return Target{}, errors.New("不支持没有 selector 的 Service")
	}
	pods, err := c.API.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: labels.SelectorFromSet(s.Spec.Selector).String()})
	if err != nil {
		return Target{}, SafeError(err)
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	for _, p := range pods.Items {
		if Ready(&p) {
			n, e := ResolvePort(s, &p, port)
			if e != nil {
				return Target{}, e
			}
			return Target{Pod: p.Name, Port: n}, nil
		}
	}
	return Target{}, errors.New("Service 没有 Ready Pod")
}

func externalPort(s *v1.Service, selected string) (int, error) {
	if selected == "" {
		return 0, errors.New("请填写 ExternalName 目标端口（1–65535），或选择已声明的 TCP 端口名称")
	}
	number, numericErr := strconv.Atoi(selected)
	if numericErr == nil && (number < 1 || number > 65535) {
		return 0, errors.New("ExternalName 目标端口必须是 1–65535 的整数")
	}
	nonTCPMatch := false
	for _, port := range s.Spec.Ports {
		if selected != port.Name && (numericErr != nil || number != int(port.Port)) {
			continue
		}
		if port.Protocol != v1.ProtocolTCP && port.Protocol != "" {
			if numericErr != nil {
				return 0, errors.New("仅支持 TCP Service 端口")
			}
			// TCP and UDP may declare the same number in either order.
			nonTCPMatch = true
			continue
		}
		if port.Port < 1 || port.Port > 65535 {
			return 0, errors.New("ExternalName 目标端口必须是 1–65535 的整数")
		}
		// ExternalName is a DNS alias; targetPort only applies to Pod routing.
		return int(port.Port), nil
	}
	if nonTCPMatch {
		return 0, errors.New("仅支持 TCP Service 端口")
	}
	if numericErr == nil {
		return number, nil
	}
	return 0, errors.New("请填写 ExternalName 目标端口（1–65535），或选择已声明的 TCP 端口名称")
}

type upgradeDialer func(...string) (httpstream.Connection, string, error)

func (d upgradeDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	return d(protocols...)
}

type upgradedConn struct {
	net.Conn
	reader io.Reader
}

func (c upgradedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

type forwardContextKey struct{}

// A plaintext API server also needs a TCP tunnel when reached through an HTTP
// proxy. Preserve client-go's CONNECT behavior for port-forward upgrades.
func configureForwardProxy(config *rest.Config) error {
	server, _, err := rest.DefaultServerUrlFor(config)
	if err != nil || server.Scheme != "http" {
		return err
	}
	proxy := config.Proxy
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	proxyURL, err := proxy(&http.Request{URL: server})
	if err != nil || proxyURL == nil || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") {
		return err
	}
	tlsConfig, err := rest.TLSConfigFor(config)
	if err != nil {
		return err
	}
	config.Proxy = func(*http.Request) (*url.URL, error) { return nil, nil }
	config.Dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		// net/http allows dials to outlive a canceled request for ordinary
		// keep-alive use. Bound these CONNECT handshakes by the caller's context.
		if caller, ok := ctx.Value(forwardContextKey{}).(context.Context); ok {
			ctx = caller
		}
		return dialProxyTunnel(ctx, proxyURL, tlsConfig, network, address)
	}
	return nil
}

func dialProxyTunnel(ctx context.Context, proxyURL *url.URL, tlsConfig *tls.Config, network, address string) (net.Conn, error) {
	proxyPort := proxyURL.Port()
	if proxyPort == "" {
		proxyPort = "80"
		if proxyURL.Scheme == "https" {
			proxyPort = "443"
		}
	}
	proxyAddress := net.JoinHostPort(proxyURL.Hostname(), proxyPort)
	dial := (&net.Dialer{}).DialContext
	if proxyURL.Scheme == "https" {
		dial = (&tls.Dialer{Config: tlsConfig}).DialContext
	}
	conn, err := dial(ctx, network, proxyAddress)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: make(http.Header)}
	if user := proxyURL.User; user != nil {
		password, _ := user.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(user.Username() + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+credentials)
	}
	if err := req.Write(conn); err != nil {
		stop()
		_ = conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	stop()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_ = conn.Close()
		_ = resp.Body.Close()
		return nil, fmt.Errorf("转发代理拒绝 CONNECT 请求：HTTP %d", resp.StatusCode)
	}
	if err := ctx.Err(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return upgradedConn{Conn: conn, reader: reader}, nil
}

// Forward uses a dynamically allocated loopback port. The caller owns cancellation.
func (c *Client) Forward(ctx context.Context, ns, pod string, target int) (int, <-chan error, error) {
	u := c.API.CoreV1().RESTClient().Post().Resource("pods").Namespace(ns).Name(pod).SubResource("portforward").URL()
	dialer := upgradeDialer(func(protocols ...string) (httpstream.Connection, string, error) {
		return c.dialForward(ctx, u.String(), protocols)
	})
	stop := make(chan struct{})
	ready := make(chan struct{})
	done := make(chan error, 1)
	pf, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{fmt.Sprintf("0:%d", target)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return 0, nil, errors.New("无法创建端口转发")
	}
	stopOnCancel := context.AfterFunc(ctx, func() { close(stop) })
	go func() {
		defer stopOnCancel()
		defer close(done)
		done <- pf.ForwardPorts()
	}()
	select {
	case <-ready:
		ports, err := pf.GetPorts()
		if err != nil || len(ports) == 0 {
			return 0, nil, errors.New("无法读取内部端口")
		}
		return int(ports[0].Local), done, nil
	case <-ctx.Done():
		<-done
		return 0, nil, ctx.Err()
	case <-done:
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		return 0, nil, errors.New("端口转发建立失败，请检查 pods/portforward 权限及网络")
	}
}

func (c *Client) dialForward(ctx context.Context, url string, protocols []string) (httpstream.Connection, string, error) {
	// The HTTP transport cancels stalled upgrades and hands ownership of the
	// socket to the SPDY connection after a successful protocol switch.
	var conn net.Conn
	ctx = context.WithValue(ctx, forwardContextKey{}, ctx)
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) { conn = info.Conn },
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set(httpstream.HeaderConnection, httpstream.HeaderUpgrade)
	req.Header.Set(httpstream.HeaderUpgrade, spdy.HeaderSpdy31)
	for _, protocol := range protocols {
		req.Header.Add(httpstream.HeaderProtocolVersion, protocol)
	}
	resp, err := c.forwardTransport.RoundTrip(req)
	if err != nil {
		return nil, "", err
	}
	if _, ok := resp.Body.(io.ReadWriteCloser); !ok || conn == nil || resp.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(resp.Header.Get(httpstream.HeaderUpgrade), spdy.HeaderSpdy31) {
		_ = resp.Body.Close()
		return nil, "", errors.New("端口转发协议协商失败")
	}
	// Read through the response body to drain bytes buffered while reading
	// the HTTP headers before reading directly from the upgraded socket.
	stream, err := spdy.NewClientConnectionWithPings(upgradedConn{Conn: conn, reader: resp.Body}, 5*time.Second)
	return stream, resp.Header.Get(httpstream.HeaderProtocolVersion), err
}
