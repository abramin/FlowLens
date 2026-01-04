package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/abramin/flowlens/internal/server"
	"github.com/spf13/cobra"
)

var (
	uiPort      int
	uiNoBrowser bool
	uiDir       string
)

var uiCmd = &cobra.Command{
	Use:   "ui [project-dir]",
	Short: "Start the FlowLens UI server",
	Long: `Start a local HTTP server that serves the FlowLens UI.

The UI provides:
- Entrypoint browser (HTTP, gRPC, CLI, Cron, Main)
- Interactive call graph visualization
- Node inspector with tags and relationships
- Filtering and export capabilities

The server connects to the SQLite index created by 'flowlens index'.
Make sure to run 'flowlens index' first to create the index.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine project directory
		projectDir := uiDir
		if projectDir == "" {
			if len(args) > 0 {
				projectDir = args[0]
			} else {
				var err error
				projectDir, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("getting current directory: %w", err)
				}
			}
		}

		// Make path absolute
		absDir, err := filepath.Abs(projectDir)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}

		// Check if index exists
		indexPath := filepath.Join(absDir, ".flowlens", "index.db")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			return fmt.Errorf("no FlowLens index found at %s\nRun 'flowlens index %s' first to create the index", indexPath, absDir)
		}

		// Create and start server
		srv, err := server.New(server.Config{
			Port:       uiPort,
			ProjectDir: absDir,
		})
		if err != nil {
			return fmt.Errorf("creating server: %w", err)
		}

		url := fmt.Sprintf("http://localhost:%d", uiPort)
		fmt.Printf("Starting FlowLens UI server at %s\n", url)
		fmt.Printf("Project: %s\n", absDir)
		fmt.Println("Press Ctrl+C to stop")

		// Open browser
		if !uiNoBrowser {
			go openBrowser(url)
		}

		return srv.Start()
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
	uiCmd.Flags().IntVarP(&uiPort, "port", "p", 8080, "port to run the UI server on")
	uiCmd.Flags().BoolVar(&uiNoBrowser, "no-browser", false, "don't open browser automatically")
	uiCmd.Flags().StringVarP(&uiDir, "dir", "d", "", "project directory (default: current directory)")
}

// openBrowser opens the default browser to the given URL.
func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}

	cmd.Run()
}
