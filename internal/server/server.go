package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/abramin/flowlens/internal/store"
)

// Server is the FlowLens HTTP server.
type Server struct {
	store      *store.Store
	httpServer *http.Server
	port       int
}

// Config holds server configuration.
type Config struct {
	Port       int
	ProjectDir string
}

// New creates a new server instance.
func New(cfg Config) (*Server, error) {
	st, err := store.Open(cfg.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	s := &Server{
		store: st,
		port:  cfg.Port,
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/entrypoints", s.corsMiddleware(s.handleEntrypoints))
	mux.HandleFunc("/api/entrypoints/", s.corsMiddleware(s.handleEntrypointByID))
	mux.HandleFunc("/api/symbol/", s.corsMiddleware(s.handleSymbol))
	mux.HandleFunc("/api/search", s.corsMiddleware(s.handleSearch))
	mux.HandleFunc("/api/graph/", s.corsMiddleware(s.handleGraph))
	mux.HandleFunc("/api/stats", s.corsMiddleware(s.handleStats))

	// Health check
	mux.HandleFunc("/api/health", s.corsMiddleware(s.handleHealth))

	// Static files (placeholder - will serve embedded UI later)
	mux.HandleFunc("/", s.handleStatic)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s, nil
}

// Start starts the server and blocks until shutdown.
func (s *Server) Start() error {
	// Setup graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on http://localhost:%d", s.port)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	if err := s.store.Close(); err != nil {
		return fmt.Errorf("closing store: %w", err)
	}

	log.Println("Server stopped")
	return nil
}

// Port returns the configured port.
func (s *Server) Port() int {
	return s.port
}

// corsMiddleware adds CORS headers for local development.
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStats returns index statistics.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats, err := s.store.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleEntrypoints handles GET /api/entrypoints
func (s *Server) handleEntrypoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	filter := store.EntrypointFilter{
		Type:  store.EntrypointType(r.URL.Query().Get("type")),
		Query: r.URL.Query().Get("query"),
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = limit
		}
	}

	entrypoints, err := s.store.GetEntrypoints(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get entrypoints")
		return
	}

	writeJSON(w, http.StatusOK, entrypoints)
}

// handleEntrypointByID handles GET /api/entrypoints/:id
func (s *Server) handleEntrypointByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract ID from path: /api/entrypoints/123
	path := strings.TrimPrefix(r.URL.Path, "/api/entrypoints/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entrypoint ID")
		return
	}

	ep, err := s.store.GetEntrypointByID(store.EntrypointID(id))
	if err != nil {
		writeError(w, http.StatusNotFound, "entrypoint not found")
		return
	}

	writeJSON(w, http.StatusOK, ep)
}

// handleSymbol handles GET /api/symbol/:id
func (s *Server) handleSymbol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Extract ID from path: /api/symbol/123
	path := strings.TrimPrefix(r.URL.Path, "/api/symbol/")
	id, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid symbol ID")
		return
	}

	sym, err := s.store.GetSymbolByID(store.SymbolID(id))
	if err != nil {
		writeError(w, http.StatusNotFound, "symbol not found")
		return
	}

	tags, err := s.store.GetSymbolTags(store.SymbolID(id))
	if err != nil {
		tags = []store.Tag{} // Don't fail if tags can't be fetched
	}

	// Get package info
	pkg, _ := s.store.GetPackageByPath(sym.PkgPath)

	response := struct {
		*store.Symbol
		Tags    []store.Tag    `json:"tags"`
		Package *store.Package `json:"package,omitempty"`
	}{
		Symbol:  sym,
		Tags:    tags,
		Package: pkg,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleSearch handles GET /api/search?query=xxx
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		writeError(w, http.StatusBadRequest, "query parameter required")
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	results, err := s.store.SearchSymbols(query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	writeJSON(w, http.StatusOK, results)
}

// handleGraph handles graph-related endpoints
// GET /api/graph/root/:symbolId - get graph starting from symbol
// GET /api/graph/expand/:symbolId - get callees of a symbol
func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/graph/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, "invalid graph endpoint")
		return
	}

	action := parts[0]
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid symbol ID")
		return
	}

	symbolID := store.SymbolID(id)

	switch action {
	case "root", "expand":
		// Get the root symbol
		sym, err := s.store.GetSymbolByID(symbolID)
		if err != nil {
			writeError(w, http.StatusNotFound, "symbol not found")
			return
		}

		tags, _ := s.store.GetSymbolTags(symbolID)

		// Get callees
		callees, err := s.store.GetCallees(symbolID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get callees")
			return
		}

		response := struct {
			Symbol  *store.Symbol      `json:"symbol"`
			Tags    []store.Tag        `json:"tags"`
			Callees []store.CalleeInfo `json:"callees"`
		}{
			Symbol:  sym,
			Tags:    tags,
			Callees: callees,
		}

		writeJSON(w, http.StatusOK, response)

	default:
		writeError(w, http.StatusBadRequest, "invalid graph action")
	}
}

// handleStatic serves the UI static files.
// For now, returns a placeholder until the UI is built.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Placeholder HTML until React UI is embedded
	html := `<!DOCTYPE html>
<html>
<head>
    <title>FlowLens</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
               max-width: 800px; margin: 50px auto; padding: 20px; }
        h1 { color: #333; }
        .api-list { background: #f5f5f5; padding: 20px; border-radius: 8px; }
        .api-list a { display: block; margin: 10px 0; color: #0066cc; }
        pre { background: #f0f0f0; padding: 10px; border-radius: 4px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>FlowLens API Server</h1>
    <p>The React UI is not yet built. Use the API endpoints below:</p>
    <div class="api-list">
        <h3>Available Endpoints:</h3>
        <a href="/api/stats">GET /api/stats</a> - Index statistics
        <a href="/api/entrypoints">GET /api/entrypoints</a> - List all entrypoints
        <a href="/api/entrypoints?type=http">GET /api/entrypoints?type=http</a> - HTTP entrypoints only
        <a href="/api/search?query=main">GET /api/search?query=main</a> - Search symbols
        <a href="/api/health">GET /api/health</a> - Health check
    </div>
    <h3>Example Usage:</h3>
    <pre>
# Get all entrypoints
curl http://localhost:` + strconv.Itoa(s.port) + `/api/entrypoints

# Search for symbols
curl http://localhost:` + strconv.Itoa(s.port) + `/api/search?query=Handler

# Get symbol details
curl http://localhost:` + strconv.Itoa(s.port) + `/api/symbol/1

# Get call graph from symbol
curl http://localhost:` + strconv.Itoa(s.port) + `/api/graph/expand/1
    </pre>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
