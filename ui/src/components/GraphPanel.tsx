import { useEffect, useMemo, useCallback } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  useReactFlow,
  MarkerType,
  Handle,
  Position,
} from '@xyflow/react';
import type { Node, Edge } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { GraphNode, GraphEdge, GraphFilter } from '../types';
import type { BreadcrumbItem } from '../hooks/useBreadcrumbs';
import { Toolbar } from './Toolbar';
import { Breadcrumbs } from './Breadcrumbs';

interface GraphPanelProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  rootId: number | null;
  isLoading: boolean;
  error: Error | null;
  onNodeClick: (nodeId: number) => void;
  onNodeExpand: (nodeId: number) => void;
  onNodePin?: (nodeId: number) => void;
  selectedNodeId: number | null;
  pinnedNodeIds: Set<number>;
  filters: GraphFilter;
  breadcrumbs: BreadcrumbItem[];
  onBreadcrumbNavigate: (nodeId: number) => void;
  onClearFocus: () => void;
  onCopyLink: () => Promise<boolean>;
  onExportSVG: () => Promise<void>;
  onExportPNG: () => Promise<void>;
}

// Colors for different node states
const NODE_COLORS = {
  root: { bg: '#3b82f6', border: '#1d4ed8', text: '#ffffff' },
  io: { bg: '#f59e0b', border: '#d97706', text: '#ffffff' },
  expanded: { bg: '#374151', border: '#4b5563', text: '#e5e7eb' },
  selected: { bg: '#4f46e5', border: '#6366f1', text: '#ffffff' },
  pinned: { bg: '#059669', border: '#10b981', text: '#ffffff' },
  default: { bg: '#1f2937', border: '#374151', text: '#9ca3af' },
};

// Layer badge colors (prominent badges like in design)
const LAYER_BADGE_COLORS: Record<string, { bg: string; text: string }> = {
  handler: { bg: '#8b5cf6', text: '#ffffff' },   // Purple
  service: { bg: '#10b981', text: '#ffffff' },   // Green
  store: { bg: '#06b6d4', text: '#ffffff' },     // Cyan/Teal
  domain: { bg: '#ec4899', text: '#ffffff' },    // Pink
};

// Get the layer from tags
function getNodeLayer(tags: string[]): string | null {
  for (const tag of tags) {
    if (tag.startsWith('layer:')) {
      return tag.split(':')[1];
    }
  }
  return null;
}

// Tag colors for badges
const TAG_COLORS: Record<string, string> = {
  'io:db': 'bg-amber-700 text-amber-100',
  'io:net': 'bg-orange-700 text-orange-100',
  'io:fs': 'bg-yellow-700 text-yellow-100',
  'io:bus': 'bg-red-700 text-red-100',
  'layer:handler': 'bg-blue-700 text-blue-100',
  'layer:service': 'bg-purple-700 text-purple-100',
  'layer:store': 'bg-indigo-700 text-indigo-100',
  'layer:domain': 'bg-pink-700 text-pink-100',
  'pure': 'bg-green-700 text-green-100',
  'impure': 'bg-red-700 text-red-100',
};

function getTagColor(tag: string): string {
  if (TAG_COLORS[tag]) return TAG_COLORS[tag];
  if (tag.startsWith('io:')) return 'bg-amber-800 text-amber-200';
  if (tag.startsWith('layer:')) return 'bg-purple-800 text-purple-200';
  return 'bg-gray-600 text-gray-200';
}

function getNodeColor(node: GraphNode, isRoot: boolean, isSelected: boolean, isPinned: boolean) {
  if (isSelected) return NODE_COLORS.selected;
  if (isRoot) return NODE_COLORS.root;
  if (isPinned) return NODE_COLORS.pinned;
  if (node.tags.some((t) => t.startsWith('io:'))) return NODE_COLORS.io;
  if (node.expanded) return NODE_COLORS.expanded;
  return NODE_COLORS.default;
}

interface CustomNodeProps {
  data: {
    label: string;
    node: GraphNode;
    isRoot: boolean;
    isSelected: boolean;
    isPinned: boolean;
    expandCount: number;
    onExpand: () => void;
    onClick: (e: React.MouseEvent) => void;
  };
}

function CustomNode({ data }: CustomNodeProps) {
  const colors = getNodeColor(data.node, data.isRoot, data.isSelected, data.isPinned);
  const layer = getNodeLayer(data.node.tags);
  const layerColors = layer ? LAYER_BADGE_COLORS[layer] : null;

  // Filter out layer tags from visible tags (we show layer as prominent badge)
  const nonLayerTags = data.node.tags.filter(t => !t.startsWith('layer:'));
  const visibleTags = nonLayerTags.slice(0, 2); // Show max 2 non-layer tags
  const hasMoreTags = nonLayerTags.length > 2;

  return (
    <div
      className={`relative px-3 py-2 rounded-lg shadow-lg cursor-pointer transition-all duration-200 hover:scale-105 ${
        data.isSelected ? 'ring-2 ring-indigo-400 ring-offset-2 ring-offset-gray-900' : ''
      } ${data.isPinned ? 'ring-2 ring-emerald-400' : ''}`}
      style={{
        backgroundColor: colors.bg,
        borderWidth: 2,
        borderColor: layerColors ? layerColors.bg : colors.border,
        borderStyle: 'solid',
        minWidth: 180,
        maxWidth: 260,
      }}
      onClick={data.onClick}
      onDoubleClick={(e) => {
        e.stopPropagation();
        data.onExpand();
      }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      {/* Layer badge - prominent, positioned at top right */}
      {layerColors && (
        <div
          className="absolute -top-0 -right-0 px-2.5 py-1 text-[10px] font-semibold rounded-tr-md rounded-bl-md uppercase tracking-wide"
          style={{ backgroundColor: layerColors.bg, color: layerColors.text }}
        >
          {layer}
        </div>
      )}

      {/* Expand count indicator - positioned at right side */}
      {data.expandCount > 0 && !data.node.expanded && (
        <div className="absolute -right-2 top-1/2 -translate-y-1/2 flex flex-col gap-0.5">
          <div className="px-1.5 py-0.5 text-[9px] font-medium bg-gray-700 text-gray-300 rounded-full border border-gray-600">
            ↓ +{data.expandCount}
          </div>
        </div>
      )}

      <div className="text-center pt-1">
        {/* Status indicators */}
        <div className="flex justify-center gap-1 mb-1">
          {data.isPinned && (
            <span className="text-[10px] text-emerald-300" title="Pinned">📌</span>
          )}
          {data.node.expanded && (
            <span className="text-[10px] text-gray-400" title="Expanded">▼</span>
          )}
        </div>

        {/* Function name */}
        <div
          className="text-sm font-medium truncate"
          style={{ color: colors.text }}
          title={data.node.recv_type ? `(${data.node.recv_type}).${data.node.name}` : data.node.name}
        >
          {data.node.recv_type ? `(*${data.node.recv_type}).` : ''}
          {data.node.name}
        </div>

        {/* Package name */}
        <div className="text-xs opacity-60 truncate" style={{ color: colors.text }}>
          {data.node.pkg_path.split('/').pop()}
        </div>

        {/* Non-layer tags (IO, pure/impure, etc.) */}
        {visibleTags.length > 0 && (
          <div className="mt-1.5 flex flex-wrap justify-center gap-0.5">
            {visibleTags.map((tag) => (
              <span
                key={tag}
                className={`px-1.5 py-0.5 text-[9px] rounded ${getTagColor(tag)}`}
                title={tag}
              >
                {tag.includes(':') ? tag.split(':')[1] : tag}
              </span>
            ))}
            {hasMoreTags && (
              <span className="px-1 py-0.5 text-[9px] bg-gray-600 text-gray-300 rounded">
                +{nonLayerTags.length - 2}
              </span>
            )}
          </div>
        )}
      </div>
      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
}

const nodeTypes = {
  custom: CustomNode,
};

export function GraphPanel({
  nodes,
  edges,
  rootId,
  isLoading,
  error,
  onNodeClick,
  onNodeExpand,
  onNodePin,
  selectedNodeId,
  pinnedNodeIds,
  breadcrumbs,
  onBreadcrumbNavigate,
  onClearFocus,
  onCopyLink,
  onExportSVG,
  onExportPNG,
}: GraphPanelProps) {
  const { fitView } = useReactFlow();

  // Handle node click with shift detection for pinning
  const handleNodeClick = useCallback((nodeId: number, e: React.MouseEvent) => {
    if (e.shiftKey && onNodePin) {
      onNodePin(nodeId);
    } else {
      onNodeClick(nodeId);
    }
  }, [onNodeClick, onNodePin]);

  // Convert API nodes to React Flow nodes
  const flowNodes = useMemo(() => {
    if (!nodes.length) return [];

    // Simple tree layout
    const nodeMap = new Map(nodes.map((n) => [n.id, n]));
    const childrenMap = new Map<number, number[]>();

    for (const edge of edges) {
      const children = childrenMap.get(edge.source_id) || [];
      children.push(edge.target_id);
      childrenMap.set(edge.source_id, children);
    }

    // BFS to assign positions
    const positions = new Map<number, { x: number; y: number }>();
    const visited = new Set<number>();

    if (rootId && nodeMap.has(rootId)) {
      const queue: { id: number; depth: number; index: number; parentX: number }[] = [
        { id: rootId, depth: 0, index: 0, parentX: 0 },
      ];

      // First pass: count nodes at each depth
      const depthCounts = new Map<number, number>();
      const tempQueue = [...queue];
      const tempVisited = new Set<number>();

      while (tempQueue.length > 0) {
        const { id, depth } = tempQueue.shift()!;
        if (tempVisited.has(id)) continue;
        tempVisited.add(id);

        depthCounts.set(depth, (depthCounts.get(depth) || 0) + 1);

        const children = childrenMap.get(id) || [];
        for (const childId of children) {
          if (!tempVisited.has(childId)) {
            tempQueue.push({ id: childId, depth: depth + 1, index: 0, parentX: 0 });
          }
        }
      }

      // Second pass: position nodes
      const depthIndices = new Map<number, number>();
      const nodeWidth = 200;
      const nodeHeight = 120;

      while (queue.length > 0) {
        const { id, depth } = queue.shift()!;
        if (visited.has(id)) continue;
        visited.add(id);

        const countAtDepth = depthCounts.get(depth) || 1;
        const index = depthIndices.get(depth) || 0;
        depthIndices.set(depth, index + 1);

        const totalWidth = countAtDepth * nodeWidth;
        const startX = -totalWidth / 2 + nodeWidth / 2;

        positions.set(id, {
          x: startX + index * nodeWidth,
          y: depth * nodeHeight,
        });

        const children = childrenMap.get(id) || [];
        for (const childId of children) {
          if (!visited.has(childId)) {
            queue.push({ id: childId, depth: depth + 1, index: 0, parentX: 0 });
          }
        }
      }
    }

    return nodes.map((node): Node => {
      const pos = positions.get(node.id) || { x: 0, y: 0 };
      // Count children (outgoing edges) for this node
      const expandCount = childrenMap.get(node.id)?.length || 0;
      return {
        id: String(node.id),
        type: 'custom',
        position: pos,
        data: {
          label: node.name,
          node,
          isRoot: node.id === rootId,
          isSelected: node.id === selectedNodeId,
          isPinned: pinnedNodeIds.has(node.id),
          expandCount: node.expanded ? 0 : expandCount, // Only show count if not expanded
          onExpand: () => onNodeExpand(node.id),
          onClick: (e: React.MouseEvent) => handleNodeClick(node.id, e),
        },
      };
    });
  }, [nodes, edges, rootId, selectedNodeId, pinnedNodeIds, handleNodeClick, onNodeExpand]);

  // Convert API edges to React Flow edges
  const flowEdges = useMemo(() => {
    return edges.map((edge): Edge => {
      // Different colors for different call kinds
      const getEdgeColor = (kind: string) => {
        switch (kind) {
          case 'interface': return '#a78bfa'; // purple for interface calls
          case 'funcval': return '#f472b6'; // pink for function values
          case 'defer': return '#facc15'; // yellow for defer
          case 'go': return '#34d399'; // green for goroutines
          case 'unknown': return '#f87171'; // red for unknown
          default: return '#6b7280'; // gray for static
        }
      };

      const edgeColor = getEdgeColor(edge.call_kind);
      const label = edge.call_kind !== 'static'
        ? `${edge.call_kind}${edge.callsite_count > 1 ? ` (×${edge.callsite_count})` : ''}`
        : edge.callsite_count > 1 ? `×${edge.callsite_count}` : undefined;

      return {
        id: `${edge.source_id}-${edge.target_id}`,
        source: String(edge.source_id),
        target: String(edge.target_id),
        markerEnd: {
          type: MarkerType.ArrowClosed,
          width: 15,
          height: 15,
          color: edgeColor,
        },
        style: {
          stroke: edgeColor,
          strokeWidth: edge.callsite_count > 1 ? 2.5 : 1.5,
          strokeDasharray: edge.call_kind === 'interface' ? '5,5' : undefined,
        },
        label,
        labelStyle: { fontSize: 10, fill: '#d1d5db' },
        labelBgStyle: { fill: '#1f2937', fillOpacity: 0.8 },
        animated: edge.call_kind === 'go', // Animate goroutine calls
      };
    });
  }, [edges]);

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState(flowNodes);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState(flowEdges);

  // Update nodes/edges when props change
  useEffect(() => {
    setRfNodes(flowNodes);
    setRfEdges(flowEdges);
  }, [flowNodes, flowEdges, setRfNodes, setRfEdges]);

  const hasGraph = rootId !== null;

  if (error) {
    return (
      <div className="h-full flex flex-col bg-gray-900">
        <Toolbar
          onCopyLink={onCopyLink}
          onExportSVG={onExportSVG}
          onExportPNG={onExportPNG}
          disabled={true}
        />
        <div className="flex-1 flex items-center justify-center text-red-400">
          Error: {error.message}
        </div>
      </div>
    );
  }

  if (!rootId && !isLoading) {
    return (
      <div className="h-full flex flex-col bg-gray-900">
        <Toolbar
          onCopyLink={onCopyLink}
          onExportSVG={onExportSVG}
          onExportPNG={onExportPNG}
          disabled={true}
        />
        <div className="flex-1 flex items-center justify-center text-gray-500">
          <div className="text-center">
            <div className="text-lg mb-2">Select an entrypoint</div>
            <div className="text-sm">Choose an entrypoint from the left panel to view its call graph</div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-gray-900">
      <Toolbar
        onCopyLink={onCopyLink}
        onExportSVG={onExportSVG}
        onExportPNG={onExportPNG}
        disabled={!hasGraph}
      />
      {breadcrumbs.length > 0 && (
        <Breadcrumbs
          items={breadcrumbs}
          onNavigate={onBreadcrumbNavigate}
          onClearFocus={onClearFocus}
        />
      )}
      <div className="flex-1 relative">
        {isLoading && (
          <div className="absolute inset-0 flex items-center justify-center bg-gray-900/80 z-10">
            <div className="text-gray-400">Loading graph...</div>
          </div>
        )}
        <ReactFlow
        nodes={rfNodes}
        edges={rfEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        minZoom={0.1}
        maxZoom={2}
        defaultEdgeOptions={{
          type: 'smoothstep',
        }}
      >
        <Background color="#374151" gap={20} />
        <Controls className="!bg-gray-800 !border-gray-700" />
        <MiniMap
          nodeColor={(node) => {
            const data = node.data as { isRoot: boolean; node: GraphNode };
            if (data?.isRoot) return '#3b82f6';
            if (data?.node?.tags?.some((t: string) => t.startsWith('io:'))) return '#f59e0b';
            return '#4b5563';
          }}
          className="!bg-gray-800 !border-gray-700"
        />
      </ReactFlow>
      {/* Help text and fit button */}
      <div className="absolute bottom-4 left-4 right-4 flex items-center justify-between">
        <div className="text-xs text-gray-500 space-y-0.5">
          <div>Double-click to expand • Shift+click to pin</div>
          <div className="flex gap-3">
            <span className="flex items-center gap-1">
              <span className="w-2 h-0.5 bg-gray-500"></span> static
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-0.5 bg-purple-400" style={{borderStyle: 'dashed', borderWidth: 1, borderColor: '#a78bfa'}}></span> interface
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-0.5 bg-green-400"></span> go
            </span>
          </div>
        </div>
        <button
          onClick={() => fitView({ padding: 0.2, duration: 300 })}
          className="px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs rounded border border-gray-700 transition-colors"
          title="Fit graph to view"
        >
          Fit View
        </button>
      </div>
      </div>
    </div>
  );
}
