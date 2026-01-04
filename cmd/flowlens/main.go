package main

import (
	"os"

	"github.com/abramin/flowlens/cmd/flowlens/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
