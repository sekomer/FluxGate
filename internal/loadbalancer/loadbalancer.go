package loadbalancer

import (
	"net/url"
	"sync"
	"sync/atomic"
)

type Backend struct {
	URL         *url.URL
	Weight      int
	Active      bool
	Connections int64
}

type LoadBalancer interface {
	Add(backend *Backend)
	Remove(url *url.URL)
	Next() *Backend
	MarkHealthy(backend *Backend)
	MarkUnhealthy(backend *Backend)
}

type RoundRobin struct {
	backends []*Backend
	current  uint64
	mu       sync.RWMutex
}

func NewRoundRobin() LoadBalancer {
	return &RoundRobin{
		backends: make([]*Backend, 0),
	}
}

func (rr *RoundRobin) Add(backend *Backend) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	rr.backends = append(rr.backends, backend)
}

func (rr *RoundRobin) Remove(url *url.URL) {
	rr.mu.Lock()
	defer rr.mu.Unlock()

	for i, b := range rr.backends {
		if b.URL.String() == url.String() {
			rr.backends = append(rr.backends[:i], rr.backends[i+1:]...)
			return
		}
	}
}

func (rr *RoundRobin) Next() *Backend {
	rr.mu.RLock()
	defer rr.mu.RUnlock()

	if len(rr.backends) == 0 {
		return nil
	}

	activeBackends := make([]*Backend, 0)
	for _, b := range rr.backends {
		if b.Active {
			activeBackends = append(activeBackends, b)
		}
	}

	if len(activeBackends) == 0 {
		return nil
	}

	n := atomic.AddUint64(&rr.current, 1)
	return activeBackends[n%uint64(len(activeBackends))]
}

func (rr *RoundRobin) MarkHealthy(backend *Backend) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	backend.Active = true
}

func (rr *RoundRobin) MarkUnhealthy(backend *Backend) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	backend.Active = false
}

type LeastConnection struct {
	backends []*Backend
	mu       sync.RWMutex
}

func NewLeastConnection() LoadBalancer {
	return &LeastConnection{
		backends: make([]*Backend, 0),
	}
}

func (lc *LeastConnection) Add(backend *Backend) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	backend.Active = true
	lc.backends = append(lc.backends, backend)
}

func (lc *LeastConnection) Remove(url *url.URL) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	for i, b := range lc.backends {
		if b.URL.String() == url.String() {
			lc.backends = append(lc.backends[:i], lc.backends[i+1:]...)
			return
		}
	}
}

func (lc *LeastConnection) Next() *Backend {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var selected *Backend
	minConnections := int64(^uint64(0) >> 1)

	for _, b := range lc.backends {
		if b.Active && b.Connections < minConnections {
			selected = b
			minConnections = b.Connections
		}
	}

	if selected != nil {
		atomic.AddInt64(&selected.Connections, 1)
	}

	return selected
}

func (lc *LeastConnection) MarkHealthy(backend *Backend) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	backend.Active = true
}

func (lc *LeastConnection) MarkUnhealthy(backend *Backend) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	backend.Active = false
}

func (lc *LeastConnection) ReleaseConnection(backend *Backend) {
	atomic.AddInt64(&backend.Connections, -1)
}
