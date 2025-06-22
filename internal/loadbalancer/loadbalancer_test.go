package loadbalancer

import (
	"fmt"
	"net/url"
	"sync"
	"testing"
)

func TestRoundRobin(t *testing.T) {
	rr := NewRoundRobin()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: true}
	backend3 := &Backend{URL: parseURL("http://backend3:8080"), Weight: 1, Active: true}

	rr.Add(backend1)
	rr.Add(backend2)
	rr.Add(backend3)

	counts := make(map[string]int)
	for i := 0; i < 300; i++ {
		backend := rr.Next()
		if backend == nil {
			t.Fatal("Expected backend, got nil")
		}
		counts[backend.URL.String()]++
	}

	for backend, count := range counts {
		if count < 90 || count > 110 {
			t.Errorf("Backend %s: expected ~100 requests, got %d", backend, count)
		}
	}
}

func TestRoundRobinWithInactiveBackends(t *testing.T) {
	rr := NewRoundRobin()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: true}
	backend3 := &Backend{URL: parseURL("http://backend3:8080"), Weight: 1, Active: true}

	rr.Add(backend1)
	rr.Add(backend2)
	rr.Add(backend3)

	rr.MarkUnhealthy(backend2)

	for i := 0; i < 10; i++ {
		backend := rr.Next()
		if backend == nil {
			t.Fatal("Expected backend, got nil")
		}
		if backend.URL.String() == "http://backend2:8080" {
			t.Error("Got inactive backend")
		}
	}
}

func TestLeastConnection(t *testing.T) {
	lc := NewLeastConnection()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true, Connections: 5}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: true, Connections: 2}
	backend3 := &Backend{URL: parseURL("http://backend3:8080"), Weight: 1, Active: true, Connections: 8}

	lc.Add(backend1)
	lc.Add(backend2)
	lc.Add(backend3)

	backend := lc.Next()
	if backend == nil {
		t.Fatal("Expected backend, got nil")
	}

	if backend.URL.String() != "http://backend2:8080" {
		t.Errorf("Expected backend2 (least connections), got %s", backend.URL.String())
	}

	if backend.Connections != 3 {
		t.Errorf("Expected connection count to be incremented to 3, got %d", backend.Connections)
	}
}

func TestLoadBalancerConcurrency(t *testing.T) {
	rr := NewRoundRobin()

	for i := 0; i < 10; i++ {
		backend := &Backend{
			URL:    parseURL(fmt.Sprintf("http://backend%d:8080", i)),
			Weight: 1,
			Active: true,
		}
		rr.Add(backend)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				backend := rr.Next()
				if backend == nil {
					t.Error("Got nil backend")
				}
			}
		}()
	}

	wg.Wait()
}

func TestHealthStateChanges(t *testing.T) {
	rr := NewRoundRobin()
	backend := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true}

	rr.Add(backend)

	if b := rr.Next(); b == nil {
		t.Error("Expected healthy backend")
	}

	rr.MarkUnhealthy(backend)

	if b := rr.Next(); b != nil {
		t.Error("Expected no backend when all are unhealthy")
	}

	rr.MarkHealthy(backend)

	if b := rr.Next(); b == nil {
		t.Error("Expected healthy backend after marking healthy")
	}
}

func TestRoundRobinRemoveBackend(t *testing.T) {
	rr := NewRoundRobin()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: true}
	backend3 := &Backend{URL: parseURL("http://backend3:8080"), Weight: 1, Active: true}

	rr.Add(backend1)
	rr.Add(backend2)
	rr.Add(backend3)

	rr.Remove(backend2.URL)

	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		backend := rr.Next()
		if backend == nil {
			t.Fatal("Expected backend, got nil")
		}
		seen[backend.URL.String()] = true

		if backend.URL.String() == "http://backend2:8080" {
			t.Error("Got removed backend")
		}
	}

	if len(seen) != 2 {
		t.Errorf("Expected 2 different backends, got %d", len(seen))
	}
}

func TestLeastConnectionWithEqualConnections(t *testing.T) {
	lc := NewLeastConnection()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: true, Connections: 5}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: true, Connections: 5}
	backend3 := &Backend{URL: parseURL("http://backend3:8080"), Weight: 1, Active: true, Connections: 5}

	lc.Add(backend1)
	lc.Add(backend2)
	lc.Add(backend3)

	backend := lc.Next()
	if backend == nil {
		t.Fatal("Expected backend, got nil")
	}

	if backend.URL.String() != "http://backend1:8080" {
		t.Errorf("Expected backend1 (first with equal connections), got %s", backend.URL.String())
	}
}

func TestLoadBalancerWithNoActiveBackends(t *testing.T) {
	rr := NewRoundRobin()

	backend1 := &Backend{URL: parseURL("http://backend1:8080"), Weight: 1, Active: false}
	backend2 := &Backend{URL: parseURL("http://backend2:8080"), Weight: 1, Active: false}

	rr.Add(backend1)
	rr.Add(backend2)

	backend := rr.Next()
	if backend != nil {
		t.Error("Expected nil when no active backends")
	}
}

func TestLoadBalancerWithNoBackends(t *testing.T) {
	rr := NewRoundRobin()
	lc := NewLeastConnection()

	if backend := rr.Next(); backend != nil {
		t.Error("Expected nil from empty round robin")
	}

	if backend := lc.Next(); backend != nil {
		t.Error("Expected nil from empty least connection")
	}
}

func parseURL(urlStr string) *url.URL {
	u, _ := url.Parse(urlStr)
	return u
}
