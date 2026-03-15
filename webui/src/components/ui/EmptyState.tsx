import styles from './EmptyState.module.css';

interface EmptyStateProps {
  reason?: 'no-data' | 'filtered';
  filterCount?: number;
  onClearFilters?: () => void;
}

export default function EmptyState({ reason = 'no-data', filterCount, onClearFilters }: EmptyStateProps) {
  if (reason === 'filtered') {
    return (
      <div className={styles.container}>
        <div className={styles.content}>
          <div className={styles.icon}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
              <circle cx="11" cy="11" r="8" />
              <path d="M21 21l-4.35-4.35" />
              <path d="M8 11h6" />
            </svg>
          </div>
          <h2 className={styles.title}>No matching nodes</h2>
          <p className={styles.message}>
            {filterCount !== undefined && filterCount > 0
              ? `${filterCount} active filter${filterCount > 1 ? 's' : ''} hiding all nodes.`
              : 'Current filters hide all nodes.'}
          </p>
          {onClearFilters && (
            <button className={styles.clearFiltersBtn} onClick={onClearFilters}>
              Clear all filters
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <div className={styles.content}>
        <div className={styles.icon}>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
            <circle cx="12" cy="12" r="3" />
            <path d="M12 2v4m0 12v4M2 12h4m12 0h4" />
            <path d="M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83" />
            <path d="M19.07 4.93l-2.83 2.83m-8.48 8.48l-2.83 2.83" />
          </svg>
        </div>
        <h2 className={styles.title}>No nodes discovered</h2>
        <p className={styles.message}>
          Start Docker containers or configure connections in <code className={styles.code}>conf/config.yaml</code> to visualize your infrastructure.
        </p>
      </div>
    </div>
  );
}
