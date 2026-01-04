package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	uiPort    int
	uiNoBrowser bool
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Start the FlowLens UI server",
	Long: `Start a local HTTP server that serves the FlowLens UI.

The UI provides:
- Entrypoint browser (HTTP, gRPC, CLI, Cron, Main)
- Interactive call graph visualization
- Node inspector with tags and relationships
- Filtering and export capabilities`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := GetConfig()
		_ = cfg // Will be used when server is implemented

		fmt.Printf("Starting FlowLens UI on http://localhost:%d\n", uiPort)
		if !uiNoBrowser {
			fmt.Println("Opening browser...")
		}

		// TODO: Implement UI server (Slice 7)
		fmt.Println("UI server not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().IntVarP(&uiPort, "port", "p", 8080, "port to run the UI server on")
	uiCmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "don't open browser automatically")
}
