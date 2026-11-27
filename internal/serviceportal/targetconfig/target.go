// Package targetconfig validates service targets shared by persistence and runtime.
package targetconfig

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
)

const (
	Kubernetes    = "kubernetes"
	KubernetesTCP = "kubernetes_tcp"
	HTTP          = "http"
	HTTPDirect    = "http_direct"
)

// Type keeps resource definitions saved before access types were introduced valid.
func Type(value string) (string, error) {
	switch value {
	case "", Kubernetes:
		return Kubernetes, nil
	case KubernetesTCP, HTTP, HTTPDirect:
		return value, nil
	default:
		return "", errors.New("服务接入方式无效")
	}
}

// ParseHTTPURL accepts a complete initial page address. The runtime proxies its
// origin; the path, query and fragment are used only when opening the service.
func ParseHTTPURL(raw string) (*url.URL, error) {
	invalid := errors.New("请输入有效的 HTTP 或 HTTPS 地址，不支持嵌入账号密码")
	if raw == "" || len(raw) > 2048 || strings.Contains(raw, "\\") || strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return nil, invalid
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || u.Host == "" || u.Hostname() == "" || u.User != nil || strings.HasSuffix(u.Host, ":") {
		return nil, invalid
	}
	host := u.Hostname()
	if strings.HasPrefix(u.Host, "[") && net.ParseIP(host) == nil {
		return nil, invalid
	}
	if net.ParseIP(host) == nil {
		ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(host, "."))
		if err != nil || ascii == "" || len(ascii) > 253 {
			return nil, invalid
		}
		for _, label := range strings.Split(ascii, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, invalid
			}
			for _, r := range label {
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-') {
					return nil, invalid
				}
			}
		}
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("HTTP URL 端口必须是 1–65535 的整数")
		}
	}
	if strings.HasPrefix(u.Path, "//") || strings.ContainsAny(u.Path, "\\\r\n") || strings.HasPrefix(u.Path, "/_portal/") || len(OpenPath(u)) > 1000 {
		return nil, errors.New("HTTP URL 打开路径无效或过长，不能使用 /_portal/ 保留路径")
	}
	return u, nil
}

// OpenPath retains escaping and application query/hash routes in a local redirect.
func OpenPath(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" || u.ForceQuery {
		path += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		path += "#" + u.EscapedFragment()
	}
	return path
}
