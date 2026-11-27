package serviceportal

import (
	"context"
	"sync"
	"time"

	"guanlan/internal/serviceportal/kube"
	"guanlan/internal/serviceportal/store"
)

const (
	maxConcurrentConnectsPerCluster = 4
	targetCacheTTL                  = 20 * time.Second
)

type kubernetesConnections struct {
	mu       sync.Mutex
	clusters map[string]*clusterClient
	slots    map[string]*connectionSlots
}

type connectionSlots struct {
	active chan struct{}
	users  int // Includes callers waiting for a slot.
}

// acquire limits setup work per datasource, including across credential rotations.
func (k *kubernetesConnections) acquire(ctx context.Context, clusterID string) (func(), error) {
	k.mu.Lock()
	if k.slots == nil {
		k.slots = make(map[string]*connectionSlots)
	}
	slots := k.slots[clusterID]
	if slots == nil {
		slots = &connectionSlots{active: make(chan struct{}, maxConcurrentConnectsPerCluster)}
		k.slots[clusterID] = slots
	}
	slots.users++
	k.mu.Unlock()

	releaseUser := func() {
		k.mu.Lock()
		defer k.mu.Unlock()
		slots.users--
		if slots.users == 0 {
			delete(k.slots, clusterID)
		}
	}
	select {
	case slots.active <- struct{}{}:
		return func() {
			<-slots.active
			releaseUser()
		}, nil
	case <-ctx.Done():
		releaseUser()
		return nil, ctx.Err()
	}
}

type clusterClient struct {
	config store.Cluster
	*kube.Client
	mu      sync.Mutex
	targets map[targetKey]*cachedTarget
}

type targetKey struct {
	namespace string
	service   string
	port      string
}

type cachedTarget struct {
	kube.Target
	expires time.Time
}

func (k *kubernetesConnections) client(cluster store.Cluster) (*clusterClient, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if cached := k.clusters[cluster.ID]; cached != nil && sameCluster(cached.config, cluster) {
		return cached, nil
	}
	client, err := kube.New([]byte(cluster.Kubeconfig), cluster.Context)
	if err != nil {
		return nil, err
	}
	if k.clusters == nil {
		k.clusters = make(map[string]*clusterClient)
	}
	result := &clusterClient{config: cluster, Client: client, targets: make(map[targetKey]*cachedTarget)}
	k.clusters[cluster.ID] = result
	return result, nil
}

func (k *kubernetesConnections) prune(state store.State) {
	now := time.Now()
	k.mu.Lock()
	defer k.mu.Unlock()
	for id, client := range k.clusters {
		cluster, ok := state.Cluster(id)
		if !ok || !sameCluster(cluster, client.config) {
			delete(k.clusters, id)
			continue
		}
		client.mu.Lock()
		for key, target := range client.targets {
			if !now.Before(target.expires) {
				delete(client.targets, key)
			}
		}
		client.mu.Unlock()
	}
}

func (c *clusterClient) resolve(ctx context.Context, key targetKey) (*cachedTarget, error) {
	c.mu.Lock()
	cached := c.targets[key]
	c.mu.Unlock()
	if cached != nil && time.Now().Before(cached.expires) {
		return cached, nil
	}
	target, err := c.ResolveTarget(ctx, key.namespace, key.service, key.port)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if current := c.targets[key]; current != nil && now.Before(current.expires) {
		return current, nil
	}
	resolved := &cachedTarget{Target: target, expires: now.Add(targetCacheTTL)}
	c.targets[key] = resolved
	return resolved, nil
}

func (c *clusterClient) invalidate(key targetKey, used *cachedTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.targets[key] == used {
		delete(c.targets, key)
	}
}
