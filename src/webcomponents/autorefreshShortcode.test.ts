// Which [autorefresh] blocks a save refreshes, and how saves close together are
// coalesced. Both are pure decisions that an E2E test can only sample.
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';

beforeAll(() => {
  vi.stubGlobal('HTMLElement', class {});
  vi.stubGlobal('customElements', { get: () => undefined, define: () => {} });
  vi.stubGlobal('window', {});
  vi.stubGlobal('document', { addEventListener: () => {} });
});

let hostsToRefresh: (hosts: any[], detail: any, captured?: any[]) => any[];
let createRefreshScheduler: (run: (hosts: any[]) => void, delay?: number) => { add(hosts: any[]): void };

beforeAll(async () => {
  ({ hostsToRefresh, createRefreshScheduler } = await import('./autorefreshShortcode.js'));
});

function host(entityType: string, entityId: number, children: any[] = []) {
  const attrs: Record<string, string> = {
    'data-entity-type': entityType,
    'data-entity-id': String(entityId),
  };
  const el: any = {
    isConnected: true,
    getAttribute: (k: string) => attrs[k] ?? null,
    contains: (other: any) => other === el || children.some((c) => c === other || c.contains?.(other)),
  };
  return el;
}

describe('hostsToRefresh', () => {
  it('matches every block rendered for the saved entity', () => {
    const a = host('group', 1);
    const b = host('group', 1);
    const other = host('group', 2);
    const note = host('note', 1);
    expect(hostsToRefresh([a, b, other, note], { entityType: 'group', entityId: 1 })).toEqual([a, b]);
  });

  it('matches a block holding the editor that saved, whatever entity it edited', () => {
    const editor = { contains: () => false };
    const holder = host('group', 1, [editor]);
    expect(hostsToRefresh([holder], { entityType: 'resource', entityId: 9, source: editor })).toEqual([holder]);
  });

  it('still matches the block an editor was in when its save began, after a refresh replaced it', () => {
    // The editor that saved has left the page: a refresh landed while it was
    // saving and rebuilt the block around it.
    const detachedEditor = { contains: () => false };
    const holder = host('group', 1);
    expect(
      hostsToRefresh([holder], { entityType: 'resource', entityId: 9, source: detachedEditor }, [holder]),
    ).toEqual([holder]);
  });

  it('ignores a captured block that has itself left the page', () => {
    const holder = host('group', 1);
    holder.isConnected = false;
    expect(hostsToRefresh([], { entityType: 'resource', entityId: 9 }, [holder])).toEqual([]);
  });

  it('keeps only the outermost of nested matches', () => {
    const inner = host('group', 1);
    const outer = host('group', 1, [inner]);
    expect(hostsToRefresh([outer, inner], { entityType: 'group', entityId: 1 })).toEqual([outer]);
  });

  it('skips a block that has left the page', () => {
    const gone = host('group', 1);
    gone.isConnected = false;
    expect(hostsToRefresh([gone], { entityType: 'group', entityId: 1 })).toEqual([]);
  });

  it('compares ids across number and string forms', () => {
    const a = host('group', 12);
    expect(hostsToRefresh([a], { entityType: 'group', entityId: '12' })).toEqual([a]);
  });
});

describe('createRefreshScheduler', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('refreshes once for saves inside the window', () => {
    vi.useFakeTimers();
    const run = vi.fn();
    const scheduler = createRefreshScheduler(run, 250);
    const a = host('group', 1);

    scheduler.add([a]);
    vi.advanceTimersByTime(200);
    scheduler.add([a]);
    vi.advanceTimersByTime(200);
    expect(run).not.toHaveBeenCalled();

    vi.advanceTimersByTime(50);
    expect(run).toHaveBeenCalledTimes(1);
    expect(run).toHaveBeenCalledWith([a]);
  });

  it('merges blocks from different saves and drops nested ones', () => {
    vi.useFakeTimers();
    const run = vi.fn();
    const scheduler = createRefreshScheduler(run, 250);
    const inner = host('group', 1);
    const outer = host('note', 4, [inner]);
    const sibling = host('group', 1);

    scheduler.add([inner, sibling]);
    scheduler.add([outer]);
    vi.advanceTimersByTime(250);

    expect(run).toHaveBeenCalledWith([sibling, outer]);
  });

  it('does not schedule anything for a save that matched nothing', () => {
    vi.useFakeTimers();
    const run = vi.fn();
    const scheduler = createRefreshScheduler(run, 250);

    scheduler.add([]);
    vi.advanceTimersByTime(1000);

    expect(run).not.toHaveBeenCalled();
  });
});
