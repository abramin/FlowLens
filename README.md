# FlowLens

A Go code analysis tool that generates forward call graphs from entrypoints (HTTP routes, gRPC methods, CLI commands, main functions). FlowLens helps developers answer "What happens next?" by visualizing the flow from entrypoint to handler to service to store.

## Quick Start

```bash
# Install dependencies
make install

# Index a Go project
make index PATH=/path/to/your/go/project

# Start the UI
make run
```

Then open http://localhost:8080 in your browser.

## Features

- **Entrypoint Detection**: Automatically finds HTTP handlers (stdlib, chi, gin), gRPC methods, Cobra CLI commands, and main functions
- **Call Graph Visualization**: Interactive directed graph showing function calls
- **Smart Tagging**: Identifies I/O boundaries (database, network, filesystem), layer classification, and purity analysis
- **Filtering**: Hide stdlib, vendors, or stop at I/O boundaries
- **Inspector Panel**: View symbol details, callers, and callees

## Usage

### Indexing a Project

```bash
# Index current directory
./flowlens index .

# Index a specific project
./flowlens index /path/to/go/project
```

This creates a `.flowlens/index.db` SQLite database with the call graph data.

### Starting the UI

```bash
./flowlens ui
```

Opens the web UI at http://localhost:8080.

### Development Mode

```bash
make dev
```

Runs the Go API server and Vite dev server with hot reload.

## UI Guide

### Three-Panel Layout

1. **Left Panel (Entrypoints)**: Browse and search entrypoints by type (HTTP, gRPC, CLI, Main)
2. **Center Panel (Graph)**: Interactive call graph visualization
3. **Right Panel (Inspector)**: Symbol details, filters, and navigation

### Graph Interactions

- **Click** a node to select it and view details in the inspector
- **Double-click** a node to expand it (show its callees)
- **Shift+click** a node to pin/unpin it
- **Scroll** to zoom, **drag** to pan
- **Fit View** button to reset the view

### Node Colors

- **Blue**: Root entrypoint
- **Amber**: I/O boundary (database, network, etc.)
- **Green**: Pinned node
- **Purple**: Selected node
- **Gray**: Expanded/collapsed nodes

### Edge Styles

- **Solid gray**: Static function calls
- **Dashed purple**: Interface method calls
- **Animated green**: Goroutine calls (`go func()`)

### Filter Presets

- **Default**: Depth 6, show everything
- **Deep Dive**: Depth 10, show everything
- **High Level**: Depth 3, hide stdlib/vendors, stop at I/O

## Configuration

Create a `flowlens.yaml` in your project root:

```yaml
exclude:
  dirs: ["vendor", "third_party"]
  files_glob: ["**/*.pb.go", "**/*_gen.go"]

layers:
  handler: ["**/handlers/**"]
  service: ["**/service/**", "**/services/**"]
  store: ["**/store/**", "**/stores/**"]

io_packages:
  db: ["database/sql", "github.com/jackc/pgx", "gorm.io/*"]
  net: ["net/http", "google.golang.org/grpc"]

noise_packages:
  - "log/slog"
  - "go.uber.org/zap"
```

## Requirements

- Go 1.21+
- Node.js 18+
- npm 9+

## License

MIT
