package handlers

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func k8sTestJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

func k8sTestDatasource(server *httptest.Server) models.Datasource {
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))
	return models.Datasource{Type: "k8s", Name: "cluster", URL: server.URL + "/cluster/prod", AuthType: "bearer", AuthConfig: `{"token":"test-secret"}`, Config: k8sTestJSON(map[string]any{"kubernetes": kubernetesConnectionConfig{Namespace: "default", CACertificate: ca}}), Status: 1}
}

func TestKubernetesConnection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		body    string
		version string
		wantOK  bool
		message string
	}{
		{name: "empty namespace succeeds", code: 200, body: `{"kind":"PodList","apiVersion":"v1","items":[]}`, wantOK: true, message: "default"},
		{name: "invalid token", code: 401, message: "401"},
		{name: "RBAC denied", code: 403, message: "403"},
		{name: "missing namespace", code: 404, message: "404"},
		{name: "redirect", code: 302, message: "302"},
		{name: "html endpoint", code: 200, body: `<html>console</html>`, message: "API 响应"},
		{name: "wrong resource", code: 200, body: `{"kind":"NodeList","apiVersion":"v1","items":[]}`, message: "Pod 列表"},
		{name: "missing items", code: 200, body: `{"kind":"PodList","apiVersion":"v1"}`, message: "Pod 列表"},
		{name: "bad version", version: `{"status":"ok"}`, message: "版本"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Error("token not applied")
				}
				if r.URL.Path == "/cluster/prod/version" {
					if tc.version != "" {
						fmt.Fprint(w, tc.version)
					} else {
						fmt.Fprint(w, `{"gitVersion":"v1.34.0"}`)
					}
					return
				}
				if r.URL.Path != "/cluster/prod/api/v1/namespaces/default/pods" || r.URL.Query().Get("limit") != "1" {
					t.Errorf("wrong resource request: %s", r.URL)
				}
				w.Header().Set("Location", "/redirect-target")
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			ok, msg := testConnectionForDatasource(k8sTestDatasource(server))
			if ok != tc.wantOK || !strings.Contains(msg, tc.message) {
				t.Fatalf("got %v %s", ok, msg)
			}
			if strings.Contains(msg, "test-secret") {
				t.Fatal("token leaked")
			}
			expected := 2
			if tc.version != "" {
				expected = 1
			}
			if calls != expected {
				t.Fatalf("unexpected requests: %d", calls)
			}
		})
	}
}

func TestKubernetesTLSAndValidation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/version") {
			fmt.Fprint(w, `{"gitVersion":"v1.34.0"}`)
			return
		}
		if r.URL.Path != "/cluster/prod/api/v1/pods" {
			t.Errorf("all-namespace path: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[]}`)
	}))
	defer server.Close()
	ds := k8sTestDatasource(server)
	for _, mutate := range []func(*models.Datasource){
		func(d *models.Datasource) { d.URL = "http://example.com" },
		func(d *models.Datasource) { d.AuthType = "none" },
		func(d *models.Datasource) { d.AuthType = "basic"; d.AuthConfig = `{"username":"a","password":"b"}` },
		func(d *models.Datasource) { d.AuthConfig = `{"token":""}` },
		func(d *models.Datasource) { d.Config = `{"kubernetes":{"namespace":"../secrets"}}` },
		func(d *models.Datasource) { d.Config = `{"kubernetes":{"namespace":"UPPER"}}` },
		func(d *models.Datasource) { d.Config = `{"kubernetes":{"ca_certificate":"invalid"}}` },
		func(d *models.Datasource) {
			d.AuthType = "client_certificate"
			d.AuthConfig = `{"client_certificate":"invalid","client_key":"secret-key"}`
		},
	} {
		invalid := ds
		mutate(&invalid)
		if err := validateDatasourceConnection(invalid); err == nil {
			t.Fatalf("invalid configuration accepted: %s", invalid.AuthType)
		}
	}
	ds.Config = `{"kubernetes":{}}`
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "证书校验失败") {
		t.Fatalf("untrusted server accepted: %v %s", ok, msg)
	}
	ds.Config = `{"kubernetes":{"insecure_skip_verify":true}}`
	if ok, msg := testConnectionForDatasource(ds); !ok || !strings.Contains(msg, "所有命名空间") {
		t.Fatalf("explicit insecure test: %v %s", ok, msg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ok, _ := testConnectionForDatasourceContext(ctx, k8sTestDatasource(server)); ok {
		t.Fatal("cancelled request succeeded")
	}
}

func TestKubernetesClientCertificate(t *testing.T) {
	// Generate a test certificate valid for both sides of the mutual TLS handshake.
	reference := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := reference.TLS.Certificates[0]
	template, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.KeyUsage |= x509.KeyUsageCertSign
	der, err := x509.CreateCertificate(rand.Reader, template, template, cert.PrivateKey.(crypto.Signer).Public(), cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	cert.Certificate = [][]byte{der}
	cert.Leaf = nil
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}))
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	reference.Close()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(ca))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) == 0 {
			t.Error("client certificate missing")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected token")
		}
		if r.URL.Path == "/version" {
			fmt.Fprint(w, `{"gitVersion":"v1.34.0"}`)
			return
		}
		fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[]}`)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	server.StartTLS()
	defer server.Close()
	ds := models.Datasource{Type: "k8s", URL: server.URL, AuthType: "client_certificate", AuthConfig: k8sTestJSON(DatasourceAuthConfig{ClientCertificate: ca, ClientKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))}), Config: k8sTestJSON(map[string]any{"kubernetes": kubernetesConnectionConfig{CACertificate: ca}})}
	if ok, msg := testConnectionForDatasource(ds); !ok {
		t.Fatal(msg)
	}
	maskKubernetesAuth(&ds)
	if strings.Contains(ds.AuthConfig, "PRIVATE KEY") || strings.Contains(ds.AuthConfig, "CERTIFICATE") || !strings.Contains(ds.AuthConfig, `"has_client_certificate":true`) {
		t.Fatal("certificate credentials not masked")
	}
}

func TestKubernetesDatasourceLifecycle(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "k8s.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/version") {
			fmt.Fprint(w, `{"gitVersion":"v1.34.0"}`)
			return
		}
		fmt.Fprint(w, `{"kind":"PodList","apiVersion":"v1","items":[]}`)
	}))
	defer server.Close()
	ds := k8sTestDatasource(server)
	router := gin.New()
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.GET("/sources/:id", GetDatasourceAuth)
	router.PUT("/sources/:id", UpdateDatasourceWithTunnel)
	router.POST("/sources/:id/test", TestSavedDatasourceConnection)
	router.DELETE("/sources/:id", DeleteDatasource)
	call := func(method, path string, body any) string {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, strings.NewReader(k8sTestJSON(body)))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != 200 || response.Code != 0 {
			t.Fatalf("%s %s: %s", method, path, recorder.Body.String())
		}
		return recorder.Body.String()
	}
	call("POST", "/sources", ds)
	var stored models.Datasource
	db.First(&stored)
	if stored.Category != "container" || stored.Capabilities != "k8s" || isMetricDatasourceType(stored.Type) {
		t.Fatalf("wrong registry assignment: %+v", stored)
	}
	path := fmt.Sprintf("/sources/%d", stored.ID)
	for _, url := range []string{"/sources", path} {
		body := call("GET", url, nil)
		if strings.Contains(body, "test-secret") {
			t.Fatal("credentials leaked")
		}
		if !strings.Contains(body, "has_token") {
			t.Fatal("missing saved credential indicator")
		}
	}
	ds.AuthConfig = ""
	ds.Name = "renamed cluster"
	call("PUT", path, ds)
	call("POST", path+"/test", ds)
	db.First(&stored, stored.ID)
	if !strings.Contains(stored.AuthConfig, "test-secret") {
		t.Fatal("saved token lost")
	}
	call("DELETE", path, nil)
	var count int64
	db.Model(&models.Datasource{}).Count(&count)
	if count != 0 {
		t.Fatal("datasource not deleted")
	}
}
