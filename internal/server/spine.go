package server

import (
	"sort"
	"strings"

	"github.com/abramin/flowlens/internal/store"
)

// SpineNode represents a node in the call spine visualization.
type SpineNode struct {
	ID          store.SymbolID `json:"id"`
	Name        string         `json:"name"`
	PkgPath     string         `json:"pkg_path"`
	RecvType    string         `json:"recv_type,omitempty"`
	File        string         `json:"file"`
	Line        int            `json:"line"`
	Tags        []string       `json:"tags"`
	Depth       int            `json:"depth"`
	IsMainPath  bool           `json:"is_main_path"`
	BranchBadge *BranchBadge   `json:"branch_badge,omitempty"`
	Layer       string         `json:"layer,omitempty"` // handler, service, store, domain
}

// BranchBadge summarizes collapsed branch calls from a spine node.
type BranchBadge struct {
	CallCount    int      `json:"call_count"`    // Number of collapsed calls ("+4 calls")
	CollapsedIDs []int64  `json:"collapsed_ids"` // IDs of collapsed nodes for expansion
	Labels       []string `json:"labels"`        // Brief labels for tooltip
}

// SpineResponse is the response for call spine visualization.
type SpineResponse struct {
	Nodes         []SpineNode `json:"nodes"`
	MainPath      []int64     `json:"main_path"`       // Ordered node IDs forming spine
	TotalNodes    int         `json:"total_nodes"`     // Including collapsed
	CollapsedCount int        `json:"collapsed_count"`
}

// SpineBuilder builds a call spine from the call graph.
type SpineBuilder struct {
	store  *store.Store
	filter GraphFilter
}

// NewSpineBuilder creates a new spine builder.
func NewSpineBuilder(st *store.Store, filter GraphFilter) *SpineBuilder {
	return &SpineBuilder{
		store:  st,
		filter: filter,
	}
}

// ScoredCallee represents a callee with a score for main path selection.
type ScoredCallee struct {
	ID       store.SymbolID
	Symbol   *store.Symbol
	CallKind store.CallKind
	Score    int
	Tags     []string
}

// BuildSpine constructs the call spine from a root symbol.
func (sb *SpineBuilder) BuildSpine(rootID store.SymbolID, maxDepth int) (*SpineResponse, error) {
	if maxDepth <= 0 {
		maxDepth = 10
	}

	// Load all callees recursively to build the call graph
	allCallees := make(map[store.SymbolID][]store.CalleeInfo)
	visited := make(map[store.SymbolID]bool)

	if err := sb.loadCalleesRecursive(rootID, maxDepth, 0, allCallees, visited); err != nil {
		return nil, err
	}

	// Determine main path using scoring heuristics
	mainPath := sb.determineMainPath(rootID, allCallees, maxDepth)

	// Build spine nodes with branch badges for non-main-path calls
	mainPathSet := make(map[store.SymbolID]bool)
	for _, id := range mainPath {
		mainPathSet[store.SymbolID(id)] = true
	}

	var nodes []SpineNode
	totalNodes := 0
	collapsedCount := 0

	for i, id := range mainPath {
		symID := store.SymbolID(id)
		sym, err := sb.store.GetSymbolByID(symID)
		if err != nil {
			continue
		}

		tags, _ := sb.store.GetSymbolTags(symID)
		tagStrs := make([]string, len(tags))
		for j, t := range tags {
			tagStrs[j] = t.Tag
		}

		node := SpineNode{
			ID:         symID,
			Name:       sym.Name,
			PkgPath:    sym.PkgPath,
			RecvType:   sym.RecvType,
			File:       sym.File,
			Line:       sym.Line,
			Tags:       tagStrs,
			Depth:      i,
			IsMainPath: true,
			Layer:      extractLayer(tagStrs),
		}

		// Build branch badge for non-main-path callees
		callees := allCallees[symID]
		var collapsedIDs []int64
		var collapsedLabels []string

		for _, callee := range callees {
			if !mainPathSet[callee.Symbol.ID] && !sb.shouldFilterCallee(&callee.Symbol) {
				collapsedIDs = append(collapsedIDs, int64(callee.Symbol.ID))
				label := callee.Symbol.Name
				if callee.Symbol.RecvType != "" {
					label = "(" + callee.Symbol.RecvType + ")." + label
				}
				collapsedLabels = append(collapsedLabels, label)
				collapsedCount++
			}
			totalNodes++
		}

		if len(collapsedIDs) > 0 {
			node.BranchBadge = &BranchBadge{
				CallCount:    len(collapsedIDs),
				CollapsedIDs: collapsedIDs,
				Labels:       collapsedLabels,
			}
		}

		nodes = append(nodes, node)
	}

	return &SpineResponse{
		Nodes:          nodes,
		MainPath:       mainPath,
		TotalNodes:     totalNodes + len(mainPath),
		CollapsedCount: collapsedCount,
	}, nil
}

// loadCalleesRecursive loads callees recursively up to maxDepth.
func (sb *SpineBuilder) loadCalleesRecursive(
	symbolID store.SymbolID,
	maxDepth int,
	currentDepth int,
	allCallees map[store.SymbolID][]store.CalleeInfo,
	visited map[store.SymbolID]bool,
) error {
	if currentDepth >= maxDepth {
		return nil
	}

	if visited[symbolID] {
		return nil
	}
	visited[symbolID] = true

	callees, err := sb.store.GetCallees(symbolID)
	if err != nil {
		return nil // Ignore errors, just skip
	}

	// Filter callees
	var filteredCallees []store.CalleeInfo
	for _, c := range callees {
		if !sb.shouldFilterCallee(&c.Symbol) {
			filteredCallees = append(filteredCallees, c)
		}
	}

	allCallees[symbolID] = filteredCallees

	// Recurse into callees
	for _, c := range filteredCallees {
		if err := sb.loadCalleesRecursive(c.Symbol.ID, maxDepth, currentDepth+1, allCallees, visited); err != nil {
			return err
		}
	}

	return nil
}

// shouldFilterCallee checks if a callee should be filtered out.
func (sb *SpineBuilder) shouldFilterCallee(sym *store.Symbol) bool {
	// Filter stdlib
	if sb.filter.HideStdlib && isStdlib(sym.PkgPath) {
		return true
	}

	// Filter vendor packages
	if sb.filter.HideVendors && isVendor(sym.PkgPath) {
		return true
	}

	// Filter cmd/* packages
	if sb.filter.HideCmdMain && isCmdPackage(sym.PkgPath) {
		return true
	}

	// Filter noise packages
	for _, noise := range sb.filter.NoisePackages {
		if matchPackagePattern(noise, sym.PkgPath) {
			return true
		}
	}

	return false
}

// determineMainPath uses scoring heuristics to find the "happy path".
func (sb *SpineBuilder) determineMainPath(
	rootID store.SymbolID,
	allCallees map[store.SymbolID][]store.CalleeInfo,
	maxDepth int,
) []int64 {
	// Get root symbol for package context
	rootSym, err := sb.store.GetSymbolByID(rootID)
	if err != nil {
		return []int64{int64(rootID)}
	}
	rootPkg := rootSym.PkgPath

	// Greedy path selection with scoring
	path := []int64{int64(rootID)}
	current := rootID
	visited := make(map[store.SymbolID]bool)
	visited[rootID] = true

	for len(path) < maxDepth {
		callees := allCallees[current]
		if len(callees) == 0 {
			break
		}

		// Score each callee
		scored := sb.scoreCallees(current, callees, rootPkg, visited)
		if len(scored) == 0 {
			break
		}

		// Sort by score descending
		sort.Slice(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})

		// Pick the best unvisited callee
		var best *ScoredCallee
		for i := range scored {
			if !visited[scored[i].ID] {
				best = &scored[i]
				break
			}
		}

		if best == nil {
			break
		}

		visited[best.ID] = true
		path = append(path, int64(best.ID))
		current = best.ID
	}

	return path
}

// scoreCallees assigns scores to callees for main path selection.
func (sb *SpineBuilder) scoreCallees(
	callerID store.SymbolID,
	callees []store.CalleeInfo,
	rootPkg string,
	visited map[store.SymbolID]bool,
) []ScoredCallee {
	var scored []ScoredCallee

	for _, c := range callees {
		if visited[c.Symbol.ID] {
			continue
		}

		score := 0

		// Heuristic 1: Same package bonus (business logic likely in same module)
		if c.Symbol.PkgPath == rootPkg {
			score += 10
		} else if strings.HasPrefix(c.Symbol.PkgPath, strings.Split(rootPkg, "/")[0]) {
			score += 5 // Same module/org
		}

		// Heuristic 2: Service/domain layer bonus
		tagStrs := make([]string, len(c.Tags))
		for i, t := range c.Tags {
			tagStrs[i] = t.Tag
		}
		for _, tag := range tagStrs {
			switch tag {
			case "layer:service":
				score += 8
			case "layer:domain":
				score += 7
			case "layer:store":
				score += 6
			case "layer:handler":
				score += 5
			}
		}

		// Heuristic 3: Logging/telemetry penalty
		if isLoggingPackage(c.Symbol.PkgPath) {
			score -= 15
		}

		// Heuristic 4: Wiring function penalty
		if sb.filter.CollapseWiring && isWiringFunction(c.Symbol.Name) {
			score -= 10
		}

		// Heuristic 5: Error construction penalty
		if isErrorConstruction(c.Symbol.Name, c.Symbol.PkgPath) {
			score -= 20
		}

		// Heuristic 6: Method calls on receiver types are likely business logic
		if c.Symbol.RecvType != "" {
			score += 3
		}

		// Heuristic 7: Interface calls are often abstraction boundaries
		if c.CallKind == store.CallKindInterface {
			score += 2
		}

		scored = append(scored, ScoredCallee{
			ID:       c.Symbol.ID,
			Symbol:   &c.Symbol,
			CallKind: c.CallKind,
			Score:    score,
			Tags:     tagStrs,
		})
	}

	return scored
}

// extractLayer extracts the layer tag from tags.
func extractLayer(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "layer:") {
			return strings.TrimPrefix(tag, "layer:")
		}
	}
	return ""
}

// isLoggingPackage checks if a package is a logging/telemetry package.
func isLoggingPackage(pkgPath string) bool {
	loggingPatterns := []string{
		"log",
		"slog",
		"zap",
		"logrus",
		"zerolog",
		"telemetry",
		"metrics",
		"tracing",
		"opentelemetry",
		"prometheus",
	}

	lower := strings.ToLower(pkgPath)
	for _, pattern := range loggingPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// isErrorConstruction checks if a function constructs errors.
func isErrorConstruction(name, pkgPath string) bool {
	if pkgPath == "errors" && (name == "New" || name == "Wrap" || name == "Wrapf") {
		return true
	}
	if pkgPath == "fmt" && (name == "Errorf" || name == "Sprintf") {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, "error") || strings.HasPrefix(lower, "error")
}
