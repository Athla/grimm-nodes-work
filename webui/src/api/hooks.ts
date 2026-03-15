import { useCallback } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from './client';
import type { Graph, HealthStatus } from '../types';

// Query keys for cache management
export const queryKeys = {
  graph: ['graph'] as const,
  node: (id: string) => ['node', id] as const,
  health: ['health'] as const,
};

export function useGraph() {
  return useQuery({
    queryKey: queryKeys.graph,
    queryFn: async () => {
      const response = await api.getGraph();
      return response.data;
    },
    staleTime: 30_000,
    retry: 3,
    refetchOnWindowFocus: false,
  });
}

export function useNode(id: string | null) {
  return useQuery({
    queryKey: queryKeys.node(id ?? ''),
    queryFn: async () => {
      if (!id) return null;
      const response = await api.getNode(id);
      return response.data;
    },
    enabled: !!id,
    staleTime: 30_000,
  });
}

export function useHealth() {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: api.getHealth,
    refetchInterval: 30_000, // Poll every 30s
    staleTime: 10_000,
  });
}

// Hook to update node health in cache (used by WebSocket)
export function useUpdateNodeHealth() {
  const queryClient = useQueryClient();

  return useCallback(
    (nodeId: string, health: HealthStatus) => {
      queryClient.setQueryData<Graph>(queryKeys.graph, (oldData) => {
        if (!oldData?.nodes) return oldData;

        return {
          ...oldData,
          nodes: oldData.nodes.map((node) =>
            node.id === nodeId ? { ...node, health } : node
          ),
        };
      });
    },
    [queryClient]
  );
}

// Hook to get a specific node from cached graph data
export function useNodeFromGraph(nodeId: string | null) {
  const { data: graph } = useGraph();

  if (!nodeId || !graph?.nodes) return null;

  return graph.nodes.find((node) => node.id === nodeId) ?? null;
}
