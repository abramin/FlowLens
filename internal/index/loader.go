package index

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
	"golang.org/x/tools/go/packages"
)

// LoadMode defines the packages.Load mode required for FlowLens indexing.
const LoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedSyntax |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedModule

// Loader handles loading Go packages and extracting symbols.
type Loader struct {
	cfg         *config.Config
	projectDir  string
	fset        *token.FileSet
	pkgs        []*packages.Package
	fileToPackage map[string]*packages.Package
}

// NewLoader creates a new package loader.
func NewLoader(cfg *config.Config, projectDir string) *Loader {
	return &Loader{
		cfg:           cfg,
		projectDir:    projectDir,
		fset:          token.NewFileSet(),
		fileToPackage: make(map[string]*packages.Package),
	}
}

// Load loads all Go packages from the project directory.
func (l *Loader) Load() error {
	cfg := &packages.Config{
		Mode: LoadMode,
		Dir:  l.projectDir,
		Fset: l.fset,
		// Build constraints can be added here if needed
	}

	// Load all packages in the directory tree
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("loading packages: %w", err)
	}

	// Filter out excluded packages and build file mapping
	var filtered []*packages.Package
	for _, pkg := range pkgs {
		if l.shouldExcludePackage(pkg) {
			continue
		}
		filtered = append(filtered, pkg)

		// Build file → package mapping
		for _, file := range pkg.GoFiles {
			l.fileToPackage[file] = pkg
		}
		for _, file := range pkg.OtherFiles {
			l.fileToPackage[file] = pkg
		}
	}

	l.pkgs = filtered

	// Check for loading errors
	var errs []string
	packages.Visit(l.pkgs, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			errs = append(errs, fmt.Sprintf("%s: %s", pkg.PkgPath, err.Msg))
		}
	})
	if len(errs) > 0 {
		// Log errors but continue - some errors are acceptable
		fmt.Printf("Warning: %d package loading errors\n", len(errs))
		for _, err := range errs[:min(5, len(errs))] {
			fmt.Printf("  - %s\n", err)
		}
		if len(errs) > 5 {
			fmt.Printf("  ... and %d more\n", len(errs)-5)
		}
	}

	return nil
}

// shouldExcludePackage checks if a package should be excluded based on config.
func (l *Loader) shouldExcludePackage(pkg *packages.Package) bool {
	// Check if package directory is excluded
	if pkg.Module != nil {
		relPath, err := filepath.Rel(l.projectDir, pkg.Module.Dir)
		if err == nil {
			for _, dir := range l.cfg.Exclude.Dirs {
				if strings.HasPrefix(relPath, dir) || relPath == dir {
					return true
				}
			}
		}
	}

	// Check each file against exclusion patterns
	for _, file := range pkg.GoFiles {
		for _, pattern := range l.cfg.Exclude.FilesGlob {
			// Convert glob to a simpler match for common patterns
			if matchesGlob(file, pattern) {
				// If any file matches an exclusion pattern, we might want to skip it
				// but still include the package. For now, we include packages
				// with some excluded files - actual file filtering happens later.
				continue
			}
		}
	}

	return false
}

// matchesGlob performs a simplified glob match.
func matchesGlob(path, pattern string) bool {
	// Handle **/ prefix
	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		return matchesSuffix(path, suffix)
	}
	// Simple wildcard match
	matched, _ := filepath.Match(pattern, filepath.Base(path))
	return matched
}

// matchesSuffix checks if path ends with the given suffix pattern.
func matchesSuffix(path, suffix string) bool {
	// Handle patterns like *.pb.go
	if strings.HasPrefix(suffix, "*") {
		ext := suffix[1:] // e.g., ".pb.go"
		return strings.HasSuffix(path, ext)
	}
	return strings.HasSuffix(path, suffix)
}

// Packages returns the loaded packages.
func (l *Loader) Packages() []*packages.Package {
	return l.pkgs
}

// FileSet returns the file set used for parsing.
func (l *Loader) FileSet() *token.FileSet {
	return l.fset
}

// GetPackageForFile returns the package containing the given file.
func (l *Loader) GetPackageForFile(file string) *packages.Package {
	return l.fileToPackage[file]
}

// shouldExcludeFile checks if a file should be excluded from indexing.
func (l *Loader) shouldExcludeFile(file string) bool {
	for _, pattern := range l.cfg.Exclude.FilesGlob {
		if matchesGlob(file, pattern) {
			return true
		}
	}
	return false
}

// ExtractSymbols extracts all symbols from loaded packages and persists them.
func (l *Loader) ExtractSymbols(st *store.Store) error {
	batch, err := st.BeginBatch()
	if err != nil {
		return fmt.Errorf("starting batch: %w", err)
	}
	defer batch.Rollback()

	for _, pkg := range l.pkgs {
		// Insert package record
		storePkg := &store.Package{
			PkgPath: pkg.PkgPath,
			Dir:     packageDir(pkg),
			Layer:   l.cfg.GetLayerForPackage(pkg.PkgPath),
		}
		if pkg.Module != nil {
			storePkg.Module = pkg.Module.Path
		}
		if err := batch.InsertPackage(storePkg); err != nil {
			return fmt.Errorf("inserting package %s: %w", pkg.PkgPath, err)
		}

		// Extract symbols from each file
		for i, file := range pkg.Syntax {
			goFile := pkg.GoFiles[i]
			if l.shouldExcludeFile(goFile) {
				continue
			}
			if err := l.extractFileSymbols(batch, pkg, file, goFile); err != nil {
				return fmt.Errorf("extracting symbols from %s: %w", goFile, err)
			}
		}
	}

	return batch.Commit()
}

// extractFileSymbols extracts symbols from a single AST file.
func (l *Loader) extractFileSymbols(batch *store.BatchTx, pkg *packages.Package, file *ast.File, goFile string) error {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := l.funcDeclToSymbol(pkg, d, goFile)
			if _, err := batch.InsertSymbol(sym); err != nil {
				return err
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := l.typeSpecToSymbol(pkg, s, d.Tok, goFile)
					if _, err := batch.InsertSymbol(sym); err != nil {
						return err
					}

				case *ast.ValueSpec:
					for _, name := range s.Names {
						sym := l.valueSpecToSymbol(pkg, name, d.Tok, goFile)
						if _, err := batch.InsertSymbol(sym); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

// funcDeclToSymbol converts a function declaration to a Symbol.
func (l *Loader) funcDeclToSymbol(pkg *packages.Package, decl *ast.FuncDecl, file string) *store.Symbol {
	sym := &store.Symbol{
		PkgPath: pkg.PkgPath,
		Name:    decl.Name.Name,
		Kind:    store.SymbolKindFunc,
		File:    file,
		Line:    l.fset.Position(decl.Pos()).Line,
	}

	// Check if it's a method (has receiver)
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		sym.Kind = store.SymbolKindMethod
		sym.RecvType = formatReceiverType(decl.Recv.List[0].Type)
	}

	// Extract signature from types info if available
	if obj := pkg.TypesInfo.Defs[decl.Name]; obj != nil {
		if fn, ok := obj.(*types.Func); ok {
			sym.Sig = fn.Type().String()
		}
	}

	return sym
}

// typeSpecToSymbol converts a type spec to a Symbol.
func (l *Loader) typeSpecToSymbol(pkg *packages.Package, spec *ast.TypeSpec, tok token.Token, file string) *store.Symbol {
	return &store.Symbol{
		PkgPath: pkg.PkgPath,
		Name:    spec.Name.Name,
		Kind:    store.SymbolKindType,
		File:    file,
		Line:    l.fset.Position(spec.Pos()).Line,
	}
}

// valueSpecToSymbol converts a value spec (var/const) to a Symbol.
func (l *Loader) valueSpecToSymbol(pkg *packages.Package, name *ast.Ident, tok token.Token, file string) *store.Symbol {
	kind := store.SymbolKindVar
	if tok == token.CONST {
		kind = store.SymbolKindConst
	}
	return &store.Symbol{
		PkgPath: pkg.PkgPath,
		Name:    name.Name,
		Kind:    kind,
		File:    file,
		Line:    l.fset.Position(name.Pos()).Line,
	}
}

// formatReceiverType formats a receiver type expression as a string.
func formatReceiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatReceiverType(t.X)
	case *ast.IndexExpr:
		// Generic type: Type[T]
		return formatReceiverType(t.X) + "[...]"
	case *ast.IndexListExpr:
		// Generic type with multiple params: Type[T, U]
		return formatReceiverType(t.X) + "[...]"
	default:
		return ""
	}
}

// packageDir returns the directory of a package.
func packageDir(pkg *packages.Package) string {
	if len(pkg.GoFiles) > 0 {
		return filepath.Dir(pkg.GoFiles[0])
	}
	if len(pkg.OtherFiles) > 0 {
		return filepath.Dir(pkg.OtherFiles[0])
	}
	return ""
}
