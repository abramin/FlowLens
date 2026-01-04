package cmd

import (
	"fmt"

	"github.com/abramin/flowlens/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "flowlens",
	Short: "FlowLens - Visualize Go call graphs from entrypoints",
	Long: `FlowLens analyzes Go codebases and generates forward call graphs
from entrypoints (HTTP routes, gRPC methods, CLI commands, main).

It helps developers answer "What happens next?" by visualizing the flow
from entrypoint → handler → service → store.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./flowlens.yaml)")
}

func GetConfig() *config.Config {
	return cfg
}
