package store

// schema contains the SQL statements to create the FlowLens database schema.
const schema = `
-- Packages table
CREATE TABLE IF NOT EXISTS packages (
    pkg_path TEXT PRIMARY KEY,
    module   TEXT,
    dir      TEXT NOT NULL,
    layer    TEXT
);

CREATE INDEX IF NOT EXISTS idx_packages_module ON packages(module);
CREATE INDEX IF NOT EXISTS idx_packages_layer ON packages(layer);

-- Symbols table
CREATE TABLE IF NOT EXISTS symbols (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    pkg_path  TEXT NOT NULL,
    name      TEXT NOT NULL,
    kind      TEXT NOT NULL,
    recv_type TEXT,
    file      TEXT NOT NULL,
    line      INTEGER NOT NULL,
    sig       TEXT,
    FOREIGN KEY (pkg_path) REFERENCES packages(pkg_path)
);

CREATE INDEX IF NOT EXISTS idx_symbols_pkg_path ON symbols(pkg_path);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_symbols_kind ON symbols(kind);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file);
CREATE UNIQUE INDEX IF NOT EXISTS idx_symbols_unique ON symbols(pkg_path, name, recv_type);

-- Call edges table
CREATE TABLE IF NOT EXISTS call_edges (
    caller_id   INTEGER NOT NULL,
    callee_id   INTEGER NOT NULL,
    caller_file TEXT NOT NULL,
    caller_line INTEGER NOT NULL,
    call_kind   TEXT NOT NULL,
    count       INTEGER DEFAULT 1,
    PRIMARY KEY (caller_id, callee_id, caller_file, caller_line),
    FOREIGN KEY (caller_id) REFERENCES symbols(id),
    FOREIGN KEY (callee_id) REFERENCES symbols(id)
);

CREATE INDEX IF NOT EXISTS idx_call_edges_caller ON call_edges(caller_id);
CREATE INDEX IF NOT EXISTS idx_call_edges_callee ON call_edges(callee_id);
CREATE INDEX IF NOT EXISTS idx_call_edges_kind ON call_edges(call_kind);

-- Entrypoints table
CREATE TABLE IF NOT EXISTS entrypoints (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    type             TEXT NOT NULL,
    label            TEXT NOT NULL,
    symbol_id        INTEGER NOT NULL,
    meta_json        TEXT,
    discovery_method TEXT DEFAULT 'router',
    FOREIGN KEY (symbol_id) REFERENCES symbols(id)
);

CREATE INDEX IF NOT EXISTS idx_entrypoints_type ON entrypoints(type);
CREATE INDEX IF NOT EXISTS idx_entrypoints_symbol ON entrypoints(symbol_id);
CREATE INDEX IF NOT EXISTS idx_entrypoints_discovery ON entrypoints(discovery_method);

-- Tags table
CREATE TABLE IF NOT EXISTS tags (
    symbol_id INTEGER NOT NULL,
    tag       TEXT NOT NULL,
    reason    TEXT,
    PRIMARY KEY (symbol_id, tag),
    FOREIGN KEY (symbol_id) REFERENCES symbols(id)
);

CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

-- Metadata table for index info
CREATE TABLE IF NOT EXISTS metadata (
    key   TEXT PRIMARY KEY,
    value TEXT
);
`
