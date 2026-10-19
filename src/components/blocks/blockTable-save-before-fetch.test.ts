/**
 * Selecting a query saves the block's configuration and then fetches its rows.
 * The fetch reads the persisted configuration, so it must wait for the save:
 * with another save for the block still in flight, the queued save lands
 * later and the rows came back for the previous query.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { reactive } from '@vue/reactivity';
import { blockTable } from './blockTable.js';
import { blockCalendar } from './blockCalendar.js';

// The blocks listen for mah:block-saved on window; Node has none.
beforeEach(() => vi.stubGlobal('window', new EventTarget()));
afterEach(() => vi.unstubAllGlobals());

const flush = () => new Promise((r) => setTimeout(r, 0));

function gate() {
  let open!: (v: boolean) => void;
  const promise = new Promise<boolean>((r) => { open = r; });
  return { promise, open };
}

describe('dependent reads wait for the block save', () => {
  it('table: selecting a query fetches rows only after the save lands', async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    const saved = gate();
    const table: any = reactive(blockTable(
      { id: 1, content: {}, state: {} },
      () => saved.promise,
      () => {},
      () => true,
      () => {},
    ));
    table.selectQuery({ ID: 2, Name: 'two' });
    await flush();
    expect(fetchMock).not.toHaveBeenCalled();
    saved.open(true);
    await flush();
    expect(fetchMock).toHaveBeenCalled();
  });

  it('calendar: adding a calendar fetches events only after the save lands', async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    const saved = gate();
    const cal: any = reactive(blockCalendar(
      { id: 1, content: { calendars: [] }, state: {} },
      () => saved.promise,
      () => {},
      () => true,
      1,
      () => {},
    ));
    cal.addCalendarFromResource(5, 'cal');
    await flush();
    expect(fetchMock).not.toHaveBeenCalled();
    saved.open(true);
    await flush();
    expect(fetchMock).toHaveBeenCalled();
  });

  it('table: a failed save skips the fetch and refetches once a later save lands', async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    const table: any = reactive(blockTable(
      { id: 1, content: {}, state: {} },
      () => Promise.resolve(false),
      () => {},
      () => true,
      () => {},
    ));
    table.init();
    table.selectQuery({ ID: 2, Name: 'two' });
    await flush();
    expect(fetchMock).not.toHaveBeenCalled();
    window.dispatchEvent(new CustomEvent('mah:block-saved', { detail: { blockId: 1 } }));
    await flush();
    expect(fetchMock).toHaveBeenCalled();
    table.destroy();
  });

  it('calendar: a failed save skips the fetch and refetches once a later save lands', async () => {
    const fetchMock = vi.fn(() => new Promise<Response>(() => {}));
    vi.stubGlobal('fetch', fetchMock);
    const cal: any = reactive(blockCalendar(
      { id: 1, content: { calendars: [] }, state: {} },
      () => Promise.resolve(false),
      () => {},
      () => true,
      1,
      () => {},
    ));
    await cal.init();
    cal.addCalendarFromResource(5, 'cal');
    await flush();
    expect(fetchMock).not.toHaveBeenCalled();
    window.dispatchEvent(new CustomEvent('mah:block-saved', { detail: { blockId: 1 } }));
    await flush();
    expect(fetchMock).toHaveBeenCalled();
    cal.destroy();
  });
});
