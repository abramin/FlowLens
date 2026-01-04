import { useState, useMemo, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  MarkerType,
  Handle,
  Position,
} from '@xyflow/react';
import type { Node, Edge } from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { getCFG } from '../api';
import type { BasicBlockInfo, InstructionInfo, CFGViewMode } from '../types';
import { deriveBlockSummary } from '../utils/cfgSummary';

interface CFGPanelProps {
  symbolId: number | null;
  symbolName: string;
  onClose: () => void;
  onFollowCall: (calleeId: number) => void;
}

// Colors for different block types
const BLOCK_COLORS = {
  entry: { bg: '#3b82f6', border: '#1d4ed8', text: '#ffffff' },
  exit: { bg: '#10b981', border: '#059669', text: '#ffffff' },
  normal: { bg: '#374151', border: '#4b5563', text: '#e5e7eb' },
};

// Colors for different instruction types
const INSTR_COLORS: Record<string, string> = {
  call: 'text-blue-300',
  go: 'text-green-300',
  defer: 'text-yellow-300',
  return: 'text-emerald-300',
  if: 'text-purple-300',
  panic: 'text-red-300',
};

interface BlockNodeData {
  block: BasicBlockInfo;
  onFollowCall: (calleeId: number) => void;
  viewMode: CFGViewMode;
}

function BlockNode({ data }: { data: BlockNodeData }) {
  const { block, onFollowCall, viewMode } = data;
  const colors = block.is_entry
    ? BLOCK_COLORS.entry
    : block.is_exit
    ? BLOCK_COLORS.exit
    : BLOCK_COLORS.normal;

  // Derive summary for this block
  const summary = useMemo(() => deriveBlockSummary(block), [block]);

  // Choose which instructions to display
  const displayInstructions = viewMode === 'summary'
    ? summary.instructions
    : block.instructions;

  // In summary mode, show minimized view for trivial blocks
  const isMinimized = viewMode === 'summary' && summary.isTrivial;

  return (
    <div
      className={`rounded-lg shadow-lg overflow-hidden ${isMinimized ? 'opacity-60' : ''}`}
      style={{
        backgroundColor: colors.bg,
        borderWidth: 2,
        borderColor: colors.border,
        borderStyle: 'solid',
        minWidth: isMinimized ? 100 : 200,
        maxWidth: 350,
      }}
    >
      <Handle type="target" position={Position.Top} className="!bg-gray-500" />

      {/* Block header */}
      <div
        className="px-3 py-1.5 text-xs font-medium border-b"
        style={{ borderColor: colors.border, color: colors.text }}
      >
        <span>Block {block.index}</span>
        {block.is_entry && <span className="ml-2 text-blue-200">(entry)</span>}
        {block.is_exit && <span className="ml-2 text-emerald-200">(exit)</span>}
      </div>

      {/* Instructions - different rendering based on mode */}
      {!isMinimized && displayInstructions.length > 0 && (
        <div className="p-2 space-y-0.5 max-h-[200px] overflow-y-auto">
          {viewMode === 'summary' ? (
            // Summary mode: show all interesting instructions
            displayInstructions.map((instr) => (
              <InstructionLine
                key={instr.index}
                instr={instr}
                onFollowCall={onFollowCall}
              />
            ))
          ) : (
            // Detail mode: show first 10, then overflow
            <>
              {displayInstructions.slice(0, 10).map((instr) => (
                <InstructionLine
                  key={instr.index}
                  instr={instr}
                  onFollowCall={onFollowCall}
                />
              ))}
              {displayInstructions.length > 10 && (
                <div className="text-xs text-gray-400 italic">
                  +{displayInstructions.length - 10} more...
                </div>
              )}
            </>
          )}
        </div>
      )}

      {/* Trivial block indicator */}
      {isMinimized && (
        <div className="px-3 py-2 text-xs text-gray-400 text-center">
          ...
        </div>
      )}

      {/* Terminator in summary mode */}
      {viewMode === 'summary' && summary.terminator && (
        <div
          className="px-3 py-1.5 text-xs bg-purple-900/50 border-t"
          style={{ borderColor: colors.border }}
        >
          <span className="text-purple-300">{summary.terminator}</span>
        </div>
      )}

      {/* Branch condition in detail mode */}
      {viewMode === 'detail' && block.branch_cond && block.branch_cond !== 'return' && (
        <div
          className="px-3 py-1.5 text-xs bg-purple-900/50 border-t"
          style={{ borderColor: colors.border }}
        >
          <span className="text-purple-300">{block.branch_cond}</span>
        </div>
      )}

      <Handle type="source" position={Position.Bottom} className="!bg-gray-500" />
    </div>
  );
}

function InstructionLine({
  instr,
  onFollowCall,
}: {
  instr: InstructionInfo;
  onFollowCall: (calleeId: number) => void;
}) {
  const colorClass = INSTR_COLORS[instr.op] || 'text-gray-300';
  const isClickable = instr.callee_id !== undefined;

  return (
    <div
      className={`text-xs font-mono truncate ${colorClass} ${
        isClickable ? 'cursor-pointer hover:underline' : ''
      }`}
      title={instr.text}
      onClick={() => {
        if (instr.callee_id !== undefined) {
          onFollowCall(instr.callee_id);
        }
      }}
    >
      <span className="text-gray-500 mr-1">{instr.index}:</span>
      {instr.text}
    </div>
  );
}

const nodeTypes = {
  block: BlockNode,
};

export function CFGPanel({ symbolId, symbolName, onClose, onFollowCall }: CFGPanelProps) {
  // View mode: summary (default) or detail (full SSA)
  const [viewMode, setViewMode] = useState<CFGViewMode>('summary');

  // Fetch CFG data
  const { data: cfgData, isLoading, error } = useQuery({
    queryKey: ['cfg', symbolId],
    queryFn: async () => {
      if (!symbolId) return null;
      return getCFG(symbolId);
    },
    enabled: !!symbolId,
  });

  // Convert CFG to ReactFlow nodes
  const flowNodes = useMemo(() => {
    if (!cfgData) return [];

    const nodeWidth = 250;
    const nodeHeight = 150;
    const horizontalSpacing = 100;
    const verticalSpacing = 80;

    // Simple layout: arrange blocks by index
    // Entry at top, then branch based on successors
    const positions = new Map<number, { x: number; y: number }>();

    // Use BFS to position nodes
    const visited = new Set<number>();
    const queue: { index: number; depth: number }[] = [
      { index: cfgData.entry_block, depth: 0 },
    ];

    const depthLanes = new Map<number, number>();

    while (queue.length > 0) {
      const { index, depth } = queue.shift()!;
      if (visited.has(index)) continue;
      visited.add(index);

      const currentLane = depthLanes.get(depth) ?? 0;
      depthLanes.set(depth, currentLane + 1);

      positions.set(index, {
        x: currentLane * (nodeWidth + horizontalSpacing),
        y: depth * (nodeHeight + verticalSpacing),
      });

      const block = cfgData.blocks.find((b) => b.index === index);
      if (block && block.successors) {
        for (const succIdx of block.successors) {
          if (!visited.has(succIdx)) {
            queue.push({ index: succIdx, depth: depth + 1 });
          }
        }
      }
    }

    return cfgData.blocks.map((block): Node => ({
      id: String(block.index),
      type: 'block',
      position: positions.get(block.index) || { x: 0, y: block.index * 200 },
      data: {
        block,
        onFollowCall,
        viewMode,
      },
    }));
  }, [cfgData, onFollowCall, viewMode]);

  // Convert CFG to ReactFlow edges
  const flowEdges = useMemo(() => {
    if (!cfgData) return [];

    const edges: Edge[] = [];

    for (const block of cfgData.blocks) {
      const successors = block.successors || [];
      for (let i = 0; i < successors.length; i++) {
        const succIdx = successors[i];
        const isTrue = i === 0 && successors.length === 2;
        const isFalse = i === 1 && successors.length === 2;

        let edgeColor = '#6b7280';
        let label: string | undefined;

        if (block.branch_cond && successors.length === 2) {
          if (isTrue) {
            edgeColor = '#10b981'; // green for true branch
            label = 'true';
          } else if (isFalse) {
            edgeColor = '#ef4444'; // red for false branch
            label = 'false';
          }
        }

        edges.push({
          id: `${block.index}-${succIdx}`,
          source: String(block.index),
          target: String(succIdx),
          markerEnd: {
            type: MarkerType.ArrowClosed,
            width: 12,
            height: 12,
            color: edgeColor,
          },
          style: {
            stroke: edgeColor,
            strokeWidth: 2,
          },
          label,
          labelStyle: { fontSize: 10, fill: '#d1d5db' },
          labelBgStyle: { fill: '#1f2937', fillOpacity: 0.8 },
        });
      }
    }

    return edges;
  }, [cfgData]);

  const [rfNodes, setRfNodes, onNodesChange] = useNodesState(flowNodes);
  const [rfEdges, setRfEdges, onEdgesChange] = useEdgesState(flowEdges);

  useEffect(() => {
    setRfNodes(flowNodes);
    setRfEdges(flowEdges);
  }, [flowNodes, flowEdges, setRfNodes, setRfEdges]);

  if (!symbolId) {
    return null;
  }

  return (
    <div className="fixed inset-4 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/70"
        onClick={onClose}
      />

      {/* Modal content */}
      <div className="relative bg-gray-900 rounded-lg shadow-2xl border border-gray-700 w-full max-w-5xl h-[80vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-700">
          <div>
            <h2 className="text-sm font-medium text-gray-200">
              Control Flow Graph
            </h2>
            <div className="text-xs text-gray-400">{symbolName}</div>
          </div>

          {/* View mode toggle and close button */}
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1 bg-gray-800 rounded-lg p-0.5">
              <button
                onClick={() => setViewMode('summary')}
                className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${
                  viewMode === 'summary'
                    ? 'bg-gray-700 text-white'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
              >
                Summary
              </button>
              <button
                onClick={() => setViewMode('detail')}
                className={`px-3 py-1 text-xs font-medium rounded-md transition-colors ${
                  viewMode === 'detail'
                    ? 'bg-gray-700 text-white'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
              >
                Detail
              </button>
            </div>

            <button
              onClick={onClose}
              className="p-1 text-gray-400 hover:text-gray-200 transition-colors"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>
        </div>

        {/* CFG visualization */}
        <div className="flex-1 relative">
          {isLoading && (
            <div className="absolute inset-0 flex items-center justify-center bg-gray-900/80 z-10">
              <div className="text-gray-400">Loading CFG...</div>
            </div>
          )}

          {error && (
            <div className="absolute inset-0 flex items-center justify-center">
              <div className="text-red-400">Error: {(error as Error).message}</div>
            </div>
          )}

          {cfgData && (
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
            </ReactFlow>
          )}
        </div>

        {/* Footer legend */}
        <div className="px-4 py-2 border-t border-gray-700 text-xs text-gray-500">
          <div className="flex items-center gap-4">
            <span>
              <span className="inline-block w-3 h-3 rounded bg-blue-600 mr-1" />
              Entry
            </span>
            <span>
              <span className="inline-block w-3 h-3 rounded bg-emerald-600 mr-1" />
              Exit
            </span>
            <span className="text-green-400">true</span>
            <span className="text-red-400">false</span>
            <span className="ml-auto">
              {viewMode === 'summary'
                ? 'Showing calls and branch conditions'
                : 'Showing all SSA instructions'
              }
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
