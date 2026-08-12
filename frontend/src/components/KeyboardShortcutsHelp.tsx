import { useEffect, useRef } from 'react';
import { SHORTCUTS, type KeyboardShortcut } from '../hooks/useKeyboardShortcuts';

interface KeyboardShortcutsHelpProps {
  isOpen: boolean;
  onClose: () => void;
}

function formatShortcut(shortcut: KeyboardShortcut): string {
  const parts: string[] = [];
  
  if (shortcut.ctrl) parts.push('Ctrl');
  if (shortcut.meta) parts.push('Cmd');
  if (shortcut.alt) parts.push('Alt');
  if (shortcut.shift) parts.push('Shift');
  
  // Format the key
  let key = shortcut.key;
  if (key.startsWith('Arrow')) {
    const direction = key.replace('Arrow', '');
    const arrowSymbols: Record<string, string> = {
      Up: '↑',
      Down: '↓',
      Left: '←',
      Right: '→',
    };
    const symbol = arrowSymbols[direction] ?? '';
    key = `${direction} ${symbol}`.trimEnd();
  } else if (key === 'Escape') {
    key = 'Esc';
  }
  
  parts.push(key.toUpperCase());
  
  return parts.join('+');
}

export default function KeyboardShortcutsHelp({ isOpen, onClose }: KeyboardShortcutsHelpProps) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!isOpen) return;
    const priorFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeRef.current?.focus();
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab' || !dialogRef.current) return;
      const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>('button, [href], [tabindex]:not([tabindex="-1"])'));
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => { document.removeEventListener('keydown', handleKeyDown); priorFocus?.focus(); };
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  // Group shortcuts by category
  const categorized = SHORTCUTS.reduce((acc, shortcut) => {
    if (!acc[shortcut.category]) {
      acc[shortcut.category] = [];
    }
    acc[shortcut.category].push(shortcut);
    return acc;
  }, {} as Record<string, KeyboardShortcut[]>);

  // Category order
  const categoryOrder: Array<'Search' | 'Navigation' | 'View' | 'Help'> = [
    'Search',
    'Navigation',
    'View',
    'Help',
  ];

  return (
    <div
      className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/80 p-4 backdrop-blur-sm"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="shortcuts-title"
    >
      <div
        ref={dialogRef}
        className="instrument-panel max-h-[80vh] w-full max-w-2xl overflow-hidden rounded-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between border-b border-white/10 px-6 py-4">
          <h2 id="shortcuts-title" className="text-xl font-semibold text-white">
            Keyboard Shortcuts
          </h2>
          <button
            ref={closeRef}
            onClick={onClose}
            className="instrument-button rounded-full px-3 text-[#9aaba8]"
            aria-label="Close shortcuts help"
          >
            <svg
              className="w-6 h-6"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>
        </div>

        {/* Content */}
        <div className="px-6 py-4 overflow-y-auto max-h-[calc(80vh-5rem)]">
          {categoryOrder.map((category) => {
            const shortcuts = categorized[category];
            if (!shortcuts || shortcuts.length === 0) return null;

            return (
              <div key={category} className="mb-6 last:mb-0">
                <h3 className="instrument-label mb-3">
                  {category}
                </h3>
                <div className="space-y-2">
                  {shortcuts.map((shortcut, index) => (
                    <div
                      key={`${shortcut.key}-${index}`}
                      className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-white/5"
                    >
                      <span className="text-[#c7d2cf]">
                        {shortcut.description}
                      </span>
                      <kbd className="inline-flex items-center gap-1 rounded border border-white/15 bg-white/5 px-2 py-1 font-mono text-xs font-semibold text-white">
                        {formatShortcut(shortcut)}
                      </kbd>
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>

        {/* Footer */}
        <div className="border-t border-white/10 bg-white/[.02] px-6 py-3">
          <p className="text-center text-sm text-[#9aaba8]">
            Press <kbd className="rounded bg-white/5 px-1.5 py-0.5 font-mono text-xs">Esc</kbd> or{' '}
            <kbd className="rounded bg-white/5 px-1.5 py-0.5 font-mono text-xs">?</kbd> to close
          </p>
        </div>
      </div>
    </div>
  );
}
