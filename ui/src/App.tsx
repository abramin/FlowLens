import { useState, useCallback } from 'react';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { ReactFlowProvider } from '@xyflow/react';
import { EntrypointsPanel } from './components/EntrypointsPanel';
import { GraphPanel } from './components/GraphPanel';
import { InspectorPanel } from './components/InspectorPanel';
import { getGraphRoot, getGraphExpand } from './api';
import type { Entrypoint, GraphNode, GraphEdge, GraphFilter } from './types';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});

function AppContent() {
  const [selectedEntrypoint, setSelectedEntrypoint] = useState<Entrypoint | null>(null);
  const [selectedNode, setSelectedNode] = useState<GraphNode | null>(null);
  const [pinnedNodeIds, setPinnedNodeIds] = useState<Set<number>>(new Set());
  const [filters, setFilters] = useState<GraphFilter>({
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 6,
  });

  // Store expanded nodes and their graph data
  const [graphNodes, setGraphNodes] = useState<GraphNode[]>([]);
  const [graphEdges, setGraphEdges] = useState<GraphEdge[]>([]);

  // Query for initial graph load
  const { isLoading, error } = useQuery({
    queryKey: ['graph', selectedEntrypoint?.symbol_id, filters],
    queryFn: async () => {
      if (!selectedEntrypoint) return null;
      const response = await getGraphRoot(selectedEntrypoint.symbol_id, filters.maxDepth, filters);
      setGraphNodes(response.nodes);
      setGraphEdges(response.edges);
      return response;
    },
    enabled: !!selectedEntrypoint,
  });

  const handleSelectEntrypoint = useCallback((entrypoint: Entrypoint) => {
    setSelectedEntrypoint(entrypoint);
    setSelectedNode(null);
    setPinnedNodeIds(new Set());
    setGraphNodes([]);
    setGraphEdges([]);
  }, []);

  const handleNodePin = useCallback((nodeId: number) => {
    setPinnedNodeIds((prev) => {
      const next = new Set(prev);
      if (next.has(nodeId)) {
        next.delete(nodeId);
      } else {
        next.add(nodeId);
      }
      return next;
    });
  }, []);

  const handleNodeClick = useCallback((nodeId: number) => {
    const node = graphNodes.find((n) => n.id === nodeId);
    if (node) {
      setSelectedNode(node);
    }
  }, [graphNodes]);

  const handleNodeExpand = useCallback(async (nodeId: number) => {
    try {
      const response = await getGraphExpand(nodeId, 1, filters);

      // Merge new nodes (avoid duplicates)
      setGraphNodes((prev) => {
        const existingIds = new Set(prev.map((n) => n.id));
        const newNodes = response.nodes.filter((n) => !existingIds.has(n.id));

        // Mark the expanded node as expanded
        const updated = prev.map((n) =>
          n.id === nodeId ? { ...n, expanded: true } : n
        );

        return [...updated, ...newNodes];
      });

      // Merge new edges (avoid duplicates)
      setGraphEdges((prev) => {
        const existingEdgeKeys = new Set(
          prev.map((e) => `${e.source_id}-${e.target_id}`)
        );
        const newEdges = response.edges.filter(
          (e) => !existingEdgeKeys.has(`${e.source_id}-${e.target_id}`)
        );
        return [...prev, ...newEdges];
      });
    } catch (err) {
      console.error('Failed to expand node:', err);
    }
  }, [filters]);

  const handleFiltersChange = useCallback((newFilters: GraphFilter) => {
    setFilters(newFilters);
  }, []);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-gray-950">
      {/* Left Panel - Entrypoints */}
      <div className="w-[280px] flex-shrink-0 border-r border-gray-800">
        <EntrypointsPanel
          selectedId={selectedEntrypoint?.symbol_id ?? null}
          onSelect={handleSelectEntrypoint}
        />
      </div>

      {/* Center Panel - Graph */}
      <div className="flex-1 min-w-0">
        <ReactFlowProvider>
          <GraphPanel
            nodes={graphNodes}
            edges={graphEdges}
            rootId={selectedEntrypoint?.symbol_id ?? null}
            isLoading={isLoading}
            error={error as Error | null}
            onNodeClick={handleNodeClick}
            onNodeExpand={handleNodeExpand}
            onNodePin={handleNodePin}
            selectedNodeId={selectedNode?.id ?? null}
            pinnedNodeIds={pinnedNodeIds}
            filters={filters}
          />
        </ReactFlowProvider>
      </div>

      {/* Right Panel - Inspector */}
      <div className="w-[320px] flex-shrink-0 border-l border-gray-800">
        <InspectorPanel
          selectedNode={selectedNode}
          filters={filters}
          onFiltersChange={handleFiltersChange}
        />
      </div>
    </div>
  );
}

function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppContent />
    </QueryClientProvider>
  );
}

export default App;
