package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAndClose(t *testing.T) {
	tmpDir := t.TempDir()

	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Verify .flowlens directory was created
	flowlensDir := filepath.Join(tmpDir, ".flowlens")
	if _, err := os.Stat(flowlensDir); os.IsNotExist(err) {
		t.Error(".flowlens directory was not created")
	}

	// Verify database file exists
	dbPath := filepath.Join(flowlensDir, "index.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("index.db was not created")
	}

	if err := st.Close(); err != nil {
		t.Errorf("failed to close store: %v", err)
	}
}

func TestInsertAndRetrievePackage(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	pkg := &Package{
		PkgPath: "github.com/test/pkg",
		Module:  "github.com/test",
		Dir:     "/path/to/pkg",
		Layer:   "service",
	}

	if err := st.InsertPackage(pkg); err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	// Verify by querying
	var count int
	err = st.Tx().QueryRow("SELECT COUNT(*) FROM packages WHERE pkg_path = ?", pkg.PkgPath).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query package: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 package, got %d", count)
	}
}

func TestInsertAndRetrieveSymbol(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// Insert package first (foreign key constraint)
	pkg := &Package{
		PkgPath: "github.com/test/pkg",
		Dir:     "/path/to/pkg",
	}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	sym := &Symbol{
		PkgPath:  "github.com/test/pkg",
		Name:     "MyFunc",
		Kind:     SymbolKindFunc,
		RecvType: "",
		File:     "/path/to/pkg/file.go",
		Line:     42,
		Sig:      "func() error",
	}

	id, err := st.InsertSymbol(sym)
	if err != nil {
		t.Fatalf("failed to insert symbol: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero symbol ID")
	}

	// Retrieve by ID
	lookupID, err := st.GetSymbolID(sym.PkgPath, sym.Name, sym.RecvType)
	if err != nil {
		t.Fatalf("failed to get symbol ID: %v", err)
	}
	if lookupID != id {
		t.Errorf("expected ID %d, got %d", id, lookupID)
	}
}

func TestInsertMethod(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	pkg := &Package{
		PkgPath: "github.com/test/pkg",
		Dir:     "/path/to/pkg",
	}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	// Insert a method (function with receiver)
	sym := &Symbol{
		PkgPath:  "github.com/test/pkg",
		Name:     "Process",
		Kind:     SymbolKindMethod,
		RecvType: "*Handler",
		File:     "/path/to/pkg/handler.go",
		Line:     100,
		Sig:      "func(*Handler) Process() error",
	}

	id, err := st.InsertSymbol(sym)
	if err != nil {
		t.Fatalf("failed to insert method: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero symbol ID")
	}

	// Should be able to retrieve with receiver type
	lookupID, err := st.GetSymbolID(sym.PkgPath, sym.Name, sym.RecvType)
	if err != nil {
		t.Fatalf("failed to get method ID: %v", err)
	}
	if lookupID != id {
		t.Errorf("expected ID %d, got %d", id, lookupID)
	}
}

func TestBatchInsert(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	batch, err := st.BeginBatch()
	if err != nil {
		t.Fatalf("failed to begin batch: %v", err)
	}

	// Insert multiple packages
	for i := 0; i < 10; i++ {
		pkg := &Package{
			PkgPath: "github.com/test/pkg" + string(rune('a'+i)),
			Dir:     "/path/to/pkg",
		}
		if err := batch.InsertPackage(pkg); err != nil {
			batch.Rollback()
			t.Fatalf("failed to insert package: %v", err)
		}
	}

	if err := batch.Commit(); err != nil {
		t.Fatalf("failed to commit batch: %v", err)
	}

	// Verify count
	stats, err := st.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.PackageCount != 10 {
		t.Errorf("expected 10 packages, got %d", stats.PackageCount)
	}
}

func TestClear(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// Insert some data
	pkg := &Package{PkgPath: "github.com/test/pkg", Dir: "/path"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	sym := &Symbol{PkgPath: "github.com/test/pkg", Name: "Func", Kind: SymbolKindFunc, File: "f.go", Line: 1}
	if _, err := st.InsertSymbol(sym); err != nil {
		t.Fatalf("failed to insert symbol: %v", err)
	}

	// Clear
	if err := st.Clear(); err != nil {
		t.Fatalf("failed to clear: %v", err)
	}

	// Verify empty
	stats, err := st.GetStats()
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}
	if stats.PackageCount != 0 || stats.SymbolCount != 0 {
		t.Errorf("expected 0 packages and symbols, got %d and %d", stats.PackageCount, stats.SymbolCount)
	}
}

func TestMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	if err := st.SetMetadata("version", "1.0"); err != nil {
		t.Fatalf("failed to set metadata: %v", err)
	}

	val, err := st.GetMetadata("version")
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	if val != "1.0" {
		t.Errorf("expected '1.0', got '%s'", val)
	}

	// Update existing key
	if err := st.SetMetadata("version", "2.0"); err != nil {
		t.Fatalf("failed to update metadata: %v", err)
	}

	val, err = st.GetMetadata("version")
	if err != nil {
		t.Fatalf("failed to get updated metadata: %v", err)
	}
	if val != "2.0" {
		t.Errorf("expected '2.0', got '%s'", val)
	}
}

func TestWriteIndexJSON(t *testing.T) {
	tmpDir := t.TempDir()
	st, err := Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	// Insert some data
	pkg := &Package{PkgPath: "github.com/test/pkg", Dir: "/path"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatalf("failed to insert package: %v", err)
	}

	if err := st.SetMetadata("indexed_at", "2024-01-01T00:00:00Z"); err != nil {
		t.Fatalf("failed to set metadata: %v", err)
	}

	if err := st.WriteIndexJSON(); err != nil {
		t.Fatalf("failed to write index.json: %v", err)
	}

	// Verify file exists
	indexPath := filepath.Join(tmpDir, ".flowlens", "index.json")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		t.Error("index.json was not created")
	}
}
