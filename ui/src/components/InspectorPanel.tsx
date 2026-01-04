import { useQuery } from '@tanstack/react-query';
import { getSymbol } from '../api';
import type { GraphNode, GraphFilter } from '../types';

interface InspectorPanelProps {
  selectedNode: GraphNode | null;
  filters: GraphFilter;
  onFiltersChange: (filters: GraphFilter) => void;
}

export function InspectorPanel({ selectedNode, filters, onFiltersChange }: InspectorPanelProps) {
  const { data: symbolData, isLoading } = useQuery({
    queryKey: ['symbol', selectedNode?.id],
    queryFn: () => getSymbol(selectedNode!.id),
    enabled: !!selectedNode,
  });

  return (
    <div className="flex flex-col h-full bg-gray-900 text-gray-100">
      {/* Filters Section */}
      <div className="p-3 border-b border-gray-700">
        <h3 className="text-sm font-semibold text-gray-300 mb-3">Filters</h3>
        <div className="space-y-2">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.hideStdlib ?? false}
              onChange={(e) => onFiltersChange({ ...filters, hideStdlib: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Hide stdlib</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.hideVendors ?? false}
              onChange={(e) => onFiltersChange({ ...filters, hideVendors: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Hide vendors</span>
          </label>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={filters.stopAtIO ?? false}
              onChange={(e) => onFiltersChange({ ...filters, stopAtIO: e.target.checked })}
              className="rounded bg-gray-800 border-gray-600"
            />
            <span>Stop at I/O</span>
          </label>
          <div className="flex items-center gap-2 text-sm">
            <span>Max depth:</span>
            <input
              type="number"
              min={1}
              max={10}
              value={filters.maxDepth ?? 6}
              onChange={(e) => onFiltersChange({ ...filters, maxDepth: parseInt(e.target.value) || 6 })}
              className="w-16 px-2 py-1 bg-gray-800 border border-gray-700 rounded text-center"
            />
          </div>
        </div>
      </div>

      {/* Symbol Details Section */}
      <div className="flex-1 overflow-y-auto p-3">
        <h3 className="text-sm font-semibold text-gray-300 mb-3">Symbol Details</h3>

        {!selectedNode && (
          <div className="text-sm text-gray-500">Click a node to view details</div>
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
              <div className="text-sm font-mono">
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

            {/* Full Tag Details */}
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

function getTagColor(tag: string): string {
  if (tag.startsWith('io:')) return 'bg-amber-900 text-amber-300';
  if (tag.startsWith('layer:')) return 'bg-purple-900 text-purple-300';
  if (tag === 'pure') return 'bg-green-900 text-green-300';
  if (tag === 'impure') return 'bg-red-900 text-red-300';
  return 'bg-gray-700 text-gray-300';
}
