.PHONY: build build-cli build-mcp build-sqlbot test clean install install-all run run-mcp docker-build docker-up docker-down docker-restart docker-logs help

# Build all binaries
build: build-cli build-mcp build-sqlbot

# Build just the CLI client
build-cli:
	@echo "Building go-mycli..."
	@go build -o bin/go-mycli ./cmd/go-mycli

# Build just the MCP server
build-mcp:
	@echo "Building mcp-server..."
	@cd mcp-server && go build -o ../bin/mcp-server .

# Build sqlbot MCP server
build-sqlbot:
	@echo "Building sqlbot..."
	@cd sqlbot && go build -o ../bin/sqlbot .

# Build Docker image for sqlbot
docker-sqlbot: build-sqlbot
	@echo "Building sqlbot Docker image..."
	@cd sqlbot && docker build -t go-mycli-sqlbot .

# Build Docker containers using docker-compose
docker-build:
	@echo "Building Docker containers..."
	@docker-compose build

# Start Docker containers (build and run)
docker-up:
	@echo "Starting Docker containers..."
	@docker-compose up --build -d
	@echo "MCP server is running on http://localhost:8800"
	@echo "Use 'make docker-logs' to view logs"

# Stop Docker containers
docker-down:
	@echo "Stopping Docker containers..."
	@docker-compose down

# Restart Docker containers
docker-restart: docker-down docker-up

# View Docker container logs
docker-logs:
	@docker-compose logs -f

test:
	go test ./...

clean:
	rm -rf bin/

# Install CLI only
install: build-cli
	cp bin/go-mycli /usr/local/bin/

# Install all binaries
install-all: build
	cp bin/go-mycli /usr/local/bin/
	cp bin/mcp-server /usr/local/bin/
	cp bin/sqlbot /usr/local/bin/

run: build-cli
	./bin/go-mycli

# Run MCP server with integrated sqlbot
run-mcp: build-mcp build-sqlbot
	./bin/mcp-server --listen :8800 --mcp-command "./bin/sqlbot"

help:
	@echo "Available targets:"
	@echo "  build          - Build all binaries (go-mycli, mcp-server, sqlbot)"
	@echo "  build-cli      - Build only go-mycli binary"
	@echo "  build-mcp      - Build only mcp-server binary"
	@echo "  build-sqlbot   - Build only sqlbot MCP server binary"
	@echo "  docker-sqlbot  - Build sqlbot Docker image"
	@echo "  docker-build   - Build Docker containers using docker-compose"
	@echo "  docker-up      - Start Docker containers (build and run in background)"
	@echo "  docker-down    - Stop Docker containers"
	@echo "  docker-restart - Restart Docker containers"
	@echo "  docker-logs    - View Docker container logs (follow mode)"
	@echo "  test           - Run all tests"
	@echo "  clean          - Remove bin/ directory"
	@echo "  install        - Install go-mycli to /usr/local/bin/"
	@echo "  install-all    - Install all binaries to /usr/local/bin/"
	@echo "  run            - Build and run go-mycli"
	@echo "  run-mcp        - Build and run mcp-server with integrated sqlbot on port 8800"
	@echo "  help           - Show this help message"