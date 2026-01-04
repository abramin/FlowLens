import { useState } from 'react';
import type { BranchBadge } from '../types';

interface BranchBadgeProps {
  badge: BranchBadge;
  onExpandBranch: (symbolIds: number[]) => void;
}

export function BranchBadgeComponent({ badge, onExpandBranch }: BranchBadgeProps) {
  const [isHovered, setIsHovered] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isExpanded) {
      setIsExpanded(true);
    }
    onExpandBranch(badge.collapsed_ids);
  };

  return (
    <div
      className="relative inline-flex flex-col items-end gap-1"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
    >
      {/* Depth indicator */}
      <button
        onClick={handleClick}
        className={`
          flex items-center gap-1 px-2 py-1 text-xs rounded
          transition-all duration-200
          ${isExpanded
            ? 'bg-gray-700 text-gray-400'
            : 'bg-gray-800 hover:bg-gray-700 text-gray-300'
          }
        `}
        title={`${badge.call_count} branch calls - click to expand`}
      >
        <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 5l7 7-7 7M5 5l7 7-7 7" />
        </svg>
        <span className="font-medium">+{badge.call_count}</span>
      </button>

      {/* Tooltip showing collapsed function names */}
      {isHovered && badge.labels.length > 0 && (
        <div className="absolute right-0 top-full mt-1 z-50 min-w-[200px] max-w-[300px]">
          <div className="bg-gray-800 border border-gray-700 rounded-lg shadow-xl p-2">
            <div className="text-xs text-gray-400 mb-1.5 font-medium">Branch calls:</div>
            <div className="space-y-1 max-h-[200px] overflow-y-auto">
              {badge.labels.slice(0, 10).map((label, idx) => (
                <div
                  key={idx}
                  className="text-xs text-gray-300 truncate pl-2 border-l border-gray-700"
                  title={label}
                >
                  {label}
                </div>
              ))}
              {badge.labels.length > 10 && (
                <div className="text-xs text-gray-500 italic pl-2">
                  +{badge.labels.length - 10} more...
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
