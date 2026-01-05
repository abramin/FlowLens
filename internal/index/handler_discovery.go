package index

import (
	"encoding/json"
	"fmt"
	"go/types"
	"strings"

	"github.com/abramin/flowlens/internal/store"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// HandlerDiscovery finds HTTP handlers by function signature using SSA.
// This complements the AST-based router detection in entrypoints.go by finding
// handlers that match HTTP handler signatures but aren't registered via parsed routers.
type HandlerDiscovery struct {
	loader      *Loader
	prog        *ssa.Program
	projectPkgs map[string]bool
}

// NewHandlerDiscovery creates a handler discoverer.
func NewHandlerDiscovery(loader *Loader, prog *ssa.Program) *HandlerDiscovery {
	projectPkgs := make(map[string]bool)
	for _, pkg := range loader.pkgs {
		projectPkgs[pkg.PkgPath] = true
	}
	return &HandlerDiscovery{
		loader:      loader,
		prog:        prog,
		projectPkgs: projectPkgs,
	}
}

// DiscoveredHandler represents an HTTP handler found by signature matching.
type DiscoveredHandler struct {
	SymbolID     store.SymbolID
	ReceiverType string // e.g., "*Handler"
	MethodName   string // e.g., "Authorize"
	SignatureType string // "stdlib", "gin", "echo", "chi"
	PkgPath      string
}

// DiscoverResult holds the results of handler discovery.
type DiscoverResult struct {
	StdlibCount int
	GinCount    int
	EchoCount   int
	ChiCount    int
	TotalCount  int
	Handlers    []DiscoveredHandler
}

// Discover scans all SSA functions for HTTP handler signatures.
func (hd *HandlerDiscovery) Discover(batch *store.BatchTx) (*DiscoverResult, error) {
	result := &DiscoverResult{}

	// Get existing HTTP entrypoint symbol IDs to avoid duplicates
	existingSymbols := make(map[store.SymbolID]bool)
	existing, err := hd.getExistingHTTPEntrypoints(batch)
	if err != nil {
		return nil, fmt.Errorf("getting existing entrypoints: %w", err)
	}
	for _, id := range existing {
		existingSymbols[id] = true
	}

	// Iterate all functions in the SSA program
	allFuncs := ssautil.AllFunctions(hd.prog)
	for fn := range allFuncs {
		if fn.Pkg == nil {
			continue
		}

		pkgPath := fn.Pkg.Pkg.Path()

		// Only process project packages
		if !hd.projectPkgs[pkgPath] {
			continue
		}

		// Check if function matches an HTTP handler signature
		sigType := hd.matchHTTPHandlerSignature(fn)
		if sigType == "" {
			continue
		}

		// Get receiver type and method name
		recvType := ""
		if fn.Signature.Recv() != nil {
			recvType = formatSSAReceiverType(fn.Signature.Recv().Type())
		}

		// Look up the symbol ID
		symbolID, err := batch.GetSymbolID(pkgPath, fn.Name(), recvType)
		if err != nil {
			continue // Symbol not found in DB
		}

		// Skip if already registered as an entrypoint
		if existingSymbols[symbolID] {
			continue
		}

		// Create entrypoint
		handler := DiscoveredHandler{
			SymbolID:      symbolID,
			ReceiverType:  recvType,
			MethodName:    fn.Name(),
			SignatureType: sigType,
			PkgPath:       pkgPath,
		}

		// Build label
		label := handler.MethodName
		if recvType != "" {
			label = fmt.Sprintf("(%s).%s", recvType, handler.MethodName)
		}

		// Create metadata
		meta := HTTPMeta{
			Method: "ANY", // We don't know the HTTP method without router parsing
			Path:   "",    // Unknown path - discovered by signature
		}
		metaJSON, _ := json.Marshal(meta)

		// Insert entrypoint
		ep := &store.Entrypoint{
			Type:            store.EntrypointHTTP,
			Label:           label,
			SymbolID:        symbolID,
			MetaJSON:        string(metaJSON),
			DiscoveryMethod: "signature",
		}

		if err := batch.InsertEntrypoint(ep); err != nil {
			continue // Skip on error
		}

		result.Handlers = append(result.Handlers, handler)

		// Update counts
		switch sigType {
		case "stdlib":
			result.StdlibCount++
		case "gin":
			result.GinCount++
		case "echo":
			result.EchoCount++
		case "chi":
			result.ChiCount++
		}
		result.TotalCount++
	}

	return result, nil
}

// matchHTTPHandlerSignature checks if an SSA function matches known HTTP handler patterns.
// Returns the signature type ("stdlib", "gin", "echo", "chi") or empty string if no match.
func (hd *HandlerDiscovery) matchHTTPHandlerSignature(fn *ssa.Function) string {
	if fn == nil || fn.Signature == nil {
		return ""
	}

	// Skip init functions and unexported functions
	name := fn.Name()
	if name == "init" || !isExported(name) {
		return ""
	}

	params := fn.Signature.Params()
	if params == nil {
		return ""
	}

	numParams := params.Len()

	// Pattern 1: stdlib - func(http.ResponseWriter, *http.Request)
	// Pattern 2: stdlib with context - func(context.Context, http.ResponseWriter, *http.Request)
	if numParams >= 2 {
		// Check last two params for ResponseWriter and *Request
		if numParams >= 2 {
			if hd.matchStdlibPattern(params, numParams) {
				return "stdlib"
			}
		}
	}

	// Pattern 3: gin - func(*gin.Context)
	if numParams == 1 {
		if hd.isGinContext(params.At(0).Type()) {
			return "gin"
		}
	}

	// Pattern 4: echo - func(echo.Context) error
	if numParams == 1 && fn.Signature.Results() != nil && fn.Signature.Results().Len() == 1 {
		if hd.isEchoContext(params.At(0).Type()) {
			return "echo"
		}
	}

	// Pattern 5: chi - func(http.ResponseWriter, *http.Request) (same as stdlib)
	// Already covered by stdlib pattern

	return ""
}

// matchStdlibPattern checks for func(w http.ResponseWriter, r *http.Request)
// or func(ctx context.Context, w http.ResponseWriter, r *http.Request)
func (hd *HandlerDiscovery) matchStdlibPattern(params *types.Tuple, numParams int) bool {
	// Try without context first (last 2 params)
	if numParams >= 2 {
		wIdx := numParams - 2
		rIdx := numParams - 1
		if hd.isHTTPResponseWriter(params.At(wIdx).Type()) &&
			hd.isHTTPRequest(params.At(rIdx).Type()) {
			// If there's a third param, it should be context.Context
			if numParams == 3 {
				if !hd.isContext(params.At(0).Type()) {
					return false
				}
			} else if numParams > 3 {
				return false // Too many params
			}
			return true
		}
	}
	return false
}

// isHTTPResponseWriter checks if the type is http.ResponseWriter.
func (hd *HandlerDiscovery) isHTTPResponseWriter(t types.Type) bool {
	// http.ResponseWriter is an interface type
	if iface, ok := t.Underlying().(*types.Interface); ok {
		// Check if it has the expected methods: Header, Write, WriteHeader
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			if m.Name() == "Header" || m.Name() == "Write" || m.Name() == "WriteHeader" {
				// This is a heuristic - checking method names
				return true
			}
		}
	}

	// Also check by name
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj != nil && obj.Pkg() != nil {
			if obj.Pkg().Path() == "net/http" && obj.Name() == "ResponseWriter" {
				return true
			}
		}
	}

	return false
}

// isHTTPRequest checks if the type is *http.Request.
func (hd *HandlerDiscovery) isHTTPRequest(t types.Type) bool {
	// Should be a pointer to http.Request
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}

	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	return obj.Pkg().Path() == "net/http" && obj.Name() == "Request"
}

// isContext checks if the type is context.Context.
func (hd *HandlerDiscovery) isContext(t types.Type) bool {
	if iface, ok := t.Underlying().(*types.Interface); ok {
		// context.Context has methods: Deadline, Done, Err, Value
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			if m.Name() == "Deadline" || m.Name() == "Done" {
				return true
			}
		}
	}

	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj != nil && obj.Pkg() != nil {
			if obj.Pkg().Path() == "context" && obj.Name() == "Context" {
				return true
			}
		}
	}

	return false
}

// isGinContext checks if the type is *gin.Context.
func (hd *HandlerDiscovery) isGinContext(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}

	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	return strings.HasSuffix(obj.Pkg().Path(), "github.com/gin-gonic/gin") && obj.Name() == "Context"
}

// isEchoContext checks if the type is echo.Context (interface).
func (hd *HandlerDiscovery) isEchoContext(t types.Type) bool {
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if obj != nil && obj.Pkg() != nil {
			return strings.HasSuffix(obj.Pkg().Path(), "github.com/labstack/echo") && obj.Name() == "Context"
		}
	}

	// Echo Context is an interface, so also check underlying
	if iface, ok := t.Underlying().(*types.Interface); ok {
		// Check for echo-specific methods
		for i := 0; i < iface.NumMethods(); i++ {
			m := iface.Method(i)
			if m.Name() == "Request" || m.Name() == "Response" || m.Name() == "Path" {
				// Could be echo.Context
				return true
			}
		}
	}

	return false
}

// getExistingHTTPEntrypoints returns the symbol IDs of existing HTTP entrypoints.
func (hd *HandlerDiscovery) getExistingHTTPEntrypoints(batch *store.BatchTx) ([]store.SymbolID, error) {
	return batch.GetHTTPEntrypointSymbolIDs()
}

// isExported checks if a Go identifier is exported (starts with uppercase).
func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}
