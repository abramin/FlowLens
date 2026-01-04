import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getEntrypoints } from '../api';
import type { Entrypoint, EntrypointType, HTTPMeta, GRPCMeta, CLIMeta } from '../types';

const TABS: { type: EntrypointType | 'all'; label: string }[] = [
  { type: 'all', label: 'All' },
  { type: 'http', label: 'HTTP' },
  { type: 'grpc', label: 'gRPC' },
  { type: 'cli', label: 'CLI' },
  { type: 'main', label: 'Main' },
];

interface EntrypointsPanelProps {
  selectedId: number | null;
  onSelect: (entrypoint: Entrypoint) => void;
}

export function EntrypointsPanel({ selectedId, onSelect }: EntrypointsPanelProps) {
  const [activeTab, setActiveTab] = useState<EntrypointType | 'all'>('all');
  const [searchQuery, setSearchQuery] = useState('');

  const { data: entrypoints = [], isLoading, error } = useQuery({
    queryKey: ['entrypoints'],
    queryFn: () => getEntrypoints(),
    staleTime: 30000,
  });

  const filteredEntrypoints = useMemo(() => {
    let filtered = entrypoints;

    // Filter by tab
    if (activeTab !== 'all') {
      filtered = filtered.filter((ep) => ep.type === activeTab);
    }

    // Filter by search query
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      filtered = filtered.filter((ep) => ep.label.toLowerCase().includes(query));
    }

    return filtered;
  }, [entrypoints, activeTab, searchQuery]);

  // Count by type for tab badges
  const counts = useMemo(() => {
    const c: Record<string, number> = { all: entrypoints.length };
    for (const ep of entrypoints) {
      c[ep.type] = (c[ep.type] || 0) + 1;
    }
    return c;
  }, [entrypoints]);

  return (
    <div className="flex flex-col h-full bg-gray-900 text-gray-100">
      {/* Header */}
      <div className="p-3 border-b border-gray-700">
        <h2 className="text-sm font-semibold text-gray-300 mb-2">Entrypoints</h2>
        <input
          type="text"
          placeholder="Search..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          className="w-full px-3 py-1.5 text-sm bg-gray-800 border border-gray-700 rounded focus:outline-none focus:border-blue-500 text-gray-100 placeholder-gray-500"
        />
      </div>

      {/* Tabs */}
      <div className="flex border-b border-gray-700 overflow-x-auto">
        {TABS.map((tab) => (
          <button
            key={tab.type}
            onClick={() => setActiveTab(tab.type)}
            className={`px-3 py-2 text-xs font-medium whitespace-nowrap transition-colors ${
              activeTab === tab.type
                ? 'text-blue-400 border-b-2 border-blue-400 bg-gray-800'
                : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800'
            }`}
          >
            {tab.label}
            {counts[tab.type] > 0 && (
              <span className="ml-1.5 px-1.5 py-0.5 text-xs bg-gray-700 rounded-full">
                {counts[tab.type]}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Entrypoint List */}
      <div className="flex-1 overflow-y-auto">
        {isLoading && (
          <div className="p-4 text-center text-gray-500">Loading...</div>
        )}
        {error && (
          <div className="p-4 text-center text-red-400">
            Error: {error instanceof Error ? error.message : 'Unknown error'}
          </div>
        )}
        {!isLoading && !error && filteredEntrypoints.length === 0 && (
          <div className="p-4 text-center text-gray-500">
            {searchQuery ? 'No matches found' : 'No entrypoints'}
          </div>
        )}
        {filteredEntrypoints.map((ep) => (
          <EntrypointItem
            key={ep.id}
            entrypoint={ep}
            selected={selectedId === ep.symbol_id}
            onClick={() => onSelect(ep)}
          />
        ))}
      </div>
    </div>
  );
}

interface EntrypointItemProps {
  entrypoint: Entrypoint;
  selected: boolean;
  onClick: () => void;
}

function EntrypointItem({ entrypoint, selected, onClick }: EntrypointItemProps) {
  const { label, badgeColor } = formatEntrypoint(entrypoint);

  return (
    <button
      onClick={onClick}
      className={`w-full px-3 py-2 text-left border-b border-gray-800 transition-colors ${
        selected
          ? 'bg-blue-900/30 border-l-2 border-l-blue-400'
          : 'hover:bg-gray-800'
      }`}
    >
      <div className="flex items-center gap-2">
        <span
          className={`px-1.5 py-0.5 text-xs font-medium rounded ${badgeColor}`}
        >
          {entrypoint.type.toUpperCase()}
        </span>
        <span className="text-sm text-gray-100 truncate">{label}</span>
      </div>
      <div className="mt-1 text-xs text-gray-500 truncate">
        {entrypoint.symbol.pkg_path}
      </div>
    </button>
  );
}

function formatEntrypoint(ep: Entrypoint): { label: string; badgeColor: string } {
  const badgeColors: Record<EntrypointType, string> = {
    http: 'bg-green-900 text-green-300',
    grpc: 'bg-purple-900 text-purple-300',
    cli: 'bg-yellow-900 text-yellow-300',
    main: 'bg-blue-900 text-blue-300',
  };

  let label = ep.label;

  if (ep.meta_json) {
    try {
      const meta = JSON.parse(ep.meta_json);
      if (ep.type === 'http') {
        const httpMeta = meta as HTTPMeta;
        label = `${httpMeta.method} ${httpMeta.path}`;
      } else if (ep.type === 'grpc') {
        const grpcMeta = meta as GRPCMeta;
        label = `${grpcMeta.service}.${grpcMeta.method}`;
      } else if (ep.type === 'cli') {
        const cliMeta = meta as CLIMeta;
        label = cliMeta.command;
      }
    } catch {
      // Use default label
    }
  }

  return { label, badgeColor: badgeColors[ep.type] };
}
