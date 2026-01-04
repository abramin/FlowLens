package index

import (
	"testing"

	"github.com/abramin/flowlens/internal/config"
	"github.com/abramin/flowlens/internal/store"
)

func setupTestStore(t *testing.T) *store.Store {
	tmpDir := t.TempDir()
	st, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	return st
}

func TestTagger_IOTagFromPackageImport(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	// Create packages
	servicePkg := &store.Package{PkgPath: "myapp/service", Dir: "/service"}
	dbPkg := &store.Package{PkgPath: "database/sql", Dir: "/sql"}
	if err := st.InsertPackage(servicePkg); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPackage(dbPkg); err != nil {
		t.Fatal(err)
	}

	// Create symbols
	serviceFunc := &store.Symbol{
		PkgPath: "myapp/service",
		Name:    "GetUser",
		Kind:    store.SymbolKindFunc,
		File:    "service.go",
		Line:    10,
	}
	dbFunc := &store.Symbol{
		PkgPath: "database/sql",
		Name:    "Query",
		Kind:    store.SymbolKindFunc,
		File:    "sql.go",
		Line:    100,
	}

	serviceFuncID, err := st.InsertSymbol(serviceFunc)
	if err != nil {
		t.Fatal(err)
	}
	dbFuncID, err := st.InsertSymbol(dbFunc)
	if err != nil {
		t.Fatal(err)
	}

	// Create call edge: service.GetUser -> database/sql.Query
	edge := &store.CallEdge{
		CallerID:   serviceFuncID,
		CalleeID:   dbFuncID,
		CallerFile: "service.go",
		CallerLine: 15,
		CallKind:   store.CallKindStatic,
		Count:      1,
	}
	if err := st.InsertCallEdge(edge); err != nil {
		t.Fatal(err)
	}

	// Run tagger
	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	result, err := tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Should have IO tags
	if result.IOTags == 0 {
		t.Error("expected IO tags to be applied")
	}

	// Verify the tag exists
	var tag, reason string
	err = st.Tx().QueryRow(`
		SELECT tag, reason FROM tags WHERE symbol_id = ? AND tag LIKE 'io:%'
	`, serviceFuncID).Scan(&tag, &reason)
	if err != nil {
		t.Fatalf("failed to query tag: %v", err)
	}
	if tag != "io:db" {
		t.Errorf("expected tag 'io:db', got '%s'", tag)
	}
	if reason == "" {
		t.Error("expected reason to be set")
	}
}

func TestTagger_IOTagFromReceiverType(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	pkg := &store.Package{PkgPath: "myapp/store", Dir: "/store"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	// Create a method on *UserStore
	method := &store.Symbol{
		PkgPath:  "myapp/store",
		Name:     "FindByID",
		Kind:     store.SymbolKindMethod,
		RecvType: "*UserStore",
		File:     "user_store.go",
		Line:     20,
	}
	methodID, err := st.InsertSymbol(method)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	result, err := tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	if result.IOTags == 0 {
		t.Error("expected IO tags from receiver type")
	}

	// Verify the tag
	var tag, reason string
	err = st.Tx().QueryRow(`
		SELECT tag, reason FROM tags WHERE symbol_id = ? AND tag = 'io:db'
	`, methodID).Scan(&tag, &reason)
	if err != nil {
		t.Fatalf("failed to query tag: %v", err)
	}
	if reason != "Method on *UserStore type" {
		t.Errorf("expected reason 'Method on *UserStore type', got '%s'", reason)
	}
}

func TestTagger_LayerClassification(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	// Create packages matching layer patterns
	tests := []struct {
		pkgPath       string
		expectedLayer string
	}{
		{"myapp/internal/handlers/user", "handler"},
		{"myapp/internal/services/auth", "service"},
		{"myapp/internal/store/postgres", "store"},
		{"myapp/internal/domain/user", "domain"},
		{"myapp/internal/util", ""}, // no layer
	}

	cfg := config.Default()

	for _, tt := range tests {
		pkg := &store.Package{PkgPath: tt.pkgPath, Dir: "/" + tt.pkgPath}
		if err := st.InsertPackage(pkg); err != nil {
			t.Fatal(err)
		}

		fn := &store.Symbol{
			PkgPath: tt.pkgPath,
			Name:    "DoSomething",
			Kind:    store.SymbolKindFunc,
			File:    "file.go",
			Line:    1,
		}
		if _, err := st.InsertSymbol(fn); err != nil {
			t.Fatal(err)
		}
	}

	tagger := NewTagger(cfg, st)
	result, err := tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Should have some layer tags (4 packages match layers)
	if result.LayerTags < 4 {
		t.Errorf("expected at least 4 layer tags, got %d", result.LayerTags)
	}
}

func TestTagger_PurityNoOutgoingCalls(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	pkg := &store.Package{PkgPath: "myapp/util", Dir: "/util"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	// Create a pure function with no outgoing calls
	pureFunc := &store.Symbol{
		PkgPath: "myapp/util",
		Name:    "Add",
		Kind:    store.SymbolKindFunc,
		File:    "math.go",
		Line:    5,
	}
	pureFuncID, err := st.InsertSymbol(pureFunc)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	result, err := tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	if result.PurityTags == 0 {
		t.Error("expected purity tags")
	}

	// Verify pure-ish tag
	var tag, reason string
	err = st.Tx().QueryRow(`
		SELECT tag, reason FROM tags WHERE symbol_id = ? AND tag = 'pure-ish'
	`, pureFuncID).Scan(&tag, &reason)
	if err != nil {
		t.Fatalf("failed to query purity tag: %v", err)
	}
	if reason != "No outgoing function calls" {
		t.Errorf("expected reason 'No outgoing function calls', got '%s'", reason)
	}
}

func TestTagger_PurityWithNonIOCalls(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	pkg := &store.Package{PkgPath: "myapp/util", Dir: "/util"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	// Create two pure functions where one calls the other
	helperFunc := &store.Symbol{
		PkgPath: "myapp/util",
		Name:    "Helper",
		Kind:    store.SymbolKindFunc,
		File:    "util.go",
		Line:    5,
	}
	mainFunc := &store.Symbol{
		PkgPath: "myapp/util",
		Name:    "Main",
		Kind:    store.SymbolKindFunc,
		File:    "util.go",
		Line:    10,
	}

	helperID, err := st.InsertSymbol(helperFunc)
	if err != nil {
		t.Fatal(err)
	}
	mainID, err := st.InsertSymbol(mainFunc)
	if err != nil {
		t.Fatal(err)
	}

	// Main calls Helper (non-IO call)
	edge := &store.CallEdge{
		CallerID:   mainID,
		CalleeID:   helperID,
		CallerFile: "util.go",
		CallerLine: 12,
		CallKind:   store.CallKindStatic,
		Count:      1,
	}
	if err := st.InsertCallEdge(edge); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	result, err := tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Both should be pure-ish
	if result.PurityTags < 2 {
		t.Errorf("expected at least 2 purity tags, got %d", result.PurityTags)
	}

	// Verify Main has pure-ish with correct reason
	var reason string
	err = st.Tx().QueryRow(`
		SELECT reason FROM tags WHERE symbol_id = ? AND tag = 'pure-ish'
	`, mainID).Scan(&reason)
	if err != nil {
		t.Fatalf("failed to query purity tag for Main: %v", err)
	}
	if reason != "No calls to I/O functions" {
		t.Errorf("expected reason 'No calls to I/O functions', got '%s'", reason)
	}
}

func TestTagger_NotPureWithIOCall(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	// Create packages
	servicePkg := &store.Package{PkgPath: "myapp/service", Dir: "/service"}
	storePkg := &store.Package{PkgPath: "myapp/store", Dir: "/store"}
	if err := st.InsertPackage(servicePkg); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertPackage(storePkg); err != nil {
		t.Fatal(err)
	}

	// Create service function that calls store method
	serviceFunc := &store.Symbol{
		PkgPath: "myapp/service",
		Name:    "GetUser",
		Kind:    store.SymbolKindFunc,
		File:    "service.go",
		Line:    10,
	}
	storeMethod := &store.Symbol{
		PkgPath:  "myapp/store",
		Name:     "FindByID",
		Kind:     store.SymbolKindMethod,
		RecvType: "*UserStore",
		File:     "store.go",
		Line:     20,
	}

	serviceFuncID, err := st.InsertSymbol(serviceFunc)
	if err != nil {
		t.Fatal(err)
	}
	storeMethodID, err := st.InsertSymbol(storeMethod)
	if err != nil {
		t.Fatal(err)
	}

	// Create call edge
	edge := &store.CallEdge{
		CallerID:   serviceFuncID,
		CalleeID:   storeMethodID,
		CallerFile: "service.go",
		CallerLine: 15,
		CallKind:   store.CallKindStatic,
		Count:      1,
	}
	if err := st.InsertCallEdge(edge); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	_, err = tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Service function should NOT have pure-ish tag because it calls io:db tagged function
	var count int
	err = st.Tx().QueryRow(`
		SELECT COUNT(*) FROM tags WHERE symbol_id = ? AND tag = 'pure-ish'
	`, serviceFuncID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query: %v", err)
	}
	if count != 0 {
		t.Error("service function should not be tagged as pure-ish since it calls io:db function")
	}
}

func TestTagger_ClientReceiverType(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	pkg := &store.Package{PkgPath: "myapp/client", Dir: "/client"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	// Create a method on *HTTPClient
	method := &store.Symbol{
		PkgPath:  "myapp/client",
		Name:     "Get",
		Kind:     store.SymbolKindMethod,
		RecvType: "*HTTPClient",
		File:     "client.go",
		Line:     30,
	}
	methodID, err := st.InsertSymbol(method)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	_, err = tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Verify io:net tag for Client type
	var tag string
	err = st.Tx().QueryRow(`
		SELECT tag FROM tags WHERE symbol_id = ? AND tag = 'io:net'
	`, methodID).Scan(&tag)
	if err != nil {
		t.Fatalf("failed to query tag: %v (expected io:net for *Client receiver)", err)
	}
}

func TestTagger_RepoReceiverType(t *testing.T) {
	st := setupTestStore(t)
	defer st.Close()

	pkg := &store.Package{PkgPath: "myapp/repo", Dir: "/repo"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	// Create a method on *UserRepo
	method := &store.Symbol{
		PkgPath:  "myapp/repo",
		Name:     "Save",
		Kind:     store.SymbolKindMethod,
		RecvType: "*UserRepo",
		File:     "user_repo.go",
		Line:     25,
	}
	methodID, err := st.InsertSymbol(method)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	tagger := NewTagger(cfg, st)
	_, err = tagger.Tag()
	if err != nil {
		t.Fatalf("tagging failed: %v", err)
	}

	// Verify io:db tag for Repo type
	var tag string
	err = st.Tx().QueryRow(`
		SELECT tag FROM tags WHERE symbol_id = ? AND tag = 'io:db'
	`, methodID).Scan(&tag)
	if err != nil {
		t.Fatalf("failed to query tag: %v (expected io:db for *Repo receiver)", err)
	}
}
