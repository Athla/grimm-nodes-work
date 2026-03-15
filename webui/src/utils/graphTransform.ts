import type { Graph, GraphNode } from '../types';

interface HierarchyNode {
  node: GraphNode;
  children: HierarchyNode[];
}

export interface Hierarchy {
  roots: HierarchyNode[];
  parentMap: Map<string, string>;
  childrenMap: Map<string, GraphNode[]>;
}

// Detect parent-child hierarchy from graph
export function detectHierarchy(graph: Graph): Hierarchy {
  const parentMap = new Map<string, string>();
  const childrenMap = new Map<string, GraphNode[]>();

  // Build parent-child relationships
  graph.nodes.forEach(node => {
    if (node.parent) {
      parentMap.set(node.id, node.parent);

      if (!childrenMap.has(node.parent)) {
        childrenMap.set(node.parent, []);
      }
      childrenMap.get(node.parent)!.push(node);
    }
  });

  // Build tree structure for roots (nodes without parents or with parents not in graph)
  const nodeMap = new Map(graph.nodes.map(n => [n.id, n]));
  const roots: HierarchyNode[] = [];

  const treeVisited = new Set<string>();
  function buildTree(node: GraphNode): HierarchyNode {
    if (treeVisited.has(node.id)) {
      return { node, children: [] };
    }
    treeVisited.add(node.id);
    const children = childrenMap.get(node.id) || [];
    return {
      node,
      children: children.map(buildTree),
    };
  }

  graph.nodes.forEach(node => {
    // Root if no parent or parent not in graph
    if (!node.parent || !nodeMap.has(node.parent)) {
      roots.push(buildTree(node));
    }
  });

  return { roots, parentMap, childrenMap };
}

// Calculate hierarchical layout positions
export function calculateHierarchicalLayout(graph: Graph): Map<string, { x: number; y: number }> {
  const NODE_W = 220;
  const NODE_H = 64;
  const GAP_X = 60;
  const GAP_Y = 80;

  const nodeIds = new Set(graph.nodes.map(n => n.id));
  const children = new Map<string, string[]>();
  const parents = new Map<string, string[]>();

  for (const id of nodeIds) {
    children.set(id, []);
    parents.set(id, []);
  }

  for (const edge of graph.edges) {
    if (nodeIds.has(edge.source) && nodeIds.has(edge.target)) {
      children.get(edge.source)!.push(edge.target);
      parents.get(edge.target)!.push(edge.source);
    }
  }

  // Assign ranks using longest-path from roots (nodes with no incoming edges)
  const rank = new Map<string, number>();

  const visiting = new Set<string>();
  function assignRank(id: string): number {
    if (rank.has(id)) return rank.get(id)!;
    if (visiting.has(id)) return 0; // Break cycle
    visiting.add(id);
    const pars = parents.get(id)!;
    const r = pars.length === 0 ? 0 : Math.max(...pars.map(assignRank)) + 1;
    rank.set(id, r);
    visiting.delete(id);
    return r;
  }

  for (const id of nodeIds) assignRank(id);

  // Group nodes by rank
  const ranks = new Map<number, GraphNode[]>();
  for (const node of graph.nodes) {
    const r = rank.get(node.id) ?? 0;
    if (!ranks.has(r)) ranks.set(r, []);
    ranks.get(r)!.push(node);
  }

  // Position: center each rank row horizontally
  const positions = new Map<string, { x: number; y: number }>();
  const sortedRanks = [...ranks.keys()].sort((a, b) => a - b);

  for (const r of sortedRanks) {
    const nodesInRank = ranks.get(r)!;
    const totalWidth = nodesInRank.length * NODE_W + (nodesInRank.length - 1) * GAP_X;
    const startX = -totalWidth / 2;

    nodesInRank.forEach((node, i) => {
      positions.set(node.id, {
        x: startX + i * (NODE_W + GAP_X),
        y: r * (NODE_H + GAP_Y),
      });
    });
  }

  return positions;
}
