package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
)

func TestMatchesGlob(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
	}{
		{"foo.pb.go", "*.pb.go", true},
		{"foo.go", "*.pb.go", false},
		{"/path/to/foo.pb.go", "**/*.pb.go", true},
		{"/path/to/foo_gen.go", "**/*_gen.go", true},
		{"/path/to/foo.go", "**/*_gen.go", false},
		{"foo_mock.go", "**/*_mock.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			got := matchesGlob(tt.path, tt.pattern)
			if got != tt.want {
				t.Errorf("matchesGlob(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestMatchesSuffix(t *testing.T) {
	tests := []struct {
		path   string
		suffix string
		want   bool
	}{
		{"foo.pb.go", "*.pb.go", true},
		{"foo.go", "*.pb.go", false},
		{"/path/to/file.pb.go", "*.pb.go", true},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.suffix, func(t *testing.T) {
			got := matchesSuffix(tt.path, tt.suffix)
			if got != tt.want {
				t.Errorf("matchesSuffix(%q, %q) = %v, want %v", tt.path, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestFormatReceiverType(t *testing.T) {
	// This test is more for documentation - actual testing requires parsing AST
	// Just verify the function doesn't panic on nil
	result := formatReceiverType(nil)
	if result != "" {
		t.Errorf("expected empty string for nil, got %q", result)
	}
}

// TestLoaderOnProject tests the loader on the FlowLens project itself.
func TestLoaderOnProject(t *testing.T) {
	// Find project root (go up from test file location)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Go up two directories to get to project root
	projectRoot := filepath.Dir(filepath.Dir(wd))

	// Check if this is actually the FlowLens project
	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		t.Skip("not running in FlowLens project, skipping integration test")
	}

	cfg := config.Default()
	loader := NewLoader(cfg, projectRoot)

	if err := loader.Load(); err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	pkgs := loader.Packages()
	if len(pkgs) == 0 {
		t.Error("expected at least one package")
	}

	// Check that we can get packages for files
	if len(pkgs) > 0 && len(pkgs[0].GoFiles) > 0 {
		file := pkgs[0].GoFiles[0]
		pkg := loader.GetPackageForFile(file)
		if pkg == nil {
			t.Errorf("expected to find package for file %s", file)
		}
	}
}

// TestExtractSymbols tests symbol extraction on a real project.
func TestExtractSymbols(t *testing.T) {
	// Find project root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	projectRoot := filepath.Dir(filepath.Dir(wd))

	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); os.IsNotExist(err) {
		t.Skip("not running in FlowLens project, skipping integration test")
	}

	cfg := config.Default()
	loader := NewLoader(cfg, projectRoot)

	if err := loader.Load(); err != nil {
		t.Fatalf("failed to load packages: %v", err)
	}

	// Create a temporary store
	tmpDir := t.TempDir()
	st, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	if err := loader.ExtractSymbols(st); err != nil {
		t.Fatalf("failed to extract symbols: %v", err)
	}

	stats, err := st.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.PackageCount == 0 {
		t.Error("expected at least one package")
	}
	if stats.SymbolCount == 0 {
		t.Error("expected at least one symbol")
	}

	t.Logf("Extracted %d packages and %d symbols", stats.PackageCount, stats.SymbolCount)
}
