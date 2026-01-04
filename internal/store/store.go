package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store handles persistence of indexed data to SQLite.
type Store struct {
	db      *sql.DB
	dbPath  string
	baseDir string // Project root directory
}

// Open creates or opens a FlowLens index database.
// By default, stores at .flowlens/index.db relative to the given project directory.
func Open(projectDir string) (*Store, error) {
	flowlensDir := filepath.Join(projectDir, ".flowlens")
	if err := os.MkdirAll(flowlensDir, 0755); err != nil {
		return nil, fmt.Errorf("creating .flowlens directory: %w", err)
	}

	dbPath := filepath.Join(flowlensDir, "index.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable foreign keys and WAL mode for better performance
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	// Create schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &Store{
		db:      db,
		dbPath:  dbPath,
		baseDir: projectDir,
	}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DBPath returns the path to the database file.
func (s *Store) DBPath() string {
	return s.dbPath
}

// Clear removes all data from the database (for re-indexing).
func (s *Store) Clear() error {
	tables := []string{"tags", "entrypoints", "call_edges", "symbols", "packages", "metadata"}
	for _, table := range tables {
		if _, err := s.db.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("clearing table %s: %w", table, err)
		}
	}
	return nil
}

// InsertPackage inserts or updates a package.
func (s *Store) InsertPackage(pkg *Package) error {
	_, err := s.db.Exec(`
		INSERT INTO packages (pkg_path, module, dir, layer)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pkg_path) DO UPDATE SET
			module = excluded.module,
			dir = excluded.dir,
			layer = excluded.layer
	`, pkg.PkgPath, pkg.Module, pkg.Dir, pkg.Layer)
	return err
}

// InsertSymbol inserts a symbol and returns its ID.
func (s *Store) InsertSymbol(sym *Symbol) (SymbolID, error) {
	result, err := s.db.Exec(`
		INSERT INTO symbols (pkg_path, name, kind, recv_type, file, line, sig)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pkg_path, name, recv_type) DO UPDATE SET
			kind = excluded.kind,
			file = excluded.file,
			line = excluded.line,
			sig = excluded.sig
	`, sym.PkgPath, sym.Name, sym.Kind, sym.RecvType, sym.File, sym.Line, sym.Sig)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		// If LastInsertId fails (e.g., on conflict update), look it up
		return s.GetSymbolID(sym.PkgPath, sym.Name, sym.RecvType)
	}
	return SymbolID(id), nil
}

// GetSymbolID looks up a symbol's ID by its unique key.
func (s *Store) GetSymbolID(pkgPath, name, recvType string) (SymbolID, error) {
	var id int64
	err := s.db.QueryRow(`
		SELECT id FROM symbols
		WHERE pkg_path = ? AND name = ? AND (recv_type = ? OR (recv_type IS NULL AND ? = ''))
	`, pkgPath, name, recvType, recvType).Scan(&id)
	if err != nil {
		return 0, err
	}
	return SymbolID(id), nil
}

// InsertCallEdge inserts a call edge.
func (s *Store) InsertCallEdge(edge *CallEdge) error {
	_, err := s.db.Exec(`
		INSERT INTO call_edges (caller_id, callee_id, caller_file, caller_line, call_kind, count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(caller_id, callee_id, caller_file, caller_line) DO UPDATE SET
			count = call_edges.count + excluded.count
	`, edge.CallerID, edge.CalleeID, edge.CallerFile, edge.CallerLine, edge.CallKind, edge.Count)
	return err
}

// InsertEntrypoint inserts an entrypoint and returns its ID.
func (s *Store) InsertEntrypoint(ep *Entrypoint) (EntrypointID, error) {
	result, err := s.db.Exec(`
		INSERT INTO entrypoints (type, label, symbol_id, meta_json)
		VALUES (?, ?, ?, ?)
	`, ep.Type, ep.Label, ep.SymbolID, ep.MetaJSON)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return EntrypointID(id), err
}

// InsertTag inserts a tag on a symbol.
func (s *Store) InsertTag(tag *Tag) error {
	_, err := s.db.Exec(`
		INSERT INTO tags (symbol_id, tag, reason)
		VALUES (?, ?, ?)
		ON CONFLICT(symbol_id, tag) DO UPDATE SET
			reason = excluded.reason
	`, tag.SymbolID, tag.Tag, tag.Reason)
	return err
}

// SetMetadata stores a key-value pair in the metadata table.
func (s *Store) SetMetadata(key, value string) error {
	_, err := s.db.Exec(`
		INSERT INTO metadata (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

// GetMetadata retrieves a value from the metadata table.
func (s *Store) GetMetadata(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM metadata WHERE key = ?", key).Scan(&value)
	return value, err
}

// Stats holds statistics about the indexed data.
type Stats struct {
	PackageCount    int       `json:"package_count"`
	SymbolCount     int       `json:"symbol_count"`
	CallEdgeCount   int       `json:"call_edge_count"`
	EntrypointCount int       `json:"entrypoint_count"`
	TagCount        int       `json:"tag_count"`
	IndexedAt       time.Time `json:"indexed_at"`
}

// GetStats returns statistics about the indexed data.
func (s *Store) GetStats() (*Stats, error) {
	stats := &Stats{}

	rows := []struct {
		table string
		dest  *int
	}{
		{"packages", &stats.PackageCount},
		{"symbols", &stats.SymbolCount},
		{"call_edges", &stats.CallEdgeCount},
		{"entrypoints", &stats.EntrypointCount},
		{"tags", &stats.TagCount},
	}

	for _, r := range rows {
		err := s.db.QueryRow("SELECT COUNT(*) FROM " + r.table).Scan(r.dest)
		if err != nil {
			return nil, fmt.Errorf("counting %s: %w", r.table, err)
		}
	}

	// Get indexed timestamp from metadata
	if ts, err := s.GetMetadata("indexed_at"); err == nil {
		stats.IndexedAt, _ = time.Parse(time.RFC3339, ts)
	}

	return stats, nil
}

// IndexMetadata holds metadata written to index.json for quick UI boot.
type IndexMetadata struct {
	Version         string    `json:"version"`
	ProjectPath     string    `json:"project_path"`
	IndexedAt       time.Time `json:"indexed_at"`
	PackageCount    int       `json:"package_count"`
	SymbolCount     int       `json:"symbol_count"`
	EntrypointCount int       `json:"entrypoint_count"`
	Packages        []string  `json:"packages"` // List of package paths
}

// WriteIndexJSON writes index.json for quick UI boot.
func (s *Store) WriteIndexJSON() error {
	stats, err := s.GetStats()
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	// Get list of packages
	rows, err := s.db.Query("SELECT pkg_path FROM packages ORDER BY pkg_path")
	if err != nil {
		return fmt.Errorf("querying packages: %w", err)
	}
	defer rows.Close()

	var packages []string
	for rows.Next() {
		var pkgPath string
		if err := rows.Scan(&pkgPath); err != nil {
			return fmt.Errorf("scanning package: %w", err)
		}
		packages = append(packages, pkgPath)
	}

	meta := &IndexMetadata{
		Version:         "1",
		ProjectPath:     s.baseDir,
		IndexedAt:       stats.IndexedAt,
		PackageCount:    stats.PackageCount,
		SymbolCount:     stats.SymbolCount,
		EntrypointCount: stats.EntrypointCount,
		Packages:        packages,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling index.json: %w", err)
	}

	indexPath := filepath.Join(filepath.Dir(s.dbPath), "index.json")
	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("writing index.json: %w", err)
	}

	return nil
}

// Tx returns the underlying database for advanced queries.
// Use with caution - prefer adding methods to Store instead.
func (s *Store) Tx() *sql.DB {
	return s.db
}

// BeginBatch starts a transaction for batch inserts.
// Call Commit() when done, or Rollback() on error.
func (s *Store) BeginBatch() (*BatchTx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &BatchTx{tx: tx}, nil
}

// BatchTx wraps a transaction for batch operations.
type BatchTx struct {
	tx *sql.Tx
}

// Commit commits the batch transaction.
func (b *BatchTx) Commit() error {
	return b.tx.Commit()
}

// Rollback rolls back the batch transaction.
func (b *BatchTx) Rollback() error {
	return b.tx.Rollback()
}

// InsertPackage inserts a package within the batch.
func (b *BatchTx) InsertPackage(pkg *Package) error {
	_, err := b.tx.Exec(`
		INSERT INTO packages (pkg_path, module, dir, layer)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(pkg_path) DO UPDATE SET
			module = excluded.module,
			dir = excluded.dir,
			layer = excluded.layer
	`, pkg.PkgPath, pkg.Module, pkg.Dir, pkg.Layer)
	return err
}

// InsertSymbol inserts a symbol within the batch and returns its ID.
func (b *BatchTx) InsertSymbol(sym *Symbol) (SymbolID, error) {
	result, err := b.tx.Exec(`
		INSERT INTO symbols (pkg_path, name, kind, recv_type, file, line, sig)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(pkg_path, name, recv_type) DO UPDATE SET
			kind = excluded.kind,
			file = excluded.file,
			line = excluded.line,
			sig = excluded.sig
	`, sym.PkgPath, sym.Name, sym.Kind, sym.RecvType, sym.File, sym.Line, sym.Sig)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return SymbolID(id), nil
}

// InsertCallEdge inserts a call edge within the batch.
func (b *BatchTx) InsertCallEdge(edge *CallEdge) error {
	_, err := b.tx.Exec(`
		INSERT INTO call_edges (caller_id, callee_id, caller_file, caller_line, call_kind, count)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(caller_id, callee_id, caller_file, caller_line) DO UPDATE SET
			count = call_edges.count + excluded.count
	`, edge.CallerID, edge.CalleeID, edge.CallerFile, edge.CallerLine, edge.CallKind, edge.Count)
	return err
}

// GetSymbolID looks up a symbol's ID by its unique key within the batch.
func (b *BatchTx) GetSymbolID(pkgPath, name, recvType string) (SymbolID, error) {
	var id int64
	err := b.tx.QueryRow(`
		SELECT id FROM symbols
		WHERE pkg_path = ? AND name = ? AND (recv_type = ? OR (recv_type IS NULL AND ? = ''))
	`, pkgPath, name, recvType, recvType).Scan(&id)
	if err != nil {
		return 0, err
	}
	return SymbolID(id), nil
}

// InsertEntrypoint inserts an entrypoint within the batch and returns its ID.
func (b *BatchTx) InsertEntrypoint(ep *Entrypoint) error {
	_, err := b.tx.Exec(`
		INSERT INTO entrypoints (type, label, symbol_id, meta_json)
		VALUES (?, ?, ?, ?)
	`, ep.Type, ep.Label, ep.SymbolID, ep.MetaJSON)
	return err
}

// InsertTag inserts a tag on a symbol within the batch.
func (b *BatchTx) InsertTag(tag *Tag) error {
	_, err := b.tx.Exec(`
		INSERT INTO tags (symbol_id, tag, reason)
		VALUES (?, ?, ?)
		ON CONFLICT(symbol_id, tag) DO UPDATE SET
			reason = excluded.reason
	`, tag.SymbolID, tag.Tag, tag.Reason)
	return err
}

// SymbolForTagging holds symbol data needed for tagging.
type SymbolForTagging struct {
	ID       SymbolID
	PkgPath  string
	Name     string
	Kind     SymbolKind
	RecvType string
}

// GetAllSymbolsForTagging returns all symbols with the data needed for tagging.
func (s *Store) GetAllSymbolsForTagging() ([]SymbolForTagging, error) {
	rows, err := s.db.Query(`
		SELECT id, pkg_path, name, kind, COALESCE(recv_type, '') as recv_type
		FROM symbols
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []SymbolForTagging
	for rows.Next() {
		var sym SymbolForTagging
		if err := rows.Scan(&sym.ID, &sym.PkgPath, &sym.Name, &sym.Kind, &sym.RecvType); err != nil {
			return nil, err
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
}

// PackageImport represents an import relationship between packages.
type PackageImport struct {
	PkgPath       string
	ImportedPkg   string
}

// GetPackageImports returns all package import relationships from call edges.
// A package is considered to import another if it has any call edges to symbols in that package.
func (s *Store) GetPackageImports() (map[string][]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT s1.pkg_path as caller_pkg, s2.pkg_path as callee_pkg
		FROM call_edges ce
		JOIN symbols s1 ON ce.caller_id = s1.id
		JOIN symbols s2 ON ce.callee_id = s2.id
		WHERE s1.pkg_path != s2.pkg_path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	imports := make(map[string][]string)
	for rows.Next() {
		var callerPkg, calleePkg string
		if err := rows.Scan(&callerPkg, &calleePkg); err != nil {
			return nil, err
		}
		imports[callerPkg] = append(imports[callerPkg], calleePkg)
	}
	return imports, rows.Err()
}

// SymbolCallee represents a callee symbol with its tags.
type SymbolCallee struct {
	CallerID SymbolID
	CalleeID SymbolID
	Tags     []string // Tags on the callee
}

// GetSymbolCalleesWithTags returns all caller-callee relationships with callee tags.
// Used for purity analysis.
func (s *Store) GetSymbolCalleesWithTags() (map[SymbolID][]SymbolCallee, error) {
	rows, err := s.db.Query(`
		SELECT ce.caller_id, ce.callee_id, COALESCE(GROUP_CONCAT(t.tag), '') as tags
		FROM call_edges ce
		LEFT JOIN tags t ON ce.callee_id = t.symbol_id
		GROUP BY ce.caller_id, ce.callee_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[SymbolID][]SymbolCallee)
	for rows.Next() {
		var callerID, calleeID SymbolID
		var tagsStr string
		if err := rows.Scan(&callerID, &calleeID, &tagsStr); err != nil {
			return nil, err
		}
		var tags []string
		if tagsStr != "" {
			tags = splitTags(tagsStr)
		}
		result[callerID] = append(result[callerID], SymbolCallee{
			CallerID: callerID,
			CalleeID: calleeID,
			Tags:     tags,
		})
	}
	return result, rows.Err()
}

// splitTags splits a comma-separated tag string.
func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// ============================================================================
// Query Methods for API
// ============================================================================

// GetSymbolByID retrieves a symbol by its ID with full details.
func (s *Store) GetSymbolByID(id SymbolID) (*Symbol, error) {
	sym := &Symbol{}
	var recvType sql.NullString
	err := s.db.QueryRow(`
		SELECT id, pkg_path, name, kind, recv_type, file, line, COALESCE(sig, '') as sig
		FROM symbols WHERE id = ?
	`, id).Scan(&sym.ID, &sym.PkgPath, &sym.Name, &sym.Kind, &recvType, &sym.File, &sym.Line, &sym.Sig)
	if err != nil {
		return nil, err
	}
	if recvType.Valid {
		sym.RecvType = recvType.String
	}
	return sym, nil
}

// GetSymbolTags retrieves all tags for a symbol.
func (s *Store) GetSymbolTags(id SymbolID) ([]Tag, error) {
	rows, err := s.db.Query(`
		SELECT symbol_id, tag, COALESCE(reason, '') as reason
		FROM tags WHERE symbol_id = ?
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.SymbolID, &t.Tag, &t.Reason); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// EntrypointFilter specifies filtering options for GetEntrypoints.
type EntrypointFilter struct {
	Type  EntrypointType // Filter by type (empty = all)
	Query string         // Search in label (empty = all)
	Limit int            // Max results (0 = no limit)
}

// EntrypointWithSymbol combines entrypoint with its symbol details.
type EntrypointWithSymbol struct {
	Entrypoint
	Symbol Symbol `json:"symbol"`
}

// GetEntrypoints retrieves entrypoints with optional filtering.
func (s *Store) GetEntrypoints(filter EntrypointFilter) ([]EntrypointWithSymbol, error) {
	query := `
		SELECT e.id, e.type, e.label, e.symbol_id, COALESCE(e.meta_json, '') as meta_json,
		       s.id, s.pkg_path, s.name, s.kind, COALESCE(s.recv_type, '') as recv_type,
		       s.file, s.line, COALESCE(s.sig, '') as sig
		FROM entrypoints e
		JOIN symbols s ON e.symbol_id = s.id
		WHERE 1=1
	`
	var args []interface{}

	if filter.Type != "" {
		query += " AND e.type = ?"
		args = append(args, filter.Type)
	}
	if filter.Query != "" {
		query += " AND e.label LIKE ?"
		args = append(args, "%"+filter.Query+"%")
	}

	query += " ORDER BY e.type, e.label"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EntrypointWithSymbol
	for rows.Next() {
		var ep EntrypointWithSymbol
		err := rows.Scan(
			&ep.ID, &ep.Type, &ep.Label, &ep.SymbolID, &ep.MetaJSON,
			&ep.Symbol.ID, &ep.Symbol.PkgPath, &ep.Symbol.Name, &ep.Symbol.Kind,
			&ep.Symbol.RecvType, &ep.Symbol.File, &ep.Symbol.Line, &ep.Symbol.Sig,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, ep)
	}
	return results, rows.Err()
}

// GetEntrypointByID retrieves a single entrypoint with its symbol.
func (s *Store) GetEntrypointByID(id EntrypointID) (*EntrypointWithSymbol, error) {
	ep := &EntrypointWithSymbol{}
	err := s.db.QueryRow(`
		SELECT e.id, e.type, e.label, e.symbol_id, COALESCE(e.meta_json, '') as meta_json,
		       s.id, s.pkg_path, s.name, s.kind, COALESCE(s.recv_type, '') as recv_type,
		       s.file, s.line, COALESCE(s.sig, '') as sig
		FROM entrypoints e
		JOIN symbols s ON e.symbol_id = s.id
		WHERE e.id = ?
	`, id).Scan(
		&ep.ID, &ep.Type, &ep.Label, &ep.SymbolID, &ep.MetaJSON,
		&ep.Symbol.ID, &ep.Symbol.PkgPath, &ep.Symbol.Name, &ep.Symbol.Kind,
		&ep.Symbol.RecvType, &ep.Symbol.File, &ep.Symbol.Line, &ep.Symbol.Sig,
	)
	if err != nil {
		return nil, err
	}
	return ep, nil
}

// SearchResult represents a symbol search result.
type SearchResult struct {
	Symbol Symbol `json:"symbol"`
	Tags   []Tag  `json:"tags,omitempty"`
}

// SearchSymbols performs a fuzzy search on symbol names.
func (s *Store) SearchSymbols(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	// Search by name containing the query (case-insensitive)
	rows, err := s.db.Query(`
		SELECT id, pkg_path, name, kind, COALESCE(recv_type, '') as recv_type,
		       file, line, COALESCE(sig, '') as sig
		FROM symbols
		WHERE name LIKE ? OR pkg_path LIKE ?
		ORDER BY
			CASE WHEN name = ? THEN 0
			     WHEN name LIKE ? THEN 1
			     ELSE 2
			END,
			name
		LIMIT ?
	`, "%"+query+"%", "%"+query+"%", query, query+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sym Symbol
		err := rows.Scan(&sym.ID, &sym.PkgPath, &sym.Name, &sym.Kind,
			&sym.RecvType, &sym.File, &sym.Line, &sym.Sig)
		if err != nil {
			return nil, err
		}
		results = append(results, SearchResult{Symbol: sym})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch tags for each result
	for i := range results {
		tags, err := s.GetSymbolTags(results[i].Symbol.ID)
		if err != nil {
			return nil, err
		}
		results[i].Tags = tags
	}

	return results, nil
}

// CalleeInfo represents a callee with call site information.
type CalleeInfo struct {
	Symbol     Symbol   `json:"symbol"`
	CallKind   CallKind `json:"call_kind"`
	CallerFile string   `json:"caller_file"`
	CallerLine int      `json:"caller_line"`
	Count      int      `json:"count"`
	Tags       []Tag    `json:"tags,omitempty"`
}

// GetCallees retrieves all symbols called by the given symbol.
func (s *Store) GetCallees(callerID SymbolID) ([]CalleeInfo, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.pkg_path, s.name, s.kind, COALESCE(s.recv_type, '') as recv_type,
		       s.file, s.line, COALESCE(s.sig, '') as sig,
		       ce.call_kind, ce.caller_file, ce.caller_line, ce.count
		FROM call_edges ce
		JOIN symbols s ON ce.callee_id = s.id
		WHERE ce.caller_id = ?
		ORDER BY ce.caller_line
	`, callerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CalleeInfo
	for rows.Next() {
		var c CalleeInfo
		err := rows.Scan(
			&c.Symbol.ID, &c.Symbol.PkgPath, &c.Symbol.Name, &c.Symbol.Kind,
			&c.Symbol.RecvType, &c.Symbol.File, &c.Symbol.Line, &c.Symbol.Sig,
			&c.CallKind, &c.CallerFile, &c.CallerLine, &c.Count,
		)
		if err != nil {
			return nil, err
		}
		results = append(results, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch tags for each callee
	for i := range results {
		tags, err := s.GetSymbolTags(results[i].Symbol.ID)
		if err != nil {
			return nil, err
		}
		results[i].Tags = tags
	}

	return results, nil
}

// GetPackageByPath retrieves a package by its path.
func (s *Store) GetPackageByPath(pkgPath string) (*Package, error) {
	pkg := &Package{}
	var module, layer sql.NullString
	err := s.db.QueryRow(`
		SELECT pkg_path, module, dir, layer FROM packages WHERE pkg_path = ?
	`, pkgPath).Scan(&pkg.PkgPath, &module, &pkg.Dir, &layer)
	if err != nil {
		return nil, err
	}
	if module.Valid {
		pkg.Module = module.String
	}
	if layer.Valid {
		pkg.Layer = layer.String
	}
	return pkg, nil
}
