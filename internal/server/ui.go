package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
)

// UIHandler creates a handler for serving the React UI.
// It looks for UI files in the following locations:
// 1. ./ui/dist (development)
// 2. <executable-dir>/ui/dist (installed)
// 3. Falls back to a placeholder page
func UIHandler() http.Handler {
	// Try to find UI files
	uiPath := findUIPath()
	if uiPath != "" {
		return &spaHandler{root: uiPath}
	}

	// Fallback to placeholder
	return http.HandlerFunc(placeholderHandler)
}

// findUIPath looks for the UI dist directory in common locations.
func findUIPath() string {
	// Check relative to working directory
	if info, err := os.Stat("ui/dist/index.html"); err == nil && !info.IsDir() {
		return "ui/dist"
	}

	// Check relative to executable
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		uiPath := filepath.Join(exeDir, "ui", "dist")
		if info, err := os.Stat(filepath.Join(uiPath, "index.html")); err == nil && !info.IsDir() {
			return uiPath
		}
	}

	return ""
}

// spaHandler serves a single-page application, falling back to index.html for routes.
type spaHandler struct {
	root string
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Clean the path
	p := path.Clean(r.URL.Path)
	if p == "/" {
		p = "/index.html"
	}

	// Build the file path
	filePath := filepath.Join(h.root, filepath.FromSlash(p))

	// Check if file exists
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		// File not found or is directory, serve index.html for SPA routing
		filePath = filepath.Join(h.root, "index.html")
	}

	// Set content type based on extension
	ext := path.Ext(filePath)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	}

	http.ServeFile(w, r, filePath)
}

// placeholderHandler returns a placeholder page when the UI is not built.
func placeholderHandler(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>FlowLens</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
            background: #1a1a1a;
            color: #e5e5e5;
        }
        h1 { color: #60a5fa; }
        .api-list { background: #2a2a2a; padding: 20px; border-radius: 8px; }
        .api-list a { display: block; margin: 10px 0; color: #60a5fa; }
        pre { background: #333; padding: 10px; border-radius: 4px; overflow-x: auto; }
        .warning { background: #78350f; padding: 15px; border-radius: 8px; margin-bottom: 20px; }
    </style>
</head>
<body>
    <h1>FlowLens API Server</h1>
    <div class="warning">
        <strong>UI Not Found</strong><br>
        The React UI is not built. Build it with:
        <pre>cd ui && npm install && npm run build</pre>
    </div>
    <div class="api-list">
        <h3>Available API Endpoints:</h3>
        <a href="/api/stats">GET /api/stats</a> - Index statistics
        <a href="/api/entrypoints">GET /api/entrypoints</a> - List all entrypoints
        <a href="/api/entrypoints?type=http">GET /api/entrypoints?type=http</a> - HTTP entrypoints only
        <a href="/api/search?query=main">GET /api/search?query=main</a> - Search symbols
        <a href="/api/health">GET /api/health</a> - Health check
    </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// Helper to suppress unused import warning
var _ fs.FS
