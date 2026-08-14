import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { perfMonitor } from './performance';

type MutableMonitor = typeof perfMonitor & { enabled: boolean };

describe('perfMonitor', () => {
  const monitor = perfMonitor as MutableMonitor;

  beforeEach(() => {
    monitor.clearStats();
    monitor.enabled = true;
  });

  afterEach(() => {
    vi.restoreAllMocks();
    monitor.clearStats();
  });

  it('bypasses timing when disabled for sync and async work', async () => {
    monitor.enabled = false;

    expect(monitor.measure('sync', () => 42)).toBe(42);
    await expect(monitor.measureAsync('async', async () => 'done')).resolves.toBe('done');
    expect(monitor.getAllStats().size).toBe(0);
  });

  it('aggregates timings and warns when work exceeds a frame budget', () => {
    vi.spyOn(performance, 'now')
      .mockReturnValueOnce(0)
      .mockReturnValueOnce(5)
      .mockReturnValueOnce(10)
      .mockReturnValueOnce(30);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined);

    expect(monitor.measure('render', () => 'first')).toBe('first');
    expect(monitor.measure('render', () => 'second')).toBe('second');

    expect(monitor.getStats('render')).toEqual({
      totalMs: 25,
      lastMs: 20,
      minMs: 5,
      maxMs: 20,
      count: 2,
    });
    expect(warn).toHaveBeenCalledOnce();
  });

  it('records async work and returns a defensive stats map', async () => {
    vi.spyOn(performance, 'now').mockReturnValueOnce(2).mockReturnValueOnce(8);

    await expect(monitor.measureAsync('load', async () => 'loaded')).resolves.toBe('loaded');
    const snapshot = monitor.getAllStats();
    snapshot.clear();

    expect(monitor.getStats('load')?.lastMs).toBe(6);
    expect(monitor.getStats('missing')).toBeUndefined();
  });

  it('logs a summary only when enabled stats exist', () => {
    const group = vi.spyOn(console, 'group').mockImplementation(() => undefined);
    const log = vi.spyOn(console, 'log').mockImplementation(() => undefined);
    const groupEnd = vi.spyOn(console, 'groupEnd').mockImplementation(() => undefined);

    monitor.logSummary();
    expect(group).not.toHaveBeenCalled();

    vi.spyOn(performance, 'now').mockReturnValueOnce(1).mockReturnValueOnce(4);
    monitor.measure('layout', () => undefined);
    monitor.logSummary();

    expect(group).toHaveBeenCalledWith('[Performance Summary]');
    expect(log).toHaveBeenCalledWith(expect.stringContaining('layout: avg=3.00ms'));
    expect(groupEnd).toHaveBeenCalledOnce();

    monitor.clearStats();
    expect(monitor.getAllStats().size).toBe(0);
    monitor.enabled = false;
    monitor.logSummary();
    expect(group).toHaveBeenCalledOnce();
  });
});
