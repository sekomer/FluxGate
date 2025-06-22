# FluxGate Architecture

FluxGate is designed as a **dynamic service discovery reverse proxy** that operates without external dependencies. Its architecture emphasizes **real-time service registration**, distributed state sharing, and operational simplicity.

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────┐
│   Client    │────▶│   FluxGate      │────▶│  Backend    │
└─────────────┘     │   Proxy         │     │  Services   │
                    │                 │     └─────────────┘
                    │ ┌─────────────┐ │            │
                    │ │ Service     │ │            │
                    │ │ Registry    │ │◀───────────┘
                    │ └─────────────┘ │    API Registration
                    └─────────────────┘
                           │
                    ┌──────┴──────┐
                    │             │
              ┌─────▼─────┐ ┌─────▼─────┐
              │  Gossip   │ │ Prometheus│
              │  Cluster  │ │  Metrics  │
              └───────────┘ └───────────┘
```
