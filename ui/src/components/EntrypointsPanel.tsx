import { useState, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getEntrypoints } from '../api';
import type { Entrypoint, HTTPMeta } from '../types';

interface EntrypointsPanelProps {
  selectedId: number | null;
  onSelect: (entrypoint: Entrypoint) => void;
}

// Group entrypoints by path prefix (e.g., api/auth, api/users)
function groupEntrypoints(entrypoints: Entrypoint[]): Map<string, Entrypoint[]> {
  const groups = new Map<string, Entrypoint[]>();

  for (const ep of entrypoints) {
    let groupKey = 'other';

    if (ep.type === 'http' && ep.meta_json) {
      try {
        const meta = JSON.parse(ep.meta_json) as HTTPMeta;
        // Extract first two path segments (e.g., /api/auth/login -> api/auth)
        const pathParts = meta.path.split('/').filter(Boolean);
        if (pathParts.length >= 2) {
          groupKey = `${pathParts[0]}/${pathParts[1]}`;
        } else if (pathParts.length === 1) {
          groupKey = pathParts[0];
        }
      } catch {
        // Use package-based grouping for non-HTTP
      }
    } else if (ep.type === 'grpc') {
      groupKey = 'grpc';
    } else if (ep.type === 'cli') {
      groupKey = 'cli';
    } else if (ep.type === 'main') {
      groupKey = 'main';
    } else {
      // Group by package for handlers without clear path
      const pkgParts = ep.symbol.pkg_path.split('/');
      const lastPart = pkgParts[pkgParts.length - 1];
      if (lastPart === 'handler' || lastPart === 'handlers') {
        const secondLast = pkgParts[pkgParts.length - 2];
        groupKey = secondLast || lastPart;
      } else {
        groupKey = lastPart;
      }
    }

    if (!groups.has(groupKey)) {
      groups.set(groupKey, []);
    }
    groups.get(groupKey)!.push(ep);
  }

  return groups;
}

export function EntrypointsPanel({ selectedId, onSelect }: EntrypointsPanelProps) {
  const [searchQuery, setSearchQuery] = useState('');
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(new Set());

  const { data: entrypoints = [], isLoading, error } = useQuery({
    queryKey: ['entrypoints'],
    queryFn: () => getEntrypoints(),
    staleTime: 30000,
  });

  const filteredEntrypoints = useMemo(() => {
    if (!searchQuery.trim()) return entrypoints;
    const query = searchQuery.toLowerCase();
    return entrypoints.filter((ep) =>
      ep.label.toLowerCase().includes(query) ||
      ep.symbol.name.toLowerCase().includes(query)
    );
  }, [entrypoints, searchQuery]);

  const groupedEntrypoints = useMemo(() => {
    return groupEntrypoints(filteredEntrypoints);
  }, [filteredEntrypoints]);

  const toggleGroup = (groupKey: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupKey)) {
        next.delete(groupKey);
      } else {
        next.add(groupKey);
      }
      return next;
    });
  };

  // Sort groups alphabetically
  const sortedGroups = useMemo(() => {
    return Array.from(groupedEntrypoints.entries()).sort(([a], [b]) => a.localeCompare(b));
  }, [groupedEntrypoints]);

  return (
    <div className="flex flex-col h-full bg-[#0d1117] text-gray-100">
      {/* Header */}
      <div className="p-4 border-b border-gray-800/50">
        <h2 className="text-sm font-semibold text-gray-200 mb-3">Entry Points</h2>
        <div className="relative">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-600"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            placeholder="Search handlers..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full pl-10 pr-3 py-2 text-sm bg-[#161b22] border border-gray-800 rounded-md focus:outline-none focus:border-blue-500 text-gray-200 placeholder-gray-600"
          />
        </div>
      </div>

      {/* Entrypoint Groups */}
      <div className="flex-1 overflow-y-auto">
        {isLoading && (
          <div className="p-4 text-center text-gray-500">Loading...</div>
        )}
        {error && (
          <div className="p-4 text-center text-red-400">
            Error: {error instanceof Error ? error.message : 'Unknown error'}
          </div>
        )}
        {!isLoading && !error && sortedGroups.length === 0 && (
          <div className="p-4 text-center text-gray-600">
            {searchQuery ? 'No matches found' : 'No entrypoints'}
          </div>
        )}

        {sortedGroups.map(([groupKey, entries]) => (
          <EntrypointGroup
            key={groupKey}
            groupKey={groupKey}
            entries={entries}
            selectedId={selectedId}
            isCollapsed={collapsedGroups.has(groupKey)}
            onToggle={() => toggleGroup(groupKey)}
            onSelect={onSelect}
          />
        ))}
      </div>
    </div>
  );
}

interface EntrypointGroupProps {
  groupKey: string;
  entries: Entrypoint[];
  selectedId: number | null;
  isCollapsed: boolean;
  onToggle: () => void;
  onSelect: (entrypoint: Entrypoint) => void;
}

function EntrypointGroup({
  groupKey,
  entries,
  selectedId,
  isCollapsed,
  onToggle,
  onSelect
}: EntrypointGroupProps) {
  return (
    <div className="border-b border-gray-800/30">
      {/* Group header */}
      <button
        onClick={onToggle}
        className="w-full px-4 py-2.5 flex items-center justify-between hover:bg-[#161b22] transition-colors"
      >
        <div className="flex items-center gap-2">
          <svg
            className={`w-3 h-3 text-gray-500 transition-transform ${isCollapsed ? '' : 'rotate-90'}`}
            fill="currentColor"
            viewBox="0 0 20 20"
          >
            <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
          </svg>
          <span className="text-sm text-gray-300 font-medium">{groupKey}</span>
        </div>
        <span className="text-xs text-gray-600 bg-gray-800/50 px-2 py-0.5 rounded">
          {entries.length}
        </span>
      </button>

      {/* Group entries */}
      {!isCollapsed && (
        <div className="pb-1">
          {entries.map((ep) => (
            <EntrypointItem
              key={ep.id}
              entrypoint={ep}
              selected={selectedId === ep.symbol_id}
              onClick={() => onSelect(ep)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface EntrypointItemProps {
  entrypoint: Entrypoint;
  selected: boolean;
  onClick: () => void;
}

function EntrypointItem({ entrypoint, selected, onClick }: EntrypointItemProps) {
  // Get method name from the handler
  const methodName = entrypoint.label;

  return (
    <button
      onClick={onClick}
      className={`w-full pl-8 pr-4 py-2 text-left transition-colors ${
        selected
          ? 'bg-[#1e3a5f] border-l-2 border-l-blue-500'
          : 'hover:bg-[#161b22] border-l-2 border-l-transparent'
      }`}
    >
      <div className="text-sm text-gray-300 truncate">{methodName}</div>
    </button>
  );
}
