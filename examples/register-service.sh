#!/bin/bash

set -euo pipefail

# FluxGate Service Registration Helper
# ===================================

FLUXGATE_HOST=${FLUXGATE_HOST:-"localhost"}
FLUXGATE_PORT=${FLUXGATE_PORT:-"8080"}
BASE_URL="http://${FLUXGATE_HOST}:${FLUXGATE_PORT}/api/v1"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

usage() {
    echo "FluxGate Service Registration Helper"
    echo "Usage: $0 <command> [options]"
    echo
    echo "Commands:"
    echo "  register    Register a service"
    echo "  deregister  Deregister a service"
    echo "  list        List services"
    echo "  health      Check FluxGate health"
    echo
    echo "Environment Variables:"
    echo "  FLUXGATE_HOST  FluxGate host (default: localhost)"
    echo "  FLUXGATE_PORT  FluxGate port (default: 8080)"
    echo
    echo "Examples:"
    echo "  $0 register my-service my-service-1 localhost 8001"
    echo "  $0 register my-service my-service-1 localhost 8001 weight=2 version=1.0"
    echo "  $0 deregister my-service-1"
    echo "  $0 list"
    echo "  $0 list my-service"
    echo
    exit 1
}

register_service() {
    local service_name="$1"
    local service_id="$2"
    local address="$3"
    local port="$4"
    shift 4

    # Parse metadata from remaining arguments
    local metadata="{}"
    if [ $# -gt 0 ]; then
        local meta_pairs=""
        for arg in "$@"; do
            if [[ $arg == *"="* ]]; then
                local key=$(echo "$arg" | cut -d'=' -f1)
                local value=$(echo "$arg" | cut -d'=' -f2)
                if [ -n "$meta_pairs" ]; then
                    meta_pairs="$meta_pairs, \"$key\": \"$value\""
                else
                    meta_pairs="\"$key\": \"$value\""
                fi
            fi
        done
        if [ -n "$meta_pairs" ]; then
            metadata="{$meta_pairs}"
        fi
    fi

    local payload=$(cat <<EOF
{
    "id": "$service_id",
    "service": "$service_name",
    "address": "$address",
    "port": $port,
    "metadata": $metadata
}
EOF
)

    echo -e "${YELLOW}Registering service...${NC}"
    echo "Service: $service_name"
    echo "ID: $service_id"
    echo "Address: $address:$port"
    echo "Metadata: $metadata"
    echo

    local response=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/services/register" \
        -H "Content-Type: application/json" \
        -d "$payload" 2>/dev/null)
    
    local body=$(echo "$response" | sed '$d')
    local status_code=$(echo "$response" | tail -n1)

    if [ "$status_code" = "201" ]; then
        echo -e "${GREEN}✓ Service registered successfully${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        echo -e "${RED}✗ Registration failed (HTTP $status_code)${NC}"
        echo "$body"
        exit 1
    fi
}

deregister_service() {
    local service_id="$1"

    echo -e "${YELLOW}Deregistering service...${NC}"
    echo "Service ID: $service_id"
    echo

    local response=$(curl -s -w "\n%{http_code}" -X DELETE "$BASE_URL/services/deregister?id=$service_id" \
        -H "Content-Type: application/json" 2>/dev/null)
    
    local body=$(echo "$response" | sed '$d')
    local status_code=$(echo "$response" | tail -n1)

    if [ "$status_code" = "200" ]; then
        echo -e "${GREEN}✓ Service deregistered successfully${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        echo -e "${RED}✗ Deregistration failed (HTTP $status_code)${NC}"
        echo "$body"
        exit 1
    fi
}

list_services() {
    local service_name="${1:-}"
    
    local url="$BASE_URL/services"
    if [ -n "$service_name" ]; then
        url="$url?service=$service_name"
        echo -e "${BLUE}Listing instances for service: $service_name${NC}"
    else
        echo -e "${BLUE}Listing all services${NC}"
    fi
    echo

    local response=$(curl -s "$url" 2>/dev/null)
    
    if [ $? -eq 0 ]; then
        echo "$response" | jq '.' 2>/dev/null || echo "$response"
    else
        echo -e "${RED}✗ Failed to connect to FluxGate${NC}"
        exit 1
    fi
}

check_health() {
    echo -e "${BLUE}Checking FluxGate health...${NC}"
    echo "FluxGate URL: $BASE_URL"
    echo

    # Check if FluxGate health endpoint is responding
    if curl -s "$BASE_URL/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ FluxGate is healthy${NC}"
        
        # Get some basic info
        echo
        echo "Available endpoints:"
        echo "  GET  $BASE_URL/health             - FluxGate health check"
        echo "  GET  $BASE_URL/services           - List all services"
        echo "  GET  $BASE_URL/services?service=X - List specific service"  
        echo "  POST $BASE_URL/services/register  - Register service"
        echo "  DEL  $BASE_URL/services/deregister?id=X - Deregister service"
        
        # Try to get service count
        local services=$(curl -s "$BASE_URL/services" 2>/dev/null)
        if [ $? -eq 0 ]; then
            local count=$(echo "$services" | jq '.total // (.services | length)' 2>/dev/null || echo "unknown")
            echo
            echo "Current registered services: $count"
        fi
    else
        echo -e "${RED}✗ FluxGate is not responding${NC}"
        echo "Please check if FluxGate is running on $BASE_URL"
        
        # Try fallback check with services endpoint
        echo "Trying services endpoint as fallback..."
        if curl -s "$BASE_URL/services" > /dev/null 2>&1; then
            echo -e "${YELLOW}! Services endpoint works but health endpoint doesn't${NC}"
        fi
        
        exit 1
    fi
}

# Main command processing
case "${1:-}" in
    register)
        if [ $# -lt 5 ]; then
            echo -e "${RED}Error: Missing required arguments for register${NC}"
            echo "Usage: $0 register <service_name> <service_id> <address> <port> [metadata...]"
            echo "Example: $0 register my-api api-1 localhost 8001 weight=2 version=1.0"
            exit 1
        fi
        register_service "$2" "$3" "$4" "$5" "${@:6}"
        ;;
    deregister)
        if [ $# -lt 2 ]; then
            echo -e "${RED}Error: Missing service ID for deregister${NC}"
            echo "Usage: $0 deregister <service_id>"
            exit 1
        fi
        deregister_service "$2"
        ;;
    list)
        list_services "${2:-}"
        ;;
    health)
        check_health
        ;;
    *)
        usage
        ;;
esac 