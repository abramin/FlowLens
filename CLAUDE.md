# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

FlowLens is a Go-based code analysis tool that generates forward call graphs from entrypoints (HTTP routes, gRPC methods, CLI commands, main). It helps developers answer "What happens next?" by visualizing the flow from entrypoint → handler → service → store.

## Build Commands

```bash
# Build CLI
go build -o flowlens ./cmd/flowlens

# Run indexer on a Go project
./flowlens index [path-to-go-project]

# Start UI server
./flowlens ui

# Run tests
go test ./...

# Run single test
go test -run TestName ./path/to/package

# Run tests with verbose output
go test -v ./...
```

## Architecture

### CLI Layer (`cmd/flowlens/`)
- Cobra-based CLI with two main commands: `index` and `ui`
- `index`: Analyzes Go code and persists to SQLite
- `ui`: Starts local HTTP server serving React UI + REST API

### Indexing Pipeline (`internal/index/`)
1. **Package Loading**: Uses `go/packages` with full type info
2. **SSA Construction**: Builds SSA via `golang.org/x/tools/go/ssa`
3. **Call Graph Extraction**: Static calls from SSA, interface calls marked as dynamic
4. **Entrypoint Detection**: AST patterns for HTTP (stdlib, chi, gin), gRPC, Cobra
5. **Tagging**: I/O boundaries (db/net/fs/bus), layer classification, purity heuristics
6. **Persistence**: Write to SQLite

### Storage
- **SQLite** (`internal/store/`): Primary storage at `.flowlens/index.db`
  - Tables: `symbols`, `call_edges`, `entrypoints`, `tags`, `packages`
- **index.json**: Quick-boot metadata for UI

### API Server (`internal/server/`)
- REST endpoints for UI:
  - `GET /api/entrypoints` - list/search entrypoints
  - `GET /api/graph/root` - fetch graph from entrypoint
  - `GET /api/graph/expand` - expand a node
  - `GET /api/symbol/:id` - symbol details
  - `GET /api/search` - fuzzy symbol search

### React UI (`ui/`)
- Three-panel layout: Entrypoints (left), Graph (center), Inspector (right)
- Graph visualization with expand/collapse, pin, filter controls

## Key Design Decisions

- **SSA over AST-only**: SSA provides more accurate call graph than pure AST traversal
- **SQLite for queries**: Enables on-demand expansion without loading entire graph
- **Interface calls marked "unknown"**: MVP prioritizes correctness over completeness for dynamic dispatch
- **Configurable via flowlens.yaml**: Layer rules, I/O packages, noise packages, exclusions

## Configuration (`flowlens.yaml`)

```yaml
exclude:
  dirs: ["vendor", "third_party"]
  files_glob: ["**/*.pb.go", "**/*_gen.go"]

layers:
  handler: ["**/handlers/**"]
  service: ["**/service/**", "**/services/**"]
  store: ["**/store/**", "**/stores/**"]
  domain: ["**/domain/**"]

io_packages:
  db: ["database/sql", "github.com/jackc/pgx", "gorm.io/*"]
  net: ["net/http", "google.golang.org/grpc"]
  bus: ["github.com/nats-io/*"]

noise_packages:
  - "log/slog"
  - "go.uber.org/zap"
```

## Code Standards

From `.claude/coordinator.md`:
- **Type-safe IDs**: Distinct types for each ID kind, no raw strings
- **Parse at boundaries**: Validation at HTTP handlers/message consumers
- **Store purity**: Stores do I/O only, no business logic
- **Domain purity**: `internal/domain/*` has no I/O imports, no `context.Context`
- **Interface placement**: Interfaces live at consumer site
