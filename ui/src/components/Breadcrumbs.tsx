import type { BreadcrumbItem } from '../hooks/useBreadcrumbs';

interface BreadcrumbsProps {
  items: BreadcrumbItem[];
  onNavigate: (nodeId: number) => void;
  onClearFocus: () => void;
}

export function Breadcrumbs({ items, onNavigate, onClearFocus }: BreadcrumbsProps) {
  if (items.length === 0) {
    return null;
  }

  // Collapse middle items if path is too long
  const maxVisible = 5;
  let displayItems = items;
  let collapsed = false;

  if (items.length > maxVisible) {
    // Show first item, ellipsis, and last (maxVisible - 2) items
    const endItems = items.slice(-(maxVisible - 2));
    displayItems = [items[0], ...endItems];
    collapsed = true;
  }

  return (
    <div className="flex items-center gap-1 px-3 py-1.5 bg-gray-850 border-b border-gray-700 text-xs overflow-x-auto">
      <span className="text-gray-500 mr-1 flex-shrink-0">Path:</span>

      {displayItems.map((item, index) => {
        const isLast = index === displayItems.length - 1;
        const isFirst = index === 0;
        const showEllipsis = collapsed && isFirst;

        return (
          <span key={item.nodeId} className="flex items-center flex-shrink-0">
            <button
              onClick={() => onNavigate(item.nodeId)}
              className={`px-1.5 py-0.5 rounded hover:bg-gray-700 transition-colors ${
                isLast
                  ? 'text-indigo-400 font-medium'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
              title={item.fullLabel}
            >
              {item.label}
            </button>

            {showEllipsis && (
              <>
                <span className="text-gray-600 mx-0.5">&gt;</span>
                <span className="text-gray-500 px-1">...</span>
              </>
            )}

            {!isLast && (
              <span className="text-gray-600 mx-0.5">&gt;</span>
            )}
          </span>
        );
      })}

      {items.length > 1 && (
        <button
          onClick={onClearFocus}
          className="ml-2 p-1 text-gray-500 hover:text-gray-300 hover:bg-gray-700 rounded transition-colors flex-shrink-0"
          title="Clear focus and return to root"
        >
          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      )}
    </div>
  );
}
