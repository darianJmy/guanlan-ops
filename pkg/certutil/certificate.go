package certutil

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"time"
)

type Info struct {
	Domains      []string
	Issuer       string
	Subject      string
	SerialNumber string
	Fingerprint  string
	NotBefore    time.Time
	NotAfter     time.Time
}

func ParsePair(certPEM, keyPEM string) (Info, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(certPEM)))
	if block == nil || block.Type != "CERTIFICATE" {
		return Info{}, errors.New("公钥证书格式错误")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return Info{}, errors.New("解析公钥证书失败，请确认上传的是 PEM 格式证书")
	}
	keyBlock, _ := pem.Decode([]byte(strings.TrimSpace(keyPEM)))
	if keyBlock == nil {
		return Info{}, errors.New("私钥格式错误")
	}
	key, err := privatePublicKey(keyBlock.Bytes)
	if err != nil {
		return Info{}, err
	}
	if !publicKeyMatches(cert.PublicKey, key) {
		return Info{}, errors.New("证书和私钥不匹配")
	}
	return FromX509(cert), nil
}
func privatePublicKey(der []byte) (interface{}, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return &key.PublicKey, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch k := key.(type) {
		case *rsa.PrivateKey:
			return &k.PublicKey, nil
		case *ecdsa.PrivateKey:
			return &k.PublicKey, nil
		case ed25519.PrivateKey:
			return k.Public(), nil
		default:
			return nil, errors.New("不支持的 PKCS8 私钥类型")
		}
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return &key.PublicKey, nil
	}
	return nil, errors.New("解析私钥失败，请确认上传的是 RSA、EC 或 Ed25519 PEM 私钥")
}
func publicKeyMatches(certKey, privatePublicKey interface{}) bool {
	a, errA := x509.MarshalPKIXPublicKey(certKey)
	b, errB := x509.MarshalPKIXPublicKey(privatePublicKey)
	return errA == nil && errB == nil && string(a) == string(b)
}
func FromX509(cert *x509.Certificate) Info {
	sum := sha256.Sum256(cert.Raw)
	domains := append([]string{}, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		domains = append(domains, ip.String())
	}
	if len(domains) == 0 && cert.Subject.CommonName != "" {
		domains = append(domains, cert.Subject.CommonName)
	}
	return Info{
		Domains:      NormalizeDomains(domains),
		Issuer:       cert.Issuer.String(),
		Subject:      cert.Subject.String(),
		SerialNumber: cert.SerialNumber.String(),
		Fingerprint:  strings.ToUpper(hex.EncodeToString(sum[:])),
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
	}
}
func NormalizeDomains(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' '
		}) {
			domain := strings.TrimSpace(part)
			if domain != "" && !seen[domain] {
				seen[domain] = true
				result = append(result, domain)
			}
		}
	}
	return result
}

// Fetch returns the leaf certificate using caller-selected TLS verification and timeout.
func Fetch(address string, tlsConfig *tls.Config, timeout time.Duration) (Info, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", address, tlsConfig)
	if err != nil {
		return Info{}, err
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Info{}, errors.New("未获取到线上证书")
	}
	return FromX509(certs[0]), nil
}
