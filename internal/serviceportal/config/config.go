package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Bind          string
	PublicURL     string
	MinPort       int
	MaxPort       int
	GuanlanURL    string
	InternalToken string
	TCPGrantTTL   time.Duration
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Load() (Config, error) {
	c := Config{
		Bind:          env("PORTAL_BIND", "127.0.0.1:8081"),
		PublicURL:     env("PORTAL_PUBLIC_URL", "http://127.0.0.1:8081"),
		GuanlanURL:    strings.TrimRight(os.Getenv("PORTAL_GUANLAN_URL"), "/"),
		InternalToken: strings.TrimSpace(os.Getenv("SERVICE_PORTAL_INTERNAL_TOKEN")),
	}
	if c.InternalToken == "" {
		return c, fmt.Errorf("SERVICE_PORTAL_INTERNAL_TOKEN 必须配置")
	}
	identityURL, err := url.Parse(c.GuanlanURL)
	if err != nil || (identityURL.Scheme != "http" && identityURL.Scheme != "https") || identityURL.Hostname() == "" || identityURL.User != nil || identityURL.Path != "" || identityURL.RawQuery != "" || identityURL.Fragment != "" {
		return c, fmt.Errorf("PORTAL_GUANLAN_URL 必须是 观澜 后端的 http(s) 根地址")
	}
	c.MinPort, err = strconv.Atoi(env("PORTAL_MIN_PORT", "18000"))
	if err != nil {
		return c, err
	}
	c.MaxPort, err = strconv.Atoi(env("PORTAL_MAX_PORT", "18999"))
	if err != nil {
		return c, err
	}
	if c.MinPort < 1024 || c.MaxPort > 65535 || c.MinPort > c.MaxPort {
		return c, fmt.Errorf("业务端口范围无效")
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Port() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return c, fmt.Errorf("PORTAL_PUBLIC_URL 必须是 http(s)://外部可访问主机:管理端口")
	}
	publicPort, err := strconv.Atoi(u.Port())
	if err != nil || publicPort < 1 || publicPort > 65535 {
		return c, fmt.Errorf("PORTAL_PUBLIC_URL 管理端口无效")
	}
	_, bindPort, err := net.SplitHostPort(c.Bind)
	if err != nil {
		return c, fmt.Errorf("PORTAL_BIND 必须是 host:port")
	}
	port, err := strconv.Atoi(bindPort)
	if err != nil || port < 1 || port > 65535 {
		return c, fmt.Errorf("PORTAL_BIND 端口无效")
	}
	if port >= c.MinPort && port <= c.MaxPort {
		return c, fmt.Errorf("管理端口不能位于业务端口范围内")
	}
	c.TCPGrantTTL, err = time.ParseDuration(env("PORTAL_TCP_GRANT_TTL", "15m"))
	if err != nil || c.TCPGrantTTL <= 0 {
		return c, fmt.Errorf("PORTAL_TCP_GRANT_TTL 必须是正的时长，例如 15m")
	}
	return c, nil
}
