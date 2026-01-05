import { useEffect, useState, useCallback } from 'react';
import { useQuery } from '@tanstack/react-query';
import { getSpine, getGraphExpand } from '../api';
import { SpineNodeComponent } from './SpineNodeComponent';
import type { GraphFilter, SpineNode } from '../types';

interface CallSpineViewProps {
  rootId: number | null;
  filters: GraphFilter;
  selectedNodeId: number | null;
  onNodeClick: (nodeId: number) => void;
  onNodeExpand: (nodeId: number, expandedNodes: SpineNode[]) => void;
  onSpineNodesUpdate?: (nodes: SpineNode[]) => void;
}

export function CallSpineView({
  rootId,
  filters,
  selectedNodeId,
  onNodeClick,
  onNodeExpand,
  onSpineNodesUpdate,
}: CallSpineViewProps) {
  const [expandedBranches, setExpandedBranches] = useState<Set<number>>(new Set());
  const [branchNodes, setBranchNodes] = useState<Map<number, SpineNode[]>>(new Map());

  // Fetch spine data
  const { data: spineData, isLoading, error } = useQuery({
    queryKey: ['spine', rootId, filters],
    queryFn: async () => {
      if (!rootId) return null;
      return getSpine(rootId, filters.maxDepth ?? 10, filters);
    },
    enabled: !!rootId,
  });

  // Reset expanded branches when root changes
  useEffect(() => {
    setExpandedBranches(new Set());
    setBranchNodes(new Map());
  }, [rootId]);

  // Notify parent when spine nodes change
  useEffect(() => {
    if (spineData?.nodes && onSpineNodesUpdate) {
      onSpineNodesUpdate(spineData.nodes);
    }
  }, [spineData?.nodes, onSpineNodesUpdate]);

  // Handle expanding a branch badge
  const handleExpandBranch = useCallback(async (parentNodeId: number, symbolIds: number[]) => {
    if (expandedBranches.has(parentNodeId)) {
      // Collapse
      setExpandedBranches((prev) => {
        const next = new Set(prev);
        next.delete(parentNodeId);
        return next;
      });
    } else {
      // Expand - fetch details for collapsed nodes
      setExpandedBranches((prev) => new Set(prev).add(parentNodeId));

      // Fetch expanded node details
      try {
        const response = await getGraphExpand(parentNodeId, 1, filters);
        const expandedSpineNodes: SpineNode[] = response.nodes
          .filter((n) => symbolIds.includes(n.id))
          .map((n) => ({
            id: n.id,
            name: n.name,
            pkg_path: n.pkg_path,
            recv_type: n.recv_type,
            file: n.file,
            line: n.line,
            tags: n.tags,
            depth: n.depth,
            is_main_path: false,
            layer: n.tags.find((t) => t.startsWith('layer:'))?.replace('layer:', ''),
          }));

        setBranchNodes((prev) => new Map(prev).set(parentNodeId, expandedSpineNodes));
        onNodeExpand(parentNodeId, expandedSpineNodes);
      } catch (err) {
        console.error('Failed to expand branch:', err);
      }
    }
  }, [expandedBranches, filters, onNodeExpand]);

  if (!rootId) {
    return (
      <div className="h-full flex items-center justify-center bg-[#0d1117]">
        <div className="text-center">
          <div className="text-lg text-gray-400 mb-2">Select an entrypoint</div>
          <div className="text-sm text-gray-600">
            Choose an entrypoint from the left panel to view its call spine
          </div>
        </div>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="h-full flex items-center justify-center bg-[#0d1117]">
        <div className="text-gray-500">Loading call spine...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="h-full flex items-center justify-center bg-[#0d1117]">
        <div className="text-red-400">Error: {(error as Error).message}</div>
      </div>
    );
  }

  if (!spineData || spineData.nodes.length === 0) {
    return (
      <div className="h-full flex items-center justify-center bg-[#0d1117]">
        <div className="text-gray-500">No call spine data available</div>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-[#0d1117]">
      {/* Spine visualization */}
      <div className="flex-1 overflow-auto">
        <div className="flex flex-col items-center py-8 px-4 min-h-full">
          {/* Main spine nodes */}
          {spineData.nodes.map((node, index) => (
            <div key={node.id} className="flex flex-col items-center w-full max-w-2xl">
              <SpineNodeComponent
                node={node}
                isFirst={index === 0}
                isLast={index === spineData.nodes.length - 1 && !expandedBranches.has(node.id)}
                isSelected={node.id === selectedNodeId}
                onClick={onNodeClick}
                onExpandBranch={(symbolIds) => handleExpandBranch(node.id, symbolIds)}
              />

              {/* Expanded branch nodes */}
              {expandedBranches.has(node.id) && branchNodes.get(node.id) && (
                <div className="w-full max-w-lg ml-8 mt-3 mb-3">
                  <div className="pl-4 border-l-2 border-gray-800">
                    <div className="text-xs text-gray-600 mb-2 font-medium">Branch calls</div>
                    <div className="space-y-1.5">
                      {branchNodes.get(node.id)!.map((branchNode) => (
                        <div
                          key={branchNode.id}
                          className={`
                            px-3 py-2 rounded-md cursor-pointer
                            transition-colors
                            ${branchNode.id === selectedNodeId
                              ? 'bg-[#1e3a5f] border border-blue-500/50'
                              : 'bg-[#161b22] border border-gray-800 hover:border-gray-700'
                            }
                          `}
                          onClick={() => onNodeClick(branchNode.id)}
                        >
                          <div className="text-xs text-gray-300 font-medium">
                            {branchNode.recv_type
                              ? `(${branchNode.recv_type}).${branchNode.name}`
                              : branchNode.name}
                          </div>
                          <div className="text-xs text-gray-600 mt-0.5">
                            {branchNode.pkg_path.split('/').pop()}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              )}
            </div>
          ))}

          {/* End indicator */}
          <div className="mt-8 mb-4">
            <div className="text-xs text-gray-600 bg-[#161b22] border border-gray-800 px-4 py-1.5 rounded-full">
              End of trace
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
