package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the FlowLens configuration.
type Config struct {
	Exclude       ExcludeConfig         `yaml:"exclude"`
	Layers        map[string][]string   `yaml:"layers"`
	IOPackages    map[string][]string   `yaml:"io_packages"`
	NoisePackages []string              `yaml:"noise_packages"`
}

// ExcludeConfig defines patterns to exclude from indexing.
type ExcludeConfig struct {
	Dirs      []string `yaml:"dirs"`
	FilesGlob []string `yaml:"files_glob"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Exclude: ExcludeConfig{
			Dirs:      []string{"vendor", "third_party", "testdata"},
			FilesGlob: []string{"**/*.pb.go", "**/*_gen.go", "**/*_mock.go"},
		},
		Layers: map[string][]string{
			"handler": {"**/handlers/**", "**/http/**", "**/api/**"},
			"service": {"**/service/**", "**/services/**"},
			"store":   {"**/store/**", "**/stores/**", "**/repo/**", "**/repository/**"},
			"domain":  {"**/domain/**", "**/model/**", "**/models/**"},
		},
		IOPackages: map[string][]string{
			"db": {
				"database/sql",
				"github.com/jackc/pgx",
				"github.com/jackc/pgx/*",
				"github.com/lib/pq",
				"gorm.io/*",
				"github.com/go-sql-driver/mysql",
				"go.mongodb.org/mongo-driver/*",
			},
			"net": {
				"net/http",
				"google.golang.org/grpc",
				"google.golang.org/grpc/*",
				"github.com/go-resty/resty/*",
			},
			"fs": {
				"os",
				"io/ioutil",
				"io/fs",
			},
			"bus": {
				"github.com/nats-io/*",
				"github.com/segmentio/kafka-go",
				"github.com/rabbitmq/amqp091-go",
			},
		},
		NoisePackages: []string{
			"log",
			"log/slog",
			"go.uber.org/zap",
			"go.uber.org/zap/*",
			"github.com/sirupsen/logrus",
			"github.com/rs/zerolog",
			"github.com/rs/zerolog/*",
			"github.com/prometheus/client_golang/*",
			"go.opentelemetry.io/otel/*",
		},
	}
}

// Load reads configuration from file, falling back to defaults.
// If configPath is empty, it looks for flowlens.yaml in the current directory.
// Values in the config file replace defaults entirely (no merging).
func Load(configPath string) (*Config, error) {
	defaults := Default()

	if configPath == "" {
		configPath = "flowlens.yaml"
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file, use defaults
			return defaults, nil
		}
		return nil, err
	}

	// Unmarshal into empty struct first
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return nil, err
	}

	// Apply defaults for missing fields
	defaults.Merge(&fileCfg)
	return defaults, nil
}

// LoadFromDir loads configuration from the specified directory.
func LoadFromDir(dir string) (*Config, error) {
	return Load(filepath.Join(dir, "flowlens.yaml"))
}

// Merge combines another config into this one, with other taking precedence.
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}

	if len(other.Exclude.Dirs) > 0 {
		c.Exclude.Dirs = other.Exclude.Dirs
	}
	if len(other.Exclude.FilesGlob) > 0 {
		c.Exclude.FilesGlob = other.Exclude.FilesGlob
	}
	if len(other.Layers) > 0 {
		c.Layers = other.Layers
	}
	if len(other.IOPackages) > 0 {
		c.IOPackages = other.IOPackages
	}
	if len(other.NoisePackages) > 0 {
		c.NoisePackages = other.NoisePackages
	}
}

// IsExcludedDir checks if a directory should be excluded from indexing.
func (c *Config) IsExcludedDir(dir string) bool {
	base := filepath.Base(dir)
	for _, excluded := range c.Exclude.Dirs {
		if base == excluded {
			return true
		}
	}
	return false
}

// GetLayerForPackage returns the layer name for a given package path, or empty string if no match.
func (c *Config) GetLayerForPackage(pkgPath string) string {
	for layer, patterns := range c.Layers {
		for _, pattern := range patterns {
			if matchLayerPattern(pattern, pkgPath) {
				return layer
			}
		}
	}
	return ""
}

// matchLayerPattern matches a package path against a layer pattern.
// Supports ** for matching any number of path components.
// Example: "**/handlers/**" matches "myapp/internal/handlers/user"
func matchLayerPattern(pattern, pkgPath string) bool {
	// Handle ** patterns by extracting the fixed middle part
	// Pattern like "**/handlers/**" means: contains "/handlers/" or starts with "handlers/"
	if len(pattern) >= 4 && pattern[:2] == "**" && pattern[len(pattern)-2:] == "**" {
		// Pattern: **/xxx/** -> check if pkgPath contains /xxx/
		middle := pattern[2 : len(pattern)-2] // e.g., "/handlers/"
		if strings.Contains(pkgPath, middle) {
			return true
		}
		// Also match if it starts with the middle part (without leading slash)
		if len(middle) > 0 && middle[0] == '/' {
			trimmed := middle[1:] // e.g., "handlers/"
			if strings.HasPrefix(pkgPath, trimmed) {
				return true
			}
		}
		return false
	}

	// Fallback to filepath.Match for non-** patterns
	matched, err := filepath.Match(pattern, pkgPath)
	return err == nil && matched
}

// IsNoisePackage checks if a package should be considered noise.
func (c *Config) IsNoisePackage(pkgPath string) bool {
	for _, noise := range c.NoisePackages {
		matched, err := filepath.Match(noise, pkgPath)
		if err == nil && matched {
			return true
		}
		// Also check prefix match for wildcard patterns
		if len(noise) > 0 && noise[len(noise)-1] == '*' {
			prefix := noise[:len(noise)-1]
			if len(pkgPath) >= len(prefix) && pkgPath[:len(prefix)] == prefix {
				return true
			}
		}
	}
	return false
}

// GetIOCategory returns the I/O category (db, net, fs, bus) for a package, or empty string if not I/O.
func (c *Config) GetIOCategory(pkgPath string) string {
	for category, packages := range c.IOPackages {
		for _, pkg := range packages {
			if pkg == pkgPath {
				return category
			}
			// Check wildcard match
			if len(pkg) > 0 && pkg[len(pkg)-1] == '*' {
				prefix := pkg[:len(pkg)-1]
				if len(pkgPath) >= len(prefix) && pkgPath[:len(prefix)] == prefix {
					return category
				}
			}
		}
	}
	return ""
}
