package certutil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParsePairAndMismatch(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(123), Subject: pkix.Name{CommonName: "example.test"}, DNSNames: []string{"example.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	info, err := ParsePair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if info.SerialNumber != "123" || len(info.Fingerprint) != 64 || len(info.Domains) != 1 || info.Domains[0] != "example.test" {
		t.Fatalf("metadata: %+v", info)
	}
	_, other, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := x509.MarshalPKCS8PrivateKey(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePair(certPEM, string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: wrong}))); err == nil {
		t.Fatal("mismatched key accepted")
	}
	if _, err := ParsePair("bad PEM", keyPEM); err == nil {
		t.Fatal("invalid certificate accepted")
	}
}
func TestFetchUsesCallerVerificationPolicy(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	info, err := Fetch(server.Listener.Addr().String(), &tls.Config{InsecureSkipVerify: true}, time.Second)
	if err != nil || info.Fingerprint == "" {
		t.Fatalf("fetch: %+v %v", info, err)
	}
	if _, err := Fetch(server.Listener.Addr().String(), &tls.Config{}, time.Second); err == nil {
		t.Fatal("caller verification policy ignored")
	}
}
