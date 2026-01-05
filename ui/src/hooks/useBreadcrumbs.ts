import { useMemo } from 'react';
import type { GraphNode, GraphEdge } from '../types';

export interface BreadcrumbItem {
  nodeId: number;
  label: string;
  fullLabel: string;
  depth: number;
}

function buildParentMap(edges: GraphEdge[]): Map<number, number> {
  const parentMap = new Map<number, number>();
  for (const edge of edges) {
    // For each target, record its source as parent
    // If multiple edges point to same target, use the first one
    if (!parentMap.has(edge.target_id)) {
      parentMap.set(edge.target_id, edge.source_id);
    }
  }
  return parentMap;
}

function getPathToRoot(
  nodeId: number,
  parentMap: Map<number, number>,
  rootId: number
): number[] {
  const path: number[] = [];
  let current: number | undefined = nodeId;
  const visited = new Set<number>();

  while (current !== undefined) {
    if (visited.has(current)) break; // Prevent cycles
    visited.add(current);
    path.push(current);
    if (current === rootId) break;
    current = parentMap.get(current);
  }

  return path.reverse();
}

function formatNodeLabel(node: GraphNode): string {
  if (node.recv_type) {
    return `(${node.recv_type}).${node.name}`;
  }
  return node.name;
}

function truncateLabel(label: string, maxLen: number = 25): string {
  if (label.length <= maxLen) return label;
  return label.slice(0, maxLen - 1) + '…';
}

export function useBreadcrumbs(
  focusedNodeId: number | null,
  graphNodes: GraphNode[],
  graphEdges: GraphEdge[],
  rootId: number | null
): BreadcrumbItem[] {
  return useMemo(() => {
    if (!focusedNodeId || !rootId || graphNodes.length === 0) {
      return [];
    }

    const nodeMap = new Map(graphNodes.map((n) => [n.id, n]));
    const parentMap = buildParentMap(graphEdges);
    const path = getPathToRoot(focusedNodeId, parentMap, rootId);

    // If path is empty or doesn't start with root, return empty
    if (path.length === 0 || path[0] !== rootId) {
      // Focused node might not be connected to root
      // Just show the focused node itself
      const focusedNode = nodeMap.get(focusedNodeId);
      if (focusedNode) {
        const fullLabel = formatNodeLabel(focusedNode);
        return [{
          nodeId: focusedNodeId,
          label: truncateLabel(fullLabel),
          fullLabel,
          depth: focusedNode.depth,
        }];
      }
      return [];
    }

    return path.map((nodeId, index) => {
      const node = nodeMap.get(nodeId);
      if (!node) {
        return {
          nodeId,
          label: '...',
          fullLabel: 'Unknown',
          depth: index,
        };
      }
      const fullLabel = formatNodeLabel(node);
      return {
        nodeId,
        label: truncateLabel(fullLabel),
        fullLabel,
        depth: node.depth,
      };
    });
  }, [focusedNodeId, graphNodes, graphEdges, rootId]);
}
