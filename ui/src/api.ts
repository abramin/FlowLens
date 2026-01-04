import type { Entrypoint, GraphResponse, GraphFilter, Stats, Symbol, Tag, SymbolDetails } from './types';

const API_BASE = '/api';

async function fetchJSON<T>(url: string): Promise<T> {
  const response = await fetch(url);
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }
  return response.json();
}

export async function getStats(): Promise<Stats> {
  return fetchJSON<Stats>(`${API_BASE}/stats`);
}

export async function getEntrypoints(type?: string, query?: string): Promise<Entrypoint[]> {
  const params = new URLSearchParams();
  if (type) params.set('type', type);
  if (query) params.set('query', query);
  const queryString = params.toString();
  const url = queryString ? `${API_BASE}/entrypoints?${queryString}` : `${API_BASE}/entrypoints`;
  return fetchJSON<Entrypoint[]>(url);
}

export async function getEntrypointById(id: number): Promise<Entrypoint> {
  return fetchJSON<Entrypoint>(`${API_BASE}/entrypoints/${id}`);
}

export async function getSymbol(id: number): Promise<SymbolDetails> {
  return fetchJSON<SymbolDetails>(`${API_BASE}/symbol/${id}`);
}

export async function getGraphRoot(
  symbolId: number,
  depth?: number,
  filters?: GraphFilter
): Promise<GraphResponse> {
  const params = new URLSearchParams();
  if (depth) params.set('depth', depth.toString());
  if (filters) params.set('filters', JSON.stringify(filters));
  const queryString = params.toString();
  const url = queryString
    ? `${API_BASE}/graph/root/${symbolId}?${queryString}`
    : `${API_BASE}/graph/root/${symbolId}`;
  return fetchJSON<GraphResponse>(url);
}

export async function getGraphExpand(
  symbolId: number,
  depth?: number,
  filters?: GraphFilter
): Promise<GraphResponse> {
  const params = new URLSearchParams();
  if (depth) params.set('depth', depth.toString());
  if (filters) params.set('filters', JSON.stringify(filters));
  const queryString = params.toString();
  const url = queryString
    ? `${API_BASE}/graph/expand/${symbolId}?${queryString}`
    : `${API_BASE}/graph/expand/${symbolId}`;
  return fetchJSON<GraphResponse>(url);
}

export async function searchSymbols(query: string, limit?: number): Promise<Array<{ symbol: Symbol; tags: Tag[] }>> {
  const params = new URLSearchParams({ query });
  if (limit) params.set('limit', limit.toString());
  return fetchJSON<Array<{ symbol: Symbol; tags: Tag[] }>>(`${API_BASE}/search?${params}`);
}
