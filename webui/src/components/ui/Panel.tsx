import { useEffect, useRef, type ReactNode } from 'react';
import styles from './Panel.module.css';

interface PanelProps {
  isOpen: boolean;
  onClose: () => void;
  children: ReactNode;
  position?: 'right' | 'bottom';
  width?: string;
  ariaLabel?: string;
}

export default function Panel({
  isOpen,
  onClose,
  children,
  position = 'right',
  width = '360px',
  ariaLabel,
}: PanelProps) {
  const panelRef = useRef<HTMLDivElement>(null);

  // Handle click outside
  useEffect(() => {
    if (!isOpen) return;

    function handleClickOutside(event: MouseEvent) {
      // Skip close when clicking on a graph node — the node click handler will update panel content
      if ((event.target as HTMLElement).closest('.react-flow__node')) return;
      if (panelRef.current && !panelRef.current.contains(event.target as Node)) {
        onClose();
      }
    }

    // Delay to prevent immediate close
    const timeoutId = setTimeout(() => {
      document.addEventListener('mousedown', handleClickOutside);
    }, 100);

    return () => {
      clearTimeout(timeoutId);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isOpen, onClose]);

  // Handle escape key
  useEffect(() => {
    if (!isOpen) return;

    function handleEscape(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose();
      }
    }

    document.addEventListener('keydown', handleEscape);
    return () => document.removeEventListener('keydown', handleEscape);
  }, [isOpen, onClose]);

  // Focus trapping
  const previousFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!isOpen) return;

    previousFocusRef.current = document.activeElement as HTMLElement;

    const panel = panelRef.current;
    if (!panel) return;

    const focusableSelector = 'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

    // Auto-focus first focusable element
    requestAnimationFrame(() => {
      const first = panel.querySelector<HTMLElement>(focusableSelector);
      first?.focus();
    });

    function handleTabTrap(event: KeyboardEvent) {
      if (event.key !== 'Tab' || !panel) return;

      const focusable = Array.from(panel.querySelectorAll<HTMLElement>(focusableSelector));
      if (focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];

      if (event.shiftKey) {
        if (document.activeElement === first) {
          event.preventDefault();
          last.focus();
        }
      } else {
        if (document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    }

    document.addEventListener('keydown', handleTabTrap);
    return () => {
      document.removeEventListener('keydown', handleTabTrap);
      previousFocusRef.current?.focus();
    };
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div
      ref={panelRef}
      className={`${styles.panel} ${styles[position]}`}
      style={{ '--panel-width': width } as React.CSSProperties}
      role="dialog"
      aria-modal="true"
      aria-label={ariaLabel}
    >
      {children}
    </div>
  );
}
