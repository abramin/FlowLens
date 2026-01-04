package index

import (
	"fmt"
	"strings"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
)

// Tagger applies tags to symbols based on I/O boundaries, layers, and purity heuristics.
type Tagger struct {
	cfg   *config.Config
	store *store.Store
}

// TagResult holds the results of the tagging operation.
type TagResult struct {
	IOTags     int // Number of I/O boundary tags applied
	LayerTags  int // Number of layer tags applied
	PurityTags int // Number of purity tags applied
	TotalTags  int // Total tags applied
}

// NewTagger creates a new tagger.
func NewTagger(cfg *config.Config, st *store.Store) *Tagger {
	return &Tagger{
		cfg:   cfg,
		store: st,
	}
}

// Tag applies all tags to symbols and returns the result.
func (t *Tagger) Tag() (*TagResult, error) {
	result := &TagResult{}

	// Start a batch transaction
	batch, err := t.store.BeginBatch()
	if err != nil {
		return nil, fmt.Errorf("starting batch: %w", err)
	}
	defer batch.Rollback()

	// Get all symbols
	symbols, err := t.store.GetAllSymbolsForTagging()
	if err != nil {
		return nil, fmt.Errorf("getting symbols: %w", err)
	}

	// Get package imports (which packages call into which other packages)
	pkgImports, err := t.store.GetPackageImports()
	if err != nil {
		return nil, fmt.Errorf("getting package imports: %w", err)
	}

	// Build a map of package -> IO categories it imports
	pkgIOCategories := t.buildPackageIOCategories(pkgImports)

	// Apply I/O boundary tags and layer tags
	for _, sym := range symbols {
		// I/O boundary detection
		ioTags := t.getIOTags(sym, pkgIOCategories)
		for _, tag := range ioTags {
			if err := batch.InsertTag(tag); err != nil {
				return nil, fmt.Errorf("inserting IO tag: %w", err)
			}
			result.IOTags++
		}

		// Layer classification
		if layerTag := t.getLayerTag(sym); layerTag != nil {
			if err := batch.InsertTag(layerTag); err != nil {
				return nil, fmt.Errorf("inserting layer tag: %w", err)
			}
			result.LayerTags++
		}
	}

	// Commit to persist IO and layer tags before purity analysis
	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("committing batch: %w", err)
	}

	// Start new batch for purity tags
	batch, err = t.store.BeginBatch()
	if err != nil {
		return nil, fmt.Errorf("starting purity batch: %w", err)
	}
	defer batch.Rollback()

	// Get callee relationships with their tags for purity analysis
	calleeMap, err := t.store.GetSymbolCalleesWithTags()
	if err != nil {
		return nil, fmt.Errorf("getting callees with tags: %w", err)
	}

	// Build set of symbols that have callees (for purity)
	symbolsWithCallees := make(map[store.SymbolID]bool)
	for callerID := range calleeMap {
		symbolsWithCallees[callerID] = true
	}

	// Apply purity tags
	for _, sym := range symbols {
		// Only consider functions and methods for purity
		if sym.Kind != store.SymbolKindFunc && sym.Kind != store.SymbolKindMethod {
			continue
		}

		if purityTag := t.getPurityTag(sym, calleeMap); purityTag != nil {
			if err := batch.InsertTag(purityTag); err != nil {
				return nil, fmt.Errorf("inserting purity tag: %w", err)
			}
			result.PurityTags++
		}
	}

	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("committing purity batch: %w", err)
	}

	result.TotalTags = result.IOTags + result.LayerTags + result.PurityTags
	return result, nil
}

// buildPackageIOCategories builds a map of package path -> set of IO categories it uses.
func (t *Tagger) buildPackageIOCategories(pkgImports map[string][]string) map[string]map[string]string {
	// pkg path -> (io category -> first imported package that caused it)
	result := make(map[string]map[string]string)

	for pkgPath, imports := range pkgImports {
		for _, importedPkg := range imports {
			if category := t.cfg.GetIOCategory(importedPkg); category != "" {
				if result[pkgPath] == nil {
					result[pkgPath] = make(map[string]string)
				}
				// Only store first occurrence
				if _, exists := result[pkgPath][category]; !exists {
					result[pkgPath][category] = importedPkg
				}
			}
		}
	}

	return result
}

// getIOTags returns I/O boundary tags for a symbol.
func (t *Tagger) getIOTags(sym store.SymbolForTagging, pkgIOCategories map[string]map[string]string) []*store.Tag {
	var tags []*store.Tag

	// Only tag functions and methods
	if sym.Kind != store.SymbolKindFunc && sym.Kind != store.SymbolKindMethod {
		return nil
	}

	// Check package-level IO imports
	if categories := pkgIOCategories[sym.PkgPath]; categories != nil {
		for category, importedPkg := range categories {
			tags = append(tags, &store.Tag{
				SymbolID: sym.ID,
				Tag:      "io:" + category,
				Reason:   fmt.Sprintf("Package imports %s", importedPkg),
			})
		}
	}

	// Check receiver type name for methods
	if sym.Kind == store.SymbolKindMethod && sym.RecvType != "" {
		if ioTag := t.getIOTagFromReceiverType(sym.RecvType); ioTag != "" {
			// Only add if not already tagged with this category
			alreadyTagged := false
			for _, tag := range tags {
				if tag.Tag == ioTag {
					alreadyTagged = true
					break
				}
			}
			if !alreadyTagged {
				tags = append(tags, &store.Tag{
					SymbolID: sym.ID,
					Tag:      ioTag,
					Reason:   fmt.Sprintf("Method on %s type", sym.RecvType),
				})
			}
		}
	}

	return tags
}

// getIOTagFromReceiverType returns an I/O tag based on the receiver type name.
func (t *Tagger) getIOTagFromReceiverType(recvType string) string {
	// Normalize: strip pointer and package prefix
	typeName := recvType
	if strings.HasPrefix(typeName, "*") {
		typeName = typeName[1:]
	}
	// Get just the type name if it includes package path
	if idx := strings.LastIndex(typeName, "."); idx != -1 {
		typeName = typeName[idx+1:]
	}

	lowerName := strings.ToLower(typeName)

	// Check for store/repo patterns -> io:db
	if strings.HasSuffix(lowerName, "store") ||
		strings.HasSuffix(lowerName, "repo") ||
		strings.HasSuffix(lowerName, "repository") {
		return "io:db"
	}

	// Check for client patterns -> io:net
	if strings.HasSuffix(lowerName, "client") {
		return "io:net"
	}

	return ""
}

// getLayerTag returns a layer tag for a symbol based on its package path.
func (t *Tagger) getLayerTag(sym store.SymbolForTagging) *store.Tag {
	// Only tag functions and methods
	if sym.Kind != store.SymbolKindFunc && sym.Kind != store.SymbolKindMethod {
		return nil
	}

	layer := t.cfg.GetLayerForPackage(sym.PkgPath)
	if layer == "" {
		return nil
	}

	return &store.Tag{
		SymbolID: sym.ID,
		Tag:      "layer:" + layer,
		Reason:   fmt.Sprintf("Package path matches %s layer pattern", layer),
	}
}

// getPurityTag returns a purity tag for a symbol based on its call edges.
func (t *Tagger) getPurityTag(sym store.SymbolForTagging, calleeMap map[store.SymbolID][]store.SymbolCallee) *store.Tag {
	callees, hasCallees := calleeMap[sym.ID]

	// If no outgoing calls, it's pure-ish
	if !hasCallees || len(callees) == 0 {
		return &store.Tag{
			SymbolID: sym.ID,
			Tag:      "pure-ish",
			Reason:   "No outgoing function calls",
		}
	}

	// Check if any callee has an io:* tag
	for _, callee := range callees {
		for _, tag := range callee.Tags {
			if strings.HasPrefix(tag, "io:") {
				// Has I/O dependency, not pure
				return nil
			}
		}
	}

	// Has outgoing calls but none are I/O
	return &store.Tag{
		SymbolID: sym.ID,
		Tag:      "pure-ish",
		Reason:   "No calls to I/O functions",
	}
}
