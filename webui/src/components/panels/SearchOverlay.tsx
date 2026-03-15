import { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { useGraph } from '../../api';
import type { GraphNode } from '../../types';
import styles from './SearchOverlay.module.css';

interface SearchOverlayProps {
  isOpen: boolean;
  onClose: () => void;
  onNodeSelect: (nodeId: string) => void;
}

function HighlightMatch({ text, query }: { text: string; query: string }) {
  if (!query.trim()) return <>{text}</>;

  const lowerText = text.toLowerCase();
  const lowerQuery = query.toLowerCase();
  const index = lowerText.indexOf(lowerQuery);

  if (index === -1) return <>{text}</>;

  return (
    <>
      {text.slice(0, index)}
      <mark className={styles.highlight}>{text.slice(index, index + query.length)}</mark>
      {text.slice(index + query.length)}
    </>
  );
}

export default function SearchOverlay({
  isOpen,
  onClose,
  onNodeSelect,
}: SearchOverlayProps) {
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [displayLimit, setDisplayLimit] = useState(20);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const { data: graph } = useGraph();

  // Debounce search query
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query);
    }, 150);
    return () => clearTimeout(timer);
  }, [query]);

  const { results, totalMatchCount } = useMemo(() => {
    if (!graph?.nodes || !debouncedQuery.trim()) {
      return { results: [] as GraphNode[], totalMatchCount: 0 };
    }

    const lowerQuery = debouncedQuery.toLowerCase();

    const scored = graph.nodes
      .map(node => {
        let score = 0;
        const lowerName = node.name.toLowerCase();
        const lowerType = node.type.toLowerCase();
        const lowerId = node.id.toLowerCase();

        if (lowerName === lowerQuery) score = 100;
        else if (lowerName.startsWith(lowerQuery)) score = 80;
        else if (lowerName.includes(lowerQuery)) score = 60;
        else if (lowerType.includes(lowerQuery)) score = 40;
        else if (lowerId.includes(lowerQuery)) score = 20;

        return { node, score };
      })
      .filter(item => item.score > 0)
      .sort((a, b) => b.score - a.score);

    return {
      results: scored.slice(0, displayLimit).map(item => item.node),
      totalMatchCount: scored.length,
    };
  }, [graph, debouncedQuery, displayLimit]);

  // Focus input when opening
  useEffect(() => {
    if (isOpen) {
      setQuery('');
      setDebouncedQuery('');
      setSelectedIndex(0);
      // Small delay to ensure animation doesn't conflict
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [isOpen]);

  // Reset selection and display limit when query changes
  useEffect(() => {
    setSelectedIndex(0);
    setDisplayLimit(20);
  }, [debouncedQuery]);

  // Scroll selected item into view
  useEffect(() => {
    if (listRef.current && results.length > 0) {
      const selectedItem = listRef.current.children[selectedIndex] as HTMLElement;
      selectedItem?.scrollIntoView({ block: 'nearest' });
    }
  }, [selectedIndex, results.length]);

  const handleSelect = useCallback(
    (node: GraphNode) => {
      onNodeSelect(node.id);
      onClose();
    },
    [onNodeSelect, onClose]
  );

  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent) => {
      switch (event.key) {
        case 'ArrowDown':
          event.preventDefault();
          setSelectedIndex((prev) => Math.min(prev + 1, results.length - 1));
          break;
        case 'ArrowUp':
          event.preventDefault();
          setSelectedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case 'Enter':
          event.preventDefault();
          if (results[selectedIndex]) {
            handleSelect(results[selectedIndex]);
          }
          break;
        case 'Escape':
          event.preventDefault();
          onClose();
          break;
      }
    },
    [results, selectedIndex, handleSelect, onClose]
  );

  if (!isOpen) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.inputWrapper}>
          <span className={styles.searchIcon} aria-hidden>
            ⌘
          </span>
          <input
            ref={inputRef}
            type="text"
            className={styles.input}
            placeholder="Search nodes..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={handleKeyDown}
            role="combobox"
            aria-controls="search-results-list"
            autoComplete="off"
            name="search"
            aria-label="Search nodes"
            aria-activedescendant={
              results[selectedIndex] ? `result-${results[selectedIndex].id}` : undefined
            }
          />
          {query && (
            <button
              className={styles.clearBtn}
              onClick={() => { setQuery(''); inputRef.current?.focus(); }}
              aria-label="Clear search"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          )}
          <kbd className={styles.hint}>esc</kbd>
        </div>

        {debouncedQuery && (
          <ul ref={listRef} className={styles.results} id="search-results-list" role="listbox">
            {results.length > 0 ? (
              results.map((node, index) => (
                <li
                  key={node.id}
                  id={`result-${node.id}`}
                  role="option"
                  aria-selected={index === selectedIndex}
                  className={`${styles.result} ${index === selectedIndex ? styles.selected : ''}`}
                  onClick={() => handleSelect(node)}
                  onMouseEnter={() => setSelectedIndex(index)}
                >
                  <span className={`${styles.healthIndicator} ${styles[node.health]}`} />
                  <div className={styles.resultContent}>
                    <span className={styles.resultName}>
                      <HighlightMatch text={node.name} query={debouncedQuery} />
                    </span>
                    <span className={styles.resultType}>
                      <HighlightMatch text={node.type} query={debouncedQuery} />
                    </span>
                  </div>
                </li>
              ))
            ) : (
              <li className={styles.empty}>No nodes found</li>
            )}
          </ul>
        )}

        {debouncedQuery && totalMatchCount > displayLimit && (
          <div className={styles.truncationHint}>
            Showing {displayLimit} of {totalMatchCount} results
            <button
              className={styles.showMoreBtn}
              onClick={() => setDisplayLimit(prev => prev + 20)}
            >
              Show more
            </button>
          </div>
        )}

        {!debouncedQuery && graph?.nodes && (
          <div className={styles.hints}>
            <span>Type to search {graph.nodes.length} nodes</span>
          </div>
        )}
      </div>
    </div>
  );
}
