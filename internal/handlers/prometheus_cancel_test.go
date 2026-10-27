package handlers

import (
	"context"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPromQueryCancelsUpstreamRequest(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
			close(stopped)
		case <-time.After(3 * time.Second):
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := doPromHTTPContext(ctx, models.Datasource{URL: server.URL}, server.URL+"/api/v1/query?query=up")
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled request succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not cancel")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not reach upstream")
	}
}
