import type { Graph, GraphNode, NodeType } from '../types';

// ── Constants ───────────────────────────────────────────────────────────
const NODE_W = 200;
const NODE_H_SERVICE = 80;
const NODE_H_WORKLOAD = 80;
const NODE_H_POD = 72;
const LANE_GAP_Y = 32;
const LANE_PAD_X = 24;
const LANE_PAD_Y = 16;
const LANE_HEADER_H = 40;
const COLUMN_GAP = 48;
const ROW_GAP = 16;

// ── Zone classification for lane ordering ───────────────────────────────
const SYSTEM_NAMESPACES = new Set([
  'kube-system',
  'kube-public',
  'kube-node-lease',
]);

const INFRA_NAMESPACES = new Set([
  'ingress-nginx',
  'cert-manager',
  'local-path-storage',
  'metrics-server',
  'monitoring',
  'observability',
  'metallb-system',
  'kube-flannel',
  'calico-system',
  'tigera-operator',
]);

const K8S_SERVICE_TYPES: ReadonlySet<NodeType> = new Set<NodeType>([
  'k8s_service',
]);

const K8S_WORKLOAD_TYPES: ReadonlySet<NodeType> = new Set<NodeType>([
  'deployment',
  'statefulset',
  'daemonset',
]);

const K8S_POD_TYPES: ReadonlySet<NodeType> = new Set<NodeType>([
  'pod',
]);

type Zone = 'system' | 'user' | 'infra';

function classifyNamespace(name: string): Zone {
  if (SYSTEM_NAMESPACES.has(name)) return 'system';
  if (INFRA_NAMESPACES.has(name)) return 'infra';
  return 'user';
}

const ZONE_ORDER: Record<Zone, number> = { system: 0, user: 1, infra: 2 };

// ── Lane data structures ────────────────────────────────────────────────
interface Lane {
  namespaceId: string;
  namespaceName: string;
  zone: Zone;
  services: GraphNode[];
  workloads: GraphNode[];
  pods: GraphNode[];
}

// ── Exported types for consumers ────────────────────────────────────────
export interface SwimlanePositions {
  positions: Map<string, { x: number; y: number }>;
  laneBounds: Map<string, { x: number; y: number; w: number; h: number }>;
}

/**
 * Calculate deterministic swimlane layout for k8s graphs.
 *
 * Each namespace becomes a horizontal lane.
 * Lanes are sorted: system namespaces top, user namespaces middle (alpha),
 * infra namespaces bottom.
 *
 * Within each lane: three columns left-to-right:
 *   Services → Workloads → Pods
 *
 * Pods of the same workload cluster together vertically.
 */
export function calculateSwimlaneLayout(graph: Graph): SwimlanePositions {
  const positions = new Map<string, { x: number; y: number }>();
  const laneBounds = new Map<string, { x: number; y: number; w: number; h: number }>();

  if (!graph.nodes.length) return { positions, laneBounds };

  // ── Build lanes ───────────────────────────────────────────────────────
  const namespaceNodes = new Map<string, GraphNode>();
  const childrenByNs = new Map<string, GraphNode[]>();

  for (const node of graph.nodes) {
    if (node.type === 'namespace') {
      namespaceNodes.set(node.id, node);
      if (!childrenByNs.has(node.id)) childrenByNs.set(node.id, []);
    }
  }

  // Bucket children into their parent namespace.
  for (const node of graph.nodes) {
    if (node.type === 'namespace') continue;
    const parentId = node.parent;
    if (parentId && childrenByNs.has(parentId)) {
      childrenByNs.get(parentId)!.push(node);
    } else if (parentId) {
      // Parent namespace not in graph — create a placeholder lane.
      if (!childrenByNs.has(parentId)) childrenByNs.set(parentId, []);
      childrenByNs.get(parentId)!.push(node);
    }
    // Non-k8s nodes without parent are skipped (they shouldn't be in k8s view)
  }

  // Build ownership index: workload → pod names (for pod clustering).
  // We derive ownership from "contains" edges: workload -contains-> pod.
  const nodeById = new Map(graph.nodes.map(n => [n.id, n]));
  const podOwner = new Map<string, string>();
  for (const edge of graph.edges) {
    if (edge.type === 'contains') {
      // source = workload, target = pod (or source = namespace, target = workload)
      const targetNode = nodeById.get(edge.target);
      if (targetNode && targetNode.type === 'pod') {
        podOwner.set(edge.target, edge.source);
      }
    }
  }

  // Build sorted lanes.
  const lanes: Lane[] = [];
  for (const [nsId, children] of childrenByNs) {
    const nsNode = namespaceNodes.get(nsId);
    const nsName = nsNode?.name ?? nsId.replace(/^k8s-namespace-/, '');

    const services: GraphNode[] = [];
    const workloads: GraphNode[] = [];
    const pods: GraphNode[] = [];

    for (const child of children) {
      if (K8S_SERVICE_TYPES.has(child.type)) {
        services.push(child);
      } else if (K8S_WORKLOAD_TYPES.has(child.type)) {
        workloads.push(child);
      } else if (K8S_POD_TYPES.has(child.type)) {
        pods.push(child);
      }
      // Other types fall through — placed at end of workloads column.
    }

    // Sort workloads alphabetically.
    workloads.sort((a, b) => a.name.localeCompare(b.name));
    services.sort((a, b) => a.name.localeCompare(b.name));

    // Sort pods: cluster by owning workload, then alphabetically within cluster.
    const workloadOrder = new Map(workloads.map((w, i) => [w.id, i]));
    pods.sort((a, b) => {
      const ownerA = podOwner.get(a.id);
      const ownerB = podOwner.get(b.id);
      const orderA = ownerA !== undefined ? (workloadOrder.get(ownerA) ?? 999) : 999;
      const orderB = ownerB !== undefined ? (workloadOrder.get(ownerB) ?? 999) : 999;
      if (orderA !== orderB) return orderA - orderB;
      return a.name.localeCompare(b.name);
    });

    lanes.push({
      namespaceId: nsId,
      namespaceName: nsName,
      zone: classifyNamespace(nsName),
      services,
      workloads,
      pods,
    });
  }

  // Sort lanes: system → user (alpha) → infra (alpha).
  lanes.sort((a, b) => {
    const zoneA = ZONE_ORDER[a.zone];
    const zoneB = ZONE_ORDER[b.zone];
    if (zoneA !== zoneB) return zoneA - zoneB;
    return a.namespaceName.localeCompare(b.namespaceName);
  });

  // ── Position nodes in lanes ───────────────────────────────────────────
  let currentY = 0;

  for (const lane of lanes) {
    const laneStartY = currentY;

    // Namespace header position.
    const nsNode = namespaceNodes.get(lane.namespaceId);
    if (nsNode) {
      positions.set(nsNode.id, { x: 0, y: laneStartY });
    }

    const contentStartY = laneStartY + LANE_HEADER_H + LANE_PAD_Y;

    // Compute column x offsets. Empty columns collapse.
    let colX = LANE_PAD_X;
    const serviceColX = colX;

    if (lane.services.length > 0) {
      colX += NODE_W + COLUMN_GAP;
    }
    const workloadColX = colX;

    if (lane.workloads.length > 0) {
      colX += NODE_W + COLUMN_GAP;
    }
    const podColX = colX;

    // Place services.
    let serviceMaxY = contentStartY;
    for (let i = 0; i < lane.services.length; i++) {
      const y = contentStartY + i * (NODE_H_SERVICE + ROW_GAP);
      positions.set(lane.services[i].id, { x: serviceColX, y });
      serviceMaxY = y + NODE_H_SERVICE;
    }

    // Place workloads.
    let workloadMaxY = contentStartY;
    for (let i = 0; i < lane.workloads.length; i++) {
      const y = contentStartY + i * (NODE_H_WORKLOAD + ROW_GAP);
      positions.set(lane.workloads[i].id, { x: workloadColX, y });
      workloadMaxY = y + NODE_H_WORKLOAD;
    }

    // Place pods.
    let podMaxY = contentStartY;
    for (let i = 0; i < lane.pods.length; i++) {
      const y = contentStartY + i * (NODE_H_POD + ROW_GAP);
      positions.set(lane.pods[i].id, { x: podColX, y });
      podMaxY = y + NODE_H_POD;
    }

    // Lane height = max column height + padding.
    const contentMaxY = Math.max(serviceMaxY, workloadMaxY, podMaxY, contentStartY);
    const hasContent = lane.services.length + lane.workloads.length + lane.pods.length > 0;
    const laneH = hasContent
      ? (contentMaxY - laneStartY) + LANE_PAD_Y
      : LANE_HEADER_H + LANE_PAD_Y;

    // Lane width = rightmost column right edge + padding.
    let laneW: number;
    if (lane.pods.length > 0) {
      laneW = podColX + NODE_W + LANE_PAD_X;
    } else if (lane.workloads.length > 0) {
      laneW = workloadColX + NODE_W + LANE_PAD_X;
    } else if (lane.services.length > 0) {
      laneW = serviceColX + NODE_W + LANE_PAD_X;
    } else {
      laneW = LANE_PAD_X * 2 + NODE_W; // minimum width for empty lanes
    }

    laneBounds.set(lane.namespaceId, {
      x: 0,
      y: laneStartY,
      w: laneW,
      h: laneH,
    });

    currentY = laneStartY + laneH + LANE_GAP_Y;
  }

  return { positions, laneBounds };
}

/**
 * Returns true if the graph contains namespace nodes, indicating it's a k8s
 * graph that should default to swimlane layout.
 */
export function hasNamespaces(graph: Graph): boolean {
  return graph.nodes.some(n => n.type === 'namespace');
}
