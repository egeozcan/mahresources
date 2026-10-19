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

  it('table: an older fetch response never overwrites a newer one', async () => {
    const table: any = reactive(blockTable(
      { id: 1, content: { queryId: 2 }, state: {} },
      () => Promise.resolve(true),
      () => {},
      () => true,
      () => {},
    ));
    const gates: Array<(v: unknown) => void> = [];
    table.fetchFromServer = () => new Promise((r) => gates.push(r));
    table.fetchQueryData(true);
    table.queryId = 3;
    table.fetchQueryData(true);
    gates[1]({ columns: [], rows: [{ q: 3 }] });
    await flush();
    gates[0]({ columns: [], rows: [{ q: 2 }] });
    await flush();
    expect(table.queryRows).toEqual([{ q: 3 }]);
  });

  it('calendar: an older fetch response never overwrites a newer one', async () => {
    const cal: any = reactive(blockCalendar(
      { id: 1, content: { calendars: [{ id: 'a', name: 'a', color: '#000', source: { type: 'url', url: 'x' } }] }, state: {} },
      () => Promise.resolve(true),
      () => {},
      () => true,
      1,
      () => {},
    ));
    const gates: Array<(v: unknown) => void> = [];
    cal.fetchFromServer = () => new Promise((r) => gates.push(r));
    cal.fetchEvents(true);
    cal.fetchEvents(true);
    gates[1]({ events: [{ id: 'new' }] });
    await flush();
    gates[0]({ events: [{ id: 'old' }] });
    await flush();
    expect(cal.events.map((e: any) => e.id)).toEqual(['new']);
  });

  it('calendar: removing the last calendar is not undone by an older fetch', async () => {
    const cal: any = reactive(blockCalendar(
      { id: 1, content: { calendars: [{ id: 'a', name: 'a', color: '#000', source: { type: 'url', url: 'x' } }] }, state: {} },
      () => Promise.resolve(true), () => {}, () => true, 1, () => {},
    ));
    const gates: Array<(v: unknown) => void> = [];
    cal.fetchFromServer = () => new Promise((r) => gates.push(r));
    cal.fetchEvents(true);
    cal.calendars = [];
    cal.fetchEvents(true);
    gates[0]({ events: [{ id: 'old' }] });
    await flush();
    expect(cal.events).toEqual([]);
  });

  it('calendar: a background refresh older than a forced fetch is dropped', async () => {
    const cal: any = reactive(blockCalendar(
      { id: 1, content: { calendars: [{ id: 'a', name: 'a', color: '#000', source: { type: 'url', url: 'x' } }] }, state: {} },
      () => Promise.resolve(true), () => {}, () => true, 1, () => {},
    ));
    const gates: Array<(v: unknown) => void> = [];
    cal.fetchFromServer = () => new Promise((r) => gates.push(r));
    const { start, end } = cal.fetchRange;
    const key = `1:${cal.formatDate(start)}:${cal.formatDate(end)}`;
    cal.backgroundRefresh(key, start, end);
    cal.fetchEvents(true);
    gates[1]({ events: [{ id: 'new' }] });
    await flush();
    gates[0]({ events: [{ id: 'old' }] });
    await flush();
    expect(cal.events.map((e: any) => e.id)).toEqual(['new']);
  });

  it('table: a background refresh older than a forced fetch is dropped', async () => {
    const table: any = reactive(blockTable(
      { id: 1, content: { queryId: 2 }, state: {} },
      () => Promise.resolve(true), () => {}, () => true, () => {},
    ));
    const gates: Array<(v: unknown) => void> = [];
    table.fetchFromServer = () => new Promise((r) => gates.push(r));
    table.backgroundRefresh('k');
    table.fetchQueryData(true);
    gates[1]({ columns: [], rows: [{ v: 'new' }] });
    await flush();
    gates[0]({ columns: [], rows: [{ v: 'old' }] });
    await flush();
    expect(table.queryRows).toEqual([{ v: 'new' }]);
  });

  // Alpine writes a property a nested x-data never declared to the OUTERMOST
  // scope (the blockEditor), so undeclared per-block state is silently shared
  // by every table and calendar on the note: one block's fetch then
  // invalidated another's. Each must declare its own.
  it('table and calendar declare their per-block fetch state', () => {
    const table = blockTable({ id: 1, content: {}, state: {} }, () => {}, () => {}, () => true, () => {});
    const cal = blockCalendar({ id: 1, content: {}, state: {} }, () => {}, () => {}, () => true, 1, () => {});
    for (const obj of [table, cal]) {
      for (const key of ['_fetchGeneration', '_refetchOnSave', '_onBlockSaved']) {
        expect(Object.prototype.hasOwnProperty.call(obj, key), key).toBe(true);
      }
    }
  });

  it('calendar: a calendar change drops cached events for every range', async () => {
    const cal: any = reactive(blockCalendar(
      { id: 77, content: { calendars: [
        { id: 'a', name: 'a', color: '#000', source: { type: 'url', url: 'x' } },
        { id: 'b', name: 'b', color: '#111', source: { type: 'url', url: 'y' } },
      ] }, state: {} },
      () => Promise.resolve(true), () => {}, () => true, 1, () => {},
    ));
    let served = [{ id: 'with-a' }];
    cal.fetchFromServer = () => Promise.resolve({ events: served });
    await cal.fetchEvents(true); // caches this month with calendar a
    cal.currentDate = new Date(cal.currentDate.getFullYear(), cal.currentDate.getMonth() + 1, 1);
    served = [];
    cal.removeCalendar('a');
    await flush();
    cal.currentDate = new Date(cal.currentDate.getFullYear(), cal.currentDate.getMonth() - 1, 1);
    let fetched = false;
    cal.fetchFromServer = () => { fetched = true; return Promise.resolve({ events: [] }); };
    await cal.fetchEvents();
    expect(fetched).toBe(true);
  });
});
