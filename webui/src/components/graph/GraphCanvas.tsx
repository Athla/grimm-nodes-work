import { useCallback, useMemo, useEffect, useRef, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  useReactFlow,
  ReactFlowProvider,
  type Node,
  type Edge,
  type NodeTypes,
  type EdgeTypes,
  BackgroundVariant,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import CustomNode, { type CustomNodeData } from './CustomNode';
import CustomEdge, { type CustomEdgeData } from './CustomEdge';
import { EmptyState } from '../ui';
import type { Graph, GraphEdge } from '../../types';
import { calculatePriority, countConnections, calculateHierarchicalLayout, calculateForceDirectedLayout, debounce } from '../../utils';
import { useLayoutPersistence } from '../../hooks';
import type { LayoutMode } from '../HeaderBar';
import styles from './GraphCanvas.module.css';

const nodeTypes: NodeTypes = {
  custom: CustomNode,
};

const edgeTypes: EdgeTypes = {
  custom: CustomEdge,
};

interface GraphCanvasProps {
  graph: Graph | undefined;
  selectedNodeId: string | null;
  onNodeSelect: (nodeId: string | null) => void;
  onEdgeClick?: (edge: GraphEdge) => void;
  layoutMode: LayoutMode;
  resetKey?: number;
  isLoading?: boolean;
  error?: Error | null;
  onRetry?: () => void;
}

function getConnectedNodeIds(nodeId: string, edges: GraphEdge[]) {
  const sources = new Set<string>();
  const targets = new Set<string>();

  edges.forEach(edge => {
    if (edge.source === nodeId) targets.add(edge.target);
    if (edge.target === nodeId) sources.add(edge.source);
  });

  const all = new Set([...sources, ...targets]);
  return { sources, targets, all };
}

function computeLayout(
  graph: Graph,
  layoutMode: LayoutMode,
  savedPositions: Map<string, { x: number; y: number; isPinned?: boolean }>,
  viewportSize: { width: number; height: number }
): {
  nodes: Node<CustomNodeData>[];
  edges: Edge<CustomEdgeData>[];
} {
  // Calculate base layout positions (expensive)
  const basePositions = layoutMode === 'hierarchical'
    ? calculateHierarchicalLayout(graph)
    : calculateForceDirectedLayout(graph, viewportSize);

  // Merge with saved positions (pinned nodes override calculated positions)
  const finalPositions = new Map(basePositions);
  savedPositions.forEach((pos, nodeId) => {
    if (pos.isPinned && basePositions.has(nodeId)) {
      finalPositions.set(nodeId, { x: pos.x, y: pos.y });
    }
  });

  const nodes: Node<CustomNodeData>[] = graph.nodes.map(node => {
    const connectionCount = countConnections(node.id, graph.edges);
    const priority = calculatePriority(node, connectionCount);
    const position = finalPositions.get(node.id) || { x: 0, y: 0 };
    const savedPos = savedPositions.get(node.id);

    return {
      id: node.id,
      type: 'custom',
      position,
      data: {
        name: node.name,
        type: node.type,
        health: node.health,
        priority,
        connectionCount,
        isConnected: undefined,
        isSource: false,
        isTarget: false,
        isPinned: savedPos?.isPinned || false,
      },
    };
  });

  const edges: Edge<CustomEdgeData>[] = graph.edges.map(edge => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: 'custom',
    data: {
      label: edge.label,
      isActive: undefined,
    },
  }));

  return { nodes, edges };
}

function applyHighlighting(
  layoutNodes: Node<CustomNodeData>[],
  layoutEdges: Edge<CustomEdgeData>[],
  selectedNodeId: string | null,
  graphEdges: GraphEdge[]
): {
  nodes: Node<CustomNodeData>[];
  edges: Edge<CustomEdgeData>[];
} {
  if (!selectedNodeId) return { nodes: layoutNodes, edges: layoutEdges };

  const connectedInfo = getConnectedNodeIds(selectedNodeId, graphEdges);

  const nodes = layoutNodes.map(node => {
    if (node.id === selectedNodeId) return node;

    return {
      ...node,
      data: {
        ...node.data,
        isConnected: connectedInfo.all.has(node.id),
        isSource: connectedInfo.sources.has(node.id),
        isTarget: connectedInfo.targets.has(node.id),
      },
    };
  });

  const edges = layoutEdges.map(edge => ({
    ...edge,
    data: {
      ...edge.data,
      isActive: edge.source === selectedNodeId || edge.target === selectedNodeId
        ? true
        : false,
    },
  }));

  return { nodes, edges };
}

function GraphCanvasInner({
  graph,
  selectedNodeId,
  onNodeSelect,
  onEdgeClick,
  layoutMode,
  resetKey,
  isLoading,
  error,
  onRetry,
}: GraphCanvasProps) {
  const { fitView } = useReactFlow();
  const prevNodeCountRef = useRef(0);
  const canvasRef = useRef<HTMLDivElement>(null);
  const { savedPositions, savePosition, clearLayout } = useLayoutPersistence(graph, layoutMode);
  const [isTransitioning, setIsTransitioning] = useState(false);
  const [justSavedNodeId, setJustSavedNodeId] = useState<string | null>(null);
  const prevLayoutModeRef = useRef(layoutMode);
  const transitionTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Dynamic viewport via ResizeObserver
  const [viewportSize, setViewportSize] = useState({ width: 1200, height: 800 });
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const ro = new ResizeObserver(entries => {
      const entry = entries[0];
      if (entry) {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0) {
          setViewportSize({ width, height });
        }
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Layout transition animation — subscribe to layoutMode changes via effect
  useEffect(() => {
    if (prevLayoutModeRef.current === layoutMode) return;
    prevLayoutModeRef.current = layoutMode;

    setIsTransitioning(true);
    clearTimeout(transitionTimerRef.current);
    transitionTimerRef.current = setTimeout(() => {
      setIsTransitioning(false);
      fitView({ padding: 0.15, duration: 500 });
    }, 100);
    return () => clearTimeout(transitionTimerRef.current);
  }, [layoutMode, fitView]);

  // Memo 1: Expensive layout computation — NOT dependent on selectedNodeId
  const { layoutNodes, layoutEdges } = useMemo(() => {
    if (!graph?.nodes) return { layoutNodes: [], layoutEdges: [] };
    const { nodes, edges } = computeLayout(graph, layoutMode, savedPositions, viewportSize);
    return { layoutNodes: nodes, layoutEdges: edges };
  }, [graph, layoutMode, savedPositions, viewportSize]);

  // Memo 2: Cheap highlighting — runs on node click, no layout recomputation
  const { flowNodes, flowEdges } = useMemo(() => {
    if (layoutNodes.length === 0) return { flowNodes: [], flowEdges: [] };
    const { nodes, edges } = applyHighlighting(layoutNodes, layoutEdges, selectedNodeId, graph?.edges ?? []);
    return { flowNodes: nodes, flowEdges: edges };
  }, [layoutNodes, layoutEdges, selectedNodeId, graph?.edges]);

  const [nodes, setNodes, onNodesChange] = useNodesState(flowNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(flowEdges);

  useEffect(() => {
    setNodes(flowNodes);
    setEdges(flowEdges);
  }, [flowNodes, flowEdges, setNodes, setEdges]);

  useEffect(() => {
    const count = graph?.nodes?.length ?? 0;
    if (count > 0 && count !== prevNodeCountRef.current) {
      prevNodeCountRef.current = count;
      setTimeout(() => fitView({ padding: 0.15, duration: 500 }), 100);
    }
  }, [graph?.nodes?.length, fitView]);

  // Reset positions when resetKey changes (triggered by "Reset Positions" button)
  const prevResetKeyRef = useRef(resetKey);
  useEffect(() => {
    if (resetKey === undefined || resetKey === prevResetKeyRef.current) return;
    prevResetKeyRef.current = resetKey;
    clearLayout();
    setTimeout(() => fitView({ padding: 0.15, duration: 500 }), 50);
  }, [resetKey, clearLayout, fitView]);

  const nodesWithSelection = useMemo(() => {
    return nodes.map(node => ({
      ...node,
      selected: node.id === selectedNodeId,
      data: {
        ...node.data,
        justSaved: node.id === justSavedNodeId,
      },
    }));
  }, [nodes, selectedNodeId, justSavedNodeId]);

  const handleNodeClick = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      onNodeSelect(node.id);
    },
    [onNodeSelect]
  );

  const handleEdgeClick = useCallback(
    (_event: React.MouseEvent, edge: Edge) => {
      if (!onEdgeClick || !graph) return;
      const graphEdge = graph.edges.find(e => e.id === edge.id);
      if (graphEdge) onEdgeClick(graphEdge);
    },
    [onEdgeClick, graph]
  );

  const handlePaneClick = useCallback(() => {
    onNodeSelect(null);
  }, [onNodeSelect]);

  // Debounced save position handler
  const debouncedSavePosition = useMemo(
    () => debounce((...args: unknown[]) => {
      const [nodeId, position] = args as [string, { x: number; y: number }];
      savePosition(nodeId, position, true);
    }, 500),
    [savePosition]
  );

  const handleNodeDragStop = useCallback(
    (_event: React.MouseEvent, node: Node) => {
      debouncedSavePosition(node.id, node.position);
      setJustSavedNodeId(node.id);
      setTimeout(() => setJustSavedNodeId(prev => prev === node.id ? null : prev), 600);
    },
    [debouncedSavePosition]
  );

  if (error) {
    return (
      <div className={styles.loading}>
        <div className={styles.loadingContent}>
          <div className={styles.errorIcon}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="12" cy="12" r="10" />
              <path d="M12 8v4m0 4h.01" />
            </svg>
          </div>
          <span className={styles.loadingText}>Failed to load graph</span>
          <span className={styles.errorDetail}>{error.message}</span>
          {onRetry && (
            <button className={styles.retryBtn} onClick={onRetry}>
              Try again
            </button>
          )}
        </div>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className={styles.skeleton}>
        <div className={styles.skeletonNode} style={{ top: '20%', left: '15%' }} />
        <div className={styles.skeletonNode} style={{ top: '35%', left: '45%' }} />
        <div className={styles.skeletonNode} style={{ top: '15%', left: '65%' }} />
        <div className={styles.skeletonNode} style={{ top: '55%', left: '25%' }} />
        <div className={styles.skeletonNode} style={{ top: '50%', left: '70%' }} />
        <div className={styles.skeletonNode} style={{ top: '70%', left: '50%' }} />
      </div>
    );
  }

  if (graph && graph.nodes?.length === 0) {
    return <EmptyState />;
  }

  return (
    <div ref={canvasRef} className={`${styles.canvas} ${isTransitioning ? styles.transitioning : ''}`}>
      <ReactFlow
        nodes={nodesWithSelection}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={handleNodeClick}
        onEdgeClick={handleEdgeClick}
        onPaneClick={handlePaneClick}
        onNodeDragStop={handleNodeDragStop}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        fitViewOptions={{ padding: 0.15 }}
        minZoom={0.2}
        maxZoom={2.5}
        proOptions={{ hideAttribution: true }}
        defaultEdgeOptions={{ type: 'custom' }}
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={28}
          size={1.2}
          color="rgba(255, 255, 255, 0.06)"
        />
        <Controls
          className={styles.controls}
          showInteractive={false}
          position="bottom-right"
        />
        <MiniMap
          className={styles.minimap}
          nodeColor="#2a2a2a"
          nodeStrokeColor="rgba(255, 255, 255, 0.15)"
          nodeBorderRadius={4}
          maskColor="rgba(0, 0, 0, 0.7)"
          bgColor="#111111"
          position="bottom-left"
          pannable
          zoomable
        />
      </ReactFlow>
    </div>
  );
}

export default function GraphCanvas(props: GraphCanvasProps) {
  return (
    <ReactFlowProvider>
      <GraphCanvasInner {...props} />
    </ReactFlowProvider>
  );
}
