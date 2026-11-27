// Package store contains the service portal's in-memory runtime state.
// Durable service definitions and Kubernetes credentials belong to 观澜.
package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
)

type User struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Admin        bool            `json:"admin"`
	Enabled      bool            `json:"enabled"`
	FolderAccess map[uint]string `json:"folderAccess,omitempty"`
}

type Cluster struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Kubeconfig string `json:"kubeconfig,omitempty"`
}

type Resource struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	TargetURL  string `json:"targetURL,omitempty"`
	ClusterID  string `json:"clusterId"`
	Namespace  string `json:"namespace"`
	Service    string `json:"service"`
	Port       string `json:"port"`
	PublicPort int    `json:"publicPort"`
	Path       string `json:"path"`
	Desired    bool   `json:"desired"`
	OnDemand   bool   `json:"onDemand"`
	Status     string `json:"status,omitempty"`
	LastError  string `json:"lastError,omitempty"`
	Recovered  string `json:"recovered,omitempty"`
	FolderID   uint   `json:"folder_id"`
}

type State struct {
	Clusters  []Cluster           `json:"clusters"`
	Resources []Resource          `json:"resources"`
	Grants    map[string][]string `json:"grants"`
}

type Store struct {
	mu    sync.Mutex
	state State
}

func ID() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func New() *Store { return &Store{state: normalize(State{})} }

func normalize(st State) State {
	if st.Clusters == nil {
		st.Clusters = []Cluster{}
	}
	if st.Resources == nil {
		st.Resources = []Resource{}
	}
	if st.Grants == nil {
		st.Grants = map[string][]string{}
	}
	return st
}

func clone(st State) State {
	encoded, _ := json.Marshal(st)
	var result State
	_ = json.Unmarshal(encoded, &result)
	return normalize(result)
}

func (s *Store) Load(st State)   { s.mu.Lock(); defer s.mu.Unlock(); s.state = clone(st) }
func (s *Store) Snapshot() State { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.state) }
func (s *Store) Update(fn func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.state)
	if err := fn(&next); err != nil {
		return err
	}
	s.state = normalize(next)
	return nil
}

func (s State) Cluster(id string) (Cluster, bool) {
	for _, item := range s.Clusters {
		if item.ID == id {
			return item, true
		}
	}
	return Cluster{}, false
}
func (s State) Resource(id string) (Resource, bool) {
	for _, item := range s.Resources {
		if item.ID == id {
			return item, true
		}
	}
	return Resource{}, false
}
func (s State) Allowed(u User, id string) bool {
	return AccessRank(s.AccessLevel(u, id)) >= AccessRank("view")
}
func AccessRank(level string) int {
	switch level {
	case "view":
		return 1
	case "edit":
		return 2
	case "manage":
		return 3
	}
	return 0
}
func (s State) AccessLevel(u User, id string) string {
	if !u.Enabled || !strings.HasPrefix(u.ID, "guanlan:") {
		return "none"
	}
	resource, exists := s.Resource(id)
	if !exists {
		return "none"
	}
	if resource.FolderID != 0 {
		level := u.FolderAccess[resource.FolderID]
		if AccessRank(level) == 0 {
			return "none"
		}
		return level
	}
	if u.Admin {
		return "manage"
	}
	for _, resourceID := range s.Grants[u.ID] {
		if resourceID == id {
			return "view"
		}
	}
	return "none"
}
