export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy' | 'unknown';

export type NodeType =
  | 'database'
  | 'bucket'
  | 'service'
  | 'table'
  | 'collection'
  | 'api'
  | 'queue'
  | 'cache'
  | 'storage'
  | 'gateway'
  | 'payment'
  | 'auth'
  | 'postgres'
  | 'mongodb'
  | 's3'
  | 'redis'
  | 'http'
  | 'namespace'
  | 'deployment'
  | 'statefulset'
  | 'daemonset'
  | 'pod'
  | 'k8s_service';

// Priority tiers for visual hierarchy
export type PriorityTier = 'critical' | 'high' | 'medium' | 'low';

export interface NodeMetadata {
  tables?: string[];
  collections?: string[];
  endpoints?: string[];
  size?: string;
  region?: string;
  version?: string;
  priority?: PriorityTier;
  isExpanded?: boolean; // For collapsible containers
  isPinned?: boolean; // For position locking
  [key: string]: unknown;
}

export interface GraphNode {
  id: string;
  type: NodeType;
  name: string;
  parent?: string;
  metadata: NodeMetadata;
  health: HealthStatus;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  type: string;
  label?: string;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}
