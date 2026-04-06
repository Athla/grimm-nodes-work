import styles from './CustomEdge.module.css';

/**
 * Shared SVG marker definitions for edge arrowheads.
 * Rendered once inside <ReactFlow> instead of per-edge.
 */
export function EdgeMarkerDefs() {
  return (
    <svg style={{ position: 'absolute', width: 0, height: 0 }}>
      <defs>
        <marker
          id="arrow-routes-to"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" className={styles.arrowRoutesTo} />
        </marker>
        <marker
          id="arrow-cross-ns"
          viewBox="0 0 10 10"
          refX="8"
          refY="5"
          markerWidth="6"
          markerHeight="6"
          orient="auto-start-reverse"
        >
          <path d="M 0 0 L 10 5 L 0 10 z" className={styles.arrowCrossNs} />
        </marker>
      </defs>
    </svg>
  );
}
