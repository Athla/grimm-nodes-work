import { Panel } from '../ui';
import type { Graph, GraphEdge } from '../../types';
import styles from './EdgeInspector.module.css';

interface EdgeInspectorProps {
  edge: GraphEdge | null;
  onClose: () => void;
  graph?: Graph;
  onNodeSelect?: (nodeId: string) => void;
}

export default function EdgeInspector({ edge, onClose, graph, onNodeSelect }: EdgeInspectorProps) {
  if (!edge) return null;

  const nodeMap = new Map(graph?.nodes?.map(n => [n.id, n.name]) ?? []);
  const sourceName = nodeMap.get(edge.source) || edge.source;
  const targetName = nodeMap.get(edge.target) || edge.target;

  return (
    <Panel isOpen={!!edge} onClose={onClose} position="bottom" ariaLabel="Edge inspector">
      <div className={styles.inspector}>
        <div className={styles.header}>
          <h3 className={styles.title}>Edge Details</h3>
          <button className={styles.closeBtn} onClick={onClose} aria-label="Close edge inspector">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>

        <div className={styles.details}>
          <div className={styles.flow}>
            <button
              className={styles.nodeNameBtn}
              onClick={() => onNodeSelect?.(edge.source)}
              aria-label={`Navigate to ${sourceName}`}
            >
              {sourceName}
            </button>
            <span className={styles.arrow}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M5 12h14M12 5l7 7-7 7" />
              </svg>
            </span>
            <button
              className={styles.nodeNameBtn}
              onClick={() => onNodeSelect?.(edge.target)}
              aria-label={`Navigate to ${targetName}`}
            >
              {targetName}
            </button>
          </div>

          <div className={styles.meta}>
            <div className={styles.metaItem}>
              <span className={styles.metaLabel}>Type</span>
              <span className={styles.metaValue}>{edge.type}</span>
            </div>
            {edge.label && (
              <div className={styles.metaItem}>
                <span className={styles.metaLabel}>Label</span>
                <span className={styles.metaValue}>{edge.label}</span>
              </div>
            )}
            <div className={styles.metaItem}>
              <span className={styles.metaLabel}>ID</span>
              <span className={styles.metaValue}>{edge.id}</span>
            </div>
          </div>
        </div>
      </div>
    </Panel>
  );
}
