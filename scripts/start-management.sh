#!/bin/bash

# SovereignStack Management API - Quick Start Script
# This script builds and starts the management docker in the main stack
# Usage: ./scripts/start-management.sh [--build] [--logs] [--stop]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
print_header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

show_help() {
    cat << EOF
${BLUE}SovereignStack Management API - Start Script${NC}

Usage: $0 [OPTIONS]

Options:
    --build         Force rebuild Docker image (without --build, uses cached image)
    --logs          Follow logs after starting (shows real-time container output)
    --stop          Stop the management container (opposite of start)
    --status        Show status of management container
    --health        Check health of management API
    -h, --help      Show this help message

Examples:
    # Start with Docker cache (fastest)
    $0

    # Rebuild image and start
    $0 --build

    # Start and watch logs
    $0 --logs

    # Check if it's running
    $0 --status

    # Stop the container
    $0 --stop

EOF
}

setup_env() {
    print_info "Checking environment..."

    if [ ! -f "$PROJECT_ROOT/.env" ]; then
        print_info "No .env file found, creating from .env.example"
        if [ -f "$PROJECT_ROOT/.env.example" ]; then
            cp "$PROJECT_ROOT/.env.example" "$PROJECT_ROOT/.env"
            print_success "Created .env file"
        else
            print_error "Cannot find .env.example"
            exit 1
        fi
    fi
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed"
        exit 1
    fi
    print_success "Docker is installed"

    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
        print_error "docker-compose is not installed"
        exit 1
    fi
    print_success "Docker Compose is available"
}

build_image() {
    print_header "Building Management API Docker Image"

    cd "$PROJECT_ROOT"

    if docker-compose build management 2>/dev/null || docker compose build management; then
        print_success "Docker image built successfully"
    else
        print_error "Failed to build Docker image"
        exit 1
    fi
}

start_container() {
    print_header "Starting Management API Container"

    cd "$PROJECT_ROOT"

    if docker-compose up -d management 2>/dev/null || docker compose up -d management; then
        print_success "Management container started"

        # Wait for service to be healthy
        print_info "Waiting for service to be healthy..."
        sleep 2

        # Check health
        if curl -s http://localhost:8888/api/v1/health > /dev/null 2>&1; then
            print_success "Management API is healthy"
            echo ""
            echo -e "${GREEN}🎉 Management API is running!${NC}"
            echo ""
            echo "Endpoints:"
            echo "  Health:        http://localhost:8888/api/v1/health"
            echo "  Running Models: http://localhost:8888/api/v1/models/running"
            echo ""
            echo "Test it:"
            echo "  curl http://localhost:8888/api/v1/health"
            echo "  curl http://localhost:8888/api/v1/models/running | jq ."
            echo ""
        else
            print_error "Service is not responding to health check"
            print_info "Check logs with: docker-compose logs management"
            exit 1
        fi
    else
        print_error "Failed to start container"
        exit 1
    fi
}

stop_container() {
    print_header "Stopping Management API Container"

    cd "$PROJECT_ROOT"

    if docker-compose down management 2>/dev/null || docker compose down management; then
        print_success "Management container stopped"
    else
        print_error "Failed to stop container"
        exit 1
    fi
}

show_status() {
    print_header "Management API Status"

    cd "$PROJECT_ROOT"

    if docker-compose ps management 2>/dev/null | grep -q management || docker compose ps management | grep -q management; then
        if curl -s http://localhost:8888/api/v1/health > /dev/null 2>&1; then
            print_success "Management API is running and healthy"
            echo ""
            docker-compose ps management 2>/dev/null || docker compose ps management
        else
            print_error "Management API is running but NOT responding"
            echo ""
            docker-compose ps management 2>/dev/null || docker compose ps management
        fi
    else
        print_info "Management API container is not running"
        echo ""
        echo "Start it with: $0"
    fi
}

check_health() {
    print_header "Checking Management API Health"

    echo "Testing /api/v1/health endpoint..."
    if response=$(curl -s http://localhost:8888/api/v1/health); then
        print_success "API is responding"
        echo "Response: $response"
        echo ""

        echo "Testing /api/v1/models/running endpoint..."
        if models=$(curl -s http://localhost:8888/api/v1/models/running); then
            echo "Response: $(echo "$models" | jq . 2>/dev/null || echo "$models")"
        else
            print_error "Failed to query models"
        fi
    else
        print_error "API is not responding. Is it running?"
        echo ""
        print_info "Start it with: $0"
        exit 1
    fi
}

show_logs() {
    print_header "Management API Logs"
    print_info "Press Ctrl+C to stop following logs"
    echo ""

    cd "$PROJECT_ROOT"

    if docker-compose logs -f management 2>/dev/null || docker compose logs -f management; then
        :
    else
        print_error "Failed to show logs"
        exit 1
    fi
}

# Main script logic
FORCE_BUILD=false
SHOW_LOGS=false
STOP=false
STATUS=false
HEALTH=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --build)
            FORCE_BUILD=true
            shift
            ;;
        --logs)
            SHOW_LOGS=true
            shift
            ;;
        --stop)
            STOP=true
            shift
            ;;
        --status)
            STATUS=true
            shift
            ;;
        --health)
            HEALTH=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Execute requested operations
if [ "$STOP" = true ]; then
    stop_container
    exit 0
fi

if [ "$STATUS" = true ]; then
    show_status
    exit 0
fi

if [ "$HEALTH" = true ]; then
    check_health
    exit 0
fi

# Default flow: check env, docker, build if needed, start, optionally show logs
print_header "🚀 SovereignStack Management API Startup"

setup_env
check_docker

if [ "$FORCE_BUILD" = true ]; then
    build_image
fi

start_container

if [ "$SHOW_LOGS" = true ]; then
    show_logs
fi
