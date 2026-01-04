package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if len(cfg.Exclude.Dirs) == 0 {
		t.Error("expected default excluded dirs")
	}
	if len(cfg.Layers) == 0 {
		t.Error("expected default layers")
	}
	if len(cfg.IOPackages) == 0 {
		t.Error("expected default IO packages")
	}
	if len(cfg.NoisePackages) == 0 {
		t.Error("expected default noise packages")
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load("nonexistent.yaml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected default config")
	}
	if len(cfg.Exclude.Dirs) == 0 {
		t.Error("expected default excluded dirs")
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `
exclude:
  dirs:
    - vendor
    - custom_exclude
  files_glob:
    - "**/*.generated.go"

layers:
  handler:
    - "**/api/**"
  service:
    - "**/svc/**"

io_packages:
  db:
    - "database/sql"
    - "custom/db"

noise_packages:
  - "custom/logger"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "flowlens.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if len(cfg.Exclude.Dirs) != 2 {
		t.Errorf("expected 2 excluded dirs, got %d", len(cfg.Exclude.Dirs))
	}
	if cfg.Exclude.Dirs[1] != "custom_exclude" {
		t.Errorf("expected custom_exclude, got %s", cfg.Exclude.Dirs[1])
	}

	if len(cfg.Layers) != 2 {
		t.Errorf("expected 2 layers, got %d", len(cfg.Layers))
	}

	if len(cfg.IOPackages["db"]) != 2 {
		t.Errorf("expected 2 db packages, got %d", len(cfg.IOPackages["db"]))
	}

	if len(cfg.NoisePackages) != 1 {
		t.Errorf("expected 1 noise package, got %d", len(cfg.NoisePackages))
	}
}

func TestIsExcludedDir(t *testing.T) {
	cfg := Default()

	tests := []struct {
		dir      string
		excluded bool
	}{
		{"vendor", true},
		{"/path/to/vendor", true},
		{"third_party", true},
		{"src", false},
		{"internal", false},
	}

	for _, tt := range tests {
		got := cfg.IsExcludedDir(tt.dir)
		if got != tt.excluded {
			t.Errorf("IsExcludedDir(%q) = %v, want %v", tt.dir, got, tt.excluded)
		}
	}
}

func TestIsNoisePackage(t *testing.T) {
	cfg := Default()

	tests := []struct {
		pkg   string
		noise bool
	}{
		{"log/slog", true},
		{"go.uber.org/zap", true},
		{"go.uber.org/zap/zapcore", true},
		{"github.com/prometheus/client_golang/prometheus", true},
		{"net/http", false},
		{"myapp/service", false},
	}

	for _, tt := range tests {
		got := cfg.IsNoisePackage(tt.pkg)
		if got != tt.noise {
			t.Errorf("IsNoisePackage(%q) = %v, want %v", tt.pkg, got, tt.noise)
		}
	}
}

func TestGetIOCategory(t *testing.T) {
	cfg := Default()

	tests := []struct {
		pkg      string
		category string
	}{
		{"database/sql", "db"},
		{"github.com/jackc/pgx/v5", "db"},
		{"net/http", "net"},
		{"google.golang.org/grpc/codes", "net"},
		{"os", "fs"},
		{"github.com/nats-io/nats.go", "bus"},
		{"myapp/service", ""},
		{"fmt", ""},
	}

	for _, tt := range tests {
		got := cfg.GetIOCategory(tt.pkg)
		if got != tt.category {
			t.Errorf("GetIOCategory(%q) = %q, want %q", tt.pkg, got, tt.category)
		}
	}
}
