/**
 * Selecting a query saves the block's configuration and then fetches its rows.
 * The fetch reads the persisted configuration, so it must wait for the save:
 * with another save for the block still in flight, the queued save lands
 * later and the rows came back for the previous query.
 */
import { describe, it, expect, vi, afterEach } from 'vitest';
import { reactive } from '@vue/reactivity';
import { blockTable } from './blockTable.js';
import { blockCalendar } from './blockCalendar.js';

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
});
