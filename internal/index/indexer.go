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
	PackageCount int
	SymbolCount  int
	Duration     time.Duration
	DBPath       string
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
		PackageCount: stats.PackageCount,
		SymbolCount:  stats.SymbolCount,
		Duration:     time.Since(start),
		DBPath:       st.DBPath(),
	}, nil
}
