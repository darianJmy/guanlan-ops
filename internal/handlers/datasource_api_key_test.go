package handlers

import (
	"guanlan/internal/models"
	"net/http/httptest"
	"testing"
)

func TestElasticsearchAPIKeyAuthentication(t *testing.T) {
	ds := models.Datasource{Type: "elasticsearch", URL: "https://es.example", AuthType: "api_key", AuthConfig: `{"token":"encoded-key"}`, Config: adapterConfig("elasticsearch")}
	if err := validateDatasourceConnection(ds); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "https://es.example/_search", nil)
	setAuthHeader(req, ds.AuthType, ds.AuthConfig)
	if req.Header.Get("Authorization") != "ApiKey encoded-key" {
		t.Fatal("incorrect authorization header")
	}
	ds.AuthConfig = `{"token":""}`
	if validateDatasourceConnection(ds) == nil {
		t.Fatal("empty API key accepted")
	}
	ds.AuthConfig = `{"token":"encoded-key"}`
	ds.Type = "loki"
	if validateDatasourceConnection(ds) == nil {
		t.Fatal("API key incorrectly enabled for Loki")
	}
}

func TestClickHouseAllowsEmptyPassword(t *testing.T) {
	ds := models.Datasource{Type: "clickhouse", URL: "http://localhost:8123", AuthType: "basic", AuthConfig: `{"username":"default","password":""}`, Config: `{"database":"default"}`}
	if err := validateDatasourceConnection(ds); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "http://localhost:8123", nil)
	setAuthHeader(req, ds.AuthType, ds.AuthConfig)
	user, password, ok := req.BasicAuth()
	if !ok || user != "default" || password != "" {
		t.Fatal("incorrect basic credentials")
	}
	ds.Type = "prometheus"
	if validateDatasourceConnection(ds) == nil {
		t.Fatal("empty password accepted for other datasource")
	}
}
