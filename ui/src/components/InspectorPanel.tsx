import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getSymbol } from '../api';
import type { GraphNode, GraphFilter, CallInfo, FilterPreset } from '../types';
import { FILTER_PRESETS } from '../types';

interface InspectorPanelProps {
  selectedNode: GraphNode | null;
  filters: GraphFilter;
  onFiltersChange: (filters: GraphFilter) => void;
  onNavigateToNode?: (symbolId: number) => void;
  graphNodes: GraphNode[];
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

export function InspectorPanel({
  selectedNode,
  filters,
  onFiltersChange,
  onNavigateToNode,
  graphNodes
}: InspectorPanelProps) {
  const [activePreset, setActivePreset] = useState<FilterPreset | null>('default');
  const [stopAtPackagePrefix, setStopAtPackagePrefix] = useState('');
  const [noisePackages, setNoisePackages] = useState('');

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
      // Set package prefix and noise packages from stored filters
      if (storedFilters.stopAtPackagePrefix?.length) {
        setStopAtPackagePrefix(storedFilters.stopAtPackagePrefix.join(', '));
      }
      if (storedFilters.noisePackages?.length) {
        setNoisePackages(storedFilters.noisePackages.join(', '));
      }
      // Detect if it matches a preset
      const matchingPreset = (Object.keys(FILTER_PRESETS) as FilterPreset[]).find(
        preset => JSON.stringify(FILTER_PRESETS[preset]) === JSON.stringify(storedFilters)
      );
      setActivePreset(matchingPreset || null);
    }
  }, []);

  // Save filters to localStorage when they change
  useEffect(() => {
    saveFiltersToStorage(filters);
  }, [filters]);

  const handlePresetChange = (preset: FilterPreset) => {
    setActivePreset(preset);
    onFiltersChange({ ...FILTER_PRESETS[preset] });
    setStopAtPackagePrefix('');
    setNoisePackages('');
  };

  const handleFilterChange = (updates: Partial<GraphFilter>) => {
    setActivePreset(null); // Clear preset when manually changing
    onFiltersChange({ ...filters, ...updates });
  };

  const handlePackagePrefixChange = (value: string) => {
    setStopAtPackagePrefix(value);
    const prefixes = value.split(',').map(s => s.trim()).filter(Boolean);
    handleFilterChange({ stopAtPackagePrefix: prefixes.length ? prefixes : undefined });
  };

  const handleNoisePackagesChange = (value: string) => {
    setNoisePackages(value);
    const packages = value.split(',').map(s => s.trim()).filter(Boolean);
    handleFilterChange({ noisePackages: packages.length ? packages : undefined });
  };

  const handleReset = () => {
    handlePresetChange('default');
  };

  // Check if a node exists in the current graph
  const isNodeInGraph = (symbolId: number) => {
    return graphNodes.some(n => n.id === symbolId);
  };

  // Count active filters
  const activeFilterCount = [
    filters.hideStdlib,
    filters.hideVendors,
    filters.stopAtIO,
    (filters.maxDepth ?? 6) !== 6,
    filters.stopAtPackagePrefix?.length,
    filters.noisePackages?.length,
  ].filter(Boolean).length;

  return (
    <div className="flex flex-col h-full bg-gray-900 text-gray-100">
      {/* Filters Section */}
      <div className="p-3 border-b border-gray-700">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-semibold text-gray-300">
            Filters
            {activeFilterCount > 0 && (
              <span className="ml-2 px-1.5 py-0.5 text-xs bg-blue-600 text-white rounded-full">
                {activeFilterCount}
              </span>
            )}
          </h3>
          <button
            onClick={handleReset}
            className="text-xs text-gray-400 hover:text-gray-200 transition-colors"
          >
            Reset
          </button>
        </div>

        {/* Presets */}
        <div className="flex gap-1 mb-3">
          {(['default', 'deep-dive', 'high-level'] as FilterPreset[]).map((preset) => (
            <button
              key={preset}
              onClick={() => handlePresetChange(preset)}
              className={`px-2 py-1 text-xs rounded transition-colors ${
                activePreset === preset
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700'
              }`}
            >
              {preset === 'default' ? 'Default' : preset === 'deep-dive' ? 'Deep Dive' : 'High Level'}
            </button>
          ))}
        </div>

        {/* Toggle filters */}
        <div className="space-y-2">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.hideStdlib ?? false}
              onChange={(e) => handleFilterChange({ hideStdlib: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Hide stdlib</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.hideVendors ?? false}
              onChange={(e) => handleFilterChange({ hideVendors: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Hide vendors</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.stopAtIO ?? false}
              onChange={(e) => handleFilterChange({ stopAtIO: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Stop at I/O</span>
          </label>

          {/* Max depth */}
          <div className="flex items-center gap-2 text-sm">
            <span>Max depth:</span>
            <input
              type="number"
              min={1}
              max={10}
              value={filters.maxDepth ?? 6}
              onChange={(e) => handleFilterChange({ maxDepth: parseInt(e.target.value) || 6 })}
              className="w-16 px-2 py-1 bg-gray-800 border border-gray-700 rounded text-center"
            />
          </div>

          {/* Stop at package prefix */}
          <div className="text-sm">
            <label className="block text-gray-400 mb-1">Stop at packages:</label>
            <input
              type="text"
              value={stopAtPackagePrefix}
              onChange={(e) => handlePackagePrefixChange(e.target.value)}
              placeholder="e.g., github.com/org/repo"
              className="w-full px-2 py-1 bg-gray-800 border border-gray-700 rounded text-xs"
            />
          </div>

          {/* Noise packages */}
          <div className="text-sm">
            <label className="block text-gray-400 mb-1">Hide packages:</label>
            <input
              type="text"
              value={noisePackages}
              onChange={(e) => handleNoisePackagesChange(e.target.value)}
              placeholder="e.g., log/slog, go.uber.org/zap"
              className="w-full px-2 py-1 bg-gray-800 border border-gray-700 rounded text-xs"
            />
          </div>
        </div>
      </div>

      {/* Symbol Details Section */}
      <div className="flex-1 overflow-y-auto p-3">
        <h3 className="text-sm font-semibold text-gray-300 mb-3">Symbol Details</h3>

        {!selectedNode && (
          <div className="text-center py-8">
            <div className="text-gray-500 text-sm mb-2">No node selected</div>
            <div className="text-gray-600 text-xs">
              Click a node in the graph to view its details
            </div>
            <div className="mt-4 text-gray-600 text-xs space-y-1">
              <div>💡 Tips:</div>
              <div>• Double-click to expand a node</div>
              <div>• Shift+click to pin a node</div>
            </div>
          </div>
        )}

        {selectedNode && isLoading && (
          <div className="text-sm text-gray-500">Loading...</div>
        )}

        {selectedNode && symbolData && (
          <div className="space-y-4">
            {/* Name */}
            <div>
              <div className="text-xs text-gray-500 mb-1">Name</div>
              <div className="text-sm font-mono">
                {selectedNode.recv_type && (
                  <span className="text-gray-400">({selectedNode.recv_type}).</span>
                )}
                {selectedNode.name}
              </div>
            </div>

            {/* Package */}
            <div>
              <div className="text-xs text-gray-500 mb-1">Package</div>
              <div className="text-sm font-mono text-blue-400 truncate" title={selectedNode.pkg_path}>
                {selectedNode.pkg_path}
              </div>
            </div>

            {/* Location */}
            <div>
              <div className="text-xs text-gray-500 mb-1">Location</div>
              <div className="text-sm font-mono text-green-400 cursor-pointer hover:underline"
                   title="Click to copy path">
                {selectedNode.file}:{selectedNode.line}
              </div>
            </div>

            {/* Signature */}
            {selectedNode.sig && (
              <div>
                <div className="text-xs text-gray-500 mb-1">Signature</div>
                <div className="text-sm font-mono text-gray-300 break-all">
                  {selectedNode.sig}
                </div>
              </div>
            )}

            {/* Tags */}
            {selectedNode.tags.length > 0 && (
              <div>
                <div className="text-xs text-gray-500 mb-1">Tags</div>
                <div className="flex flex-wrap gap-1">
                  {selectedNode.tags.map((tag) => (
                    <span
                      key={tag}
                      className={`px-2 py-0.5 text-xs rounded ${getTagColor(tag)}`}
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Tag Details with reasons */}
            {symbolData.tags && symbolData.tags.length > 0 && (
              <div>
                <div className="text-xs text-gray-500 mb-1">Tag Details</div>
                <div className="space-y-1">
                  {symbolData.tags.map((tag) => (
                    <div key={tag.tag} className="text-xs">
                      <span className={`px-1.5 py-0.5 rounded ${getTagColor(tag.tag)}`}>
                        {tag.tag}
                      </span>
                      {tag.reason && (
                        <span className="ml-2 text-gray-500">{tag.reason}</span>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Outgoing Calls (Callees) */}
            {symbolData.callees && symbolData.callees.length > 0 && (
              <div>
                <div className="text-xs text-gray-500 mb-1">
                  Calls ({symbolData.callees.length})
                </div>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {symbolData.callees.slice(0, 10).map((callee: CallInfo, idx: number) => (
                    <CallItem
                      key={`${callee.symbol.id}-${idx}`}
                      call={callee}
                      direction="outgoing"
                      isInGraph={isNodeInGraph(callee.symbol.id)}
                      onNavigate={onNavigateToNode}
                    />
                  ))}
                  {symbolData.callees.length > 10 && (
                    <div className="text-xs text-gray-500 pl-2">
                      +{symbolData.callees.length - 10} more...
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Incoming Calls (Callers) */}
            {symbolData.callers && symbolData.callers.length > 0 && (
              <div>
                <div className="text-xs text-gray-500 mb-1">
                  Called by ({symbolData.callers.length})
                </div>
                <div className="space-y-1 max-h-40 overflow-y-auto">
                  {symbolData.callers.slice(0, 10).map((caller: CallInfo, idx: number) => (
                    <CallItem
                      key={`${caller.symbol.id}-${idx}`}
                      call={caller}
                      direction="incoming"
                      isInGraph={isNodeInGraph(caller.symbol.id)}
                      onNavigate={onNavigateToNode}
                    />
                  ))}
                  {symbolData.callers.length > 10 && (
                    <div className="text-xs text-gray-500 pl-2">
                      +{symbolData.callers.length - 10} more...
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Legend */}
      <div className="p-3 border-t border-gray-700">
        <h3 className="text-xs font-semibold text-gray-400 mb-2">Legend</h3>
        <div className="grid grid-cols-2 gap-2 text-xs">
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-blue-500"></div>
            <span>Root</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-amber-500"></div>
            <span>I/O</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-emerald-600"></div>
            <span>Pinned</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-indigo-500"></div>
            <span>Selected</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-gray-600"></div>
            <span>Expanded</span>
          </div>
          <div className="flex items-center gap-2">
            <div className="w-3 h-3 rounded bg-gray-800 border border-gray-600"></div>
            <span>Collapsed</span>
          </div>
        </div>
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
      className={`text-xs p-1.5 rounded ${
        isInGraph
          ? 'bg-gray-800 hover:bg-gray-700 cursor-pointer'
          : 'bg-gray-850 text-gray-500'
      }`}
      onClick={handleClick}
      title={isInGraph ? 'Click to select in graph' : 'Not in current graph view'}
    >
      <div className="flex items-center gap-1">
        <span className="text-gray-500">{direction === 'outgoing' ? '→' : '←'}</span>
        <span className={isInGraph ? 'text-blue-400' : 'text-gray-500'}>
          {call.symbol.recv_type ? `(${call.symbol.recv_type}).` : ''}
          {call.symbol.name}
        </span>
        {call.call_kind !== 'static' && (
          <span className="px-1 py-0.5 text-[9px] bg-purple-900 text-purple-300 rounded">
            {call.call_kind}
          </span>
        )}
        {call.count > 1 && (
          <span className="text-gray-500">×{call.count}</span>
        )}
      </div>
      <div className="text-gray-600 truncate pl-4">
        {call.symbol.pkg_path.split('/').slice(-2).join('/')}
      </div>
    </div>
  );
}

function getTagColor(tag: string): string {
  if (tag.startsWith('io:')) return 'bg-amber-900 text-amber-300';
  if (tag.startsWith('layer:')) return 'bg-purple-900 text-purple-300';
  if (tag === 'pure') return 'bg-green-900 text-green-300';
  if (tag === 'impure') return 'bg-red-900 text-red-300';
  return 'bg-gray-700 text-gray-300';
}
