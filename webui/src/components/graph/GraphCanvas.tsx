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
import { EdgeMarkerDefs } from './EdgeMarkerDefs';
import { EmptyState } from '../ui';
import type { Graph, GraphEdge } from '../../types';
import { calculatePriority, countConnections, calculateHierarchicalLayout, calculateForceDirectedLayout, calculateSwimlaneLayout, debounce } from '../../utils';
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

// Build a map of namespace ids for each node (used for cross-ns edge detection).
function buildNodeNamespaceMap(graph: Graph): Map<string, string> {
  const nsMap = new Map<string, string>();
  for (const node of graph.nodes) {
    if (node.type === 'namespace') {
      nsMap.set(node.id, node.id);
    } else if (node.parent) {
      nsMap.set(node.id, node.parent);
    }
  }
  return nsMap;
}

// Count how many routes_to edges originate from a node (for service "→ N pods" count).
function countRoutesToTargets(nodeId: string, edges: Graph['edges']): number {
  return edges.filter(e => e.source === nodeId && e.type === 'routes_to').length;
}

// Count children in a namespace by type.
function countChildrenByType(nsId: string, nodes: Graph['nodes'], types: Set<string>): number {
  return nodes.filter(n => n.parent === nsId && types.has(n.type)).length;
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
  const isSwimLane = layoutMode === 'swimlane';

  // ── Swimlane layout ─────────────────────────────────────────────────
  if (isSwimLane) {
    const { positions: swimPositions, laneBounds } = calculateSwimlaneLayout(graph);
    const nodeNsMap = buildNodeNamespaceMap(graph);

    // Merge with saved pinned positions.
    const finalPositions = new Map(swimPositions);
    savedPositions.forEach((pos, nodeId) => {
      if (pos.isPinned && swimPositions.has(nodeId)) {
        finalPositions.set(nodeId, { x: pos.x, y: pos.y });
      }
    });

    // Compute per-node enriched data.
    const workloadTypes = new Set(['deployment', 'statefulset', 'daemonset']);
    const podTypes = new Set(['pod']);

    const nodes: Node<CustomNodeData>[] = graph.nodes.map(node => {
      const connectionCount = countConnections(node.id, graph.edges);
      const priority = calculatePriority(node, connectionCount);
      const savedPos = savedPositions.get(node.id);
      const position = finalPositions.get(node.id) || { x: 0, y: 0 };
      const isGroup = node.type === 'namespace';

      // Enriched metadata for per-type rendering.
      const metadata = node.metadata ?? {};

      // Namespace lane header: child counts.
      let workloadCount: number | undefined;
      let podCount: number | undefined;
      let routesToCount: number | undefined;

      if (isGroup) {
        workloadCount = countChildrenByType(node.id, graph.nodes, workloadTypes);
        podCount = countChildrenByType(node.id, graph.nodes, podTypes);
      }

      // Service: routes_to count.
      if (node.type === 'k8s_service') {
        routesToCount = countRoutesToTargets(node.id, graph.edges);
      }

      const rfNode: Node<CustomNodeData> = {
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
          isGroup,
          isSwimlane: true,
          // K8s enriched data
          metadata,
          workloadCount,
          podCount,
          routesToCount,
        },
      };

      // Namespace nodes in swimlane get lane dimensions as style.
      if (isGroup) {
        const bounds = laneBounds.get(node.id);
        if (bounds) {
          rfNode.style = { width: bounds.w, height: bounds.h };
        }
      }

      return rfNode;
    });

    // Namespace (group) nodes first for React Flow z-order (they render behind).
    nodes.sort((a, b) => {
      const ag = a.data.isGroup ? 0 : 1;
      const bg = b.data.isGroup ? 0 : 1;
      return ag - bg;
    });

    const edges: Edge<CustomEdgeData>[] = graph.edges.map(edge => {
      const sourceNs = nodeNsMap.get(edge.source);
      const targetNs = nodeNsMap.get(edge.target);
      const isCrossNamespace = sourceNs !== targetNs && sourceNs !== undefined && targetNs !== undefined;

      return {
        id: edge.id,
        source: edge.source,
        target: edge.target,
        type: 'custom',
        data: {
          label: edge.label,
          edgeType: edge.type,
          isActive: undefined,
          isCrossNamespace,
        },
      };
    });

    return { nodes, edges };
  }

  // ── Hierarchical / Force layout (existing) ──────────────────────────
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

  // Identify namespace groups (nodes whose metadata.group === true). For each
  // group, collect children via the parent pointer, compute their bounding
  // box, and lay the group out as a React Flow container.
  const GROUP_PAD = 24;
  const GROUP_HEADER_H = 36;
  const CHILD_W = 220;
  const CHILD_H = 64;
  const groupChildren = new Map<string, string[]>();
  for (const n of graph.nodes) {
    if (!n.parent) continue;
    if (!groupChildren.has(n.parent)) groupChildren.set(n.parent, []);
    groupChildren.get(n.parent)!.push(n.id);
  }
  const groupBounds = new Map<string, { x: number; y: number; w: number; h: number }>();
  for (const n of graph.nodes) {
    if (!n.metadata?.group) continue;
    const childIds = groupChildren.get(n.id) ?? [];
    if (childIds.length === 0) continue;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    for (const cid of childIds) {
      const p = finalPositions.get(cid);
      if (!p) continue;
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + CHILD_W);
      maxY = Math.max(maxY, p.y + CHILD_H);
    }
    if (!isFinite(minX)) continue;
    groupBounds.set(n.id, {
      x: minX - GROUP_PAD,
      y: minY - GROUP_PAD - GROUP_HEADER_H,
      w: maxX - minX + 2 * GROUP_PAD,
      h: maxY - minY + 2 * GROUP_PAD + GROUP_HEADER_H,
    });
  }

  const nodes: Node<CustomNodeData>[] = graph.nodes.map(node => {
    const connectionCount = countConnections(node.id, graph.edges);
    const priority = calculatePriority(node, connectionCount);
    const savedPos = savedPositions.get(node.id);
    const isGroup = Boolean(node.metadata?.group);
    const groupBox = isGroup ? groupBounds.get(node.id) : undefined;

    let position = finalPositions.get(node.id) || { x: 0, y: 0 };
    let parentId: string | undefined;
    let extent: 'parent' | undefined;

    if (isGroup && groupBox) {
      position = { x: groupBox.x, y: groupBox.y };
    } else if (node.parent) {
      const parentBox = groupBounds.get(node.parent);
      if (parentBox) {
        position = { x: position.x - parentBox.x, y: position.y - parentBox.y };
        parentId = node.parent;
        extent = 'parent';
      }
    }

    const rfNode: Node<CustomNodeData> = {
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
        isGroup,
        metadata: node.metadata,
      },
    };
    if (isGroup && groupBox) {
      rfNode.style = { width: groupBox.w, height: groupBox.h };
    }
    if (parentId) {
      rfNode.parentId = parentId;
      rfNode.extent = extent;
    }
    return rfNode;
  });

  // Group nodes must precede their children in the array (React Flow requirement).
  nodes.sort((a, b) => {
    const ag = a.data.isGroup ? 0 : 1;
    const bg = b.data.isGroup ? 0 : 1;
    return ag - bg;
  });

  const edges: Edge<CustomEdgeData>[] = graph.edges.map(edge => ({
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: 'custom',
    data: {
      label: edge.label,
      edgeType: edge.type,
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
        <EdgeMarkerDefs />
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
