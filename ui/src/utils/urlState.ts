import { compressToEncodedURIComponent, decompressFromEncodedURIComponent } from 'lz-string';
import type { GraphFilter } from '../types';

export interface URLState {
  entrypointId: number | null;
  filters: GraphFilter;
  pinnedNodeIds: number[];
  expandedNodeIds: number[];
  selectedNodeId: number | null;
}

// Compact representation for URL encoding
interface CompactURLState {
  e: number | null;        // entrypoint id
  f: GraphFilter;          // filters
  p: number[];             // pinned node ids
  x: number[];             // expanded node ids
  s: number | null;        // selected node id
}

function toCompact(state: URLState): CompactURLState {
  return {
    e: state.entrypointId,
    f: state.filters,
    p: state.pinnedNodeIds,
    x: state.expandedNodeIds,
    s: state.selectedNodeId,
  };
}

function fromCompact(compact: CompactURLState): URLState {
  return {
    entrypointId: compact.e,
    filters: compact.f,
    pinnedNodeIds: compact.p || [],
    expandedNodeIds: compact.x || [],
    selectedNodeId: compact.s,
  };
}

export function encodeURLState(state: URLState): string {
  const compact = toCompact(state);
  const json = JSON.stringify(compact);
  return compressToEncodedURIComponent(json);
}

export function decodeURLState(encoded: string): URLState | null {
  try {
    const json = decompressFromEncodedURIComponent(encoded);
    if (!json) return null;
    const compact = JSON.parse(json) as CompactURLState;
    return fromCompact(compact);
  } catch {
    return null;
  }
}

export function getURLState(): URLState | null {
  const params = new URLSearchParams(window.location.search);
  const encoded = params.get('s');
  if (!encoded) return null;
  return decodeURLState(encoded);
}

export function setURLState(state: URLState): void {
  const encoded = encodeURLState(state);
  const url = new URL(window.location.href);
  url.searchParams.set('s', encoded);
  window.history.replaceState({}, '', url.toString());
}

export function clearURLState(): void {
  const url = new URL(window.location.href);
  url.searchParams.delete('s');
  window.history.replaceState({}, '', url.toString());
}

export function buildShareURL(state: URLState): string {
  const encoded = encodeURLState(state);
  const url = new URL(window.location.href);
  url.searchParams.set('s', encoded);
  return url.toString();
}

export async function copyShareLink(state: URLState): Promise<void> {
  const url = buildShareURL(state);
  await navigator.clipboard.writeText(url);
}
