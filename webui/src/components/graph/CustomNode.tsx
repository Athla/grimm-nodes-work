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
  isGroup?: boolean;
  isSwimlane?: boolean;
  // K8s enriched metadata
  metadata?: Record<string, unknown>;
  workloadCount?: number;
  podCount?: number;
  routesToCount?: number;
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
    namespace: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M3 6h18M3 12h18M3 18h18" strokeDasharray="2 2" />
        <rect x="3" y="3" width="18" height="18" rx="2" />
      </svg>
    ),
    deployment: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="3" y="3" width="8" height="8" rx="1" />
        <rect x="13" y="3" width="8" height="8" rx="1" />
        <rect x="3" y="13" width="8" height="8" rx="1" />
        <rect x="13" y="13" width="8" height="8" rx="1" />
      </svg>
    ),
    statefulset: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <rect x="4" y="3" width="16" height="5" rx="1" />
        <rect x="4" y="10" width="16" height="5" rx="1" />
        <rect x="4" y="17" width="16" height="5" rx="1" />
        <circle cx="7" cy="5.5" r="0.8" fill="currentColor" />
        <circle cx="7" cy="12.5" r="0.8" fill="currentColor" />
        <circle cx="7" cy="19.5" r="0.8" fill="currentColor" />
      </svg>
    ),
    daemonset: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <circle cx="6" cy="6" r="3" />
        <circle cx="18" cy="6" r="3" />
        <circle cx="6" cy="18" r="3" />
        <circle cx="18" cy="18" r="3" />
        <path d="M9 6h6M9 18h6M6 9v6M18 9v6" />
      </svg>
    ),
    pod: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 2l8 5v10l-8 5-8-5V7l8-5z" />
        <path d="M12 2v20" />
        <path d="M4 7l8 5 8-5" />
      </svg>
    ),
    k8s_service: (
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
        <path d="M12 2L2 7v10l10 5 10-5V7l-10-5z" />
        <circle cx="12" cy="12" r="3" />
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
  namespace: 'k8s',
  deployment: 'k8s',
  statefulset: 'k8s',
  daemonset: 'k8s',
  pod: 'k8s',
  k8s_service: 'k8s',
};

const NodeIcon = ({ type }: { type: NodeType }) => {
  return NODE_ICONS[type] || NODE_ICONS.service;
};

// ── Swimlane per-type subcomponents ──────────────────────────────────────

function NamespaceLaneHeader({ data, selected }: { data: CustomNodeData; selected: boolean }) {
  const { name, health, workloadCount, podCount } = data;
  const healthyCount = podCount ?? 0; // simplified — full rollup would need children health array

  return (
    <div className={`${styles.laneHeader} ${selected ? styles.selected : ''}`}>
      <Handle type="target" position={Position.Left} className={styles.handle} id="left" />
      <Handle type="source" position={Position.Right} className={styles.handle} id="right" />
      <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
      <div className={styles.iconWrapper}>
        <NodeIcon type="namespace" />
      </div>
      <span className={styles.laneHeaderName}>{name}</span>
      <div className={styles.laneHeaderStats}>
        {(workloadCount ?? 0) > 0 && <span className={styles.statChip}>{workloadCount} workloads</span>}
        {(podCount ?? 0) > 0 && <span className={styles.statChip}>{podCount} pods</span>}
        {healthyCount > 0 && health === 'healthy' && <span className={styles.statChipGood}>all healthy</span>}
      </div>
    </div>
  );
}

function ServiceNodeContent({ data, selected }: { data: CustomNodeData; selected: boolean }) {
  const { name, health, metadata, routesToCount } = data;
  const serviceType = (metadata?.service_type as string) ?? '';
  const ports = (metadata?.ports as string[]) ?? [];
  const category = 'k8s';
  const pulseClass = health === 'unhealthy' ? styles.pulseUnhealthy : health === 'degraded' ? styles.pulseDegraded : '';

  return (
    <div
      className={`${styles.node} ${styles.swimlaneNode} ${selected ? styles.selected : ''} ${pulseClass}`}
      style={{ '--type-color': `var(--type-${category})` } as React.CSSProperties}
    >
      <Handle type="target" position={Position.Left} className={styles.handle} id="left" />
      <Handle type="source" position={Position.Right} className={styles.handle} id="right" />
      <div className={styles.swimCard}>
        <div className={styles.swimCardHeader}>
          <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
          <div className={styles.iconWrapper}><NodeIcon type="k8s_service" /></div>
          <span className={styles.name} title={name}>{name}</span>
        </div>
        <div className={styles.swimCardBody}>
          {serviceType && <span className={styles.badge}>{serviceType}</span>}
          {ports.length > 0 && <span className={styles.mono}>{ports.join(', ')}</span>}
          {routesToCount !== undefined && (
            <span className={styles.focalNumber}>&rarr; {routesToCount} pods</span>
          )}
        </div>
      </div>
    </div>
  );
}

function WorkloadNodeContent({ data, selected }: { data: CustomNodeData; selected: boolean }) {
  const { name, type, health, metadata } = data;
  const desired = metadata?.desired as number | undefined;
  const ready = metadata?.ready as number | undefined;
  const image = metadata?.image as string | undefined;
  const category = 'k8s';
  const isDegraded = desired !== undefined && ready !== undefined && ready < desired;
  const kindLabel = type === 'deployment' ? 'deploy' : type === 'statefulset' ? 'sts' : 'ds';
  const pulseClass = health === 'unhealthy' ? styles.pulseUnhealthy : health === 'degraded' ? styles.pulseDegraded : '';

  // Short image tag: "nginx:1.25" from "docker.io/library/nginx:1.25"
  const shortImage = image ? image.split('/').pop() : undefined;

  return (
    <div
      className={`${styles.node} ${styles.swimlaneNode} ${selected ? styles.selected : ''} ${pulseClass}`}
      style={{ '--type-color': `var(--type-${category})` } as React.CSSProperties}
    >
      <Handle type="target" position={Position.Left} className={styles.handle} id="left" />
      <Handle type="source" position={Position.Right} className={styles.handle} id="right" />
      <div className={styles.swimCard}>
        <div className={styles.swimCardHeader}>
          <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
          <div className={styles.iconWrapper}><NodeIcon type={type} /></div>
          <span className={styles.badge}>{kindLabel}</span>
          <span className={styles.name} title={name}>{name}</span>
        </div>
        <div className={styles.swimCardBody}>
          {desired !== undefined && ready !== undefined && (
            <span className={`${styles.focalNumber} ${isDegraded ? styles.focalDegraded : ''}`}>
              {ready}/{desired}
            </span>
          )}
          {shortImage && <span className={styles.mono} title={image}>{shortImage}</span>}
          {isDegraded && <span className={styles.chipWarning}>degraded</span>}
        </div>
      </div>
    </div>
  );
}

function PodNodeContent({ data, selected }: { data: CustomNodeData; selected: boolean }) {
  const { name, health, metadata } = data;
  const phase = (metadata?.phase as string) ?? 'Unknown';
  const containers = metadata?.containers as string[] | undefined;
  const image = metadata?.image as string | undefined;
  const restartCount = metadata?.restart_count as number | undefined;
  const category = 'k8s';
  const pulseClass = health === 'unhealthy' ? styles.pulseUnhealthy : health === 'degraded' ? styles.pulseDegraded : '';

  const isError = phase === 'CrashLoopBackOff' || phase === 'ImagePullBackOff' || phase === 'ErrImagePull' || phase === 'Failed';
  const shortImage = image ? image.split('/').pop() : undefined;
  // Truncate pod name: show first part + last 5 chars of hash
  const shortName = name.length > 30 ? name.slice(0, 20) + '...' + name.slice(-5) : name;

  return (
    <div
      className={`${styles.node} ${styles.swimlaneNode} ${selected ? styles.selected : ''} ${pulseClass}`}
      style={{ '--type-color': `var(--type-${category})` } as React.CSSProperties}
    >
      <Handle type="target" position={Position.Left} className={styles.handle} id="left" />
      <Handle type="source" position={Position.Right} className={styles.handle} id="right" />
      <div className={styles.swimCard}>
        <div className={styles.swimCardHeader}>
          <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
          <div className={styles.iconWrapper}><NodeIcon type="pod" /></div>
          <span className={styles.name} title={name}>{shortName}</span>
        </div>
        <div className={styles.swimCardBody}>
          <span className={`${styles.phaseChip} ${isError ? styles.phaseError : phase === 'Running' ? styles.phaseRunning : styles.phasePending}`}>
            {phase}
          </span>
          {containers && <span className={styles.mono}>{containers.length} container{containers.length !== 1 ? 's' : ''}</span>}
          {shortImage && <span className={styles.mono} title={image}>{shortImage}</span>}
          {restartCount !== undefined && restartCount > 0 && (
            <span className={styles.chipWarning}>{restartCount} restart{restartCount !== 1 ? 's' : ''}</span>
          )}
        </div>
      </div>
    </div>
  );
}

// ── Main component ──────────────────────────────────────────────────────

function CustomNode({ data, selected }: NodeProps<CustomNodeType>) {
  const { name, type, health, priority, isConnected, isPinned, justSaved, isGroup, isSwimlane } = data;
  const showGlow = isConnected && !selected;
  const showPriorityBorder = priority === 'critical' || priority === 'high';
  const category = TYPE_CATEGORY[type] || 'service';

  // ── Swimlane per-type rendering ─────────────────────────────────────
  if (isSwimlane) {
    if (isGroup) {
      return <NamespaceLaneHeader data={data} selected={!!selected} />;
    }
    if (type === 'k8s_service') {
      return <ServiceNodeContent data={data} selected={!!selected} />;
    }
    if (type === 'deployment' || type === 'statefulset' || type === 'daemonset') {
      return <WorkloadNodeContent data={data} selected={!!selected} />;
    }
    if (type === 'pod') {
      return <PodNodeContent data={data} selected={!!selected} />;
    }
    // Fallthrough to default node for non-k8s types in a k8s graph.
  }

  // ── Legacy group rendering (non-swimlane) ───────────────────────────
  if (isGroup) {
    return (
      <div
        className={`${styles.group} ${selected ? styles.selected : ''}`}
        style={{ '--type-color': `var(--type-${category})` } as React.CSSProperties}
      >
        <Handle type="target" position={Position.Top} className={styles.handle} id="top" />
        <Handle type="source" position={Position.Bottom} className={styles.handle} id="bottom" />
        <div className={styles.groupHeader}>
          <span className={`${styles.healthDot} ${styles[`health_${health}`]}`} />
          <div className={styles.iconWrapper}>
            <NodeIcon type={type} />
          </div>
          <span className={styles.groupName} title={name}>{name}</span>
          <span className={styles.groupType}>{type}</span>
        </div>
      </div>
    );
  }

  // ── Default node rendering ──────────────────────────────────────────
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
