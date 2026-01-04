package cmd

import (
	"fmt"
	"time"

	"github.com/abramin/flowlens/internal/index"
	"github.com/spf13/cobra"
)

var indexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Index a Go project and build the call graph",
	Long: `Analyze a Go project to build a symbol table and call graph.

The index command:
- Loads Go packages using go/packages
- Builds SSA representation for accurate call graph
- Detects entrypoints (HTTP, gRPC, CLI, main)
- Tags functions with I/O boundaries and layer info
- Persists results to .flowlens/index.db`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		cfg := GetConfig()
		fmt.Printf("Indexing project at: %s\n", path)
		fmt.Printf("Config loaded with %d excluded dirs\n", len(cfg.Exclude.Dirs))

		// Run the indexer
		indexer := index.NewIndexer(cfg, path)
		result, err := indexer.Run()
		if err != nil {
			return fmt.Errorf("indexing failed: %w", err)
		}

		fmt.Println()
		fmt.Printf("Indexing complete!\n")
		fmt.Printf("  Packages: %d\n", result.PackageCount)
		fmt.Printf("  Symbols:  %d\n", result.SymbolCount)
		fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))
		fmt.Printf("  Database: %s\n", result.DBPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(indexCmd)
}
