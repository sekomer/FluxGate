#!/bin/bash

set -e

echo "🚀 FluxGate Dynamic-Only Demo"
echo "============================="

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    pkill -f "fluxgate" 2>/dev/null || true
    pkill -f "backend.py" 2>/dev/null || true
    sleep 1
    echo -e "${GREEN}✓ Cleanup complete${NC}"
}

trap cleanup EXIT

# Build and start FluxGate
echo -e "${YELLOW}Building and starting FluxGate...${NC}"
go build -o fluxgate cmd/fluxgate/main.go
./fluxgate -config examples/fluxgate.yaml &
sleep 3

# Start backend services
echo -e "${YELLOW}Starting backend services...${NC}"
python3 examples/backend.py 8001 > /tmp/backend1.log 2>&1 &
python3 examples/backend.py 8002 > /tmp/backend2.log 2>&1 &
python3 examples/backend.py 8003 > /tmp/backend3.log 2>&1 &
sleep 2

# Verify FluxGate is ready
echo -e "${YELLOW}Verifying FluxGate is ready...${NC}"
for i in {1..10}; do
    if curl -s http://localhost:8080/api/v1/health > /dev/null 2>&1; then
        echo -e "${GREEN}✓ FluxGate is ready${NC}"
        break
    fi
    if [ $i -eq 10 ]; then
        echo -e "${RED}✗ FluxGate failed to start${NC}"
        exit 1
    fi
    echo "Waiting for FluxGate... ($i/10)"
    sleep 1
done

echo -e "\n${BLUE}📋 Current Services (should be empty):${NC}"
curl -s http://localhost:8080/api/v1/services | jq '.'

echo -e "\n${YELLOW}Registering services dynamically...${NC}"

# Register first service: my-api
echo -e "\n${BLUE}1. Registering 'my-api' service with 3 backends:${NC}"
curl -s -X POST http://localhost:8080/api/v1/services/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-api-1",
    "service": "my-api", 
    "address": "localhost",
    "port": 8001,
    "metadata": {"weight": "1"}
  }' | jq '.'

sleep 0.1

curl -s -X POST http://localhost:8080/api/v1/services/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-api-2",
    "service": "my-api",
    "address": "localhost", 
    "port": 8002,
    "metadata": {"weight": "1"}
  }' | jq '.'

sleep 0.1

curl -s -X POST http://localhost:8080/api/v1/services/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "my-api-3",
    "service": "my-api",
    "address": "localhost",
    "port": 8003,
    "metadata": {"weight": "2"}
  }' | jq '.'

sleep 0.1

# Register second service: user-service
echo -e "\n${BLUE}2. Registering 'user-service' with 1 backend:${NC}"
curl -s -X POST http://localhost:8080/api/v1/services/register \
  -H "Content-Type: application/json" \
  -d '{
    "id": "user-service-1",
    "service": "user-service",
    "address": "localhost",
    "port": 8001
  }' | jq '.'

sleep 1

echo -e "\n${BLUE}📋 Registered Services:${NC}"
curl -s http://localhost:8080/api/v1/services | jq '.'

echo -e "\n${YELLOW}Testing the dynamic routes...${NC}"

echo -e "\n${BLUE}Testing /my-api/* route (should load balance across 3 backends):${NC}"
for i in {1..6}; do
    response=$(curl -s http://localhost:8080/my-api/api/test 2>/dev/null)
    if [[ $response == *"backend_port"* ]]; then
        port=$(echo "$response" | jq -r '.backend_port' 2>/dev/null)
        echo "  Request $i -> Backend port: $port"
    else
        echo "  Request $i -> ERROR: $response"
    fi
done

echo -e "\n${BLUE}Testing /user-service/* route:${NC}"
response=$(curl -s http://localhost:8080/user-service/api/test 2>/dev/null)
if [[ $response == *"backend_port"* ]]; then
    port=$(echo "$response" | jq -r '.backend_port' 2>/dev/null)
    echo "  Response -> Backend port: $port"
else
    echo "  ERROR: $response"
fi

echo -e "\n${BLUE}Testing health checks:${NC}"
echo "  /my-api/health:"
curl -s http://localhost:8080/my-api/health | jq '.'

echo -e "\n  /user-service/health:"
curl -s http://localhost:8080/user-service/health | jq '.'

echo -e "\n${YELLOW}Testing service deregistration...${NC}"
echo -e "${BLUE}Deregistering user-service-1:${NC}"
curl -s -X DELETE "http://localhost:8080/api/v1/services/deregister?id=user-service-1" | jq '.'

sleep 2

echo -e "\n${BLUE}Services after deregistration:${NC}"
curl -s http://localhost:8080/api/v1/services | jq '.'

echo -e "\n${BLUE}Testing /user-service/* after deregistration (should fail):${NC}"
response=$(curl -s http://localhost:8080/user-service/api/test 2>/dev/null)
echo "  Response: $response"

echo -e "\n${GREEN}✅ Demo completed successfully!${NC}"

echo -e "\n${YELLOW}📚 What we demonstrated:${NC}"
echo "  ✓ Dynamic service registration via API"
echo "  ✓ Automatic route creation (/{service}/*)"  
echo "  ✓ Load balancing across multiple backends"
echo "  ✓ Service deregistration and cleanup"
echo "  ✓ No static configuration needed for routes"

echo -e "\n${YELLOW}🎯 Available endpoints:${NC}"
echo "  http://localhost:8080/my-api/*       -> my-api service (3 backends)"
echo "  http://localhost:8080/api/v1/health  -> FluxGate health"
echo "  http://localhost:8080/api/v1/services -> List services"

echo -e "\n${YELLOW}🔧 Management commands:${NC}"
echo "  # Register service:"
echo "  curl -X POST http://localhost:8080/api/v1/services/register \\"
echo "    -H 'Content-Type: application/json' \\"  
echo "    -d '{\"id\":\"my-id\",\"service\":\"my-service\",\"address\":\"localhost\",\"port\":8080}'"
echo ""
echo "  # Deregister service:"
echo "  curl -X DELETE 'http://localhost:8080/api/v1/services/deregister?id=my-id'"

read -p $'\n\033[1;33mPress Enter to cleanup and exit...\033[0m' 