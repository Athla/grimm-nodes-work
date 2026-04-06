import type { GraphNode, GraphEdge, PriorityTier } from '../types';

export function cn(...classes: (string | boolean | undefined | null)[]): string {
  return classes.filter(Boolean).join(' ');
}

export function debounce<T extends (...args: unknown[]) => unknown>(
  fn: T,
  delay: number
): (...args: Parameters<T>) => void {
  let timeoutId: ReturnType<typeof setTimeout>;

  return (...args: Parameters<T>) => {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn(...args), delay);
  };
}

export function formatNodeType(type: string): string {
  return type
    .replace(/([A-Z])/g, ' $1')
    .replace(/[_-]/g, ' ')
    .replace(/^\w/, (c) => c.toUpperCase())
    .trim();
}

export function calculatePriority(node: GraphNode, connectionCount: number): PriorityTier {
  if (node.metadata.priority) {
    return node.metadata.priority;
  }
  if (['payment', 'auth', 'gateway'].includes(node.type)) {
    return 'critical';
  }
  if (node.type === 'database' || node.type === 'api' || connectionCount >= 4) {
    return 'high';
  }
  if (['service', 'cache', 'queue'].includes(node.type) || connectionCount >= 2) {
    return 'medium';
  }
  return 'low';
}

export function countConnections(nodeId: string, edges: GraphEdge[]): number {
  return edges.filter(e => e.source === nodeId || e.target === nodeId).length;
}

export * from './graphTransform';
export * from './forceDirectedLayout';
export * from './swimlaneLayout';
