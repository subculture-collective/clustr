import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useKeyboardShortcuts, type KeyboardShortcutActions } from './useKeyboardShortcuts';

describe('useKeyboardShortcuts', () => {
  let actions: KeyboardShortcutActions;

  beforeEach(() => {
    // Create mock actions
    actions = {
      onFocusSearch: vi.fn(),
      onToggleSidebar: vi.fn(),
      onFitGraph: vi.fn(),
      onResetCamera: vi.fn(),
      onSwitch3D: vi.fn(),
      onSwitch2D: vi.fn(),
      onSwitchCommunity: vi.fn(),
      onToggleLabels: vi.fn(),
      onEscape: vi.fn(),
      onShowHelp: vi.fn(),
      onArrowUp: vi.fn(),
      onArrowDown: vi.fn(),
      onArrowLeft: vi.fn(),
      onArrowRight: vi.fn(),
    };
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('should call onFocusSearch when Ctrl+K is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: 'k',
      ctrlKey: true,
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onFocusSearch).toHaveBeenCalledTimes(1);
  });

  it('should call onFocusSearch when / is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: '/',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onFocusSearch).toHaveBeenCalledTimes(1);
  });

  it('should call onToggleSidebar when Ctrl+B is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: 'b',
      ctrlKey: true,
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onToggleSidebar).toHaveBeenCalledTimes(1);
  });

  it('should call onSwitch3D when 1 is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: '1',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onSwitch3D).toHaveBeenCalledTimes(1);
  });

  it('should call onSwitch2D when 2 is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: '2',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onSwitch2D).toHaveBeenCalledTimes(1);
  });

  it('should call onSwitchCommunity when 3 is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: '3',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onSwitchCommunity).toHaveBeenCalledTimes(1);
  });

  it('should call onToggleLabels when L is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: 'l',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onToggleLabels).toHaveBeenCalledTimes(1);
  });

  it('should call onEscape when Escape is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: 'Escape',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onEscape).toHaveBeenCalledTimes(1);
  });

  it('should call onShowHelp when ? is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: '?',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onShowHelp).toHaveBeenCalledTimes(1);
  });

  it('should call onShowHelp when F1 is pressed', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const event = new KeyboardEvent('keydown', {
      key: 'F1',
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onShowHelp).toHaveBeenCalledTimes(1);
  });

  it('should call arrow key handlers', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const upEvent = new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true });
    document.dispatchEvent(upEvent);
    expect(actions.onArrowUp).toHaveBeenCalledTimes(1);

    const downEvent = new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true });
    document.dispatchEvent(downEvent);
    expect(actions.onArrowDown).toHaveBeenCalledTimes(1);

    const leftEvent = new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true });
    document.dispatchEvent(leftEvent);
    expect(actions.onArrowLeft).toHaveBeenCalledTimes(1);

    const rightEvent = new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true });
    document.dispatchEvent(rightEvent);
    expect(actions.onArrowRight).toHaveBeenCalledTimes(1);
  });

  it('should not trigger shortcuts when typing in input', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    // Create an input element and make it the target
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    const event = new KeyboardEvent('keydown', {
      key: 'l',
      bubbles: true,
    });
    input.dispatchEvent(event);

    expect(actions.onToggleLabels).not.toHaveBeenCalled();

    document.body.removeChild(input);
  });

  it('should not trigger shortcuts when typing in textarea', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const textarea = document.createElement('textarea');
    document.body.appendChild(textarea);
    textarea.focus();

    const event = new KeyboardEvent('keydown', {
      key: 'f',
      bubbles: true,
    });
    textarea.dispatchEvent(event);

    expect(actions.onFitGraph).not.toHaveBeenCalled();

    document.body.removeChild(textarea);
  });

  it('should allow Ctrl+K to work even when typing in input', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    const event = new KeyboardEvent('keydown', {
      key: 'k',
      ctrlKey: true,
      bubbles: true,
    });
    input.dispatchEvent(event);

    expect(actions.onFocusSearch).toHaveBeenCalledTimes(1);

    document.body.removeChild(input);
  });

  it('should let / be typed into an input', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    const event = new KeyboardEvent('keydown', {
      key: '/',
      bubbles: true,
      cancelable: true,
    });
    input.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(actions.onFocusSearch).not.toHaveBeenCalled();

    document.body.removeChild(input);
  });

  it('should not trigger action if modifier key is wrong', () => {
    renderHook(() => useKeyboardShortcuts(actions));

    // Press L with Ctrl - should not toggle labels (requires no modifiers)
    const event = new KeyboardEvent('keydown', {
      key: 'l',
      ctrlKey: true,
      bubbles: true,
    });
    document.dispatchEvent(event);

    expect(actions.onToggleLabels).not.toHaveBeenCalled();
  });
});
