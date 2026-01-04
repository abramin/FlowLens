package index

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
)

// Indexer coordinates the indexing pipeline.
type Indexer struct {
	cfg        *config.Config
	projectDir string
	store      *store.Store
	loader     *Loader
}

// NewIndexer creates a new indexer for the given project directory.
func NewIndexer(cfg *config.Config, projectDir string) *Indexer {
	absPath, err := filepath.Abs(projectDir)
	if err != nil {
		absPath = projectDir
	}
	return &Indexer{
		cfg:        cfg,
		projectDir: absPath,
	}
}

// Result holds the results of an indexing run.
type Result struct {
	PackageCount    int
	SymbolCount     int
	CallEdgeCount   int
	StaticCalls     int
	InterfaceCalls  int
	DeferCalls      int
	GoCalls         int
	EntrypointCount int
	HTTPEntrypoints int
	GRPCEntrypoints int
	CLIEntrypoints  int
	MainEntrypoints int
	TagCount        int
	IOTags          int
	LayerTags       int
	PurityTags      int
	Duration        time.Duration
	DBPath          string
}

// Run executes the indexing pipeline.
func (idx *Indexer) Run() (*Result, error) {
	start := time.Now()

	// Open (or create) the store
	st, err := store.Open(idx.projectDir)
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()
	idx.store = st

	// Clear existing data for fresh index
	if err := st.Clear(); err != nil {
		return nil, fmt.Errorf("clearing store: %w", err)
	}

	// Load packages
	fmt.Println("Loading packages...")
	loader := NewLoader(idx.cfg, idx.projectDir)
	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}
	idx.loader = loader

	fmt.Printf("Loaded %d packages\n", len(loader.Packages()))

	// Extract and persist symbols
	fmt.Println("Extracting symbols...")
	if err := loader.ExtractSymbols(st); err != nil {
		return nil, fmt.Errorf("extracting symbols: %w", err)
	}

	// Detect entrypoints
	fmt.Println("Detecting entrypoints...")
	epResult, err := idx.detectEntrypoints(loader, st)
	if err != nil {
		return nil, fmt.Errorf("detecting entrypoints: %w", err)
	}
	fmt.Printf("Found %d entrypoints (%d http, %d grpc, %d cli, %d main)\n",
		epResult.TotalCount, epResult.HTTPCount, epResult.GRPCCount,
		epResult.CLICount, epResult.MainCount)

	// Build SSA and extract call graph
	fmt.Println("Building call graph...")
	cgResult, err := BuildAndExtract(loader, st, func(current, total int) {
		if current%500 == 0 || current == total {
			fmt.Printf("  Processing functions: %d/%d\n", current, total)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("building call graph: %w", err)
	}
	fmt.Printf("Extracted %d call edges (%d static, %d interface, %d defer, %d go)\n",
		cgResult.EdgeCount, cgResult.StaticCalls, cgResult.InterfaceCalls,
		cgResult.DeferCalls, cgResult.GoCalls)

	// Apply tags
	fmt.Println("Applying tags...")
	tagger := NewTagger(idx.cfg, st)
	tagResult, err := tagger.Tag()
	if err != nil {
		return nil, fmt.Errorf("tagging: %w", err)
	}
	fmt.Printf("Applied %d tags (%d io, %d layer, %d purity)\n",
		tagResult.TotalTags, tagResult.IOTags, tagResult.LayerTags, tagResult.PurityTags)

	// Store indexing metadata
	if err := st.SetMetadata("indexed_at", time.Now().Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("storing metadata: %w", err)
	}
	if err := st.SetMetadata("project_dir", idx.projectDir); err != nil {
		return nil, fmt.Errorf("storing metadata: %w", err)
	}

	// Get stats
	stats, err := st.GetStats()
	if err != nil {
		return nil, fmt.Errorf("getting stats: %w", err)
	}

	// Write index.json for UI quick boot
	if err := st.WriteIndexJSON(); err != nil {
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	return &Result{
		PackageCount:    stats.PackageCount,
		SymbolCount:     stats.SymbolCount,
		CallEdgeCount:   cgResult.EdgeCount,
		StaticCalls:     cgResult.StaticCalls,
		InterfaceCalls:  cgResult.InterfaceCalls,
		DeferCalls:      cgResult.DeferCalls,
		GoCalls:         cgResult.GoCalls,
		EntrypointCount: epResult.TotalCount,
		HTTPEntrypoints: epResult.HTTPCount,
		GRPCEntrypoints: epResult.GRPCCount,
		CLIEntrypoints:  epResult.CLICount,
		MainEntrypoints: epResult.MainCount,
		TagCount:        tagResult.TotalTags,
		IOTags:          tagResult.IOTags,
		LayerTags:       tagResult.LayerTags,
		PurityTags:      tagResult.PurityTags,
		Duration:        time.Since(start),
		DBPath:          st.DBPath(),
	}, nil
}

// detectEntrypoints runs entrypoint detection within a batch transaction.
func (idx *Indexer) detectEntrypoints(loader *Loader, st *store.Store) (*DetectResult, error) {
	batch, err := st.BeginBatch()
	if err != nil {
		return nil, fmt.Errorf("starting batch: %w", err)
	}
	defer batch.Rollback()

	detector := NewEntrypointDetector(loader)
	result, err := detector.Detect(batch)
	if err != nil {
		return nil, err
	}

	if err := batch.Commit(); err != nil {
		return nil, fmt.Errorf("committing batch: %w", err)
	}

	return result, nil
}
