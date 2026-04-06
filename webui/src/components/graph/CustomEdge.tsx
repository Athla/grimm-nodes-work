import { memo } from 'react';
import { BaseEdge, getBezierPath, Position, useInternalNode, type Edge, type EdgeProps } from '@xyflow/react';
import styles from './CustomEdge.module.css';

export type CustomEdgeData = Record<string, unknown> & {
  label?: string;
  isActive?: boolean;
  isHighlighted?: boolean;
  edgeType?: string;
  isCrossNamespace?: boolean;
};

type CustomEdgeType = Edge<CustomEdgeData>;

interface NodeRect {
  x: number;
  y: number;
  width: number;
  height: number;
}

// Find where a line from the node center toward a target point intersects the node border
function getNodeBorderPoint(node: NodeRect, target: { x: number; y: number }): { x: number; y: number } {
  const cx = node.x + node.width / 2;
  const cy = node.y + node.height / 2;
  const dx = target.x - cx;
  const dy = target.y - cy;

  if (dx === 0 && dy === 0) return { x: cx, y: cy };

  const halfW = node.width / 2;
  const halfH = node.height / 2;

  // Determine which edge of the rectangle the center→target line crosses
  if (Math.abs(dx) * halfH > Math.abs(dy) * halfW) {
    // Exits through left or right side
    const sign = dx > 0 ? 1 : -1;
    return { x: cx + sign * halfW, y: cy + (dy / dx) * sign * halfW };
  } else {
    // Exits through top or bottom side
    const sign = dy > 0 ? 1 : -1;
    return { x: cx + (dx / dy) * sign * halfH, y: cy + sign * halfH };
  }
}

// Determine which Position (side) a border point is on
function getBorderPosition(node: NodeRect, point: { x: number; y: number }): Position {
  const cx = node.x + node.width / 2;
  const cy = node.y + node.height / 2;
  const dx = point.x - cx;
  const dy = point.y - cy;
  const halfW = node.width / 2;
  const halfH = node.height / 2;

  // Which axis is closer to the edge?
  if (Math.abs(Math.abs(dx) - halfW) < Math.abs(Math.abs(dy) - halfH)) {
    return dx > 0 ? Position.Right : Position.Left;
  }
  return dy > 0 ? Position.Bottom : Position.Top;
}

function CustomEdge({
  id,
  source,
  target,
  data,
  selected,
}: EdgeProps<CustomEdgeType>) {
  const sourceNode = useInternalNode(source);
  const targetNode = useInternalNode(target);

  if (!sourceNode || !targetNode) return null;

  const sourceRect: NodeRect = {
    x: sourceNode.internals.positionAbsolute.x,
    y: sourceNode.internals.positionAbsolute.y,
    width: sourceNode.measured?.width ?? 180,
    height: sourceNode.measured?.height ?? 64,
  };
  const targetRect: NodeRect = {
    x: targetNode.internals.positionAbsolute.x,
    y: targetNode.internals.positionAbsolute.y,
    width: targetNode.measured?.width ?? 180,
    height: targetNode.measured?.height ?? 64,
  };

  const targetCenter = { x: targetRect.x + targetRect.width / 2, y: targetRect.y + targetRect.height / 2 };
  const sourceCenter = { x: sourceRect.x + sourceRect.width / 2, y: sourceRect.y + sourceRect.height / 2 };

  const sourcePoint = getNodeBorderPoint(sourceRect, targetCenter);
  const targetPoint = getNodeBorderPoint(targetRect, sourceCenter);

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX: sourcePoint.x,
    sourceY: sourcePoint.y,
    sourcePosition: getBorderPosition(sourceRect, sourcePoint),
    targetX: targetPoint.x,
    targetY: targetPoint.y,
    targetPosition: getBorderPosition(targetRect, targetPoint),
    curvature: 0.25,
  });

  const isActive = data?.isActive;
  const isHighlighted = data?.isHighlighted || selected;
  const edgeType = data?.edgeType;
  const isCrossNamespace = data?.isCrossNamespace;

  // Two-tier edge weighting.
  const isContains = edgeType === 'contains';
  const isRoutesTo = edgeType === 'routes_to';

  const edgeClass = [
    styles.edge,
    isContains ? styles.edgeContains : '',
    isRoutesTo ? styles.edgeRoutesTo : '',
    isCrossNamespace && isRoutesTo ? styles.edgeCrossNs : '',
    isActive ? styles.active : '',
    isHighlighted ? styles.highlighted : '',
  ].filter(Boolean).join(' ');

  return (
    <g className={styles.edgeGroup}>
      <BaseEdge
        id={id}
        path={edgePath}
        className={edgeClass}
      />

      {/* Arrowhead path — uses shared markers defined in GraphCanvas */}
      {isRoutesTo && (
        <path
          d={edgePath}
          fill="none"
          className={`${styles.edgeArrowPath} ${isCrossNamespace ? styles.edgeCrossNs : ''} ${isActive ? styles.active : ''}`}
          markerEnd={`url(#${isCrossNamespace ? 'arrow-cross-ns' : 'arrow-routes-to'})`}
        />
      )}

      {/* Animated overlay for active edges */}
      {isActive && (
        <path
          d={edgePath}
          className={styles.animatedPath}
          fill="none"
          strokeDasharray="5 5"
        />
      )}

      {data?.label && (() => {
        const labelWidth = Math.max(data.label.length * 5.5, 30) + 12;
        return (
          <g transform={`translate(${labelX}, ${labelY})`}>
            <rect
              x={-labelWidth / 2}
              y="-10"
              width={labelWidth}
              height="20"
              rx="4"
              className={`${styles.labelBg} ${isHighlighted ? styles.labelBgActive : ''}`}
            />
            <text
              className={styles.labelText}
              textAnchor="middle"
              dominantBaseline="middle"
            >
              {data.label}
            </text>
          </g>
        );
      })()}
    </g>
  );
}

export default memo(CustomEdge);
