import {
  forceSimulation,
  forceLink,
  forceManyBody,
  forceCenter,
  forceCollide,
  type SimulationNodeDatum,
  type SimulationLinkDatum,
} from 'd3-force';
import type { Graph } from '../types';

interface ForceNode extends SimulationNodeDatum {
  id: string;
  x?: number;
  y?: number;
}

interface ForceLink extends SimulationLinkDatum<ForceNode> {
  source: string;
  target: string;
}

export function calculateForceDirectedLayout(
  graph: Graph,
  containerSize: { width: number; height: number }
): Map<string, { x: number; y: number }> {
  if (!graph.nodes.length) return new Map();

  const { width, height } = containerSize;

  // Prepare nodes for d3-force
  const nodes: ForceNode[] = graph.nodes.map(node => ({
    id: node.id,
  }));

  // Prepare links for d3-force
  const links: ForceLink[] = graph.edges.map(edge => ({
    source: edge.source,
    target: edge.target,
  }));

  // Create simulation
  const simulation = forceSimulation(nodes)
    .force(
      'link',
      forceLink<ForceNode, ForceLink>(links)
        .id(d => d.id)
        .distance(180)
        .strength(0.5)
    )
    .force('charge', forceManyBody().strength(-400))
    .force('center', forceCenter(width / 2, height / 2))
    .force('collide', forceCollide().radius(150).strength(0.7))
    .stop();

  // Run simulation for 300 iterations to stabilize
  for (let i = 0; i < 300; i++) {
    simulation.tick();
  }

  // Extract final positions
  const positions = new Map<string, { x: number; y: number }>();
  nodes.forEach(node => {
    if (node.x !== undefined && node.y !== undefined) {
      // Center positions around (0, 0) for React Flow
      positions.set(node.id, {
        x: node.x - width / 2,
        y: node.y - height / 2,
      });
    }
  });

  return positions;
}
