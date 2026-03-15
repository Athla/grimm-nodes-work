import { useState, useCallback, useMemo, useEffect } from 'react';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { GraphCanvas } from './graph';
import { NodeInspector, SearchOverlay, EdgeInspector } from './panels';
import HeaderBar, { type Filters, type LayoutMode } from './HeaderBar';
import { ErrorBoundary, EmptyState } from './ui';
import { useWebSocket, useAppShortcuts } from '../hooks';
import { useGraph as useGraphData } from '../api';
import { MOCK_GRAPH } from '../data';
import type { Graph, GraphEdge } from '../types';
import styles from './Layout.module.css';

export default function Layout() {
  const { nodeId: urlNodeId } = useParams<{ nodeId: string }>();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(urlNodeId ?? null);
  const [selectedEdge, setSelectedEdge] = useState<GraphEdge | null>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [filters, setFilters] = useState<Filters>(() => {
    const typesParam = searchParams.get('types');
    const healthParam = searchParams.get('health');
    return {
      types: typesParam ? typesParam.split(',').filter(Boolean) as Filters['types'] : [],
      health: healthParam ? healthParam.split(',').filter(Boolean) as Filters['health'] : [],
    };
  });
  const [layoutMode, setLayoutMode] = useState<LayoutMode>(
    () => (localStorage.getItem('graph-layout-mode') as LayoutMode) || 'hierarchical'
  );
  const [layoutResetKey, setLayoutResetKey] = useState(0);

  // Persist layout mode to localStorage
  const handleLayoutChange = useCallback((mode: LayoutMode) => {
    setLayoutMode(mode);
    localStorage.setItem('graph-layout-mode', mode);
  }, []);

  const handleResetPositions = useCallback(() => {
    setLayoutResetKey(k => k + 1);
  }, []);

  const handleFilterChange = useCallback((newFilters: Filters) => {
    setFilters(newFilters);
    const params = new URLSearchParams(searchParams);
    if (newFilters.types.length > 0) {
      params.set('types', newFilters.types.join(','));
    } else {
      params.delete('types');
    }
    if (newFilters.health.length > 0) {
      params.set('health', newFilters.health.join(','));
    } else {
      params.delete('health');
    }
    setSearchParams(params, { replace: true });
  }, [searchParams, setSearchParams]);

  const { data: apiGraph, isLoading, error, refetch } = useGraphData();
  const isMockData = !apiGraph?.nodes;
  const graph: Graph = isMockData ? MOCK_GRAPH : apiGraph;

  const { status: wsStatus } = useWebSocket();

  useAppShortcuts({
    onSearch: useCallback(() => setSearchOpen(true), []),
    onEscape: useCallback(() => {
      if (searchOpen) {
        setSearchOpen(false);
      } else if (selectedEdge) {
        setSelectedEdge(null);
        const params = new URLSearchParams(searchParams);
        params.delete('edge');
        setSearchParams(params, { replace: true });
      } else if (selectedNodeId) {
        setSelectedNodeId(null);
        navigate('/', { replace: true });
      }
    }, [searchOpen, selectedEdge, selectedNodeId, navigate, searchParams, setSearchParams]),
  });

  const handleNodeSelect = useCallback((nodeId: string | null) => {
    setSelectedNodeId(nodeId);
    setSelectedEdge(null);
    if (nodeId) {
      navigate(`/node/${nodeId}`, { replace: true });
    } else {
      navigate('/', { replace: true });
    }
  }, [navigate]);

  const handleSearchSelect = useCallback((nodeId: string) => {
    setSelectedNodeId(nodeId);
    setSelectedEdge(null);
    setSearchOpen(false);
    navigate(`/node/${nodeId}`, { replace: true });
  }, [navigate]);

  const handleEdgeClick = useCallback((edge: GraphEdge) => {
    setSelectedEdge(edge);
    setSelectedNodeId(null);
    const params = new URLSearchParams(searchParams);
    params.set('edge', edge.id);
    setSearchParams(params, { replace: true });
  }, [searchParams, setSearchParams]);

  // Restore edge selection from URL on mount
  useEffect(() => {
    const edgeParam = searchParams.get('edge');
    if (edgeParam && graph?.edges && !selectedEdge) {
      const edge = graph.edges.find(e => e.id === edgeParam);
      if (edge) setSelectedEdge(edge);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [graph?.edges]);

  // Filter graph for the canvas while keeping unfiltered graph for NodeInspector
  const filteredGraph = useMemo((): Graph | undefined => {
    if (!graph?.nodes) return undefined;
    if (filters.types.length === 0 && filters.health.length === 0) return graph;

    const filteredNodes = graph.nodes.filter(node => {
      if (filters.types.length > 0 && !filters.types.includes(node.type)) return false;
      if (filters.health.length > 0 && !filters.health.includes(node.health)) return false;
      return true;
    });

    const nodeIds = new Set(filteredNodes.map(n => n.id));
    const filteredEdges = graph.edges.filter(
      edge => nodeIds.has(edge.source) && nodeIds.has(edge.target)
    );

    return { nodes: filteredNodes, edges: filteredEdges };
  }, [graph, filters]);

  const hasActiveFilters = filters.types.length > 0 || filters.health.length > 0;
  const isFilteredEmpty = hasActiveFilters && filteredGraph?.nodes?.length === 0 && (graph?.nodes?.length ?? 0) > 0;
  const activeFilterCount = filters.types.length + filters.health.length;

  const handleClearFilters = useCallback(() => {
    handleFilterChange({ types: [], health: [] });
  }, [handleFilterChange]);

  return (
    <div className={styles.layout}>
      <HeaderBar
        graph={graph}
        filteredGraph={filteredGraph}
        onSearchOpen={() => setSearchOpen(true)}
        filters={filters}
        onFilterChange={handleFilterChange}
        layoutMode={layoutMode}
        onLayoutChange={handleLayoutChange}
        onResetPositions={handleResetPositions}
        wsStatus={wsStatus}
      />

      {isMockData && !isLoading && (
        <div className={styles.mockBanner}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" className={styles.mockIcon}>
            <path d="M12 9v4m0 4h.01M12 2L2 22h20L12 2z" />
          </svg>
          <span>Displaying mock data — backend unavailable</span>
        </div>
      )}

      <div className={styles.graphArea}>
        <ErrorBoundary>
          {isFilteredEmpty ? (
            <EmptyState
              reason="filtered"
              filterCount={activeFilterCount}
              onClearFilters={handleClearFilters}
            />
          ) : (
            <GraphCanvas
              graph={filteredGraph}
              selectedNodeId={selectedNodeId}
              onNodeSelect={handleNodeSelect}
              onEdgeClick={handleEdgeClick}
              layoutMode={layoutMode}
              resetKey={layoutResetKey}
              isLoading={!apiGraph && isLoading}
              error={error instanceof Error ? error : error ? new Error(String(error)) : null}
              onRetry={() => refetch()}
            />
          )}
        </ErrorBoundary>
      </div>

      <NodeInspector
        nodeId={selectedNodeId}
        onClose={() => handleNodeSelect(null)}
        graph={graph}
        onNodeSelect={handleNodeSelect}
      />

      <EdgeInspector
        edge={selectedEdge}
        onClose={() => {
          setSelectedEdge(null);
          const params = new URLSearchParams(searchParams);
          params.delete('edge');
          setSearchParams(params, { replace: true });
        }}
        graph={graph}
        onNodeSelect={handleNodeSelect}
      />

      <SearchOverlay
        isOpen={searchOpen}
        onClose={() => setSearchOpen(false)}
        onNodeSelect={handleSearchSelect}
      />
    </div>
  );
}
