import { useMemo, useState, useRef, useEffect } from 'react';
import type { Graph, NodeType, HealthStatus } from '../types';
import styles from './HeaderBar.module.css';

export type NodeSource = 'docker' | 'kubernetes';

export interface Filters {
  types: NodeType[];
  health: HealthStatus[];
  sources: NodeSource[];
}

// K8s node types — used to infer source from node type.
export const K8S_TYPES: ReadonlySet<NodeType> = new Set<NodeType>([
  'namespace', 'deployment', 'statefulset', 'daemonset', 'pod', 'k8s_service',
]);

export function nodeSource(type: NodeType): NodeSource {
  return K8S_TYPES.has(type) ? 'kubernetes' : 'docker';
}

export type LayoutMode = 'hierarchical' | 'force' | 'swimlane';

type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error';

interface HeaderBarProps {
  graph?: Graph;
  filteredGraph?: Graph;
  onSearchOpen: () => void;
  filters: Filters;
  onFilterChange: (filters: Filters) => void;
  layoutMode: LayoutMode;
  onLayoutChange: (mode: LayoutMode) => void;
  onResetPositions: () => void;
  wsStatus?: ConnectionStatus;
}

const nodeTypes: NodeType[] = [
  'database', 'postgres', 'mongodb', 'redis', 'table', 'collection',
  'bucket', 's3', 'storage', 'service', 'api', 'http', 'gateway',
  'queue', 'cache', 'payment', 'auth',
  'namespace', 'deployment', 'statefulset', 'daemonset', 'pod', 'k8s_service',
];

const sourceOptions: { value: NodeSource; label: string }[] = [
  { value: 'docker', label: 'Docker' },
  { value: 'kubernetes', label: 'Kubernetes' },
];

const healthStatuses: HealthStatus[] = ['healthy', 'degraded', 'unhealthy'];

const isMac = /Mac|iPod|iPhone|iPad/.test(navigator.platform);

const wsStatusLabels: Record<ConnectionStatus, string> = {
  connected: 'Live connection active',
  connecting: 'Connecting...',
  disconnected: 'Disconnected',
  error: 'Connection error',
};

const LAYOUT_LABELS: Record<LayoutMode, string> = {
  swimlane: 'Swimlane (K8s)',
  hierarchical: 'Hierarchical',
  force: 'Force-Directed',
};

export default function HeaderBar({ graph, filteredGraph, onSearchOpen, filters, onFilterChange, layoutMode, onLayoutChange, onResetPositions, wsStatus }: HeaderBarProps) {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [mobileFiltersOpen, setMobileFiltersOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const mobileFilterRef = useRef<HTMLDivElement>(null);

  // Click outside to close dropdown
  useEffect(() => {
    if (!dropdownOpen && !mobileFiltersOpen) return;

    function handleClickOutside(event: MouseEvent) {
      if (dropdownOpen && dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setDropdownOpen(false);
      }
      if (mobileFiltersOpen && mobileFilterRef.current && !mobileFilterRef.current.contains(event.target as Node)) {
        setMobileFiltersOpen(false);
      }
    }

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [dropdownOpen, mobileFiltersOpen]);

  const graphHasNamespaces = useMemo(() => {
    return graph?.nodes?.some(n => n.type === 'namespace') ?? false;
  }, [graph]);

  const hasActiveFilters = filters.types.length > 0 || filters.health.length > 0 || filters.sources.length > 0;
  const activeFilterCount = filters.types.length + filters.health.length + filters.sources.length;

  const healthCounts = useMemo(() => {
    const sourceGraph = hasActiveFilters && filteredGraph ? filteredGraph : graph;
    if (!sourceGraph?.nodes) return { healthy: 0, degraded: 0, unhealthy: 0, unknown: 0, total: 0 };

    const counts = { healthy: 0, degraded: 0, unhealthy: 0, unknown: 0 };
    sourceGraph.nodes.forEach(node => {
      counts[node.health]++;
    });

    return { ...counts, total: sourceGraph.nodes.length };
  }, [graph, filteredGraph, hasActiveFilters]);

  // Only show type chips that exist in the current graph
  const activeTypes = useMemo(() => {
    if (!graph?.nodes) return [];
    const typesInGraph = new Set(graph.nodes.map(n => n.type));
    return nodeTypes.filter(t => typesInGraph.has(t));
  }, [graph]);

  const toggleType = (type: NodeType) => {
    const types = filters.types.includes(type)
      ? filters.types.filter(t => t !== type)
      : [...filters.types, type];
    onFilterChange({ ...filters, types });
  };

  const toggleHealth = (status: HealthStatus) => {
    const health = filters.health.includes(status)
      ? filters.health.filter(s => s !== status)
      : [...filters.health, status];
    onFilterChange({ ...filters, health });
  };

  const toggleSource = (source: NodeSource) => {
    const sources = filters.sources.includes(source)
      ? filters.sources.filter(s => s !== source)
      : [...filters.sources, source];
    onFilterChange({ ...filters, sources });
  };

  // Only show source chips whose nodes are actually present in the graph.
  const activeSources = useMemo(() => {
    if (!graph?.nodes) return [] as NodeSource[];
    const present = new Set<NodeSource>();
    for (const n of graph.nodes) present.add(nodeSource(n.type));
    return sourceOptions.filter(opt => present.has(opt.value)).map(opt => opt.value);
  }, [graph]);

  const renderFilterChips = () => (
    <>
      {activeSources.length > 1 && activeSources.map(src => (
        <button
          key={`src-${src}`}
          className={`${styles.filterChip} ${filters.sources.includes(src) ? styles.chipActive : ''}`}
          onClick={() => toggleSource(src)}
          aria-pressed={filters.sources.includes(src)}
          title={`Source: ${src}`}
        >
          {sourceOptions.find(o => o.value === src)?.label ?? src}
        </button>
      ))}
      {activeTypes.map(type => (
        <button
          key={type}
          className={`${styles.filterChip} ${filters.types.includes(type) ? styles.chipActive : ''}`}
          onClick={() => toggleType(type)}
          aria-pressed={filters.types.includes(type)}
        >
          {type}
        </button>
      ))}
      {healthStatuses.map(status => (
        <button
          key={status}
          className={`${styles.filterChip} ${styles[`chip_${status}`]} ${filters.health.includes(status) ? styles.chipActive : ''}`}
          onClick={() => toggleHealth(status)}
          aria-pressed={filters.health.includes(status)}
        >
          <span className={`${styles.chipDot} ${styles[`chipDot_${status}`]}`} />
          {status}
        </button>
      ))}
    </>
  );

  return (
    <header className={styles.header}>
      <span className={styles.appName}>
        graph-info
        {wsStatus && (
          <span
            className={`${styles.wsIndicator} ${styles[`ws_${wsStatus}`]}`}
            title={wsStatusLabels[wsStatus]}
            aria-label={wsStatusLabels[wsStatus]}
            role="status"
          />
        )}
      </span>

      <button className={styles.searchTrigger} onClick={onSearchOpen} aria-label="Search nodes">
        <svg className={styles.searchIcon} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <span className={styles.searchLabel}>Search nodes...</span>
        <kbd className={styles.searchKbd}>{isMac ? 'Cmd\u00A0+\u00A0K' : 'Ctrl\u00A0+\u00A0K'}</kbd>
      </button>

      <div className={styles.layoutToggle} ref={dropdownRef}>
        <button
          className={styles.layoutButton}
          onClick={() => setDropdownOpen(!dropdownOpen)}
          aria-haspopup="listbox"
          aria-expanded={dropdownOpen}
        >
          Layout: {LAYOUT_LABELS[layoutMode]}
          <svg className={styles.chevron} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M6 9l6 6 6-6" />
          </svg>
        </button>
        {dropdownOpen && (
          <div className={styles.dropdown} role="listbox">
            {graphHasNamespaces && (
              <button
                role="option"
                aria-selected={layoutMode === 'swimlane'}
                className={`${styles.dropdownItem} ${layoutMode === 'swimlane' ? styles.dropdownItemActive : ''}`}
                onClick={() => {
                  onLayoutChange('swimlane');
                  setDropdownOpen(false);
                }}
              >
                Swimlane (K8s)
              </button>
            )}
            <button
              role="option"
              aria-selected={layoutMode === 'hierarchical'}
              className={`${styles.dropdownItem} ${layoutMode === 'hierarchical' ? styles.dropdownItemActive : ''}`}
              onClick={() => {
                onLayoutChange('hierarchical');
                setDropdownOpen(false);
              }}
            >
              Hierarchical (Top-Down)
            </button>
            <button
              role="option"
              aria-selected={layoutMode === 'force'}
              className={`${styles.dropdownItem} ${layoutMode === 'force' ? styles.dropdownItemActive : ''}`}
              onClick={() => {
                onLayoutChange('force');
                setDropdownOpen(false);
              }}
            >
              Force-Directed (Organic)
            </button>
          </div>
        )}
      </div>

      <button
        className={styles.resetButton}
        onClick={onResetPositions}
        title="Reset all node positions"
        aria-label="Reset positions"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className={styles.resetIcon}>
          <path d="M1 4v6h6" />
          <path d="M3.51 15a9 9 0 1 0 2.13-9.36L1 10" />
        </svg>
        <span className={styles.resetLabel}>Reset</span>
      </button>

      {activeTypes.length > 0 && (
        <div className={styles.mobileFilterWrapper} ref={mobileFilterRef}>
          <button
            className={styles.mobileFilterBtn}
            onClick={() => setMobileFiltersOpen(!mobileFiltersOpen)}
            aria-label="Toggle filters"
            aria-expanded={mobileFiltersOpen}
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className={styles.mobileFilterIcon}>
              <path d="M22 3H2l8 9.46V19l4 2v-8.54L22 3z" />
            </svg>
            {activeFilterCount > 0 ? `${activeFilterCount} filter${activeFilterCount > 1 ? 's' : ''}` : 'Filter'}
          </button>
          {mobileFiltersOpen && (
            <div className={styles.mobileFilterDropdown}>
              {renderFilterChips()}
            </div>
          )}
        </div>
      )}

      {activeTypes.length > 0 && (
        <div className={styles.filterGroup}>
          {renderFilterChips()}
        </div>
      )}

      <div className={styles.healthSummary}>
        {healthCounts.healthy > 0 && (
          <span className={styles.healthBadge}>
            <span className={`${styles.dot} ${styles.dotHealthy}`} />
            {healthCounts.healthy}
          </span>
        )}
        {healthCounts.degraded > 0 && (
          <span className={styles.healthBadge}>
            <span className={`${styles.dot} ${styles.dotDegraded}`} />
            {healthCounts.degraded}
          </span>
        )}
        {healthCounts.unhealthy > 0 && (
          <span className={styles.healthBadge}>
            <span className={`${styles.dot} ${styles.dotUnhealthy}`} />
            {healthCounts.unhealthy}
          </span>
        )}
        <span className={styles.totalCount}>{healthCounts.total} nodes</span>
      </div>
    </header>
  );
}
