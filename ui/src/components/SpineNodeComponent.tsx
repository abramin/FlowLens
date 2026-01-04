import type { SpineNode } from '../types';
import { BranchBadgeComponent } from './BranchBadgeComponent';

interface SpineNodeComponentProps {
  node: SpineNode;
  isFirst: boolean;
  isLast: boolean;
  isSelected: boolean;
  onClick: (nodeId: number) => void;
  onExpandBranch: (symbolIds: number[]) => void;
}

// Layer colors - matching mockup style
const LAYER_COLORS: Record<string, { bg: string; text: string; label: string }> = {
  handler: { bg: '#22c55e', text: '#ffffff', label: 'Handler' },   // Green
  service: { bg: '#3b82f6', text: '#ffffff', label: 'Service' },   // Blue
  store: { bg: '#a855f7', text: '#ffffff', label: 'Store' },       // Purple
  domain: { bg: '#ec4899', text: '#ffffff', label: 'Domain' },     // Pink
};

// IO tag colors
const IO_COLORS: Record<string, { bg: string; text: string }> = {
  'io:db': { bg: '#f59e0b', text: '#ffffff' },
  'io:net': { bg: '#f97316', text: '#ffffff' },
  'io:fs': { bg: '#eab308', text: '#ffffff' },
  'io:bus': { bg: '#ef4444', text: '#ffffff' },
};

function getIOTag(tags: string[]): string | null {
  return tags.find((t) => t.startsWith('io:')) ?? null;
}

export function SpineNodeComponent({
  node,
  isFirst,
  isLast,
  isSelected,
  onClick,
  onExpandBranch,
}: SpineNodeComponentProps) {
  const layerColor = node.layer ? LAYER_COLORS[node.layer] : null;
  const ioTag = getIOTag(node.tags);
  const ioColor = ioTag ? IO_COLORS[ioTag] : null;

  const displayName = node.recv_type
    ? `(*${node.recv_type}).${node.name}`
    : node.name;

  const shortPkg = node.pkg_path.split('/').pop() || node.pkg_path;

  // Calculate branch count for depth indicator
  const branchCount = node.branch_badge?.call_count ?? 0;

  return (
    <div className="flex flex-col items-center w-full max-w-xl">
      {/* Connector line from above (except for first node) */}
      {!isFirst && (
        <div className="w-px h-4 bg-gray-700" />
      )}

      {/* Main node card */}
      <div
        className={`
          w-full px-4 py-3 rounded-lg cursor-pointer
          transition-all duration-150
          border relative
          ${isSelected
            ? 'bg-[#1e3a5f] border-blue-500 shadow-lg shadow-blue-500/20'
            : 'bg-[#161b22] border-gray-800 hover:border-gray-700'
          }
        `}
        onClick={() => onClick(node.id)}
      >
        <div className="flex items-start gap-3">
          {/* Main content */}
          <div className="flex-1 min-w-0">
            {/* Function name */}
            <div className="flex items-center gap-2">
              <span
                className="text-sm font-medium text-gray-100 truncate"
                title={displayName}
              >
                {displayName}
              </span>

              {/* Layer badge */}
              {layerColor && (
                <span
                  className="px-2 py-0.5 text-xs font-medium rounded"
                  style={{ backgroundColor: layerColor.bg, color: layerColor.text }}
                >
                  {layerColor.label}
                </span>
              )}
            </div>

            {/* Package */}
            <div className="text-xs text-gray-500 mt-1 truncate" title={node.pkg_path}>
              {shortPkg}
            </div>

            {/* IO badge if present */}
            {ioColor && ioTag && (
              <div className="mt-2">
                <span
                  className="px-2 py-0.5 text-xs rounded"
                  style={{ backgroundColor: ioColor.bg, color: ioColor.text }}
                >
                  {ioTag}
                </span>
              </div>
            )}
          </div>

          {/* Right side - branch count indicator */}
          {branchCount > 0 && (
            <div className="flex flex-col items-end gap-1">
              <BranchBadgeComponent
                badge={node.branch_badge!}
                onExpandBranch={onExpandBranch}
              />
            </div>
          )}
        </div>

        {/* Expand indicator at bottom */}
        {!isLast && (
          <div className="absolute -bottom-1 left-1/2 -translate-x-1/2 w-4 h-4 bg-[#161b22] border border-gray-700 rounded flex items-center justify-center">
            <svg className="w-2.5 h-2.5 text-gray-500" fill="currentColor" viewBox="0 0 20 20">
              <path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </div>
        )}
      </div>

      {/* Connector line to below (except for last node) */}
      {!isLast && (
        <div className="w-px h-4 bg-gray-700" />
      )}
    </div>
  );
}
