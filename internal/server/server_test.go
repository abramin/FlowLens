package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abramin/flowlens/internal/store"
)

func setupTestServer(t *testing.T) *Server {
	tmpDir := t.TempDir()
	st, err := store.Open(tmpDir)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	// Insert test data
	pkg := &store.Package{PkgPath: "myapp/handlers", Dir: "/handlers", Layer: "handler"}
	if err := st.InsertPackage(pkg); err != nil {
		t.Fatal(err)
	}

	sym := &store.Symbol{
		PkgPath: "myapp/handlers",
		Name:    "GetUser",
		Kind:    store.SymbolKindFunc,
		File:    "user.go",
		Line:    10,
		Sig:     "func(w http.ResponseWriter, r *http.Request)",
	}
	symID, err := st.InsertSymbol(sym)
	if err != nil {
		t.Fatal(err)
	}

	ep := &store.Entrypoint{
		Type:     store.EntrypointHTTP,
		Label:    "GET /api/users",
		SymbolID: symID,
		MetaJSON: `{"method":"GET","path":"/api/users"}`,
	}
	if _, err := st.InsertEntrypoint(ep); err != nil {
		t.Fatal(err)
	}

	if err := st.InsertTag(&store.Tag{SymbolID: symID, Tag: "layer:handler", Reason: "Package path matches handler layer"}); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		store: st,
		port:  8080,
	}

	return s
}

func TestHandleHealth(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
}

func TestHandleStats(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	w := httptest.NewRecorder()

	s.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var stats store.Stats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if stats.PackageCount != 1 {
		t.Errorf("expected 1 package, got %d", stats.PackageCount)
	}
	if stats.SymbolCount != 1 {
		t.Errorf("expected 1 symbol, got %d", stats.SymbolCount)
	}
	if stats.EntrypointCount != 1 {
		t.Errorf("expected 1 entrypoint, got %d", stats.EntrypointCount)
	}
}

func TestHandleEntrypoints(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	// Test getting all entrypoints
	req := httptest.NewRequest(http.MethodGet, "/api/entrypoints", nil)
	w := httptest.NewRecorder()

	s.handleEntrypoints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var eps []store.EntrypointWithSymbol
	if err := json.NewDecoder(w.Body).Decode(&eps); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("expected 1 entrypoint, got %d", len(eps))
	}
	if eps[0].Label != "GET /api/users" {
		t.Errorf("expected label 'GET /api/users', got '%s'", eps[0].Label)
	}
	if eps[0].Symbol.Name != "GetUser" {
		t.Errorf("expected symbol name 'GetUser', got '%s'", eps[0].Symbol.Name)
	}
}

func TestHandleEntrypointsWithFilter(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	// Test filtering by type
	req := httptest.NewRequest(http.MethodGet, "/api/entrypoints?type=http", nil)
	w := httptest.NewRecorder()

	s.handleEntrypoints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var eps []store.EntrypointWithSymbol
	if err := json.NewDecoder(w.Body).Decode(&eps); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("expected 1 http entrypoint, got %d", len(eps))
	}

	// Test filtering by query
	req = httptest.NewRequest(http.MethodGet, "/api/entrypoints?query=users", nil)
	w = httptest.NewRecorder()

	s.handleEntrypoints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	eps = nil
	if err := json.NewDecoder(w.Body).Decode(&eps); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(eps) != 1 {
		t.Errorf("expected 1 matching entrypoint, got %d", len(eps))
	}
}

func TestHandleSymbol(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/1", nil)
	w := httptest.NewRecorder()

	s.handleSymbol(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		store.Symbol
		Tags    []store.Tag    `json:"tags"`
		Package *store.Package `json:"package"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Name != "GetUser" {
		t.Errorf("expected name 'GetUser', got '%s'", resp.Name)
	}
	if len(resp.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(resp.Tags))
	}
	if resp.Tags[0].Tag != "layer:handler" {
		t.Errorf("expected tag 'layer:handler', got '%s'", resp.Tags[0].Tag)
	}
	if resp.Package == nil {
		t.Error("expected package info")
	} else if resp.Package.Layer != "handler" {
		t.Errorf("expected layer 'handler', got '%s'", resp.Package.Layer)
	}
}

func TestHandleSymbolNotFound(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/symbol/999", nil)
	w := httptest.NewRecorder()

	s.handleSymbol(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleSearch(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/search?query=GetUser", nil)
	w := httptest.NewRecorder()

	s.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var results []store.SearchResult
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Symbol.Name != "GetUser" {
		t.Errorf("expected name 'GetUser', got '%s'", results[0].Symbol.Name)
	}
}

func TestHandleSearchNoQuery(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	w := httptest.NewRecorder()

	s.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleGraph(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/graph/expand/1", nil)
	w := httptest.NewRecorder()

	s.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp GraphResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Check that we have the root node
	if len(resp.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(resp.Nodes))
	}
	if len(resp.Nodes) > 0 && resp.Nodes[0].Name != "GetUser" {
		t.Errorf("expected node 'GetUser', got '%s'", resp.Nodes[0].Name)
	}
	// No callees in test data means no edges
	if len(resp.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(resp.Edges))
	}
}

func TestHandleGraphWithDepth(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/graph/root/1?depth=5", nil)
	w := httptest.NewRecorder()

	s.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp GraphResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MaxDepth != 5 {
		t.Errorf("expected max_depth 5, got %d", resp.MaxDepth)
	}
}

func TestHandleGraphWithFilters(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	filters := `{"hideStdlib":true,"maxDepth":3}`
	req := httptest.NewRequest(http.MethodGet, "/api/graph/root/1?filters="+filters, nil)
	w := httptest.NewRecorder()

	s.handleGraph(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp GraphResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Just verify the response is valid
	if resp.RootID != 1 {
		t.Errorf("expected root_id 1, got %d", resp.RootID)
	}
}

func TestCorsMiddleware(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	handler := s.corsMiddleware(s.handleHealth)

	// Test OPTIONS request
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header")
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := setupTestServer(t)
	defer s.store.Close()

	// POST to a GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}
