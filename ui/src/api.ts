import type { Entrypoint, GraphResponse, GraphFilter, Stats, Symbol, Tag, SymbolDetails, SpineResponse, CFGInfo } from './types';

const API_BASE = '/api';

async function fetchJSON<T>(url: string): Promise<T> {
  let response: Response;
  try {
    response = await fetch(url);
  } catch (err) {
    // Network error (server not running, CORS, etc.)
    const message = err instanceof Error ? err.message : 'Network error';
    throw new Error(`Failed to connect to server: ${message}`);
  }

  if (!response.ok) {
    // Try to get error message from JSON response
    const text = await response.text();
    let errorMessage = `HTTP ${response.status}: ${response.statusText}`;
    try {
      const errorJson = JSON.parse(text);
      if (errorJson.error) {
        errorMessage = errorJson.error;
      }
    } catch {
      // Response wasn't JSON, include raw text if short
      if (text.length < 200) {
        errorMessage = `${errorMessage} - ${text}`;
      }
    }
    throw new Error(errorMessage);
  }

  try {
    return await response.json();
  } catch {
    throw new Error('Invalid JSON response from server');
  }
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

export async function getEntrypointBySymbolId(symbolId: number): Promise<Entrypoint | null> {
  const entrypoints = await getEntrypoints();
  return entrypoints.find((e) => e.symbol_id === symbolId) ?? null;
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

export async function getSpine(
  symbolId: number,
  depth?: number,
  filters?: GraphFilter
): Promise<SpineResponse> {
  const params = new URLSearchParams();
  if (depth) params.set('depth', depth.toString());
  if (filters) params.set('filters', JSON.stringify(filters));
  const queryString = params.toString();
  const url = queryString
    ? `${API_BASE}/spine/${symbolId}?${queryString}`
    : `${API_BASE}/spine/${symbolId}`;
  return fetchJSON<SpineResponse>(url);
}

export async function getCFG(symbolId: number): Promise<CFGInfo> {
  return fetchJSON<CFGInfo>(`${API_BASE}/cfg/${symbolId}`);
}
