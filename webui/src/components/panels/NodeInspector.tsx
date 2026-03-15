import { useMemo, useState, useCallback } from 'react';
import { Panel } from '../ui';
import { useNodeFromGraph } from '../../api';
import { calculatePriority, countConnections } from '../../utils';
import type { Graph, HealthStatus, PriorityTier } from '../../types';
import styles from './NodeInspector.module.css';

interface NodeInspectorProps {
  nodeId: string | null;
  onClose: () => void;
  graph?: Graph;
  onNodeSelect?: (nodeId: string) => void;
}

const healthLabels: Record<HealthStatus, string> = {
  healthy: 'Operational',
  degraded: 'Degraded',
  unhealthy: 'Critical',
  unknown: 'Unknown',
};

const priorityLabels: Record<PriorityTier, string> = {
  critical: 'Critical',
  high: 'High',
  medium: 'Medium',
  low: 'Low',
};

export default function NodeInspector({ nodeId, onClose, graph, onNodeSelect }: NodeInspectorProps) {
  const node = useNodeFromGraph(nodeId);
  const [copied, setCopied] = useState(false);

  const handleCopyId = useCallback(() => {
    if (!node) return;
    navigator.clipboard.writeText(node.id).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [node]);

  const priority = useMemo(() => {
    if (!node || !graph?.edges) return 'low' as PriorityTier;
    const cc = countConnections(node.id, graph.edges);
    return calculatePriority(node, cc);
  }, [node, graph]);

  const connections = useMemo(() => {
    if (!nodeId || !graph?.nodes || !graph?.edges) return { incoming: [] as { id: string; name: string }[], outgoing: [] as { id: string; name: string }[] };

    const nodeMap = new Map(graph.nodes.map(n => [n.id, n.name]));
    const incoming: { id: string; name: string }[] = [];
    const outgoing: { id: string; name: string }[] = [];

    graph.edges.forEach(edge => {
      if (edge.target === nodeId) {
        incoming.push({ id: edge.source, name: nodeMap.get(edge.source) || edge.source });
      }
      if (edge.source === nodeId) {
        outgoing.push({ id: edge.target, name: nodeMap.get(edge.target) || edge.target });
      }
    });

    return { incoming, outgoing };
  }, [nodeId, graph]);

  return (
    <Panel isOpen={!!nodeId} onClose={onClose} ariaLabel="Node inspector">
      {node && (
        <div key={nodeId} className={styles.inspector}>
          <header className={styles.header}>
            <div className={`${styles.accentBar} ${styles[`accent_${priority}`]}`} />

            <div className={styles.headerContent}>
              <div className={styles.titleRow}>
                <div className={styles.titleGroup}>
                  <span className={`${styles.priorityBadge} ${styles[`priority_${priority}`]}`}>
                    {priorityLabels[priority]}
                  </span>
                  <h2 className={styles.name}>{node.name}</h2>
                </div>
                <button
                  className={styles.closeBtn}
                  onClick={onClose}
                  aria-label="Close inspector"
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M18 6L6 18M6 6l12 12" />
                  </svg>
                </button>
              </div>

              <div className={styles.meta}>
                <span className={styles.typeTag}>{node.type}</span>
                <span className={`${styles.healthTag} ${styles[`health_${node.health}`]}`}>
                  <span className={styles.healthDot} />
                  {healthLabels[node.health]}
                </span>
              </div>
            </div>
          </header>

          <div className={styles.content}>
            <section className={styles.statsGrid}>
              <div className={styles.stat}>
                <span className={styles.statLabel}>ID</span>
                <span className={styles.statValueRow}>
                  <span className={styles.statValue}>{node.id}</span>
                  <button
                    className={styles.copyBtn}
                    onClick={handleCopyId}
                    aria-label="Copy node ID"
                    title="Copy ID"
                  >
                    {copied ? (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M20 6L9 17l-5-5" />
                      </svg>
                    ) : (
                      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <rect x="9" y="9" width="13" height="13" rx="2" />
                        <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
                      </svg>
                    )}
                    {copied && <span className={styles.copiedFeedback}>Copied!</span>}
                  </button>
                </span>
              </div>
              {node.parent && (
                <div className={styles.stat}>
                  <span className={styles.statLabel}>Parent</span>
                  <span className={styles.statValue}>{node.parent}</span>
                </div>
              )}
            </section>

            {Object.keys(node.metadata).filter(k => k !== 'priority').length > 0 && (
              <section className={styles.section}>
                <h3 className={styles.sectionTitle}>
                  <span className={styles.sectionIcon}>
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <path d="M12 2L2 7l10 5 10-5-10-5z" />
                      <path d="M2 17l10 5 10-5" />
                      <path d="M2 12l10 5 10-5" />
                    </svg>
                  </span>
                  Metadata
                </h3>
                <dl className={styles.metaList}>
                  {Object.entries(node.metadata)
                    .filter(([key]) => key !== 'priority')
                    .map(([key, value]) => (
                      <div key={key} className={styles.metaItem}>
                        <dt className={styles.metaKey}>{formatKey(key)}</dt>
                        <dd className={styles.metaValue}>{formatValue(value)}</dd>
                      </div>
                    ))}
                </dl>
              </section>
            )}

            <section className={styles.section}>
              <h3 className={styles.sectionTitle}>
                <span className={styles.sectionIcon}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="12" cy="12" r="3" />
                    <path d="M12 2v4m0 12v4M2 12h4m12 0h4" />
                  </svg>
                </span>
                Connections
              </h3>
              {connections.incoming.length > 0 && (
                <div className={styles.connectionGroup}>
                  <span className={styles.connectionLabel}>Incoming</span>
                  <ul className={styles.connectionList}>
                    {connections.incoming.map((conn) => (
                      <li key={conn.id} className={styles.connectionItem}>
                        <button
                          className={styles.connectionLink}
                          onClick={() => onNodeSelect?.(conn.id)}
                          aria-label={`Navigate to ${conn.name}`}
                        >
                          {conn.name}
                          <svg className={styles.connectionArrow} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M5 12h14M12 5l7 7-7 7" />
                          </svg>
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {connections.outgoing.length > 0 && (
                <div className={styles.connectionGroup}>
                  <span className={styles.connectionLabel}>Outgoing</span>
                  <ul className={styles.connectionList}>
                    {connections.outgoing.map((conn) => (
                      <li key={conn.id} className={styles.connectionItem}>
                        <button
                          className={styles.connectionLink}
                          onClick={() => onNodeSelect?.(conn.id)}
                          aria-label={`Navigate to ${conn.name}`}
                        >
                          {conn.name}
                          <svg className={styles.connectionArrow} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                            <path d="M5 12h14M12 5l7 7-7 7" />
                          </svg>
                        </button>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
              {connections.incoming.length === 0 && connections.outgoing.length === 0 && (
                <p className={styles.placeholder}>No connections</p>
              )}
            </section>
          </div>
        </div>
      )}
    </Panel>
  );
}

function formatKey(key: string): string {
  return key
    .replace(/([A-Z])/g, ' $1')
    .replace(/[_-]/g, ' ')
    .replace(/^\w/, c => c.toUpperCase())
    .trim();
}

function formatValue(value: unknown): string {
  if (Array.isArray(value)) {
    return value.length > 0 ? value.join(', ') : '—';
  }
  if (typeof value === 'boolean') {
    return value ? 'Yes' : 'No';
  }
  if (typeof value === 'object' && value !== null) {
    return JSON.stringify(value);
  }
  if (value === null || value === undefined) {
    return '—';
  }
  return String(value);
}
