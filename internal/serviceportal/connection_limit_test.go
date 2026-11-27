package serviceportal

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycleLimitsConcurrentKubernetesConnectionsPerCluster(t *testing.T) {
	const canceledService = "queued-canceled"
	const nextService = "queued-next"
	initialRequests := make(chan struct{}, maxConcurrentConnectsPerCluster)
	queuedRequests := make(chan string, 2)
	release := make(chan struct{}, 1)
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()
	h := newLifecycleHarness(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := path.Base(r.URL.Path)
		switch {
		case name == "portforward":
			initialRequests <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		case name == "pods":
			name := strings.TrimPrefix(r.URL.Query().Get("labelSelector"), "app=")
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":%q},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}]}`, name)
			return
		case strings.HasPrefix(name, "initial-"):
			fmt.Fprintf(w, `{"apiVersion":"v1","kind":"Service","metadata":{"name":%q},"spec":{"selector":{"app":%q},"ports":[{"port":8848}]}}`, name, name)
			return
		default:
			queuedRequests <- name
		}
		http.NotFound(w, r)
	}))
	otherRequests := make(chan struct{}, 1)
	otherAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherRequests <- struct{}{}
		http.NotFound(w, r)
	}))
	t.Cleanup(otherAPI.Close)

	state := h.durable.Snapshot()
	otherCluster := state.Clusters[0]
	otherCluster.ID = "other-cluster"
	otherCluster.Kubeconfig = fmt.Sprintf(`{"apiVersion":"v1","kind":"Config","clusters":[{"name":"test","cluster":{"server":%q}}],"contexts":[{"name":"test","context":{"cluster":"test"}}],"current-context":"test"}`, otherAPI.URL)
	state.Clusters = append(state.Clusters, otherCluster)
	base := state.Resources[0]
	state.Resources = nil
	addService := func(id, clusterID string) {
		resource := base
		resource.ID, resource.Name, resource.Service, resource.ClusterID = id, id, id, clusterID
		resource.PublicPort += len(state.Resources)
		state.Resources = append(state.Resources, resource)
	}
	syncState := func() {
		t.Helper()
		h.durable.Load(state)
		if err := h.app.reconcileAt(context.Background(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for i := range maxConcurrentConnectsPerCluster {
		addService(fmt.Sprintf("initial-%d", i), base.ClusterID)
	}
	syncState()
	for range maxConcurrentConnectsPerCluster {
		select {
		case <-initialRequests:
		case <-time.After(3 * time.Second):
			t.Fatal("first cluster did not reach its connection limit")
		}
	}

	canceledIndex := len(state.Resources)
	addService(canceledService, base.ClusterID)
	addService(nextService, base.ClusterID)
	addService("other", otherCluster.ID)
	syncState()
	select {
	case <-otherRequests:
	case service := <-queuedRequests:
		t.Fatalf("%s started while its cluster was at the connection limit", service)
	case <-time.After(3 * time.Second):
		t.Fatal("the first cluster blocked the second cluster")
	}
	select {
	case service := <-queuedRequests:
		t.Fatalf("%s started before a connection slot was released", service)
	case <-time.After(100 * time.Millisecond):
	}

	state.Resources[canceledIndex].Desired = false
	syncState()
	release <- struct{}{}
	select {
	case service := <-queuedRequests:
		if service != nextService {
			t.Fatalf("unexpected service after freeing a connection slot: %s", service)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the queued service did not start after a slot became available")
	}
	releaseAll()
	awaitLifecycle(t, func() bool {
		for _, resource := range h.app.db.Snapshot().Resources {
			if resource.Desired && resource.Status != "retrying" {
				return false
			}
		}
		return true
	}, "the connection attempts did not finish")
	select {
	case service := <-queuedRequests:
		t.Fatalf("unexpected Kubernetes request for %s after releasing the remaining slots", service)
	default:
	}
	resource, _ := h.app.db.Snapshot().Resource(canceledService)
	if resource.Desired || resource.Status != "stopped" {
		t.Fatalf("disabled queued service has unexpected status: %#v", resource)
	}
}
