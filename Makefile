.PHONY: build build-cli build-ui install dev run index clean test help

# Default target
help:
	@echo "FlowLens - Go Code Call Graph Analyzer"
	@echo ""
	@echo "Usage:"
	@echo "  make build      Build CLI and UI for production"
	@echo "  make dev        Start development servers (UI + API)"
	@echo "  make run        Build and run the UI server"
	@echo "  make index      Index the current directory"
	@echo "  make test       Run all tests"
	@echo "  make clean      Remove build artifacts"
	@echo ""
	@echo "Examples:"
	@echo "  make index TARGET=/path/to/go/project"
	@echo "  make run TARGET=/path/to/go/project"
	@echo "  make dev"

# Build everything
build: build-cli build-ui

# Build CLI binary
build-cli:
	go build -o flowlens ./cmd/flowlens

# Build UI for production
build-ui:
	cd ui && npm install && npm run build

# Install dependencies
install:
	go mod download
	cd ui && npm install

# Development mode: run UI dev server and Go API server concurrently
dev: build-cli
	@echo "Starting FlowLens in development mode..."
	@echo "UI: http://localhost:5173"
	@echo "API: http://localhost:8080"
	@trap 'kill 0' EXIT; \
	./flowlens ui & \
	cd ui && npm run dev

# Build and run production server (use TARGET= to specify project path)
run: build
	@echo "Starting FlowLens..."
	@echo "Open http://localhost:8080 in your browser"
	./flowlens ui $(if $(TARGET),$(TARGET),.)

# Index a Go project (use TARGET= to specify project path)
index: build-cli
	./flowlens index $(if $(TARGET),$(TARGET),.)

# Run tests
test:
	go test ./...
	cd ui && npm run lint

# Clean build artifacts
clean:
	rm -f flowlens
	rm -rf ui/dist
	rm -rf ui/node_modules
	rm -rf .flowlens
