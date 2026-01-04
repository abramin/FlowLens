import { useState, useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getSymbol } from '../api';
import type { GraphNode, GraphFilter, CallInfo, SpineNode } from '../types';

interface InspectorPanelProps {
  selectedNode: GraphNode | null;
  filters: GraphFilter;
  onFiltersChange: (filters: GraphFilter) => void;
  onNavigateToNode?: (symbolId: number) => void;
  graphNodes: GraphNode[];
  spineNodes?: SpineNode[];
  onShowCFG?: (symbolId: number, symbolName: string) => void;
}

// Storage key for filter persistence
const FILTERS_STORAGE_KEY = 'flowlens_filters';

// Load filters from localStorage
function loadFiltersFromStorage(): GraphFilter | null {
  try {
    const stored = localStorage.getItem(FILTERS_STORAGE_KEY);
    if (stored) {
      return JSON.parse(stored);
    }
  } catch {
    // Ignore errors
  }
  return null;
}

// Save filters to localStorage
function saveFiltersToStorage(filters: GraphFilter) {
  try {
    localStorage.setItem(FILTERS_STORAGE_KEY, JSON.stringify(filters));
  } catch {
    // Ignore errors
  }
}

// Side effect info with icons and styling
const SIDE_EFFECT_INFO: Record<string, { label: string; icon: string; color: string }> = {
  'io:db': { label: 'Database', icon: '🗄️', color: '#f59e0b' },
  'io:net': { label: 'Network', icon: '🌐', color: '#3b82f6' },
  'io:fs': { label: 'Filesystem', icon: '📁', color: '#10b981' },
  'io:bus': { label: 'Message Bus', icon: '📨', color: '#ec4899' },
};

// Mock SQL queries based on function names (in real app, would come from analysis)
function inferSideEffects(nodes: GraphNode[] | SpineNode[]): Array<{ type: string; label: string; detail: string; nodeId: number }> {
  const effects: Array<{ type: string; label: string; detail: string; nodeId: number }> = [];

  for (const node of nodes) {
    const tags = node.tags || [];
    const name = node.name.toLowerCase();
    const pkgPath = node.pkg_path.toLowerCase();

    // Database operations
    if (tags.includes('io:db')) {
      let detail = 'Database operation';
      if (name.includes('get') || name.includes('find') || name.includes('list') || name.includes('fetch')) {
        detail = `SELECT FROM ${inferTableName(name, pkgPath)} WHERE ...`;
      } else if (name.includes('create') || name.includes('insert') || name.includes('add')) {
        detail = `INSERT INTO ${inferTableName(name, pkgPath)}`;
      } else if (name.includes('update') || name.includes('set')) {
        detail = `UPDATE ${inferTableName(name, pkgPath)} SET ...`;
      } else if (name.includes('delete') || name.includes('remove')) {
        detail = `DELETE FROM ${inferTableName(name, pkgPath)}`;
      }
      effects.push({ type: 'io:db', label: SIDE_EFFECT_INFO['io:db'].label, detail, nodeId: node.id });
    }

    // Network operations
    if (tags.includes('io:net')) {
      let detail = 'Network call';
      if (name.includes('post') || name.includes('send')) {
        detail = `POST /${inferEndpoint(name)}`;
      } else if (name.includes('get') || name.includes('fetch')) {
        detail = `GET /${inferEndpoint(name)}`;
      } else if (pkgPath.includes('redis') || pkgPath.includes('cache')) {
        detail = `GET redis://cache:${inferCacheKey(name)}`;
      }
      effects.push({ type: 'io:net', label: SIDE_EFFECT_INFO['io:net'].label, detail, nodeId: node.id });
    }

    // Message bus
    if (tags.includes('io:bus')) {
      const topic = inferTopicName(name);
      effects.push({ type: 'io:bus', label: SIDE_EFFECT_INFO['io:bus'].label, detail: `Publish ${topic}`, nodeId: node.id });
    }

    // Filesystem
    if (tags.includes('io:fs')) {
      effects.push({ type: 'io:fs', label: SIDE_EFFECT_INFO['io:fs'].label, detail: 'File operation', nodeId: node.id });
    }
  }

  return effects;
}

function inferTableName(funcName: string, pkgPath: string): string {
  // Try to infer table name from function or package
  const parts = pkgPath.split('/');
  const lastPart = parts[parts.length - 1];

  if (lastPart === 'store' || lastPart === 'stores') {
    const secondLast = parts[parts.length - 2];
    if (secondLast && secondLast !== 'internal') return secondLast;
  }

  // Extract from function name
  const words = funcName.replace(/([A-Z])/g, ' $1').toLowerCase().split(' ').filter(Boolean);
  const nouns = words.filter(w => !['get', 'find', 'list', 'create', 'update', 'delete', 'by', 'with', 'for'].includes(w));
  if (nouns.length > 0) return nouns[0];

  return 'table';
}

function inferEndpoint(funcName: string): string {
  const words = funcName.replace(/([A-Z])/g, '/$1').toLowerCase();
  return words.replace(/^\//, '');
}

function inferCacheKey(funcName: string): string {
  return funcName.replace(/([A-Z])/g, ':$1').toLowerCase().replace(/^:/, '');
}

function inferTopicName(funcName: string): string {
  const words = funcName.replace(/([A-Z])/g, '.$1').toLowerCase();
  return words.replace(/^\./, '').replace(/\./g, '_');
}

export function InspectorPanel({
  selectedNode,
  filters,
  onFiltersChange,
  onNavigateToNode,
  graphNodes,
  spineNodes,
  onShowCFG,
}: InspectorPanelProps) {
  const [sideEffectsExpanded, setSideEffectsExpanded] = useState(true);
  const [dependenciesExpanded, setDependenciesExpanded] = useState(false);

  const { data: symbolData, isLoading } = useQuery({
    queryKey: ['symbol', selectedNode?.id],
    queryFn: () => getSymbol(selectedNode!.id),
    enabled: !!selectedNode,
  });

  // Load filters from localStorage on mount
  useEffect(() => {
    const storedFilters = loadFiltersFromStorage();
    if (storedFilters) {
      onFiltersChange(storedFilters);
    }
  }, []);

  // Save filters to localStorage when they change
  useEffect(() => {
    saveFiltersToStorage(filters);
  }, [filters]);

  const handleFilterChange = (updates: Partial<GraphFilter>) => {
    onFiltersChange({ ...filters, ...updates });
  };

  // Collect all side effects from the graph/spine nodes
  const sideEffects = useMemo(() => {
    const nodes = spineNodes && spineNodes.length > 0 ? spineNodes : graphNodes;
    return inferSideEffects(nodes);
  }, [graphNodes, spineNodes]);

  // Group side effects by type
  const groupedSideEffects = useMemo(() => {
    const groups: Record<string, typeof sideEffects> = {};
    for (const effect of sideEffects) {
      if (!groups[effect.type]) {
        groups[effect.type] = [];
      }
      groups[effect.type].push(effect);
    }
    return groups;
  }, [sideEffects]);

  // Check if a node exists in the current graph
  const isNodeInGraph = (symbolId: number) => {
    return graphNodes.some((n) => n.id === symbolId);
  };

  // Get unique services from callees (for dependencies section)
  const dependencies = useMemo(() => {
    if (!symbolData?.callees) return [];
    const services = new Map<string, string>();
    for (const callee of symbolData.callees) {
      const pkgParts = callee.symbol.pkg_path.split('/');
      const serviceName = pkgParts[pkgParts.length - 1];
      if (!services.has(serviceName)) {
        services.set(serviceName, callee.symbol.pkg_path);
      }
    }
    return Array.from(services.entries());
  }, [symbolData]);

  return (
    <div className="flex flex-col h-full bg-[#0d1117] text-gray-100">
      {/* Header */}
      <div className="p-4 border-b border-gray-800/50">
        <h2 className="text-sm font-semibold text-gray-200">Context & Filters</h2>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* View Options Section */}
        <div className="p-4 border-b border-gray-800/50">
          <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-3">View Options</h3>
          <div className="space-y-2.5">
            <label className="flex items-center gap-3 text-sm text-gray-300 cursor-pointer">
              <input
                type="checkbox"
                checked={filters.collapseWiring ?? true}
                onChange={(e) => handleFilterChange({ collapseWiring: e.target.checked })}
                className="w-4 h-4 rounded bg-[#161b22] border-gray-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-0"
              />
              <span>Hide wiring/config</span>
              {filters.collapseWiring && (
                <span className="ml-auto text-xs text-blue-400">ON</span>
              )}
            </label>
            <label className="flex items-center gap-3 text-sm text-gray-300 cursor-pointer">
              <input
                type="checkbox"
                checked={filters.hideStdlib ?? false}
                onChange={(e) => handleFilterChange({ hideStdlib: e.target.checked })}
                className="w-4 h-4 rounded bg-[#161b22] border-gray-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-0"
              />
              <span>Hide stdlib</span>
            </label>
            <label className="flex items-center gap-3 text-sm text-gray-300 cursor-pointer">
              <input
                type="checkbox"
                checked={filters.hideVendors ?? false}
                onChange={(e) => handleFilterChange({ hideVendors: e.target.checked })}
                className="w-4 h-4 rounded bg-[#161b22] border-gray-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-0"
              />
              <span>Hide vendors</span>
            </label>
            <label className="flex items-center gap-3 text-sm text-gray-300 cursor-pointer">
              <input
                type="checkbox"
                checked={filters.stopAtIO ?? false}
                onChange={(e) => handleFilterChange({ stopAtIO: e.target.checked })}
                className="w-4 h-4 rounded bg-[#161b22] border-gray-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-0"
              />
              <span>Stop at I/O</span>
            </label>
          </div>
        </div>

        {/* Side Effects Section */}
        <div className="border-b border-gray-800/50">
          <button
            onClick={() => setSideEffectsExpanded(!sideEffectsExpanded)}
            className="w-full p-4 flex items-center justify-between hover:bg-[#161b22]/50 transition-colors"
          >
            <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide">
              Side Effects
              {sideEffects.length > 0 && (
                <span className="ml-2 px-1.5 py-0.5 text-xs bg-amber-600/20 text-amber-400 rounded">
                  {sideEffects.length}
                </span>
              )}
            </h3>
            <svg
              className={`w-4 h-4 text-gray-500 transition-transform ${sideEffectsExpanded ? 'rotate-180' : ''}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {sideEffectsExpanded && (
            <div className="px-4 pb-4 space-y-2">
              {sideEffects.length === 0 ? (
                <div className="text-xs text-gray-600 italic">No side effects detected</div>
              ) : (
                Object.entries(groupedSideEffects).map(([type, effects]) => (
                  <div key={type} className="space-y-1.5">
                    {effects.map((effect, idx) => (
                      <div
                        key={`${effect.nodeId}-${idx}`}
                        className="flex items-start gap-2 p-2 bg-[#161b22] rounded-md border border-gray-800 cursor-pointer hover:border-gray-700 transition-colors"
                        onClick={() => onNavigateToNode?.(effect.nodeId)}
                      >
                        <span className="text-sm">{SIDE_EFFECT_INFO[type]?.icon || '⚡'}</span>
                        <div className="flex-1 min-w-0">
                          <div className="text-xs text-gray-200 font-mono truncate" title={effect.detail}>
                            {effect.detail}
                          </div>
                          <div className="text-xs text-gray-600 mt-0.5">
                            {effect.label}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                ))
              )}
            </div>
          )}
        </div>

        {/* Dependencies Section */}
        <div className="border-b border-gray-800/50">
          <button
            onClick={() => setDependenciesExpanded(!dependenciesExpanded)}
            className="w-full p-4 flex items-center justify-between hover:bg-[#161b22]/50 transition-colors"
          >
            <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide">Dependencies</h3>
            <svg
              className={`w-4 h-4 text-gray-500 transition-transform ${dependenciesExpanded ? 'rotate-180' : ''}`}
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          {dependenciesExpanded && (
            <div className="px-4 pb-4">
              {!selectedNode ? (
                <div className="text-xs text-gray-600 italic">Select a node to view dependencies</div>
              ) : isLoading ? (
                <div className="text-xs text-gray-500">Loading...</div>
              ) : dependencies.length === 0 ? (
                <div className="text-xs text-gray-600 italic">No dependencies</div>
              ) : (
                <div className="space-y-1">
                  <div className="text-xs text-gray-500 uppercase mb-2">Services</div>
                  {dependencies.map(([name, path]) => (
                    <div
                      key={path}
                      className="text-xs text-gray-400 py-1 px-2 bg-[#161b22] rounded"
                    >
                      {name}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Selected Node Details */}
        {selectedNode && (
          <div className="p-4">
            <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-3">Selected Node</h3>

            <div className="space-y-3">
              {/* Name */}
              <div>
                <div className="text-sm font-mono text-gray-200">
                  {selectedNode.recv_type && (
                    <span className="text-gray-500">({selectedNode.recv_type}).</span>
                  )}
                  {selectedNode.name}
                </div>
                <div className="text-xs text-gray-600 mt-0.5 truncate" title={selectedNode.pkg_path}>
                  {selectedNode.pkg_path}
                </div>
              </div>

              {/* Location */}
              <div className="text-xs text-gray-500">
                {selectedNode.file.split('/').pop()}:{selectedNode.line}
              </div>

              {/* View CFG button */}
              {onShowCFG && (
                <button
                  onClick={() => {
                    const name = selectedNode.recv_type
                      ? `(${selectedNode.recv_type}).${selectedNode.name}`
                      : selectedNode.name;
                    onShowCFG(selectedNode.id, name);
                  }}
                  className="w-full px-3 py-2 text-xs bg-purple-600 hover:bg-purple-500 text-white rounded-md transition-colors font-medium"
                >
                  View Control Flow Graph
                </button>
              )}

              {/* Callers/Callees */}
              {symbolData && (
                <div className="space-y-3 pt-2">
                  {symbolData.callees && symbolData.callees.length > 0 && (
                    <div>
                      <div className="text-xs text-gray-500 mb-1.5">
                        Calls ({symbolData.callees.length})
                      </div>
                      <div className="space-y-1 max-h-32 overflow-y-auto">
                        {symbolData.callees.slice(0, 5).map((callee: CallInfo, idx: number) => (
                          <CallItem
                            key={`${callee.symbol.id}-${idx}`}
                            call={callee}
                            direction="outgoing"
                            isInGraph={isNodeInGraph(callee.symbol.id)}
                            onNavigate={onNavigateToNode}
                          />
                        ))}
                        {symbolData.callees.length > 5 && (
                          <div className="text-xs text-gray-600">
                            +{symbolData.callees.length - 5} more...
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {symbolData.callers && symbolData.callers.length > 0 && (
                    <div>
                      <div className="text-xs text-gray-500 mb-1.5">
                        Called by ({symbolData.callers.length})
                      </div>
                      <div className="space-y-1 max-h-32 overflow-y-auto">
                        {symbolData.callers.slice(0, 5).map((caller: CallInfo, idx: number) => (
                          <CallItem
                            key={`${caller.symbol.id}-${idx}`}
                            call={caller}
                            direction="incoming"
                            isInGraph={isNodeInGraph(caller.symbol.id)}
                            onNavigate={onNavigateToNode}
                          />
                        ))}
                        {symbolData.callers.length > 5 && (
                          <div className="text-xs text-gray-600">
                            +{symbolData.callers.length - 5} more...
                          </div>
                        )}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// Component for displaying a call (caller or callee)
interface CallItemProps {
  call: CallInfo;
  direction: 'incoming' | 'outgoing';
  isInGraph: boolean;
  onNavigate?: (symbolId: number) => void;
}

function CallItem({ call, direction, isInGraph, onNavigate }: CallItemProps) {
  const handleClick = () => {
    if (isInGraph && onNavigate) {
      onNavigate(call.symbol.id);
    }
  };

  return (
    <div
      className={`text-xs p-1.5 rounded transition-colors ${
        isInGraph
          ? 'hover:bg-[#161b22] cursor-pointer'
          : 'text-gray-600'
      }`}
      onClick={handleClick}
      title={isInGraph ? 'Click to select in graph' : 'Not in current graph view'}
    >
      <div className="flex items-center gap-1">
        <span className="text-gray-600">{direction === 'outgoing' ? '→' : '←'}</span>
        <span className={isInGraph ? 'text-blue-400' : 'text-gray-600'}>
          {call.symbol.name}
        </span>
      </div>
    </div>
  );
}
