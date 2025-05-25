# Clash Royale TCR Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Build parameters
BINARY_DIR=bin
CLIENT_BINARY=tcr-client
SERVER_BINARY=tcr-server
CLIENT_SOURCE=cmd/client/main.go
SERVER_SOURCE=cmd/server/main.go

# Version info
VERSION=1.0.0
BUILD_TIME=$(shell date +%Y-%m-%dT%H:%M:%S)

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build client server clean test deps run-client run-server help

# Default target
all: deps build

# Build all binaries
build: client server

# Build client
client:
	@echo "Building client..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(CLIENT_BINARY) $(CLIENT_SOURCE)
	@echo "Client built successfully: $(BINARY_DIR)/$(CLIENT_BINARY)"

# Build server
server:
	@echo "Building server..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(SERVER_BINARY) $(SERVER_SOURCE)
	@echo "Server built successfully: $(BINARY_DIR)/$(SERVER_BINARY)"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) tidy
	$(GOGET) github.com/fatih/color@v1.16.0
	@echo "Dependencies installed"

# Run client
run-client: client
	@echo "Starting client..."
	./$(BINARY_DIR)/$(CLIENT_BINARY)

# Run client with debug
run-client-debug: client
	@echo "Starting client with debug logging..."
	./$(BINARY_DIR)/$(CLIENT_BINARY) -log-level DEBUG

# Run client with custom server
run-client-remote: client
	@echo "Starting client with remote server..."
	./$(BINARY_DIR)/$(CLIENT_BINARY) -server "$(SERVER_ADDR)"

# Run server
run-server: server
	@echo "Starting server..."
	./$(BINARY_DIR)/$(SERVER_BINARY)

# Run server with debug
run-server-debug: server
	@echo "Starting server with debug logging..."
	./$(BINARY_DIR)/$(SERVER_BINARY) -log-level DEBUG

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)
	rm -rf logs
	@echo "Clean completed"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Setup data directory and JSON files
setup-data:
	@echo "Setting up data directory..."
	@mkdir -p data
	@if [ ! -f data/troops.json ]; then \
		echo "Creating troops.json..."; \
		cat > data/troops.json << 'EOF'
{
  "troops": {
    "Pawn": {
      "hp": 50,
      "atk": 150,
      "def": 100,
      "mana": 3,
      "exp": 5,
      "special": ""
    },
    "Bishop": {
      "hp": 100,
      "atk": 200,
      "def": 150,
      "mana": 4,
      "exp": 10,
      "special": ""
    },
    "Rook": {
      "hp": 250,
      "atk": 200,
      "def": 200,
      "mana": 5,
      "exp": 25,
      "special": ""
    },
    "Knight": {
      "hp": 200,
      "atk": 300,
      "def": 150,
      "mana": 5,
      "exp": 25,
      "special": ""
    },
    "Prince": {
      "hp": 500,
      "atk": 400,
      "def": 300,
      "mana": 6,
      "exp": 50,
      "special": ""
    },
    "Queen": {
      "hp": 0,
      "atk": 0,
      "def": 0,
      "mana": 5,
      "exp": 30,
      "special": "Heals the friendly tower with lowest HP by 300"
    }
  }
}
EOF
	fi
	@if [ ! -f data/towers.json ]; then \
		echo "Creating towers.json..."; \
		cat > data/towers.json << 'EOF'
{
  "towers": {
    "King Tower": {
      "hp": 2000,
      "atk": 500,
      "def": 300,
      "crit": 0.10,
      "exp": 200
    },
    "Guard Tower": {
      "hp": 1000,
      "atk": 300,
      "def": 100,
      "crit": 0.05,
      "exp": 100
    }
  }
}
EOF
	fi
	@if [ ! -f data/players.json ]; then \
		echo "Creating players.json..."; \
		echo '{"players": []}' > data/players.json; \
	fi
	@echo "Data setup completed"

# Development setup
dev-setup: deps setup-data
	@echo "Development environment setup completed"

# Quick start for development
dev: dev-setup build
	@echo "Development build completed. Ready to run!"
	@echo "Use 'make run-client' to start the client"
	@echo "Use 'make run-server' to start the server"

# Install client globally
install-client: client
	@echo "Installing client globally..."
	sudo cp $(BINARY_DIR)/$(CLIENT_BINARY) /usr/local/bin/
	@echo "Client installed to /usr/local/bin/$(CLIENT_BINARY)"

# Create release package
release: clean deps build
	@echo "Creating release package..."
	@mkdir -p release/clash-royale-tcr-$(VERSION)
	@cp -r $(BINARY_DIR) release/clash-royale-tcr-$(VERSION)/
	@cp -r data release/clash-royale-tcr-$(VERSION)/
	@cp README.md release/clash-royale-tcr-$(VERSION)/ 2>/dev/null || echo "README.md not found, skipping"
	@cd release && tar -czf clash-royale-tcr-$(VERSION).tar.gz clash-royale-tcr-$(VERSION)/
	@echo "Release package created: release/clash-royale-tcr-$(VERSION).tar.gz"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	@echo "Linting code..."
	golangci-lint run

# Show help
help:
	@echo "Clash Royale TCR - Available commands:"
	@echo ""
	@echo "Build commands:"
	@echo "  make build        - Build both client and server"
	@echo "  make client       - Build client only"
	@echo "  make server       - Build server only"
	@echo ""
	@echo "Run commands:"
	@echo "  make run-client   - Build and run client"
	@echo "  make run-server   - Build and run server"
	@echo "  make run-client-debug - Run client with debug logging"
	@echo "  make run-server-debug - Run server with debug logging"
	@echo ""
	@echo "Development commands:"
	@echo "  make dev-setup    - Setup development environment"
	@echo "  make dev          - Full development setup and build"
	@echo "  make setup-data   - Create data directory and JSON files"
	@echo ""
	@echo "Maintenance commands:"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make test         - Run tests"
	@echo "  make deps         - Install dependencies"
	@echo "  make fmt          - Format code"
	@echo "  make lint         - Lint code"
	@echo ""
	@echo "Release commands:"
	@echo "  make release      - Create release package"
	@echo "  make install-client - Install client globally"
	@echo ""
	@echo "Examples:"
	@echo "  make dev                    # Full development setup"
	@echo "  make run-client             # Start client"
	@echo "  SERVER_ADDR=host:port make run-client-remote  # Connect to specific server"
	@echo ""