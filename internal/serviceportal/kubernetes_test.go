package serviceportal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"guanlan/internal/serviceportal/store"
)

func TestKubernetesTargetCacheExpiresAndRefreshesOnFailureOrCredentialChange(t *testing.T) {
	var listCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/middleware/services/nacos":
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"Service","spec":{"selector":{"app":"nacos"},"ports":[{"port":8848}]}}`)
		case "/api/v1/namespaces/middleware/pods":
			pod := fmt.Sprintf("nacos-%d", listCalls.Add(1))
			if r.Header.Get("Authorization") == "Bearer rotated-token" {
				pod = "nacos-rotated"
			}
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":%q},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`, pod)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	cluster := store.Cluster{
		ID:         "cluster",
		Context:    "test",
		Kubeconfig: fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q,"insecure-skip-tls-verify":true}}],"users":[{"name":"test","user":{"token":"initial-token"}}],"contexts":[{"name":"test","context":{"cluster":"test","user":"test"}}]}`, server.URL),
	}
	var connections kubernetesConnections
	key := targetKey{namespace: "middleware", service: "nacos", port: "8848"}
	getClient := func(cluster store.Cluster) *clusterClient {
		t.Helper()
		client, err := connections.client(cluster)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	resolvePod := func(client *clusterClient, want string) *cachedTarget {
		t.Helper()
		target, err := client.resolve(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if target.Pod != want || target.Port != 8848 {
			t.Fatalf("resolved %#v, want %s:8848", target.Target, want)
		}
		return target
	}

	client := getClient(cluster)
	first := resolvePod(client, "nacos-1")
	if getClient(cluster) != client || resolvePod(client, "nacos-1") != first {
		t.Fatal("unchanged configuration did not reuse the client and target")
	}
	client.mu.Lock()
	client.targets[key] = &cachedTarget{Target: first.Target, expires: time.Now().Add(-time.Second)}
	client.mu.Unlock()
	second := resolvePod(client, "nacos-2")
	client.invalidate(key, first)
	resolvePod(client, "nacos-2")
	client.invalidate(key, second)
	resolvePod(client, "nacos-3")

	cluster.Kubeconfig = strings.ReplaceAll(cluster.Kubeconfig, "initial-token", "rotated-token")
	connections.prune(store.State{Clusters: []store.Cluster{cluster}})
	rotated := getClient(cluster)
	if rotated == client {
		t.Fatal("credential rotation reused the old Kubernetes client")
	}
	resolvePod(rotated, "nacos-rotated")
	if got := listCalls.Load(); got != 4 {
		t.Fatalf("listed Pods %d times, want 4", got)
	}
}
