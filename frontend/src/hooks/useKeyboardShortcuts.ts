import { useEffect, useCallback, useRef } from 'react';

export interface KeyboardShortcut {
  key: string;
  ctrl?: boolean;
  alt?: boolean;
  shift?: boolean;
  meta?: boolean;
  description: string;
  category: 'Navigation' | 'View' | 'Search' | 'Help';
}

export interface KeyboardShortcutActions {
  onFocusSearch?: () => void;
  onToggleSidebar?: () => void;
  onFitGraph?: () => void;
  onResetCamera?: () => void;
  onSwitch3D?: () => void;
  onSwitch2D?: () => void;
  onSwitchCommunity?: () => void;
  onToggleLabels?: () => void;
  onEscape?: () => void;
  onShowHelp?: () => void;
  onArrowUp?: () => void;
  onArrowDown?: () => void;
  onArrowLeft?: () => void;
  onArrowRight?: () => void;
}

export const SHORTCUTS: KeyboardShortcut[] = [
  // Search
  { key: 'k', ctrl: true, description: 'Focus search', category: 'Search' },
  { key: '/', description: 'Focus search', category: 'Search' },
  
  // Navigation
  { key: 'b', ctrl: true, description: 'Toggle sidebar', category: 'Navigation' },
  { key: 'Escape', description: 'Deselect / Close panels', category: 'Navigation' },
  { key: 'ArrowUp', description: 'Navigate to node above', category: 'Navigation' },
  { key: 'ArrowDown', description: 'Navigate to node below', category: 'Navigation' },
  { key: 'ArrowLeft', description: 'Navigate to node on left', category: 'Navigation' },
  { key: 'ArrowRight', description: 'Navigate to node on right', category: 'Navigation' },
  
  // View
  { key: 'f', description: 'Fit graph to screen', category: 'View' },
  { key: 'r', description: 'Reset camera', category: 'View' },
  { key: '1', description: 'Switch to 3D view', category: 'View' },
  { key: '2', description: 'Switch to 2D view', category: 'View' },
  { key: '3', description: 'Switch to Community view', category: 'View' },
  { key: 'l', description: 'Toggle labels', category: 'View' },
  
  // Help
  { key: '?', description: 'Show shortcuts help', category: 'Help' },
  { key: 'F1', description: 'Show shortcuts help', category: 'Help' },
];

/**
 * Hook for handling global keyboard shortcuts
 * Automatically excludes shortcuts when typing in text inputs
 */
export function useKeyboardShortcuts(actions: KeyboardShortcutActions) {
  const actionsRef = useRef(actions);
  
  // Update ref when actions change
  useEffect(() => {
    actionsRef.current = actions;
  }, [actions]);

  const handleKeyDown = useCallback((event: KeyboardEvent) => {
    // Don't trigger shortcuts when typing in text inputs, textareas, or contenteditable
    const target = event.target as HTMLElement;
    const isTyping = 
      target.tagName === 'INPUT' ||
      target.tagName === 'TEXTAREA' ||
      target.isContentEditable;

    // Ctrl+K focuses search even while typing. A bare "/" does so only outside
    // text fields; inside one it is a character the user is entering ("r/pics").
    const isSearchFocusShortcut =
      (event.key === 'k' && event.ctrlKey && !event.shiftKey && !event.altKey && !event.metaKey) ||
      (event.key === '/' && !isTyping && !event.ctrlKey && !event.shiftKey && !event.altKey && !event.metaKey);
    
    if (isSearchFocusShortcut && actionsRef.current.onFocusSearch) {
      event.preventDefault();
      actionsRef.current.onFocusSearch();
      return;
    }

    // For all other shortcuts, skip if typing in an input
    if (isTyping) {
      return;
    }

    const { key, ctrlKey, metaKey, shiftKey, altKey } = event;
    
    // Ctrl+K or / - Focus search
    if ((key === 'k' && ctrlKey) || key === '/') {
      event.preventDefault();
      actionsRef.current.onFocusSearch?.();
      return;
    }

    // Ctrl+B - Toggle sidebar
    if (key === 'b' && ctrlKey && !shiftKey && !altKey) {
      event.preventDefault();
      actionsRef.current.onToggleSidebar?.();
      return;
    }

    // F - Fit graph to screen
    if (
      key === 'f' &&
      !ctrlKey &&
      !metaKey &&
      !shiftKey &&
      !altKey &&
      actionsRef.current.onFitGraph
    ) {
      event.preventDefault();
      actionsRef.current.onFitGraph();
      return;
    }

    // R - Reset camera
    if (
      key === 'r' &&
      !ctrlKey &&
      !metaKey &&
      !shiftKey &&
      !altKey &&
      actionsRef.current.onResetCamera
    ) {
      event.preventDefault();
      actionsRef.current.onResetCamera();
      return;
    }

    // 1 - Switch to 3D view
    if (key === '1' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      event.preventDefault();
      actionsRef.current.onSwitch3D?.();
      return;
    }

    // 2 - Switch to 2D view
    if (key === '2' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      event.preventDefault();
      actionsRef.current.onSwitch2D?.();
      return;
    }

    // 3 - Switch to Community view
    if (key === '3' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      event.preventDefault();
      actionsRef.current.onSwitchCommunity?.();
      return;
    }

    // L - Toggle labels
    if (key === 'l' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      event.preventDefault();
      actionsRef.current.onToggleLabels?.();
      return;
    }

    // Escape - Deselect / Close panels
    if (key === 'Escape') {
      event.preventDefault();
      actionsRef.current.onEscape?.();
      return;
    }

    // ? or F1 - Show help (ignore auto-repeat to prevent rapid toggling)
    if (((key === '?' && !ctrlKey && !metaKey && !altKey) || key === 'F1') && !event.repeat) {
      event.preventDefault();
      actionsRef.current.onShowHelp?.();
      return;
    }

    // Arrow keys - Navigate between nodes (only when handler exists)
    if (key === 'ArrowUp' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      const handler = actionsRef.current.onArrowUp;
      if (handler) {
        event.preventDefault();
        handler();
      }
      return;
    }

    if (key === 'ArrowDown' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      const handler = actionsRef.current.onArrowDown;
      if (handler) {
        event.preventDefault();
        handler();
      }
      return;
    }

    if (key === 'ArrowLeft' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      const handler = actionsRef.current.onArrowLeft;
      if (handler) {
        event.preventDefault();
        handler();
      }
      return;
    }

    if (key === 'ArrowRight' && !ctrlKey && !metaKey && !shiftKey && !altKey) {
      const handler = actionsRef.current.onArrowRight;
      if (handler) {
        event.preventDefault();
        handler();
      }
      return;
    }
  }, []);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [handleKeyDown]);
}
