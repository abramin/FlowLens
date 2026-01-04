import { useEffect, useMemo } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  MarkerType,
  Handle,
  Position,
} from '@xyflow/react';
import type { Node, Edge } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import type { GraphNode, GraphEdge, GraphFilter } from '../types';

interface GraphPanelProps {
  nodes: GraphNode[];
  edges: GraphEdge[];
  rootId: number | null;
  isLoading: boolean;
  error: Error | null;
  onNodeClick: (nodeId: number) => void;
  onNodeExpand: (nodeId: number) => void;
  filters: GraphFilter;
}

// Colors for different node states
const NODE_COLORS = {
  root: { bg: '#3b82f6', border: '#1d4ed8', text: '#ffffff' },
  io: { bg: '#f59e0b', border: '#d97706', text: '#ffffff' },
  expanded: { bg: '#374151', border: '#4b5563', text: '#e5e7eb' },
  default: { bg: '#1f2937', border: '#374151', text: '#9ca3af' },
};

function getNodeColor(node: GraphNode, isRoot: boolean) {
  if (isRoot) return NODE_COLORS.root;
  if (node.tags.some((t) => t.startsWith('io:'))) return NODE_COLORS.io;
  if (node.expanded) return NODE_COLORS.expanded;
  return NODE_COLORS.default;
}

interface CustomNodeProps {
  data: {
    label: string;
    node: GraphNode;
    isRoot: boolean;
    onExpand: () => void;
    onClick: () => void;
  };
}

function CustomNode({ data }: CustomNodeProps) {
  const colors = getNodeColor(data.node, data.isRoot);
  const hasIOTag = data.node.tags.some((t) => t.startsWith('io:'));

  return (
    <div
      className="px-3 py-2 rounded-lg shadow-lg cursor-pointer transition-transform hover:scale-105"
      style={{
        backgroundColor: colors.bg,
        borderWidth: 2,
        borderColor: colors.border,
        borderStyle: 'solid',
        minWidth: 120,
        maxWidth: 200,
      }}
      onClick={data.onClick}
      onDoubleClick={(e) => {
        e.stopPropagation();
        data.onExpand();
      }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />
      <div className="text-center">
        <div
          className="text-xs font-medium truncate"
          style={{ color: colors.text }}
          title={data.node.name}
        >
          {data.node.recv_type ? `(${data.node.recv_type}).` : ''}
          {data.node.name}
        </div>
        <div className="text-xs opacity-60 truncate" style={{ color: colors.text }}>
          {data.node.pkg_path.split('/').pop()}
        </div>
        {hasIOTag && (
          <div className="mt-1 flex justify-center gap-1">
            {data.node.tags
              .filter((t) => t.startsWith('io:'))
              .map((tag) => (
                <span
                  key={tag}
                  className="px-1 py-0.5 text-[10px] bg-black/20 rounded"
                >
                  {tag.replace('io:', '')}
                </span>
              ))}
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
}: GraphPanelProps) {
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
      const nodeWidth = 180;
      const nodeHeight = 100;

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
      return {
        id: String(node.id),
        type: 'custom',
        position: pos,
        data: {
          label: node.name,
          node,
          isRoot: node.id === rootId,
          onExpand: () => onNodeExpand(node.id),
          onClick: () => onNodeClick(node.id),
        },
      };
    });
  }, [nodes, edges, rootId, onNodeClick, onNodeExpand]);

  // Convert API edges to React Flow edges
  const flowEdges = useMemo(() => {
    return edges.map((edge): Edge => ({
      id: `${edge.source_id}-${edge.target_id}`,
      source: String(edge.source_id),
      target: String(edge.target_id),
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 15,
        height: 15,
        color: '#6b7280',
      },
      style: {
        stroke: '#6b7280',
        strokeWidth: edge.callsite_count > 1 ? 2 : 1,
      },
      label: edge.call_kind !== 'static' ? edge.call_kind : undefined,
      labelStyle: { fontSize: 10, fill: '#9ca3af' },
      labelBgStyle: { fill: '#1f2937' },
    }));
  }, [edges]);

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState(flowNodes);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState(flowEdges);

  // Update nodes/edges when props change
  useEffect(() => {
    setRfNodes(flowNodes);
    setRfEdges(flowEdges);
  }, [flowNodes, flowEdges, setRfNodes, setRfEdges]);

  if (error) {
    return (
      <div className="h-full flex items-center justify-center bg-gray-900 text-red-400">
        Error: {error.message}
      </div>
    );
  }

  if (!rootId && !isLoading) {
    return (
      <div className="h-full flex items-center justify-center bg-gray-900 text-gray-500">
        <div className="text-center">
          <div className="text-lg mb-2">Select an entrypoint</div>
          <div className="text-sm">Choose an entrypoint from the left panel to view its call graph</div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full relative bg-gray-900">
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
      <div className="absolute bottom-4 left-4 text-xs text-gray-500">
        Double-click a node to expand
      </div>
    </div>
  );
}
