import { useCallback, useEffect, useState } from 'react';
import type { Graph } from '../types';

interface Position {
  x: number;
  y: number;
  isPinned?: boolean;
}

interface LayoutStorage {
  [graphHash: string]: {
    [nodeId: string]: Position;
  };
}

const STORAGE_KEY = 'graph-layout-positions';
const MAX_LAYOUTS = 50;

// Calculate stable hash from node IDs and layout mode
function calculateGraphHash(graph: Graph | undefined, layoutMode?: string): string {
  if (!graph?.nodes) return '';
  const sortedIds = graph.nodes.map(n => n.id).sort();
  const base = JSON.stringify(sortedIds);
  return layoutMode ? `${base}:${layoutMode}` : base;
}

// Load positions from localStorage
function loadPositions(graphHash: string): Map<string, Position> {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return new Map();

    const data: LayoutStorage = JSON.parse(stored);
    const positions = data[graphHash] || {};

    return new Map(Object.entries(positions));
  } catch (error) {
    console.warn('Failed to load layout positions:', error);
    return new Map();
  }
}

// Save positions to localStorage with LRU cleanup
function savePositions(graphHash: string, positions: Map<string, Position>) {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    const data: LayoutStorage = stored ? JSON.parse(stored) : {};

    // Update positions for current graph
    data[graphHash] = Object.fromEntries(positions);

    // Cleanup: keep only MAX_LAYOUTS most recent
    const keys = Object.keys(data);
    if (keys.length > MAX_LAYOUTS) {
      // Remove oldest entries (simple LRU: remove excess)
      const toRemove = keys.length - MAX_LAYOUTS;
      keys.slice(0, toRemove).forEach(key => delete data[key]);
    }

    localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
  } catch (error) {
    console.warn('Failed to save layout positions:', error);
  }
}

export function useLayoutPersistence(graph: Graph | undefined, layoutMode?: string) {
  const [graphHash, setGraphHash] = useState<string>('');
  const [savedPositions, setSavedPositions] = useState<Map<string, Position>>(new Map());

  // Calculate hash and load positions when graph or layout mode changes
  useEffect(() => {
    const hash = calculateGraphHash(graph, layoutMode);
    if (hash && hash !== graphHash) {
      setGraphHash(hash);
      const positions = loadPositions(hash);
      setSavedPositions(positions);
    }
  }, [graph, graphHash, layoutMode]);

  // Save a single position (debounced externally)
  const savePosition = useCallback((nodeId: string, position: { x: number; y: number }, isPinned: boolean = false) => {
    if (!graphHash) return;

    setSavedPositions(prev => {
      const updated = new Map(prev);
      updated.set(nodeId, { ...position, isPinned });
      savePositions(graphHash, updated);
      return updated;
    });
  }, [graphHash]);

  // Clear all saved positions for current graph
  const clearLayout = useCallback(() => {
    if (!graphHash) return;

    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const data: LayoutStorage = JSON.parse(stored);
        delete data[graphHash];
        localStorage.setItem(STORAGE_KEY, JSON.stringify(data));
      }
      setSavedPositions(new Map());
    } catch (error) {
      console.warn('Failed to clear layout:', error);
    }
  }, [graphHash]);

  return {
    savedPositions,
    savePosition,
    clearLayout,
  };
}
