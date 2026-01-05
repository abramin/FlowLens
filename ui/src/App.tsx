import { useState, useCallback, useEffect, useRef } from 'react';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { ReactFlowProvider } from '@xyflow/react';
import { EntrypointsPanel } from './components/EntrypointsPanel';
import { GraphPanel } from './components/GraphPanel';
import { CallSpineView } from './components/CallSpineView';
import { InspectorPanel } from './components/InspectorPanel';
import { CFGPanel } from './components/CFGPanel';
import { getGraphRoot, getGraphExpand, getEntrypointBySymbolId } from './api';
import { useURLState } from './hooks/useURLState';
import { useBreadcrumbs } from './hooks/useBreadcrumbs';
import { exportToSVG, exportToPNG, getReactFlowContainer } from './utils/exportGraph';
import type { Entrypoint, GraphNode, GraphEdge, GraphFilter, SpineNode } from './types';

type ViewMode = 'graph' | 'spine';

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
  const [focusedNodeId, setFocusedNodeId] = useState<number | null>(null);
  const [pinnedNodeIds, setPinnedNodeIds] = useState<Set<number>>(new Set());
  const [viewMode, setViewMode] = useState<ViewMode>('spine');
  const [cfgSymbolId, setCfgSymbolId] = useState<number | null>(null);
  const [cfgSymbolName, setCfgSymbolName] = useState<string>('');
  const [spineNodes, setSpineNodes] = useState<SpineNode[]>([]);
  const [filters, setFilters] = useState<GraphFilter>({
    hideStdlib: false,
    hideVendors: false,
    stopAtIO: false,
    maxDepth: 6,
    collapseWiring: true,
    hideCmdMain: true,
  });

  // Store expanded nodes and their graph data
  const [graphNodes, setGraphNodes] = useState<GraphNode[]>([]);
  const [graphEdges, setGraphEdges] = useState<GraphEdge[]>([]);

  // URL state management
  const { initialState, updateURL, copyShareLink } = useURLState();
  const urlRestored = useRef(false);

  // Restore state from URL on mount
  useEffect(() => {
    if (urlRestored.current || !initialState) return;
    urlRestored.current = true;

    const restoreState = async () => {
      if (initialState.entrypointId !== null) {
        // Fetch the entrypoint by symbol_id
        const entrypoint = await getEntrypointBySymbolId(initialState.entrypointId);
        if (entrypoint) {
          setSelectedEntrypoint(entrypoint);
          setFilters(initialState.filters);
          setPinnedNodeIds(new Set(initialState.pinnedNodeIds));
          if (initialState.selectedNodeId !== null) {
            setFocusedNodeId(initialState.selectedNodeId);
          }
        }
      }
    };

    restoreState();
  }, [initialState]);

  // Sync state to URL when it changes
  useEffect(() => {
    if (!urlRestored.current && initialState) return;

    const expandedNodeIds = graphNodes.filter((n) => n.expanded).map((n) => n.id);

    updateURL({
      entrypointId: selectedEntrypoint?.symbol_id ?? null,
      filters,
      pinnedNodeIds: Array.from(pinnedNodeIds),
      expandedNodeIds,
      selectedNodeId: selectedNode?.id ?? null,
    });
  }, [selectedEntrypoint, filters, pinnedNodeIds, graphNodes, selectedNode, updateURL, initialState]);

  // Breadcrumbs
  const breadcrumbs = useBreadcrumbs(
    focusedNodeId,
    graphNodes,
    graphEdges,
    selectedEntrypoint?.symbol_id ?? null
  );

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
    setFocusedNodeId(null);
    setPinnedNodeIds(new Set());
    setGraphNodes([]);
    setGraphEdges([]);
    setSpineNodes([]);
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
      setFocusedNodeId(nodeId);
    } else {
      // Check spine nodes
      const spineNode = spineNodes.find((n) => n.id === nodeId);
      if (spineNode) {
        // Convert spine node to graph node for selection
        const graphNode: GraphNode = {
          id: spineNode.id,
          name: spineNode.name,
          pkg_path: spineNode.pkg_path,
          file: spineNode.file,
          line: spineNode.line,
          kind: 'func',
          recv_type: spineNode.recv_type,
          tags: spineNode.tags,
          expanded: false,
          depth: spineNode.depth,
        };
        setSelectedNode(graphNode);
        setFocusedNodeId(nodeId);
      }
    }
  }, [graphNodes, spineNodes]);

  const handleNodeExpand = useCallback(async (nodeId: number) => {
    try {
      const response = await getGraphExpand(nodeId, 1, filters);

      setGraphNodes((prev) => {
        const existingIds = new Set(prev.map((n) => n.id));
        const newNodes = response.nodes.filter((n) => !existingIds.has(n.id));
        const updated = prev.map((n) =>
          n.id === nodeId ? { ...n, expanded: true } : n
        );
        return [...updated, ...newNodes];
      });

      setGraphEdges((prev) => {
        const existingEdgeKeys = new Set(
          prev.map((e) => `${e.source_id}-${e.target_id}`)
        );
        const newEdges = response.edges.filter(
          (e) => !existingEdgeKeys.has(`${e.source_id}-${e.target_id}`)
        );
        return [...prev, ...newEdges];
      });

      setFocusedNodeId(nodeId);
    } catch (err) {
      console.error('Failed to expand node:', err);
    }
  }, [filters]);

  const handleFiltersChange = useCallback((newFilters: GraphFilter) => {
    setFilters(newFilters);
  }, []);

  const handleNavigateToNode = useCallback((symbolId: number) => {
    const node = graphNodes.find((n) => n.id === symbolId);
    if (node) {
      setSelectedNode(node);
      setFocusedNodeId(symbolId);
    }
  }, [graphNodes]);

  const handleBreadcrumbNavigate = useCallback((nodeId: number) => {
    const node = graphNodes.find((n) => n.id === nodeId);
    if (node) {
      setSelectedNode(node);
      setFocusedNodeId(nodeId);
    }
  }, [graphNodes]);

  const handleClearFocus = useCallback(() => {
    setFocusedNodeId(selectedEntrypoint?.symbol_id ?? null);
    const rootNode = graphNodes.find((n) => n.id === selectedEntrypoint?.symbol_id);
    if (rootNode) {
      setSelectedNode(rootNode);
    }
  }, [graphNodes, selectedEntrypoint]);

  const handleCopyLink = useCallback(async (): Promise<boolean> => {
    const expandedNodeIds = graphNodes.filter((n) => n.expanded).map((n) => n.id);
    return copyShareLink({
      entrypointId: selectedEntrypoint?.symbol_id ?? null,
      filters,
      pinnedNodeIds: Array.from(pinnedNodeIds),
      expandedNodeIds,
      selectedNodeId: selectedNode?.id ?? null,
    });
  }, [selectedEntrypoint, filters, pinnedNodeIds, graphNodes, selectedNode, copyShareLink]);

  const handleExportSVG = useCallback(async (): Promise<void> => {
    const container = getReactFlowContainer();
    if (!container) throw new Error('Graph container not found');
    await exportToSVG(container);
  }, []);

  const handleExportPNG = useCallback(async (): Promise<void> => {
    const container = getReactFlowContainer();
    if (!container) throw new Error('Graph container not found');
    await exportToPNG(container);
  }, []);

  const handleShowCFG = useCallback((symbolId: number, symbolName: string) => {
    setCfgSymbolId(symbolId);
    setCfgSymbolName(symbolName);
  }, []);

  const handleCloseCFG = useCallback(() => {
    setCfgSymbolId(null);
    setCfgSymbolName('');
  }, []);

  const handleCFGFollowCall = useCallback((calleeId: number) => {
    const node = graphNodes.find((n) => n.id === calleeId);
    const name = node ? (node.recv_type ? `(${node.recv_type}).${node.name}` : node.name) : `Symbol ${calleeId}`;
    setCfgSymbolId(calleeId);
    setCfgSymbolName(name);
  }, [graphNodes]);

  const handleSpineNodesUpdate = useCallback((nodes: SpineNode[]) => {
    setSpineNodes(nodes);
  }, []);

  return (
    <div className="flex h-screen w-screen overflow-hidden bg-[#010409]">
      {/* Left Panel - Entrypoints */}
      <div className="w-[260px] flex-shrink-0 border-r border-gray-800/50">
        <EntrypointsPanel
          selectedId={selectedEntrypoint?.symbol_id ?? null}
          onSelect={handleSelectEntrypoint}
        />
      </div>

      {/* Center Panel - Graph or Spine View */}
      <div className="flex-1 min-w-0 flex flex-col">
        {/* Toolbar */}
        <div className="flex-shrink-0 h-12 px-4 bg-[#0d1117] border-b border-gray-800/50 flex items-center justify-between">
          {/* Left side - zoom controls */}
          <div className="flex items-center gap-1">
            <button className="p-1.5 text-gray-500 hover:text-gray-300 hover:bg-[#161b22] rounded transition-colors">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM10 7v3m0 0v3m0-3h3m-3 0H7" />
              </svg>
            </button>
            <button className="p-1.5 text-gray-500 hover:text-gray-300 hover:bg-[#161b22] rounded transition-colors">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0zM13 10H7" />
              </svg>
            </button>
            <button className="p-1.5 text-gray-500 hover:text-gray-300 hover:bg-[#161b22] rounded transition-colors">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 8V4m0 0h4M4 4l5 5m11-1V4m0 0h-4m4 0l-5 5M4 16v4m0 0h4m-4 0l5-5m11 5l-5-5m5 5v-4m0 4h-4" />
              </svg>
            </button>
          </div>

          {/* Center - View Mode Toggle */}
          <div className="flex items-center gap-1 bg-[#161b22] rounded-lg p-0.5">
            <button
              onClick={() => setViewMode('spine')}
              className={`px-4 py-1.5 text-xs font-medium rounded-md transition-colors ${
                viewMode === 'spine'
                  ? 'bg-[#0d1117] text-white shadow'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              Call Spine
            </button>
            <button
              onClick={() => setViewMode('graph')}
              className={`px-4 py-1.5 text-xs font-medium rounded-md transition-colors ${
                viewMode === 'graph'
                  ? 'bg-[#0d1117] text-white shadow'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              Full Graph
            </button>
          </div>

          {/* Right side - export options */}
          <div className="flex items-center gap-1">
            <button
              onClick={handleCopyLink}
              className="p-1.5 text-gray-500 hover:text-gray-300 hover:bg-[#161b22] rounded transition-colors"
              title="Copy link"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
              </svg>
            </button>
            <button
              onClick={handleExportSVG}
              className="p-1.5 text-gray-500 hover:text-gray-300 hover:bg-[#161b22] rounded transition-colors"
              title="Export SVG"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
              </svg>
            </button>
          </div>
        </div>

        {/* View Content */}
        <div className="flex-1 min-h-0">
          {viewMode === 'spine' ? (
            <CallSpineView
              rootId={selectedEntrypoint?.symbol_id ?? null}
              filters={filters}
              selectedNodeId={selectedNode?.id ?? null}
              onNodeClick={handleNodeClick}
              onNodeExpand={(_nodeId: number, expandedNodes: SpineNode[]) => {
                const newGraphNodes: GraphNode[] = expandedNodes.map((n) => ({
                  id: n.id,
                  name: n.name,
                  pkg_path: n.pkg_path,
                  file: n.file,
                  line: n.line,
                  kind: 'func' as const,
                  recv_type: n.recv_type,
                  tags: n.tags,
                  expanded: false,
                  depth: n.depth,
                }));
                setGraphNodes((prev) => {
                  const existingIds = new Set(prev.map((n) => n.id));
                  const uniqueNew = newGraphNodes.filter((n) => !existingIds.has(n.id));
                  return [...prev, ...uniqueNew];
                });
              }}
              onSpineNodesUpdate={handleSpineNodesUpdate}
            />
          ) : (
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
                breadcrumbs={breadcrumbs}
                onBreadcrumbNavigate={handleBreadcrumbNavigate}
                onClearFocus={handleClearFocus}
                onCopyLink={handleCopyLink}
                onExportSVG={handleExportSVG}
                onExportPNG={handleExportPNG}
              />
            </ReactFlowProvider>
          )}
        </div>
      </div>

      {/* Right Panel - Inspector */}
      <div className="w-[300px] flex-shrink-0 border-l border-gray-800/50">
        <InspectorPanel
          selectedNode={selectedNode}
          filters={filters}
          onFiltersChange={handleFiltersChange}
          onNavigateToNode={handleNavigateToNode}
          graphNodes={graphNodes}
          spineNodes={spineNodes}
          onShowCFG={handleShowCFG}
        />
      </div>

      {/* CFG Modal */}
      {cfgSymbolId && (
        <ReactFlowProvider>
          <CFGPanel
            symbolId={cfgSymbolId}
            symbolName={cfgSymbolName}
            onClose={handleCloseCFG}
            onFollowCall={handleCFGFollowCall}
          />
        </ReactFlowProvider>
      )}
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
