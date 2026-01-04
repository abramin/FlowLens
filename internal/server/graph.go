package server

import (
	"strings"

	"github.com/abramin/flowlens/internal/store"
)

// GraphFilter specifies filters for graph traversal.
type GraphFilter struct {
	HideStdlib          bool     `json:"hideStdlib"`
	HideVendors         bool     `json:"hideVendors"`
	StopAtIO            bool     `json:"stopAtIO"`
	StopAtPackagePrefix []string `json:"stopAtPackagePrefix"`
	MaxDepth            int      `json:"maxDepth"`
	NoisePackages       []string `json:"noisePackages"`
}

// DefaultGraphFilter returns sensible defaults for graph filtering.
func DefaultGraphFilter() GraphFilter {
	return GraphFilter{
		HideStdlib:    false,
		HideVendors:   false,
		StopAtIO:      false,
		MaxDepth:      6,
		NoisePackages: []string{},
	}
}

// GraphNode represents a node in the graph response.
type GraphNode struct {
	ID       store.SymbolID   `json:"id"`
	Name     string           `json:"name"`
	PkgPath  string           `json:"pkg_path"`
	File     string           `json:"file"`
	Line     int              `json:"line"`
	Kind     store.SymbolKind `json:"kind"`
	RecvType string           `json:"recv_type,omitempty"`
	Sig      string           `json:"sig,omitempty"`
	Tags     []string         `json:"tags"`
	Expanded bool             `json:"expanded"`
	Depth    int              `json:"depth"`
}

// GraphEdge represents an edge in the graph response.
type GraphEdge struct {
	SourceID      store.SymbolID `json:"source_id"`
	TargetID      store.SymbolID `json:"target_id"`
	CallKind      store.CallKind `json:"call_kind"`
	CallsiteCount int            `json:"callsite_count"`
	CallerFile    string         `json:"caller_file,omitempty"`
	CallerLine    int            `json:"caller_line,omitempty"`
}

// GraphResponse is the response format for graph endpoints.
type GraphResponse struct {
	Nodes    []GraphNode `json:"nodes"`
	Edges    []GraphEdge `json:"edges"`
	RootID   store.SymbolID `json:"root_id"`
	MaxDepth int            `json:"max_depth"`
	Filtered int            `json:"filtered_count"`
}

// GraphBuilder builds graphs from the store with filtering.
type GraphBuilder struct {
	store   *store.Store
	filter  GraphFilter
	nodes   map[store.SymbolID]*GraphNode
	edges   []GraphEdge
	visited map[store.SymbolID]bool
	filtered int
}

// NewGraphBuilder creates a new graph builder.
func NewGraphBuilder(s *store.Store, filter GraphFilter) *GraphBuilder {
	return &GraphBuilder{
		store:   s,
		filter:  filter,
		nodes:   make(map[store.SymbolID]*GraphNode),
		edges:   []GraphEdge{},
		visited: make(map[store.SymbolID]bool),
	}
}

// BuildFromRoot builds a graph starting from a root symbol.
func (gb *GraphBuilder) BuildFromRoot(rootID store.SymbolID, depth int) (*GraphResponse, error) {
	// Clamp depth to maxDepth
	if gb.filter.MaxDepth > 0 && depth > gb.filter.MaxDepth {
		depth = gb.filter.MaxDepth
	}

	// Add the root node
	if err := gb.addNode(rootID, 0, true); err != nil {
		return nil, err
	}

	// Recursively expand
	if err := gb.expand(rootID, depth, 0); err != nil {
		return nil, err
	}

	return gb.buildResponse(rootID, depth), nil
}

// Expand expands a single node by the given depth.
func (gb *GraphBuilder) Expand(symbolID store.SymbolID, depth int) (*GraphResponse, error) {
	// Add the node if not already present
	if _, exists := gb.nodes[symbolID]; !exists {
		if err := gb.addNode(symbolID, 0, true); err != nil {
			return nil, err
		}
	}

	// Expand from this node
	if err := gb.expand(symbolID, depth, 0); err != nil {
		return nil, err
	}

	return gb.buildResponse(symbolID, depth), nil
}

// addNode adds a node to the graph if it passes filters.
func (gb *GraphBuilder) addNode(id store.SymbolID, depth int, expanded bool) error {
	if _, exists := gb.nodes[id]; exists {
		return nil
	}

	sym, err := gb.store.GetSymbolByID(id)
	if err != nil {
		return err
	}

	// Apply filters
	if gb.shouldFilter(sym) {
		gb.filtered++
		return nil
	}

	tags, _ := gb.store.GetSymbolTags(id)
	tagStrs := make([]string, len(tags))
	for i, t := range tags {
		tagStrs[i] = t.Tag
	}

	gb.nodes[id] = &GraphNode{
		ID:       sym.ID,
		Name:     sym.Name,
		PkgPath:  sym.PkgPath,
		File:     sym.File,
		Line:     sym.Line,
		Kind:     sym.Kind,
		RecvType: sym.RecvType,
		Sig:      sym.Sig,
		Tags:     tagStrs,
		Expanded: expanded,
		Depth:    depth,
	}

	return nil
}

// shouldFilter returns true if the symbol should be filtered out.
func (gb *GraphBuilder) shouldFilter(sym *store.Symbol) bool {
	// Filter stdlib
	if gb.filter.HideStdlib && isStdlib(sym.PkgPath) {
		return true
	}

	// Filter vendor packages
	if gb.filter.HideVendors && isVendor(sym.PkgPath) {
		return true
	}

	// Filter noise packages
	for _, noise := range gb.filter.NoisePackages {
		if matchPackagePattern(noise, sym.PkgPath) {
			return true
		}
	}

	return false
}

// shouldStopExpansion returns true if we should stop expanding at this node.
func (gb *GraphBuilder) shouldStopExpansion(sym *store.Symbol, tags []store.Tag) bool {
	// Stop at I/O if configured
	if gb.filter.StopAtIO {
		for _, t := range tags {
			if strings.HasPrefix(t.Tag, "io:") {
				return true
			}
		}
	}

	// Stop at specific package prefixes
	for _, prefix := range gb.filter.StopAtPackagePrefix {
		if strings.HasPrefix(sym.PkgPath, prefix) {
			return true
		}
	}

	return false
}

// expand recursively expands the graph from a symbol.
func (gb *GraphBuilder) expand(symbolID store.SymbolID, maxDepth int, currentDepth int) error {
	if currentDepth >= maxDepth {
		return nil
	}

	if gb.visited[symbolID] {
		return nil
	}
	gb.visited[symbolID] = true

	// Get symbol for stop-at checks
	sym, err := gb.store.GetSymbolByID(symbolID)
	if err != nil {
		return nil // Symbol not found, skip
	}

	tags, _ := gb.store.GetSymbolTags(symbolID)

	// Check if we should stop expansion
	if gb.shouldStopExpansion(sym, tags) {
		return nil
	}

	// Get callees
	callees, err := gb.store.GetCallees(symbolID)
	if err != nil {
		return err
	}

	// Aggregate edges by callee (sum up call counts)
	calleeEdges := make(map[store.SymbolID]*GraphEdge)
	for _, c := range callees {
		if gb.shouldFilterCallee(&c.Symbol) {
			gb.filtered++
			continue
		}

		if existing, ok := calleeEdges[c.Symbol.ID]; ok {
			existing.CallsiteCount += c.Count
		} else {
			calleeEdges[c.Symbol.ID] = &GraphEdge{
				SourceID:      symbolID,
				TargetID:      c.Symbol.ID,
				CallKind:      c.CallKind,
				CallsiteCount: c.Count,
				CallerFile:    c.CallerFile,
				CallerLine:    c.CallerLine,
			}
		}
	}

	// Add edges and nodes
	for calleeID, edge := range calleeEdges {
		gb.edges = append(gb.edges, *edge)

		// Add callee node
		if err := gb.addNode(calleeID, currentDepth+1, false); err != nil {
			continue
		}

		// Recursively expand
		if err := gb.expand(calleeID, maxDepth, currentDepth+1); err != nil {
			continue
		}
	}

	// Mark the source node as expanded
	if node, ok := gb.nodes[symbolID]; ok {
		node.Expanded = true
	}

	return nil
}

// shouldFilterCallee applies filters to a callee symbol.
func (gb *GraphBuilder) shouldFilterCallee(sym *store.Symbol) bool {
	return gb.shouldFilter(sym)
}

// buildResponse constructs the final response.
func (gb *GraphBuilder) buildResponse(rootID store.SymbolID, maxDepth int) *GraphResponse {
	nodes := make([]GraphNode, 0, len(gb.nodes))
	for _, node := range gb.nodes {
		nodes = append(nodes, *node)
	}

	return &GraphResponse{
		Nodes:    nodes,
		Edges:    gb.edges,
		RootID:   rootID,
		MaxDepth: maxDepth,
		Filtered: gb.filtered,
	}
}

// isStdlib checks if a package path is from the Go standard library.
func isStdlib(pkgPath string) bool {
	// Stdlib packages don't contain dots in the first path segment
	if pkgPath == "" {
		return false
	}
	firstSlash := strings.Index(pkgPath, "/")
	if firstSlash == -1 {
		// Single segment like "fmt", "os", etc.
		return !strings.Contains(pkgPath, ".")
	}
	firstSegment := pkgPath[:firstSlash]
	return !strings.Contains(firstSegment, ".")
}

// isVendor checks if a package path is from a vendor directory.
func isVendor(pkgPath string) bool {
	return strings.Contains(pkgPath, "/vendor/") || strings.HasPrefix(pkgPath, "vendor/")
}

// matchPackagePattern matches a package path against a pattern.
// Supports * as a wildcard for any suffix.
func matchPackagePattern(pattern, pkgPath string) bool {
	if pattern == pkgPath {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(pkgPath, prefix)
	}
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-2]
		return strings.HasPrefix(pkgPath, prefix+"/") || pkgPath == prefix
	}
	return false
}
