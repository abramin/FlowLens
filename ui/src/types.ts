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
  collapseWiring?: boolean;  // Collapse wiring/config functions (default ON)
  hideCmdMain?: boolean;     // Hide cmd/* packages (default ON)
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

// Call Spine Types
export interface BranchBadge {
  call_count: number;
  collapsed_ids: number[];
  labels: string[];
}

export interface SpineNode {
  id: number;
  name: string;
  pkg_path: string;
  recv_type?: string;
  file: string;
  line: number;
  tags: string[];
  depth: number;
  is_main_path: boolean;
  branch_badge?: BranchBadge;
  layer?: string;
}

export interface SpineResponse {
  nodes: SpineNode[];
  main_path: number[];
  total_nodes: number;
  collapsed_count: number;
}

// CFG Types
export type CFGViewMode = 'summary' | 'detail';

export interface InstructionInfo {
  index: number;
  op: string;
  text: string;
  callee_id?: number;
}

export interface BasicBlockInfo {
  index: number;
  instructions: InstructionInfo[];
  successors: number[];
  predecessors: number[];
  is_entry: boolean;
  is_exit: boolean;
  branch_cond?: string;
}

export interface CFGInfo {
  symbol_id: number;
  name: string;
  signature?: string;
  blocks: BasicBlockInfo[];
  entry_block: number;
  exit_blocks: number[];
}

// Filter presets
export type FilterPreset = 'default' | 'deep-dive' | 'high-level';

export const FILTER_PRESETS: Record<FilterPreset, GraphFilter> = {
  'default': {
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 6,
    collapseWiring: true,  // ON by default for cleaner graphs
    hideCmdMain: true,     // ON by default
  },
  'deep-dive': {
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 10,
    collapseWiring: false, // Show everything
    hideCmdMain: false,
  },
  'high-level': {
    hideStdlib: true,
    hideVendors: true,
    stopAtIO: true,
    maxDepth: 3,
    collapseWiring: true,
    hideCmdMain: true,
  },
};
