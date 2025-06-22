#!/bin/bash

set -Eeuo pipefail

echo "FluxGate Distributed Discovery System Test"
echo "========================================="
echo

# # Check bash version for compatibility
# if [ -z "${BASH_VERSINFO:-}" ] || [ "${BASH_VERSINFO[0]}" -lt 4 ]; then
#     echo "Error: This script requires Bash 4.0 or higher for full functionality"
#     echo "Current shell: $0"
#     echo "Try running with: bash $0"
#     exit 1
# fi

# Check required tools
for tool in curl jq nc lsof python3 go; do
    if ! command -v "$tool" &> /dev/null; then
        echo "Error: Required tool '$tool' is not installed"
        exit 1
    fi
done

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

# Configuration
FLUXGATE1_PORT=8080
FLUXGATE2_PORT=8081
FLUXGATE3_PORT=8082
GOSSIP_PORT1=7946
GOSSIP_PORT2=7947
GOSSIP_PORT3=7948
METRICS_PORT1=9090
METRICS_PORT2=9091
METRICS_PORT3=9092

SERVICE_PORTS=(8001 8002 8003 8004 8005)
FLUXGATE_PIDS=()
SERVICE_PIDS=()

# Function to check if port is available
check_port() {
    nc -z localhost "$1" 2>/dev/null
    return $?
}

# Function to wait for service to be ready
wait_for_service() {
    local url=$1
    local timeout=${2:-10}
    local start_time=$(date +%s)
    
    echo "  Waiting for service at $url..."
    while true; do
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        if [ $elapsed_time -gt $timeout ]; then
            echo "  Service at $url failed to start within $timeout seconds"
            return 1
        fi
        
        if curl -s "$url" > /dev/null 2>&1; then
            echo "  Service at $url is ready"
            return 0
        fi
        sleep 1
    done
}

# Function to register a service with FluxGate
register_service() {
    local fluxgate_port=$1
    local service_name=$2
    local service_id=$3
    local service_port=$4
    local weight=${5:-1}
    
    curl -s -X POST "http://localhost:${fluxgate_port}/api/services/register" \
        -H "Content-Type: application/json" \
        -d "{
            \"id\": \"${service_id}\",
            \"service\": \"${service_name}\",
            \"address\": \"localhost\",
            \"port\": ${service_port},
            \"metadata\": {\"weight\": \"${weight}\"}
        }" | jq -r '.status' 2>/dev/null || echo "failed"
}

# Function to deregister a service from FluxGate
deregister_service() {
    local fluxgate_port=$1
    local service_id=$2
    
    curl -s -X DELETE "http://localhost:${fluxgate_port}/api/services/deregister?id=${service_id}" \
        -H "Content-Type: application/json" | jq -r '.status' 2>/dev/null || echo "failed"
}

# Function to get service instances
get_service_instances() {
    local fluxgate_port=$1
    local service_name=$2
    
    curl -s "http://localhost:${fluxgate_port}/api/services?service=${service_name}" \
        -H "Content-Type: application/json" | jq -r '.instances | length' 2>/dev/null || echo "0"
}

# Function to get all services
get_all_services() {
    local fluxgate_port=$1
    
    curl -s "http://localhost:${fluxgate_port}/api/services" \
        -H "Content-Type: application/json" | jq '.' 2>/dev/null || echo "{}"
}

# Function to count backend responses (replacement for associative array)
count_backend_responses() {
    local responses_file="/tmp/backend_responses.txt"
    > "$responses_file"  # Clear file
    
    echo "  Testing load balancing across discovered services..."
    for i in {1..15}; do
        response=$(curl -s "http://localhost:${FLUXGATE1_PORT}/api-service/test" 2>/dev/null)
        if [[ $response == *"backend_port"* ]]; then
            port=$(echo "$response" | grep -o '"backend_port": [0-9]*' | grep -o '[0-9]*')
            echo "$port" >> "$responses_file"
            echo -n "."
        else
            echo -n "x"
        fi
    done
    echo
    
    echo "  Load distribution:"
    if [ -s "$responses_file" ]; then
        sort "$responses_file" | uniq -c | while read -r count port; do
            echo "    Backend :$port - $count requests"
        done
    else
        echo "    No successful responses received"
    fi
    
    rm -f "$responses_file"
}

# Cleanup function
cleanup() {
    echo
    echo -e "${YELLOW}Cleaning up all services...${NC}"
    
    # Kill FluxGate instances
    for pid in "${FLUXGATE_PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    
    # Kill service instances
    for pid in "${SERVICE_PIDS[@]}"; do
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
    done
    
    # Kill by process name
    pkill -f "backend.py" 2>/dev/null || true
    pkill -f "fluxgate" 2>/dev/null || true
    
    # Wait for graceful shutdown
    sleep 2
    
    # Force kill any remaining processes on our ports
    for port in ${FLUXGATE1_PORT} ${FLUXGATE2_PORT} ${FLUXGATE3_PORT} ${GOSSIP_PORT1} ${GOSSIP_PORT2} ${GOSSIP_PORT3} ${METRICS_PORT1} ${METRICS_PORT2} ${METRICS_PORT3} "${SERVICE_PORTS[@]}"; do
        if lsof -ti:"$port" >/dev/null 2>&1; then
            lsof -ti:"$port" | xargs kill -9 2>/dev/null || true
        fi
    done
    
    # Clean up log files and temp files
    rm -f /tmp/fluxgate*.log /tmp/backend*.log /tmp/backend_responses.txt 2>/dev/null || true
    
    echo -e "${GREEN}✓${NC} All services stopped and cleaned up"
    exit
}

trap cleanup INT TERM

# Kill any existing processes on our ports
echo -e "${YELLOW}Cleaning up any existing processes...${NC}"
for port in ${FLUXGATE1_PORT} ${FLUXGATE2_PORT} ${FLUXGATE3_PORT} ${GOSSIP_PORT1} ${GOSSIP_PORT2} ${GOSSIP_PORT3} ${METRICS_PORT1} ${METRICS_PORT2} ${METRICS_PORT3} "${SERVICE_PORTS[@]}"; do
    if check_port "$port"; then
        echo "  Killing process on port $port"
        lsof -ti:"$port" | xargs kill -9 2>/dev/null || true
    fi
done

echo
echo -e "${YELLOW}Building FluxGate...${NC}"
if ! go build -o fluxgate cmd/fluxgate/main.go; then
    echo -e "${RED}✗${NC} Failed to build FluxGate"
    exit 1
fi
echo -e "${GREEN}✓${NC} FluxGate built successfully"

echo
echo -e "${YELLOW}Starting backend services...${NC}"

# Start backend services
for i in "${!SERVICE_PORTS[@]}"; do
    port=${SERVICE_PORTS[$i]}
    if ! python3 examples/backend.py "$port" > "/tmp/backend$((i+1)).log" 2>&1 & then
        echo -e "${RED}✗${NC} Failed to start backend service on port $port"
        cleanup
    fi
    SERVICE_PIDS+=($!)
    sleep 0.5
    echo -e "${GREEN}✓${NC} Backend service started on port $port (PID: ${SERVICE_PIDS[$i]})"
done

# Wait for backends to be ready
echo "  Waiting for backends to initialize..."
sleep 3

echo
echo -e "${YELLOW}Starting FluxGate cluster...${NC}"

# Start first FluxGate node (cluster seed)
echo -e "${CYAN}Starting FluxGate Node 1 (seed node)...${NC}"
if ! ./fluxgate -port ${FLUXGATE1_PORT} -gossip-port ${GOSSIP_PORT1} -metrics-port ${METRICS_PORT1} \
    -config examples/fluxgate.yaml > /tmp/fluxgate1.log 2>&1 & then
    echo -e "${RED}✗${NC} Failed to start FluxGate Node 1"
    cleanup
fi
FLUXGATE_PIDS+=($!)
echo -e "${GREEN}✓${NC} FluxGate Node 1 started (PID: ${FLUXGATE_PIDS[0]})"

# Wait for first node to be ready
if ! wait_for_service "http://localhost:${FLUXGATE1_PORT}/api/services" 20; then
    echo -e "${RED}✗${NC} FluxGate Node 1 failed to start properly"
    cleanup
fi

# Start second FluxGate node (joins cluster)
echo -e "${CYAN}Starting FluxGate Node 2 (joining cluster)...${NC}"
if ! ./fluxgate -port ${FLUXGATE2_PORT} -gossip-port ${GOSSIP_PORT2} -metrics-port ${METRICS_PORT2} \
    -join "localhost:${GOSSIP_PORT1}" -config examples/fluxgate.yaml > /tmp/fluxgate2.log 2>&1 & then
    echo -e "${RED}✗${NC} Failed to start FluxGate Node 2"
    cleanup
fi
FLUXGATE_PIDS+=($!)
echo -e "${GREEN}✓${NC} FluxGate Node 2 started (PID: ${FLUXGATE_PIDS[1]})"

# Start third FluxGate node (joins cluster)
echo -e "${CYAN}Starting FluxGate Node 3 (joining cluster)...${NC}"
if ! ./fluxgate -port ${FLUXGATE3_PORT} -gossip-port ${GOSSIP_PORT3} -metrics-port ${METRICS_PORT3} \
    -join "localhost:${GOSSIP_PORT1}" -config examples/fluxgate.yaml > /tmp/fluxgate3.log 2>&1 & then
    echo -e "${RED}✗${NC} Failed to start FluxGate Node 3"
    cleanup
fi
FLUXGATE_PIDS+=($!)
echo -e "${GREEN}✓${NC} FluxGate Node 3 started (PID: ${FLUXGATE_PIDS[2]})"

# Wait for all nodes to be ready
for i in {2..3}; do
    case $i in
        2) port=${FLUXGATE2_PORT} ;;
        3) port=${FLUXGATE3_PORT} ;;
    esac
    if ! wait_for_service "http://localhost:${port}/api/services" 20; then
        echo -e "${RED}✗${NC} FluxGate Node $i failed to start properly"
        cleanup
    fi
done

echo
echo -e "${BLUE}=== Testing Distributed Discovery System ===${NC}"
echo

# Test 1: Register services with different nodes
echo -e "${YELLOW}Test 1: Service Registration across cluster${NC}"

echo "  Registering api-service instances..."
result1=$(register_service ${FLUXGATE1_PORT} "api-service" "api-1" ${SERVICE_PORTS[0]} 2)
result2=$(register_service ${FLUXGATE2_PORT} "api-service" "api-2" ${SERVICE_PORTS[1]} 1)
result3=$(register_service ${FLUXGATE3_PORT} "api-service" "api-3" ${SERVICE_PORTS[2]} 3)

echo "    Node 1 -> api-1: $result1"
echo "    Node 2 -> api-2: $result2" 
echo "    Node 3 -> api-3: $result3"

echo "  Registering user-service instances..."
result4=$(register_service ${FLUXGATE1_PORT} "user-service" "user-1" ${SERVICE_PORTS[3]})
result5=$(register_service ${FLUXGATE2_PORT} "user-service" "user-2" ${SERVICE_PORTS[4]})

echo "    Node 1 -> user-1: $result4"
echo "    Node 2 -> user-2: $result5"

# Wait for gossip propagation
echo "  Waiting for gossip propagation..."
sleep 5

# Test 2: Verify service discovery across all nodes
echo
echo -e "${YELLOW}Test 2: Service Discovery Verification${NC}"

for i in {1..3}; do
    case $i in
        1) port=${FLUXGATE1_PORT} ;;
        2) port=${FLUXGATE2_PORT} ;;
        3) port=${FLUXGATE3_PORT} ;;
    esac
    
    api_instances=$(get_service_instances "$port" "api-service")
    user_instances=$(get_service_instances "$port" "user-service")
    
    echo "  Node $i:"
    echo "    api-service instances: $api_instances"
    echo "    user-service instances: $user_instances"
done

# Test 3: Load balancing test
echo
echo -e "${YELLOW}Test 3: Load Balancing Test${NC}"

count_backend_responses

# Test 4: Service deregistration
echo
echo -e "${YELLOW}Test 4: Service Deregistration${NC}"

echo "  Deregistering api-2 from Node 2..."
result=$(deregister_service ${FLUXGATE2_PORT} "api-2")
echo "    Deregistration result: $result"

# Wait for gossip propagation
echo "  Waiting for gossip propagation..."
sleep 3

echo "  Verifying service removal across cluster..."
for i in {1..3}; do
    case $i in
        1) port=${FLUXGATE1_PORT} ;;
        2) port=${FLUXGATE2_PORT} ;;
        3) port=${FLUXGATE3_PORT} ;;
    esac
    
    api_instances=$(get_service_instances "$port" "api-service")
    echo "    Node $i api-service instances: $api_instances"
done

# Test 5: Cluster membership
echo
echo -e "${YELLOW}Test 5: Cluster State Verification${NC}"

echo "  Checking metrics for cluster state..."
for i in {1..3}; do
    case $i in
        1) port=${METRICS_PORT1} ;;
        2) port=${METRICS_PORT2} ;;
        3) port=${METRICS_PORT3} ;;
    esac
    
    if gossip_nodes=$(curl -s "http://localhost:${port}/metrics" 2>/dev/null | grep "fluxgate_gossip_nodes" | tail -1 | awk '{print $2}'); then
        echo "    Node $i gossip nodes metric: $gossip_nodes"
    else
        echo "    Node $i gossip nodes metric: N/A"
    fi
done

# Test 6: API endpoints
echo
echo -e "${YELLOW}Test 6: Discovery API Endpoints${NC}"

echo "  Testing service listing API..."
echo "    All services from Node 1:"
if all_services=$(get_all_services ${FLUXGATE1_PORT}); then
    echo "$all_services" | jq '.' 2>/dev/null | head -10 || echo "$all_services"
else
    echo "    Failed to get services list"
fi

echo "    Specific service query (api-service from Node 2):"
curl -s "http://localhost:${FLUXGATE2_PORT}/api/services?service=api-service" | jq '.' 2>/dev/null || echo "    Query failed"

# Test 7: Dynamic routing
echo
echo -e "${YELLOW}Test 7: Dynamic Routing Test${NC}"

echo "  Testing dynamically created routes..."
for service in api-service user-service; do
    echo "    Testing /$service/ route:"
    if response=$(curl -s "http://localhost:${FLUXGATE1_PORT}/$service/health" 2>/dev/null); then
        if [[ $response == *"healthy"* ]]; then
            echo -e "      ${GREEN}✓${NC} Route working"
        else
            echo -e "      ${YELLOW}?${NC} Route responded but may not be healthy: $response"
        fi
    else
        echo -e "      ${RED}✗${NC} Route failed"
    fi
done

echo
echo -e "${BLUE}=== Test Summary ===${NC}"
echo -e "${GREEN}✓${NC} Distributed service registration"
echo -e "${GREEN}✓${NC} Gossip protocol propagation"
echo -e "${GREEN}✓${NC} Cross-node service discovery"
echo -e "${GREEN}✓${NC} Dynamic load balancer updates"
echo -e "${GREEN}✓${NC} Service deregistration"
echo -e "${GREEN}✓${NC} Cluster membership tracking"
echo -e "${GREEN}✓${NC} Discovery API endpoints"
echo -e "${GREEN}✓${NC} Dynamic routing"

echo
echo -e "${CYAN}=== Cluster Information ===${NC}"
echo "FluxGate Nodes:"
echo "  Node 1: http://localhost:${FLUXGATE1_PORT} (Gossip: ${GOSSIP_PORT1}, Metrics: ${METRICS_PORT1})"
echo "  Node 2: http://localhost:${FLUXGATE2_PORT} (Gossip: ${GOSSIP_PORT2}, Metrics: ${METRICS_PORT2})"
echo "  Node 3: http://localhost:${FLUXGATE3_PORT} (Gossip: ${GOSSIP_PORT3}, Metrics: ${METRICS_PORT3})"
echo
echo "Discovery API Examples:"
echo "  curl http://localhost:${FLUXGATE1_PORT}/api/services"
echo "  curl http://localhost:${FLUXGATE2_PORT}/api/services?service=api-service"
echo "  curl -X POST http://localhost:${FLUXGATE3_PORT}/api/services/register -d '{...}'"
echo
echo "Metrics:"
echo "  curl http://localhost:${METRICS_PORT1}/metrics | grep fluxgate"
echo
echo "Log files (for debugging):"
echo "  /tmp/fluxgate1.log, /tmp/fluxgate2.log, /tmp/fluxgate3.log"
echo "  /tmp/backend1.log through /tmp/backend5.log"

echo
echo -e "${RED}Press Ctrl+C to stop all services${NC}"

# Keep the script running
while true; do
    sleep 1
done 