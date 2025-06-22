package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/fluxgate/fluxgate/internal/loadbalancer"
	"github.com/fluxgate/fluxgate/internal/metrics"
)

type HealthChecker struct {
	client    *http.Client
	interval  time.Duration
	timeout   time.Duration
	endpoints map[string]*HealthEndpoint
}

type HealthEndpoint struct {
	URL           *url.URL
	Path          string
	ExpectedCode  int
	LoadBalancer  loadbalancer.LoadBalancer
	Backend       *loadbalancer.Backend
}

func NewHealthChecker(interval, timeout time.Duration) *HealthChecker {
	return &HealthChecker{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		interval:  interval,
		timeout:   timeout,
		endpoints: make(map[string]*HealthEndpoint),
	}
}

func (h *HealthChecker) AddEndpoint(backend *loadbalancer.Backend, lb loadbalancer.LoadBalancer, healthPath string) {
	endpoint := &HealthEndpoint{
		URL:          backend.URL,
		Path:         healthPath,
		ExpectedCode: http.StatusOK,
		LoadBalancer: lb,
		Backend:      backend,
	}
	
	h.endpoints[backend.URL.String()] = endpoint
}

func (h *HealthChecker) RemoveEndpoint(backendURL string) {
	delete(h.endpoints, backendURL)
}

func (h *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	h.checkAll()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-ctx.Done():
			return
		}
	}
}

func (h *HealthChecker) checkAll() {
	for _, endpoint := range h.endpoints {
		go h.check(endpoint)
	}
}

func (h *HealthChecker) check(endpoint *HealthEndpoint) {
	healthURL := fmt.Sprintf("%s%s", endpoint.URL.String(), endpoint.Path)
	
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		h.markUnhealthy(endpoint)
		return
	}
	
	resp, err := h.client.Do(req)
	if err != nil {
		h.markUnhealthy(endpoint)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == endpoint.ExpectedCode {
		h.markHealthy(endpoint)
	} else {
		h.markUnhealthy(endpoint)
	}
}

func (h *HealthChecker) markHealthy(endpoint *HealthEndpoint) {
	if !endpoint.Backend.Active {
		log.Printf("Backend %s is now healthy", endpoint.URL.String())
		endpoint.LoadBalancer.MarkHealthy(endpoint.Backend)
		metrics.BackendHealth.WithLabelValues(endpoint.URL.String()).Set(1)
	}
}

func (h *HealthChecker) markUnhealthy(endpoint *HealthEndpoint) {
	if endpoint.Backend.Active {
		log.Printf("Backend %s is now unhealthy", endpoint.URL.String())
		endpoint.LoadBalancer.MarkUnhealthy(endpoint.Backend)
		metrics.BackendHealth.WithLabelValues(endpoint.URL.String()).Set(0)
	}
}