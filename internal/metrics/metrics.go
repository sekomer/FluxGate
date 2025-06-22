package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fluxgate_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"service", "method", "status"},
	)

	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fluxgate_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"service", "method"},
	)

	ActiveConnections = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fluxgate_active_connections",
			Help: "Number of active connections per backend",
		},
		[]string{"backend"},
	)

	BackendHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "fluxgate_backend_health",
			Help: "Health status of backends (1 = healthy, 0 = unhealthy)",
		},
		[]string{"backend"},
	)

	GossipNodes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "fluxgate_gossip_nodes",
			Help: "Number of nodes in the gossip cluster",
		},
	)

	ConfigReloads = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "fluxgate_config_reloads_total",
			Help: "Total number of configuration reloads",
		},
	)
)

func init() {
	prometheus.MustRegister(
		RequestsTotal,
		RequestDuration,
		ActiveConnections,
		BackendHealth,
		GossipNodes,
		ConfigReloads,
	)
}

type Server struct {
	port int
}

func NewServer(port int) *Server {
	return &Server{port: port}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), mux)
}