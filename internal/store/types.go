package store

// SymbolID is a type-safe identifier for symbols.
type SymbolID int64

// PackageID is a type-safe identifier for packages (using pkg_path as key).
type PackageID string

// EntrypointID is a type-safe identifier for entrypoints.
type EntrypointID int64

// SymbolKind represents the kind of a symbol.
type SymbolKind string

const (
	SymbolKindFunc   SymbolKind = "func"
	SymbolKindMethod SymbolKind = "method"
	SymbolKindType   SymbolKind = "type"
	SymbolKindVar    SymbolKind = "var"
	SymbolKindConst  SymbolKind = "const"
)

// CallKind represents how a call is made.
type CallKind string

const (
	CallKindStatic    CallKind = "static"    // Direct function call
	CallKindInterface CallKind = "interface" // Call through interface
	CallKindDefer     CallKind = "defer"     // Deferred call
	CallKindGo        CallKind = "go"        // Goroutine call
	CallKindUnknown   CallKind = "unknown"   // Dynamic dispatch, can't resolve
)

// EntrypointType represents the type of entrypoint.
type EntrypointType string

const (
	EntrypointHTTP EntrypointType = "http"
	EntrypointGRPC EntrypointType = "grpc"
	EntrypointCLI  EntrypointType = "cli"
	EntrypointMain EntrypointType = "main"
)

// Symbol represents a Go symbol (function, method, type, etc.).
type Symbol struct {
	ID       SymbolID   `json:"id"`
	PkgPath  string     `json:"pkg_path"`
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	RecvType string     `json:"recv_type,omitempty"` // For methods, the receiver type
	File     string     `json:"file"`
	Line     int        `json:"line"`
	Sig      string     `json:"sig,omitempty"` // Function signature
}

// Package represents a Go package.
type Package struct {
	PkgPath string `json:"pkg_path"`
	Module  string `json:"module,omitempty"`
	Dir     string `json:"dir"`
	Layer   string `json:"layer,omitempty"` // handler, service, store, domain, or empty
}

// CallEdge represents a call from one symbol to another.
type CallEdge struct {
	CallerID   SymbolID `json:"caller_id"`
	CalleeID   SymbolID `json:"callee_id"`
	CallerFile string   `json:"caller_file"`
	CallerLine int      `json:"caller_line"`
	CallKind   CallKind `json:"call_kind"`
	Count      int      `json:"count"` // Number of times this call appears
}

// Entrypoint represents a program entrypoint.
type Entrypoint struct {
	ID       EntrypointID   `json:"id"`
	Type     EntrypointType `json:"type"`
	Label    string         `json:"label"` // Human-readable label, e.g., "GET /api/users"
	SymbolID SymbolID       `json:"symbol_id"`
	MetaJSON string         `json:"meta_json,omitempty"` // Additional metadata as JSON
}

// Tag represents a tag on a symbol.
type Tag struct {
	SymbolID SymbolID `json:"symbol_id"`
	Tag      string   `json:"tag"`    // e.g., "io:db", "pure", "layer:handler"
	Reason   string   `json:"reason"` // Why this tag was applied
}
