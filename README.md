# FluxGate

**Dynamic Service Discovery Reverse Proxy for Cloud-Native Applications**

FluxGate is a lightweight, distributed reverse proxy that **automatically discovers and routes to your services** without external dependencies. Built for microservices, containers, and dynamic environments.

## 🎯 Key Features

- **🔄 Dynamic Service Registry**: Services register themselves via simple HTTP API
- **🌐 Automatic Route Creation**: Routes are created as `/{service}/*` automatically
- **🗣️ Gossip-Based Discovery**: Peer-to-peer service sharing across instances
- **⚡ Zero Dependencies**: No Redis, Consul, or etcd required
- **🔀 Smart Load Balancing**: Round-robin and least-connection algorithms
- **📊 Built-in Observability**: Prometheus metrics out of the box
- **🔧 Hot Configuration**: Zero-downtime updates and service changes

## 🚀 Quick Start

```bash
# 1. Build and start FluxGate
go build -o fluxgate cmd/fluxgate/main.go
./fluxgate

# 2. Register a service dynamically
curl -X POST http://localhost:8080/api/v1/services/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-api-1",
    "service": "my-api",
    "address": "localhost",
    "port": 8001
  }'

# 3. Your service is now available at http://localhost:8080/my-api/*
curl http://localhost:8080/my-api/health
```

## 🎬 See It In Action

```bash
# Run the interactive demo
./examples/dynamic-demo.sh

# Test distributed discovery
./examples/test-discovery.sh
```

## 🏗️ Perfect For

- **Microservices**: Dynamic service mesh without complexity
- **Kubernetes**: Automatic ingress for scaling pods
- **Development**: Local service discovery for multiple apps
- **Edge Computing**: Lightweight proxy for resource-constrained environments
- **CI/CD**: Dynamic routing for testing environments

## 📋 Management API

| Endpoint                      | Method | Description                     |
| ----------------------------- | ------ | ------------------------------- |
| `/api/v1/services`            | GET    | List all registered services    |
| `/api/v1/services/register`   | POST   | Register a new service instance |
| `/api/v1/services/deregister` | DELETE | Remove a service instance       |
| `/api/v1/health`              | GET    | FluxGate health status          |

## 🔧 Service Registration

Services can register themselves programmatically:

```bash
# Register with metadata
curl -X POST http://localhost:8080/api/v1/services/register \
  -d '{
    "id": "user-service-v2",
    "service": "user-service",
    "address": "10.0.1.100",
    "port": 8080,
    "metadata": {
      "version": "2.0",
      "weight": "2",
      "region": "us-west"
    }
  }'
```

Routes are automatically created:

- Service `user-service` → `http://fluxgate/user-service/*`
- Multiple instances load-balanced automatically
- Health checking and failover built-in

## 🌐 Distributed Discovery

FluxGate instances automatically share service information:

```bash
# Start a cluster
./fluxgate -port 8080 -gossip-port 7946
./fluxgate -port 8081 -gossip-port 7947 -join localhost:7946

# Register on any node, available on all nodes
curl -X POST http://localhost:8080/api/v1/services/register -d '{...}'
curl http://localhost:8081/my-service/api  # Works automatically!
```

## 📊 Monitoring

Built-in Prometheus metrics at `/metrics`:

## 🤝 Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## 📄 License

MIT License
