## FlowLens PRD v0.1

### Goal

Let a developer select an **entrypoint** (HTTP route, gRPC method, command, cron, main) and see a **forward call graph** that answers: *“What happens next?”* with practical filters (package boundaries, layers, stop at I/O).

### Success criteria (measurable)

* You can select `/token` and get a graph that includes the real handler → service → store chain.
* “Stop at I/O” collapses/terminates at DB/network boundaries reliably enough to be useful (not perfect).
* Index a medium repo (100k–300k LOC) without feeling painful.

### Primary use cases

1. Trace a request: route → handler → services → stores.
2. Debug architecture drift: handler calling store directly, domain calling net/http, etc.
3. Explain a system quickly: share a URL state showing a flow.

---

## Product scope

### MVP (Phase 1)

**Indexing**

* Build a symbol table + call graph using `go/packages` + SSA (or fallback to AST when needed).
* Store results in SQLite + a compact JSON summary for UI boot.

**Entrypoint discovery**

* HTTP:

  * stdlib `http.HandleFunc`, `Handle`, `ServeMux`
  * chi: `r.Get/Post/Put/Delete`, `r.Route`, `r.Mount` (best-effort)
  * gin (optional MVP): `r.GET/POST/...`
* gRPC:

  * detect `RegisterXServer(grpcServer, impl)` and infer methods from generated interface
* CLI:

  * Cobra: `&cobra.Command{Run...}` plus `Execute()`
* main:

  * `func main()` always available as a fallback entrypoint

**Graph view**

* Expand/collapse nodes.
* Pin nodes.
* Show multiple outgoing calls but allow ranking.
* Basic search and “jump to symbol”.

**Filters**

* Stop at:

  * package boundary (only show within selected module)
  * layer boundary (config-defined layers)
  * I/O boundary (db/net/fs/bus)
* Hide:

  * stdlib calls
  * vendor calls
  * logging/metrics “noise” packages (configurable)
* Depth limit (e.g., 3, 5, 10)

**Classification**

* Tags on nodes and edges:

  * `io.db`, `io.net`, `io.fs`, `io.bus`
  * `layer.handler`, `layer.service`, `layer.domain`, `layer.store` (config-driven)
  * `test.covered` (best-effort mapping from `_test.go` references)

**Export**

* Shareable URL (query string encodes entrypoint + filters + pinned nodes).
* SVG export of current graph.

---

## Non-goals (Phase 1)

* Perfect dynamic dispatch resolution (interfaces, reflection, generics edge cases).
* Dataflow/taint.
* Runtime traces (that’s Phase 3 if ever).

---

## UX spec

### Layout

* **Left panel: Entrypoints**

  * Tabs: HTTP, gRPC, CLI, Cron, Main
  * Search: fuzzy by route/method or symbol name
  * Selecting entrypoint loads graph

* **Center: Graph**

  * Directed graph with root = entrypoint function
  * Node shows:

    * function name
    * package short path
    * tags (chips)
  * Edge shows:

    * callsite count (if multiple)
    * optional label: method call vs function call

* **Right panel: Inspector**

  * For selected node:

    * file + line
    * signature (trimmed)
    * tags and why (explainable heuristics)
    * “Top outgoing calls”
    * “Called by” (reverse edges, limited)
  * Filter controls (stop at I/O, hide noise, depth, etc.)

### Interactions (MVP)

* Click node: select + show inspector.
* Double click node: expand one level.
* Shift click: pin/unpin.
* Breadcrumb bar: Entry → … expands path focus.

---

## Technical design

### Architecture

* `flowlens index` (CLI indexer)
* `flowlens ui` (local server + React UI)
* Storage:

  * SQLite for detailed data
  * `index.json` for quick boot + entrypoint list + package metadata

### Why SQLite?

* Lets UI query on demand (expand node = fetch children edges).
* Avoid shipping huge JSON graphs.

---

## Data model (SQLite)

**symbols**

* `id` (int)
* `pkg_path` (text)
* `name` (text)
* `kind` (enum: func, method, type)
* `recv_type` (text nullable) // for methods
* `file` (text)
* `line` (int)
* `sig` (text nullable) // display signature
* indexes: `(pkg_path, name)`, `(file, line)`

**call_edges**

* `caller_id` (int)
* `callee_id` (int)
* `caller_file` (text)
* `caller_line` (int)
* `call_kind` (enum: static, interface, funcval, unknown)
* `count` (int default 1)

**entrypoints**

* `id` (int)
* `type` (enum: http, grpc, cobra, cron, main)
* `label` (text) // e.g. "POST /token" or "UserService/GetUser"
* `symbol_id` (int)
* `meta_json` (text) // method, route, receiver, etc.

**tags**

* `symbol_id` (int)
* `tag` (text) // e.g. io.db, layer.service
* `reason` (text) // short explanation for inspector

**packages**

* `pkg_path` (text)
* `module` (text nullable)
* `dir` (text)
* `layer` (text nullable)

---

## Indexing approach

### Step 1: Load packages

* Use `go/packages` with `NeedName | NeedFiles | NeedSyntax | NeedTypes | NeedTypesInfo | NeedModule`.
* Build mapping: file → package, symbol positions.

### Step 2: Build SSA

* Use `golang.org/x/tools/go/ssa` via `ssautil.AllPackages`.
* Build call graph from SSA:

  * Start with static calls: `ssa.CallCommon.StaticCallee()`
  * Handle interface calls as “unknown” edges unless pointer analysis enabled.

**MVP stance:**

* Prefer correctness for common direct calls.
* Label dynamic edges as such and let UI show “maybe”.

### Step 3: Entrypoint detection heuristics

* AST scan for known patterns:

  * `http.HandleFunc("/x", handler)`
  * chi: `r.Post("/x", handler)` etc
  * gin: `r.POST("/x", handler)`
* When handler is method value or wrapper, resolve best-effort:

  * follow simple function values (local var assigned a func)
  * otherwise point to the wrapper function

### Step 4: Tagging / classification

Configurable “tag rules”:

**I/O boundary rules**

* package prefixes:

  * db: `database/sql`, `github.com/jackc/pgx`, `gorm.io/`, etc
  * net: `net/http`, `google.golang.org/grpc`, `github.com/go-resty/resty`
  * fs: `os`, `io/ioutil`, `embed`
  * bus: `github.com/nats-io/`, `github.com/segmentio/kafka-go`
* name rules:

  * functions/methods on types named `*Store`, `*Repo`, `*Client` => tag `io.*` depending on package

**Layer rules**

* config maps package globs to layer:

  * `internal/*/handlers/** -> handler`
  * `internal/*/services/** -> service`
  * `internal/*/stores/** -> store`
  * `internal/*/domain/** -> domain`

**Pure-ish heuristic**

* If a function has no outgoing edges into any `io.*` tagged symbol, tag `pure-ish` (weak signal).
* If it mutates package-level vars, tag `impure` (AST scan).

### Step 5: Persist

* Insert symbols, edges, entrypoints, tags.

---

## Query API between UI and DB

Local server exposes endpoints:

* `GET /api/entrypoints?type=http&query=token`
* `GET /api/graph/root?entrypoint_id=123&depth=3&filters=...`
* `GET /api/graph/expand?symbol_id=456&depth=1&filters=...`
* `GET /api/symbol/:id`
* `GET /api/search?query=exchangeAuthorizationCode`

Filters are passed as a JSON blob:

```json
{
  "hideStdlib": true,
  "hideVendors": true,
  "stopAtIO": true,
  "stopAtPackagePrefix": ["internal/auth"],
  "maxDepth": 6,
  "noisePackages": ["log/slog", "go.uber.org/zap"]
}
```

---

## Performance plan (pragmatic)

* Index once, query many.
* Store only what UI needs:

  * symbol metadata
  * edges
  * entrypoints
  * tags
* For expansion:

  * only fetch outgoing edges for selected nodes.
* Cache in UI: already-expanded nodes.

---

## Edge cases and how we handle them (MVP honesty)

* Interface dispatch: show “dynamic call” edge to the interface method symbol; optionally also show known implementers if we can infer by simple type assertions or constructor wiring.
* Reflection: ignore.
* Generated code: allow `--exclude-dir` and default exclude `vendor`, `**/*_gen.go`, `**/*.pb.go` (configurable).

---

## Configuration file (`flowlens.yaml`)

Example:

```yaml
exclude:
  dirs: ["vendor", "third_party"]
  files_glob: ["**/*.pb.go", "**/*_gen.go"]

layers:
  handler: ["**/handlers/**", "**/http/**"]
  service: ["**/service/**", "**/services/**"]
  store:   ["**/store/**", "**/stores/**", "**/repo/**"]
  domain:  ["**/domain/**", "**/model/**"]

io_packages:
  db:  ["database/sql", "github.com/jackc/pgx", "gorm.io/*"]
  net: ["net/http", "google.golang.org/grpc", "github.com/go-resty/resty/*"]
  bus: ["github.com/nats-io/*", "github.com/segmentio/kafka-go"]

noise_packages:
  - "log/slog"
  - "go.uber.org/zap"
  - "github.com/prometheus/client_golang/*"
```
