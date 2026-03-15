import React, { memo } from 'react';
import { Handle, Position, type Node, type NodeProps } from '@xyflow/react';
import type { HealthStatus, NodeType, PriorityTier } from '../../types';
import styles from './CustomNode.module.css';

export type CustomNodeData = Record<string, unknown> & {
  name: string;
  type: NodeType;
  health: HealthStatus;
  priority: PriorityTier;
  connectionCount: number;
  isConnected?: boolean;
  isSource?: boolean;
  isTarget?: boolean;
  isPinned?: boolean;
  justSaved?: boolean;
};

type CustomNodeType = Node<CustomNodeData>;

const NODE_ICONS: Record<NodeType, React.ReactElement> = {
    database: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <ellipse cx="12" cy="6" rx="8" ry="3" />
        <path d="M4 6v6c0 1.657 3.582 3 8 3s8-1.343 8-3V6" />
        <path d="M4 12v6c0 1.657 3.582 3 8 3s8-1.343 8-3v-6" />
      </svg>
    ),
    bucket: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M4 7l2 13h12l2-13" />
        <path d="M3 7h18" />
        <path d="M8 7V5a4 4 0 018 0v2" />
      </svg>
    ),
    service: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="8" width="7" height="8" rx="1" />
        <rect x="14" y="8" width="7" height="8" rx="1" />
        <path d="M10 12h4" />
        <circle cx="12" cy="4" r="2" />
        <path d="M12 6v2" />
      </svg>
    ),
    api: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M4 6h16M4 12h16M4 18h10" />
        <circle cx="18" cy="18" r="3" />
        <path d="M18 16v4M16 18h4" />
      </svg>
    ),
    gateway: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 3L3 9v12h18V9l-9-6z" />
        <path d="M9 21v-6h6v6" />
        <path d="M3 9h18" />
      </svg>
    ),
    queue: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="5" width="18" height="4" rx="1" />
        <rect x="3" y="10" width="18" height="4" rx="1" />
        <rect x="3" y="15" width="18" height="4" rx="1" />
        <path d="M17 7h2M17 12h2M17 17h2" strokeLinecap="round" />
      </svg>
    ),
    cache: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 2L2 7l10 5 10-5-10-5z" />
        <path d="M2 17l10 5 10-5" />
        <path d="M2 12l10 5 10-5" />
      </svg>
    ),
    storage: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="4" y="4" width="16" height="6" rx="1" />
        <rect x="4" y="14" width="16" height="6" rx="1" />
        <circle cx="7" cy="7" r="1" fill="currentColor" />
        <circle cx="7" cy="17" r="1" fill="currentColor" />
      </svg>
    ),
    payment: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="2" y="5" width="20" height="14" rx="2" />
        <path d="M2 10h20" />
        <path d="M6 15h4" />
      </svg>
    ),
    auth: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="5" y="11" width="14" height="10" rx="2" />
        <path d="M12 16v2" />
        <path d="M8 11V7a4 4 0 118 0v4" />
      </svg>
    ),
    table: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <path d="M3 9h18M9 3v18" />
      </svg>
    ),
    collection: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="4" y="4" width="6" height="6" rx="1" />
        <rect x="14" y="4" width="6" height="6" rx="1" />
        <rect x="4" y="14" width="6" height="6" rx="1" />
        <rect x="14" y="14" width="6" height="6" rx="1" />
      </svg>
    ),
    postgres: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <ellipse cx="12" cy="6" rx="8" ry="3" />
        <path d="M4 6v6c0 1.657 3.582 3 8 3s8-1.343 8-3V6" />
        <path d="M4 12v6c0 1.657 3.582 3 8 3s8-1.343 8-3v-6" />
        <path d="M16 6v12" />
        <path d="M16 14c2.5 0 4 1 4 3" />
      </svg>
    ),
    mongodb: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 2C12 2 8 6 8 12s4 10 4 10" />
        <path d="M12 2c0 0 4 4 4 10s-4 10-4 10" />
        <path d="M12 2v20" />
        <ellipse cx="12" cy="12" rx="4" ry="8" />
      </svg>
    ),
    s3: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M4 7l2 13h12l2-13" />
        <path d="M3 7h18" />
        <path d="M8 7V5a4 4 0 018 0v2" />
        <path d="M10 12h4" />
        <path d="M10 15h4" />
      </svg>
    ),
    redis: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <ellipse cx="12" cy="6" rx="8" ry="3" />
        <path d="M4 6v4c0 1.657 3.582 3 8 3s8-1.343 8-3V6" />
        <path d="M4 10v4c0 1.657 3.582 3 8 3s8-1.343 8-3v-4" />
        <path d="M4 14v4c0 1.657 3.582 3 8 3s8-1.343 8-3v-4" />
        <circle cx="12" cy="10" r="1" fill="currentColor" />
      </svg>
    ),
    http: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <circle cx="12" cy="12" r="10" />
        <path d="M2 12h20" />
        <path d="M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10A15.3 15.3 0 0112 2z" />
      </svg>
    ),
};

const TYPE_CATEGORY: Record<NodeType, string> = {
  database: 'data',
  postgres: 'data',
  mongodb: 'data',
  redis: 'data',
  table: 'data',
  collection: 'data',
  bucket: 'storage',
  s3: 'storage',
  storage: 'storage',
  service: 'service',
  api: 'service',
  http: 'service',
  gateway: 'gateway',
  queue: 'gateway',
  cache: 'infra',
  payment: 'infra',
  auth: 'infra',
};

const NodeIcon = ({ type }: { type: NodeType }) => {
  return NODE_ICONS[type] || NODE_ICONS.service;
};

function CustomNode({ data, selected }: NodeProps<CustomNodeType>) {
  const { name, type, health, priority, isConnected, isPinned, justSaved } = data;
  const showGlow = isConnected && !selected;
  const showPriorityBorder = priority === 'critical' || priority === 'high';
  const category = TYPE_CATEGORY[type] || 'service';

  return (
    <div
      className={`
        ${styles.node}
        ${selected ? styles.selected : ''}
        ${showGlow ? styles.glowing : ''}
        ${showPriorityBorder ? styles[`priority_${priority}`] : ''}
        ${justSaved ? styles.pinSaved : ''}
      `}
      style={{ '--type-color': `var(--type-${category})` } as React.CSSProperties}
    >
      <Handle type="target" position={Position.Top} className={styles.handle} id="top" />
      <Handle type="source" position={Position.Bottom} className={styles.handle} id="bottom" />

      {isPinned && (
        <div className={styles.pinIcon} title="Position locked">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M12 2v8m0 0l3-3m-3 3l-3-3" />
            <path d="M16 12l-4 10-4-10h8z" />
          </svg>
        </div>
      )}

      <div className={styles.card}>
        <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
        <div className={styles.iconWrapper}>
          <NodeIcon type={type} />
        </div>
        <div className={styles.info}>
          <span className={styles.name} title={name}>{name}</span>
          <span className={styles.type}>{type}</span>
        </div>
      </div>
    </div>
  );
}

export default memo(CustomNode);
