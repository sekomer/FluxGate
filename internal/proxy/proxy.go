package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fluxgate/fluxgate/internal/config"
	"github.com/fluxgate/fluxgate/internal/discovery"
	"github.com/fluxgate/fluxgate/internal/loadbalancer"
	"github.com/fluxgate/fluxgate/internal/metrics"
	"github.com/fluxgate/fluxgate/pkg/router"
)

type Server struct {
	config         *config.Config
	discovery      *discovery.Service
	router         *router.Router
	loadBalancers  map[string]loadbalancer.LoadBalancer
	reverseProxies map[string]*httputil.ReverseProxy
	transport      *http.Transport
	tlsManager     *TLSManager
	mu             sync.RWMutex
	port           int
}

var reservedServiceNames = map[string]bool{
	"api":       true,
	"_fluxgate": true,
	"health":    true,
	"metrics":   true,
	"v1":        true,
}

func isReservedServiceName(name string) bool {
	return reservedServiceNames[name] || strings.HasPrefix(name, "_")
}

func New(cfg *config.Config, discovery *discovery.Service, port int) (*Server, error) {
	tlsManager, err := NewTLSManager(cfg.TLS)
	if err != nil {
		return nil, fmt.Errorf("creating TLS manager: %w", err)
	}

	s := &Server{
		config:         cfg,
		discovery:      discovery,
		router:         router.New(),
		loadBalancers:  make(map[string]loadbalancer.LoadBalancer),
		reverseProxies: make(map[string]*httputil.ReverseProxy),
		port:           port,
		tlsManager:     tlsManager,
		transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true,
			DialContext: (&net.Dialer{
				Timeout:   cfg.Timeouts.Read,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	return s, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.subscribeToServiceChanges()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	// Management API
	mux.HandleFunc("/api/v1/health", s.handleHealthCheck)
	mux.HandleFunc("/api/v1/services", s.handleServiceList)
	mux.HandleFunc("/api/v1/services/register", s.handleServiceRegistration)
	mux.HandleFunc("/api/v1/services/deregister", s.handleServiceDeregistration)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  s.config.Timeouts.Read,
		WriteTimeout: s.config.Timeouts.Write,
		IdleTimeout:  s.config.Timeouts.Idle,
		TLSConfig:    s.tlsManager.GetTLSConfig(),
	}

	s.tlsManager.Subscribe(func(tlsConfig *tls.Config) {
		srv.TLSConfig = tlsConfig
	})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if s.tlsManager.IsEnabled() {
		log.Printf("Starting HTTPS proxy server on port %d", s.port)
		return srv.ListenAndServeTLS("", "")
	}

	log.Printf("Starting HTTP proxy server on port %d", s.port)
	return srv.ListenAndServe()
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	route := s.router.Match(r)
	if route == nil {
		metrics.RequestsTotal.WithLabelValues("unknown", r.Method, "404").Inc()
		http.Error(w, "No route found", http.StatusNotFound)
		return
	}

	s.mu.RLock()
	lb, exists := s.loadBalancers[route.ServiceName]
	s.mu.RUnlock()

	if !exists {
		metrics.RequestsTotal.WithLabelValues(route.ServiceName, r.Method, "500").Inc()
		http.Error(w, "Service not configured", http.StatusInternalServerError)
		return
	}

	backend := lb.Next()
	if backend == nil {
		metrics.RequestsTotal.WithLabelValues(route.ServiceName, r.Method, "503").Inc()
		http.Error(w, "No healthy backends", http.StatusServiceUnavailable)
		return
	}

	metrics.ActiveConnections.WithLabelValues(backend.URL.String()).Inc()
	defer metrics.ActiveConnections.WithLabelValues(backend.URL.String()).Dec()

	// strip the service prefix from the path before forwarding
	originalPath := r.URL.Path
	servicePath := "/" + route.ServiceName
	if strings.HasPrefix(originalPath, servicePath) {
		strippedPath := strings.TrimPrefix(originalPath, servicePath)
		if strippedPath == "" {
			strippedPath = "/"
		}
		r.URL.Path = strippedPath
		log.Printf("Path rewrite: %s -> %s for service %s", originalPath, strippedPath, route.ServiceName)
	}

	if isWebSocketRequest(r) {
		if err := s.handleWebSocket(w, r, backend.URL.String()); err != nil {
			log.Printf("WebSocket proxy error: %v", err)
			metrics.RequestsTotal.WithLabelValues(route.ServiceName, r.Method, "502").Inc()
		} else {
			metrics.RequestsTotal.WithLabelValues(route.ServiceName, r.Method, "101").Inc()
		}
		return
	}

	proxy := s.getOrCreateProxy(backend.URL)

	wrappedWriter := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
	proxy.ServeHTTP(wrappedWriter, r)

	duration := time.Since(start).Seconds()
	metrics.RequestDuration.WithLabelValues(route.ServiceName, r.Method).Observe(duration)
	metrics.RequestsTotal.WithLabelValues(route.ServiceName, r.Method, fmt.Sprintf("%d", wrappedWriter.statusCode)).Inc()
}

func (s *Server) getOrCreateProxy(target *url.URL) *httputil.ReverseProxy {
	key := target.String()

	s.mu.RLock()
	proxy, exists := s.reverseProxies[key]
	s.mu.RUnlock()

	if exists {
		return proxy
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	proxy = httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = s.transport
	proxy.ErrorHandler = s.proxyErrorHandler
	proxy.ModifyResponse = s.modifyResponse
	s.reverseProxies[key] = proxy

	return proxy
}

func (s *Server) proxyErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("Proxy error: %v", err)
	http.Error(w, "Bad gateway", http.StatusBadGateway)
}

func (s *Server) modifyResponse(resp *http.Response) error {
	resp.Header.Add("X-Proxy", "FluxGate")
	return nil
}

func (s *Server) UpdateConfig(cfg *config.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.config = cfg

	if err := s.tlsManager.UpdateConfig(cfg.TLS); err != nil {
		log.Printf("Failed to update TLS configuration: %v", err)
	}

	s.transport.DialContext = (&net.Dialer{
		Timeout:   cfg.Timeouts.Read,
		KeepAlive: 30 * time.Second,
	}).DialContext

	metrics.ConfigReloads.Inc()
	log.Printf("Server configuration reloaded successfully")

	return nil
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) GetLoadBalancer(serviceName string) loadbalancer.LoadBalancer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadBalancers[serviceName]
}

func (s *Server) subscribeToServiceChanges() {
	s.discovery.Subscribe(func(services map[string][]discovery.ServiceInstance) {
		log.Printf("Received service discovery update: %d services", len(services))

		for serviceName, instances := range services {
			s.updateLoadBalancerBackends(serviceName, instances)
		}

		totalInstances := 0
		for _, instances := range services {
			totalInstances += len(instances)
		}
		metrics.GossipNodes.Set(float64(totalInstances))
	})
}

func (s *Server) updateLoadBalancerBackends(serviceName string, instances []discovery.ServiceInstance) {
	s.mu.Lock()
	defer s.mu.Unlock()

	lb, exists := s.loadBalancers[serviceName]
	if !exists {
		log.Printf("Creating new load balancer for discovered service: %s", serviceName)
		lb = loadbalancer.NewRoundRobin()
		s.loadBalancers[serviceName] = lb

		s.router.AddRoute("/"+serviceName+"/*", serviceName, []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"})
		log.Printf("Added dynamic route for service: %s -> /%s/*", serviceName, serviceName)
	}

	var newLB loadbalancer.LoadBalancer
	switch lb.(type) {
	case *loadbalancer.LeastConnection:
		newLB = loadbalancer.NewLeastConnection()
	default:
		newLB = loadbalancer.NewRoundRobin()
	}

	for _, instance := range instances {
		backendURL := fmt.Sprintf("http://%s:%d", instance.Address, instance.Port)
		parsedURL, err := url.Parse(backendURL)
		if err != nil {
			log.Printf("Invalid backend URL for service %s: %s", serviceName, backendURL)
			continue
		}

		weight := 1 // * Default weight
		if w, exists := instance.Metadata["weight"]; exists {
			if parsedWeight, err := strconv.Atoi(w); err == nil {
				weight = parsedWeight
			}
		}

		newLB.Add(&loadbalancer.Backend{
			URL:    parsedURL,
			Weight: weight,
			Active: true,
		})
	}

	s.loadBalancers[serviceName] = newLB
	log.Printf("Updated load balancer for service %s with %d instances", serviceName, len(instances))
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
		"services":  len(s.loadBalancers),
	})
}

func (s *Server) handleServiceRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var instance discovery.ServiceInstance
	if err := json.NewDecoder(r.Body).Decode(&instance); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if instance.ID == "" || instance.Service == "" || instance.Address == "" || instance.Port == 0 {
		http.Error(w, "Missing required fields: id, service, address, port", http.StatusBadRequest)
		return
	}

	if isReservedServiceName(instance.Service) {
		http.Error(w, fmt.Sprintf("Service name '%s' is reserved", instance.Service), http.StatusBadRequest)
		return
	}

	if err := s.discovery.Register(instance); err != nil {
		log.Printf("Failed to register service: %v", err)
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	log.Printf("Service registered: %s (%s:%d)", instance.Service, instance.Address, instance.Port)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "registered",
		"service":   instance.Service,
		"id":        instance.ID,
		"route":     "/" + instance.Service + "/*",
		"timestamp": time.Now().Unix(),
	})
}

func (s *Server) handleServiceDeregistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceID := r.URL.Query().Get("id")
	if serviceID == "" {
		http.Error(w, "Missing service ID parameter", http.StatusBadRequest)
		return
	}

	if err := s.discovery.Deregister(serviceID); err != nil {
		log.Printf("Failed to deregister service: %v", err)
		http.Error(w, "Deregistration failed", http.StatusInternalServerError)
		return
	}

	log.Printf("Service deregistered: %s", serviceID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "deregistered",
		"id":        serviceID,
		"timestamp": time.Now().Unix(),
	})
}

func (s *Server) handleServiceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	serviceName := r.URL.Query().Get("service")
	if serviceName != "" {
		instances := s.discovery.GetInstances(serviceName)
		json.NewEncoder(w).Encode(map[string]any{
			"service":   serviceName,
			"instances": instances,
			"route":     "/" + serviceName + "/*",
			"timestamp": time.Now().Unix(),
		})
		return
	}

	allServices := s.discovery.GetAllServices()

	servicesWithRoutes := make(map[string]any)
	for serviceName, instances := range allServices {
		servicesWithRoutes[serviceName] = map[string]any{
			"instances": instances,
			"route":     "/" + serviceName + "/*",
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"services":  servicesWithRoutes,
		"total":     len(allServices),
		"timestamp": time.Now().Unix(),
	})
}
