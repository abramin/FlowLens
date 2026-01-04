package index

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/abramin/flowlens/internal/store"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// CallGraphBuilder builds a call graph from SSA representation.
type CallGraphBuilder struct {
	loader       *Loader
	prog         *ssa.Program
	projectPkgs  map[string]bool // Set of project package paths (not dependencies)
	symbolCache  map[string]store.SymbolID
	onProgress   func(current, total int)
}

// NewCallGraphBuilder creates a new call graph builder.
func NewCallGraphBuilder(loader *Loader) *CallGraphBuilder {
	return &CallGraphBuilder{
		loader:      loader,
		projectPkgs: make(map[string]bool),
		symbolCache: make(map[string]store.SymbolID),
	}
}

// SetProgressCallback sets a callback for progress reporting.
func (b *CallGraphBuilder) SetProgressCallback(cb func(current, total int)) {
	b.onProgress = cb
}

// Build constructs SSA and extracts call edges.
func (b *CallGraphBuilder) Build() error {
	// Build project package set for filtering
	for _, pkg := range b.loader.pkgs {
		b.projectPkgs[pkg.PkgPath] = true
	}

	// Build SSA program from all loaded packages
	prog, _ := ssautil.AllPackages(b.loader.pkgs, ssa.SanityCheckFunctions)
	prog.Build()
	b.prog = prog

	return nil
}

// ExtractCallEdges extracts all call edges and persists them to the store.
func (b *CallGraphBuilder) ExtractCallEdges(st *store.Store) error {
	batch, err := st.BeginBatch()
	if err != nil {
		return fmt.Errorf("starting batch: %w", err)
	}
	defer batch.Rollback()

	// Pre-load symbol cache for faster lookups
	if err := b.loadSymbolCache(batch); err != nil {
		return fmt.Errorf("loading symbol cache: %w", err)
	}

	// Get all functions in the program
	allFuncs := ssautil.AllFunctions(b.prog)

	// Count functions for progress reporting
	var projectFuncs []*ssa.Function
	for fn := range allFuncs {
		if fn.Pkg == nil {
			continue
		}
		if !b.projectPkgs[fn.Pkg.Pkg.Path()] {
			continue
		}
		projectFuncs = append(projectFuncs, fn)
	}

	// Process each function
	edgeCount := 0
	for i, fn := range projectFuncs {
		if b.onProgress != nil && i%100 == 0 {
			b.onProgress(i, len(projectFuncs))
		}

		edges, err := b.extractFunctionCalls(fn)
		if err != nil {
			// Log but continue - some functions may have issues
			continue
		}

		for _, edge := range edges {
			if err := batch.InsertCallEdge(edge); err != nil {
				return fmt.Errorf("inserting call edge: %w", err)
			}
			edgeCount++
		}
	}

	if b.onProgress != nil {
		b.onProgress(len(projectFuncs), len(projectFuncs))
	}

	return batch.Commit()
}

// loadSymbolCache pre-loads all symbol IDs for faster lookups.
func (b *CallGraphBuilder) loadSymbolCache(batch *store.BatchTx) error {
	// We'll populate the cache lazily as we encounter symbols
	return nil
}

// extractFunctionCalls extracts all call edges from a function.
func (b *CallGraphBuilder) extractFunctionCalls(fn *ssa.Function) ([]*store.CallEdge, error) {
	callerID, err := b.resolveSymbolID(fn)
	if err != nil {
		return nil, err
	}
	if callerID == 0 {
		return nil, nil // Skip if we can't resolve the caller
	}

	var edges []*store.CallEdge

	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			edge := b.extractCallFromInstruction(fn, instr, callerID)
			if edge != nil {
				edges = append(edges, edge)
			}
		}
	}

	return edges, nil
}

// extractCallFromInstruction extracts a call edge from an SSA instruction.
func (b *CallGraphBuilder) extractCallFromInstruction(caller *ssa.Function, instr ssa.Instruction, callerID store.SymbolID) *store.CallEdge {
	var call *ssa.Call
	var callKind store.CallKind

	switch v := instr.(type) {
	case *ssa.Call:
		call = v
		callKind = store.CallKindStatic
	case *ssa.Go:
		// Goroutine call - wrap the call common
		call = &ssa.Call{Call: v.Call}
		callKind = store.CallKindGo
	case *ssa.Defer:
		// Deferred call - wrap the call common
		call = &ssa.Call{Call: v.Call}
		callKind = store.CallKindDefer
	default:
		return nil
	}

	common := call.Common()
	if common == nil {
		return nil
	}

	// Get call site position
	pos := b.loader.fset.Position(instr.Pos())
	if !pos.IsValid() {
		return nil
	}

	// Determine call kind and callee
	var calleeID store.SymbolID
	var err error

	// Try to get static callee first
	if callee := common.StaticCallee(); callee != nil {
		calleeID, err = b.resolveSymbolID(callee)
		if err != nil || calleeID == 0 {
			return nil
		}
		// Keep the original call kind (static, go, defer)
	} else if common.IsInvoke() {
		// Interface method call
		callKind = store.CallKindInterface
		calleeID, err = b.resolveInterfaceCall(common)
		if err != nil || calleeID == 0 {
			return nil
		}
	} else {
		// Function value call
		callKind = store.CallKindFuncval
		calleeID, err = b.resolveFuncvalCall(common)
		if err != nil || calleeID == 0 {
			// Mark as unknown if we can't resolve
			callKind = store.CallKindUnknown
			return nil // Skip unknown calls for now
		}
	}

	return &store.CallEdge{
		CallerID:   callerID,
		CalleeID:   calleeID,
		CallerFile: pos.Filename,
		CallerLine: pos.Line,
		CallKind:   callKind,
		Count:      1,
	}
}

// resolveSymbolID resolves an SSA function to its symbol ID.
func (b *CallGraphBuilder) resolveSymbolID(fn *ssa.Function) (store.SymbolID, error) {
	if fn == nil || fn.Pkg == nil {
		return 0, nil
	}

	pkgPath := fn.Pkg.Pkg.Path()

	// Only include calls to/from project packages
	if !b.projectPkgs[pkgPath] {
		return 0, nil
	}

	name := fn.Name()
	recvType := ""

	// Check if this is a method
	if fn.Signature.Recv() != nil {
		recvType = formatSSAReceiverType(fn.Signature.Recv().Type())
	}

	// Check cache first
	cacheKey := fmt.Sprintf("%s.%s.%s", pkgPath, name, recvType)
	if id, ok := b.symbolCache[cacheKey]; ok {
		return id, nil
	}

	// Look up in database - we need to use the store directly
	// This is a limitation - we'll need to look it up later
	return 0, fmt.Errorf("symbol not in cache: %s", cacheKey)
}

// resolveInterfaceCall attempts to resolve an interface method call.
func (b *CallGraphBuilder) resolveInterfaceCall(common *ssa.CallCommon) (store.SymbolID, error) {
	// For interface calls, we can't statically determine the concrete implementation
	// We could try to find all implementations, but for MVP we mark these as unresolved
	return 0, nil
}

// resolveFuncvalCall attempts to resolve a function value call.
func (b *CallGraphBuilder) resolveFuncvalCall(common *ssa.CallCommon) (store.SymbolID, error) {
	// Try to trace the function value back to its definition
	// This is complex and often impossible statically
	// For MVP, we return nil
	return 0, nil
}

// formatSSAReceiverType formats an SSA receiver type as a string.
func formatSSAReceiverType(t types.Type) string {
	switch typ := t.(type) {
	case *types.Pointer:
		return "*" + formatSSAReceiverType(typ.Elem())
	case *types.Named:
		return typ.Obj().Name()
	default:
		return types.TypeString(t, nil)
	}
}

// CallGraphResult holds the results of call graph construction.
type CallGraphResult struct {
	EdgeCount     int
	StaticCalls   int
	InterfaceCalls int
	DeferCalls    int
	GoCalls       int
	UnknownCalls  int
}

// ExtractCallEdgesWithStore extracts call edges using the store directly for lookups.
func (b *CallGraphBuilder) ExtractCallEdgesWithStore(st *store.Store) (*CallGraphResult, error) {
	batch, err := st.BeginBatch()
	if err != nil {
		return nil, fmt.Errorf("starting batch: %w", err)
	}
	defer batch.Rollback()

	result := &CallGraphResult{}

	// Get all functions in the program
	allFuncs := ssautil.AllFunctions(b.prog)

	// Filter to project functions
	var projectFuncs []*ssa.Function
	for fn := range allFuncs {
		if fn.Pkg == nil {
			continue
		}
		if !b.projectPkgs[fn.Pkg.Pkg.Path()] {
			continue
		}
		projectFuncs = append(projectFuncs, fn)
	}

	fmt.Printf("Processing %d project functions...\n", len(projectFuncs))

	// Process each function
	for i, fn := range projectFuncs {
		if b.onProgress != nil && i%100 == 0 {
			b.onProgress(i, len(projectFuncs))
		}

		callerID, err := b.lookupSymbolID(batch, fn)
		if err != nil || callerID == 0 {
			continue
		}

		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				edge, kind := b.extractCallEdge(batch, fn, instr, callerID)
				if edge != nil {
					if err := batch.InsertCallEdge(edge); err != nil {
						return nil, fmt.Errorf("inserting call edge: %w", err)
					}
					result.EdgeCount++

					switch kind {
					case store.CallKindStatic:
						result.StaticCalls++
					case store.CallKindInterface:
						result.InterfaceCalls++
					case store.CallKindDefer:
						result.DeferCalls++
					case store.CallKindGo:
						result.GoCalls++
					default:
						result.UnknownCalls++
					}
				}
			}
		}
	}

	if b.onProgress != nil {
		b.onProgress(len(projectFuncs), len(projectFuncs))
	}

	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("committing batch: %w", err)
	}

	return result, nil
}

// lookupSymbolID looks up a symbol ID from the database.
func (b *CallGraphBuilder) lookupSymbolID(batch *store.BatchTx, fn *ssa.Function) (store.SymbolID, error) {
	if fn == nil || fn.Pkg == nil {
		return 0, nil
	}

	pkgPath := fn.Pkg.Pkg.Path()

	// Only include calls to/from project packages
	if !b.projectPkgs[pkgPath] {
		return 0, nil
	}

	name := fn.Name()
	recvType := ""

	// Check if this is a method
	if fn.Signature.Recv() != nil {
		recvType = formatSSAReceiverType(fn.Signature.Recv().Type())
	}

	// Check cache first
	cacheKey := fmt.Sprintf("%s.%s.%s", pkgPath, name, recvType)
	if id, ok := b.symbolCache[cacheKey]; ok {
		return id, nil
	}

	// Look up in database
	id, err := batch.GetSymbolID(pkgPath, name, recvType)
	if err != nil {
		return 0, nil // Symbol not found - might be synthetic
	}

	// Cache for future lookups
	b.symbolCache[cacheKey] = id
	return id, nil
}

// extractCallEdge extracts a call edge from an instruction.
func (b *CallGraphBuilder) extractCallEdge(batch *store.BatchTx, caller *ssa.Function, instr ssa.Instruction, callerID store.SymbolID) (*store.CallEdge, store.CallKind) {
	var common *ssa.CallCommon
	var baseKind store.CallKind

	switch v := instr.(type) {
	case *ssa.Call:
		common = v.Common()
		baseKind = store.CallKindStatic
	case *ssa.Go:
		common = v.Common()
		baseKind = store.CallKindGo
	case *ssa.Defer:
		common = v.Common()
		baseKind = store.CallKindDefer
	default:
		return nil, ""
	}

	if common == nil {
		return nil, ""
	}

	// Get call site position
	pos := b.loader.fset.Position(instr.Pos())
	if !pos.IsValid() {
		return nil, ""
	}

	// Determine callee
	var calleeID store.SymbolID
	var callKind store.CallKind

	if callee := common.StaticCallee(); callee != nil {
		// Static call
		var err error
		calleeID, err = b.lookupSymbolID(batch, callee)
		if err != nil || calleeID == 0 {
			return nil, ""
		}
		callKind = baseKind
	} else if common.IsInvoke() {
		// Interface method call
		callKind = store.CallKindInterface
		// For interface calls, try to find the method in known types
		calleeID = b.resolveInterfaceMethod(batch, common)
		if calleeID == 0 {
			return nil, "" // Can't resolve - skip for now
		}
	} else {
		// Function value - try to trace it
		callKind = store.CallKindFuncval
		calleeID = b.traceFuncValue(batch, common)
		if calleeID == 0 {
			return nil, "" // Can't resolve - skip
		}
	}

	return &store.CallEdge{
		CallerID:   callerID,
		CalleeID:   calleeID,
		CallerFile: pos.Filename,
		CallerLine: pos.Line,
		CallKind:   callKind,
		Count:      1,
	}, callKind
}

// resolveInterfaceMethod tries to resolve an interface method call.
func (b *CallGraphBuilder) resolveInterfaceMethod(batch *store.BatchTx, common *ssa.CallCommon) store.SymbolID {
	// For MVP, we don't try to resolve interface calls
	// A future enhancement could find all implementations
	return 0
}

// traceFuncValue tries to trace a function value to its definition.
func (b *CallGraphBuilder) traceFuncValue(batch *store.BatchTx, common *ssa.CallCommon) store.SymbolID {
	// Try to trace simple cases like passing a function directly
	value := common.Value
	if value == nil {
		return 0
	}

	// Check if it's a MakeClosure (anonymous function)
	if mc, ok := value.(*ssa.MakeClosure); ok {
		if fn := mc.Fn.(*ssa.Function); fn != nil {
			id, _ := b.lookupSymbolID(batch, fn)
			return id
		}
	}

	// Check if it's a direct function reference
	if fn, ok := value.(*ssa.Function); ok {
		id, _ := b.lookupSymbolID(batch, fn)
		return id
	}

	return 0
}

// BuildAndExtract is a convenience method that builds SSA and extracts call edges.
func BuildAndExtract(loader *Loader, st *store.Store, onProgress func(current, total int)) (*CallGraphResult, error) {
	builder := NewCallGraphBuilder(loader)
	if onProgress != nil {
		builder.SetProgressCallback(onProgress)
	}

	if err := builder.Build(); err != nil {
		return nil, fmt.Errorf("building SSA: %w", err)
	}

	result, err := builder.ExtractCallEdgesWithStore(st)
	if err != nil {
		return nil, fmt.Errorf("extracting call edges: %w", err)
	}

	return result, nil
}

// GetSSAProgram returns the SSA program for testing/debugging.
func (b *CallGraphBuilder) GetSSAProgram() *ssa.Program {
	return b.prog
}

// AllPackages returns all packages including dependencies for SSA.
func AllPackages(pkgs []*packages.Package) []*packages.Package {
	seen := make(map[string]bool)
	var all []*packages.Package

	var visit func(*packages.Package)
	visit = func(pkg *packages.Package) {
		if seen[pkg.PkgPath] {
			return
		}
		seen[pkg.PkgPath] = true
		all = append(all, pkg)
		for _, imp := range pkg.Imports {
			visit(imp)
		}
	}

	for _, pkg := range pkgs {
		visit(pkg)
	}

	return all
}

// isProjectPackage checks if a package is part of the project (not a dependency).
func isProjectPackage(pkg *packages.Package, projectModule string) bool {
	if pkg.Module == nil {
		return false
	}
	return pkg.Module.Path == projectModule || strings.HasPrefix(pkg.PkgPath, projectModule)
}

// positionString returns a string representation of a token position.
func positionString(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return fmt.Sprintf("%s:%d", p.Filename, p.Line)
}
