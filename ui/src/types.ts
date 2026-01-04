// API Types matching the Go backend

export type SymbolKind = 'func' | 'method' | 'type' | 'var' | 'const';
export type CallKind = 'static' | 'interface' | 'funcval' | 'defer' | 'go' | 'unknown';
export type EntrypointType = 'http' | 'grpc' | 'cli' | 'main';

export interface Symbol {
  id: number;
  pkg_path: string;
  name: string;
  kind: SymbolKind;
  recv_type?: string;
  file: string;
  line: number;
  sig?: string;
}

export interface Tag {
  symbol_id: number;
  tag: string;
  reason: string;
}

export interface Entrypoint {
  id: number;
  type: EntrypointType;
  label: string;
  symbol_id: number;
  meta_json?: string;
  symbol: Symbol;
}

export interface GraphNode {
  id: number;
  name: string;
  pkg_path: string;
  file: string;
  line: number;
  kind: SymbolKind;
  recv_type?: string;
  sig?: string;
  tags: string[];
  expanded: boolean;
  depth: number;
}

export interface GraphEdge {
  source_id: number;
  target_id: number;
  call_kind: CallKind;
  callsite_count: number;
  caller_file?: string;
  caller_line?: number;
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
  root_id: number;
  max_depth: number;
  filtered_count: number;
}

export interface GraphFilter {
  hideStdlib?: boolean;
  hideVendors?: boolean;
  stopAtIO?: boolean;
  stopAtPackagePrefix?: string[];
  maxDepth?: number;
  noisePackages?: string[];
}

export interface Stats {
  package_count: number;
  symbol_count: number;
  call_edge_count: number;
  entrypoint_count: number;
  tag_count: number;
  indexed_at: string;
}

// HTTP metadata for entrypoints
export interface HTTPMeta {
  method: string;
  path: string;
}

// gRPC metadata for entrypoints
export interface GRPCMeta {
  service: string;
  method: string;
}

// CLI metadata for entrypoints
export interface CLIMeta {
  command: string;
  uses_run_e?: boolean;
}

export function parseEntrypointMeta(_type: EntrypointType, metaJson?: string): HTTPMeta | GRPCMeta | CLIMeta | null {
  if (!metaJson) return null;
  try {
    return JSON.parse(metaJson);
  } catch {
    return null;
  }
}

// Call info for callers/callees
export interface CallInfo {
  symbol: Symbol;
  call_kind: CallKind;
  caller_file: string;
  caller_line: number;
  count: number;
  tags?: Tag[];
}

// Extended symbol response with callers/callees
export interface SymbolDetails {
  symbol: Symbol;
  tags: Tag[];
  package?: {
    pkg_path: string;
    module?: string;
    dir?: string;
    layer?: string;
  };
  callees: CallInfo[];
  callers: CallInfo[];
}

// Filter presets
export type FilterPreset = 'default' | 'deep-dive' | 'high-level';

export const FILTER_PRESETS: Record<FilterPreset, GraphFilter> = {
  'default': {
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 6,
  },
  'deep-dive': {
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 10,
  },
  'high-level': {
    hideStdlib: true,
    hideVendors: true,
    stopAtIO: true,
    maxDepth: 3,
  },
};
