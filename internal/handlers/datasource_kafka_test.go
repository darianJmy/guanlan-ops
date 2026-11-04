package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/protocol"
	"github.com/segmentio/kafka-go/protocol/apiversions"
	"github.com/segmentio/kafka-go/protocol/metadata"
	"github.com/segmentio/kafka-go/protocol/saslauthenticate"
	"github.com/segmentio/kafka-go/protocol/saslhandshake"
	xdgscram "github.com/xdg-go/scram"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

type kafkaTestBrokerOptions struct {
	modern    bool
	mechanism string
	tls       *tls.Config
	stall     bool
	empty     bool
}

// A wire-protocol broker accepts only version negotiation, SASL and metadata.
func startKafkaTestBroker(t *testing.T, options kafkaTestBrokerOptions) (string, *atomic.Int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if options.tls != nil {
		listener = tls.NewListener(listener, options.tls)
	}
	var connections sync.Map
	var workers sync.WaitGroup
	metadataCalls := new(atomic.Int32)
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Store(conn, true)
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				defer connections.Delete(conn)
				authenticated := options.mechanism == ""
				var conversation *xdgscram.ServerConversation
				if strings.HasPrefix(options.mechanism, "SCRAM") {
					algorithm := xdgscram.SHA256
					if options.mechanism == "SCRAM-SHA-512" {
						algorithm = xdgscram.SHA512
					}
					client, err := algorithm.NewClient("reader", "kafka-secret", "")
					if err != nil {
						t.Error(err)
						return
					}
					credentials := client.GetStoredCredentials(xdgscram.KeyFactors{Salt: "test-salt", Iters: 4096})
					server, err := algorithm.NewServer(func(user string) (xdgscram.StoredCredentials, error) {
						if user != "reader" {
							return xdgscram.StoredCredentials{}, errors.New("unknown user")
						}
						return credentials, nil
					})
					if err != nil {
						t.Error(err)
						return
					}
					conversation = server.NewConversation()
				}
				for {
					version, id, _, request, err := protocol.ReadRequest(conn)
					if err != nil {
						return
					}
					if options.stall {
						continue
					}
					var response protocol.Message
					switch request := request.(type) {
					case *apiversions.Request:
						metadataMin, metadataMax := int16(1), int16(1)
						if options.modern {
							metadataMin, metadataMax = 4, 8
						}
						response = &apiversions.Response{ApiKeys: []apiversions.ApiKeyResponse{{ApiKey: int16(protocol.SaslHandshake), MinVersion: 0, MaxVersion: 1}, {ApiKey: int16(protocol.SaslAuthenticate), MinVersion: 0, MaxVersion: 0}, {ApiKey: int16(protocol.Metadata), MinVersion: metadataMin, MaxVersion: metadataMax}}}
					case *saslhandshake.Request:
						result := &saslhandshake.Response{Mechanisms: []string{options.mechanism}}
						if request.Mechanism != options.mechanism {
							result.ErrorCode = int16(kafka.UnsupportedSASLMechanism)
						}
						response = result
					case *saslauthenticate.Request:
						result := &saslauthenticate.Response{}
						if conversation == nil {
							authenticated = string(request.AuthBytes) == "\x00reader\x00kafka-secret"
							if !authenticated {
								result.ErrorCode = int16(kafka.SASLAuthenticationFailed)
							}
						} else {
							challenge, err := conversation.Step(string(request.AuthBytes))
							result.AuthBytes = []byte(challenge)
							if err != nil {
								result.ErrorCode = int16(kafka.SASLAuthenticationFailed)
							}
							authenticated = conversation.Valid()
						}
						response = result
					case *metadata.Request:
						if options.modern && version < 4 {
							t.Errorf("obsolete metadata API version %d", version)
						}
						if !authenticated {
							t.Error("metadata requested before authentication")
							return
						}
						if len(request.TopicNames) != 0 || request.AllowAutoTopicCreation {
							t.Error("test must not request/create topics")
						}
						metadataCalls.Add(1)
						brokers := []metadata.ResponseBroker{{NodeID: 1, Host: "broker.internal", Port: 9092}}
						if options.empty {
							brokers = nil
						}
						response = &metadata.Response{Brokers: brokers, ControllerID: 1, Topics: []metadata.ResponseTopic{}}
					default:
						t.Errorf("unexpected Kafka API (possible write): %T", request)
						return
					}
					if err := protocol.WriteResponse(conn, version, id, response); err != nil {
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		connections.Range(func(key, value any) bool { key.(net.Conn).Close(); return true })
		workers.Wait()
	})
	return listener.Addr().String(), metadataCalls
}

func kafkaTestDatasource(address, authType string) models.Datasource {
	ds := models.Datasource{Name: "events", Type: "kafka", URL: "kafka://" + address, AuthType: authType, Config: "{}", Status: 1}
	if authType != "none" {
		ds.AuthConfig = `{"username":"reader","password":"kafka-secret"}`
	}
	return ds
}

func TestKafkaProtocolAndAuthentication(t *testing.T) {
	for _, tc := range []struct{ auth, mechanism string }{{"none", ""}, {"sasl_plain", "PLAIN"}, {"scram_sha256", "SCRAM-SHA-256"}, {"scram_sha512", "SCRAM-SHA-512"}} {
		t.Run(tc.auth, func(t *testing.T) {
			address, calls := startKafkaTestBroker(t, kafkaTestBrokerOptions{mechanism: tc.mechanism})
			ok, msg := testConnectionForDatasource(kafkaTestDatasource(address, tc.auth))
			if !ok || !strings.Contains(msg, "1 个 Broker") || calls.Load() != 1 {
				t.Fatalf("metadata test: %v %s (%d)", ok, msg, calls.Load())
			}
		})
	}
}

func TestKafkaFallbackErrorsAndCancellation(t *testing.T) {
	address, calls := startKafkaTestBroker(t, kafkaTestBrokerOptions{mechanism: "PLAIN"})
	ds := kafkaTestDatasource(address, "sasl_plain")
	ds.AuthConfig = `{"username":"reader","password":"wrong-secret"}`
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "认证失败") || strings.Contains(msg, "wrong-secret") {
		t.Fatalf("auth failure: %v %s", ok, msg)
	}
	ds.AuthType = "scram_sha256"
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "不支持") {
		t.Fatalf("mechanism mismatch: %v %s", ok, msg)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthenticated metadata request")
	}
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	badAddress := closed.Addr().String()
	closed.Close()
	ds = kafkaTestDatasource(badAddress, "sasl_plain")
	raw, _ := json.Marshal(map[string]any{"bootstrap_servers": []string{address}})
	ds.Config = string(raw)
	if ok, msg := testConnectionForDatasource(ds); !ok {
		t.Fatal(msg)
	}
	empty, _ := startKafkaTestBroker(t, kafkaTestBrokerOptions{empty: true})
	if ok, _ := testConnectionForDatasource(kafkaTestDatasource(empty, "none")); ok {
		t.Fatal("empty metadata accepted")
	}
	stalled, _ := startKafkaTestBroker(t, kafkaTestBrokerOptions{stall: true})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if ok, _ := testConnectionForDatasourceContext(ctx, kafkaTestDatasource(stalled, "sasl_plain")); ok {
		t.Fatal("stalled handshake succeeded")
	}
	if time.Since(start) > time.Second {
		t.Fatal("SASL cancellation did not close the connection")
	}
}

func TestKafkaTLSAndValidation(t *testing.T) {
	reference := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	cert := reference.TLS.Certificates[0]
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: reference.Certificate().Raw}))
	reference.Close()
	address, _ := startKafkaTestBroker(t, kafkaTestBrokerOptions{tls: &tls.Config{Certificates: []tls.Certificate{cert}}})
	ds := kafkaTestDatasource(address, "none")
	ds.Config = `{"tls":{"enabled":true}}`
	if ok, msg := testConnectionForDatasource(ds); ok || !strings.Contains(msg, "证书校验失败") {
		t.Fatalf("untrusted cert: %v %s", ok, msg)
	}
	raw, _ := json.Marshal(map[string]any{"tls": map[string]any{"enabled": true, "ca_certificate": ca}})
	ds.Config = string(raw)
	if ok, msg := testConnectionForDatasource(ds); !ok {
		t.Fatal(msg)
	}
	for _, tc := range []struct{ url, auth, settings string }{
		{url: "http://broker:9092"}, {url: "kafka://reader:pass@broker:9092"}, {url: "kafka://broker:0"}, {url: "kafka://broker:65536"}, {url: "kafka://broker/topic"}, {url: "kafka://broker?tls=false"},
		{auth: "bearer"}, {settings: `{"bootstrap_servers":["host"]}`}, {settings: `{"bootstrap_servers":["host:0"]}`}, {settings: `{"bootstrap_servers":["user:pass@host:9092"]}`},
		{settings: `{"tls":{"enabled":true,"ca_certificate":"invalid"}}`}, {settings: `{"tls":{"ca_certificate":"invalid"}}`},
	} {
		invalid := kafkaTestDatasource("broker:9092", "none")
		if tc.url != "" {
			invalid.URL = tc.url
		}
		if tc.auth != "" {
			invalid.AuthType = tc.auth
		}
		if tc.settings != "" {
			invalid.Config = tc.settings
		}
		if err := validateDatasourceConnection(invalid); err == nil {
			t.Fatalf("invalid configuration accepted: %+v", tc)
		}
	}
	ds = kafkaTestDatasource("[::1]", "none")
	ds.Config = `{"bootstrap_servers":["[::1]:9092","host:9093"]}`
	brokers, _, err := kafkaConnectionOptions(ds)
	if err != nil || len(brokers) != 2 || brokers[0] != "[::1]:9092" {
		t.Fatalf("deduplication/IPv6: %v %v", brokers, err)
	}
}

func TestKafkaDatasourceLifecycle(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "kafka.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cleanup(); database.DB = previous }()
	address, _ := startKafkaTestBroker(t, kafkaTestBrokerOptions{mechanism: "PLAIN"})
	router := gin.New()
	router.POST("/sources", CreateDatasourceWithTunnel)
	router.GET("/sources", ListDatasourcesWithTunnel)
	router.GET("/sources/:id", GetDatasourceAuth)
	router.PUT("/sources/:id", UpdateDatasourceWithTunnel)
	router.POST("/sources/:id/test", TestSavedDatasourceConnection)
	router.DELETE("/sources/:id", DeleteDatasource)
	call := func(method, path string, body any) string {
		raw, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || response.Code != 0 {
			t.Fatalf("%s %s: %s", method, path, w.Body.String())
		}
		return w.Body.String()
	}
	ds := kafkaTestDatasource(address, "sasl_plain")
	call("POST", "/sources", ds)
	var saved models.Datasource
	db.First(&saved)
	if saved.Category != "messaging" || saved.Capabilities != "kafka" || isMetricDatasourceType(saved.Type) {
		t.Fatal("Kafka type not registered correctly")
	}
	path := fmt.Sprintf("/sources/%d", saved.ID)
	for _, url := range []string{"/sources", path} {
		body := call("GET", url, nil)
		if strings.Contains(body, "kafka-secret") || !strings.Contains(body, "has_password") {
			t.Fatal("password exposed or configured marker missing")
		}
	}
	ds.AuthConfig = ""
	ds.Name = "renamed"
	call("PUT", path, ds)
	call("POST", path+"/test", ds)
	db.First(&saved, saved.ID)
	if !strings.Contains(saved.AuthConfig, "kafka-secret") {
		t.Fatal("saved password lost")
	}
	ds.AuthType = "none"
	call("PUT", path, ds)
	db.First(&saved, saved.ID)
	if saved.AuthConfig != "" {
		t.Fatal("credentials not cleared")
	}
	call("DELETE", path, nil)
}

func TestKafkaModernMetadataVersion(t *testing.T) {
	address, calls := startKafkaTestBroker(t, kafkaTestBrokerOptions{modern: true})
	if ok, msg := testConnectionForDatasource(kafkaTestDatasource(address, "none")); !ok {
		t.Fatal(msg)
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected metadata calls: %d", calls.Load())
	}
}
