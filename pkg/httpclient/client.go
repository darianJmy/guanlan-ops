package httpclient

import (
	"context"
	"crypto/tls"
	"maps"
	"net"
	"net/http"
	"time"
)

type Config struct {
	Timeout     time.Duration
	TLSConfig   *tls.Config
	DialContext func(context.Context, string, string) (net.Conn, error)
	Auth        Auth
	Headers     map[string]string
}

// New owns a transport and returns its cleanup function. TLS policy and timeout
// are supplied by the caller; zero timeout means no client-level timeout.
func New(cfg Config) (*http.Client, func()) {
	transport := &http.Transport{DialContext: cfg.DialContext}
	if cfg.TLSConfig != nil {
		transport.TLSClientConfig = cfg.TLSConfig.Clone()
	}
	client := &http.Client{Timeout: cfg.Timeout, Transport: Transport{Base: transport, Auth: cfg.Auth, Headers: maps.Clone(cfg.Headers)}}
	return client, transport.CloseIdleConnections
}
