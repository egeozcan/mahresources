import { describe, expect, it, vi } from 'vitest';
import { createPickerSession } from './pickerSession';
import type { BrowsePage, EntityBrowseMetadata, EntityBrowseSource } from '../../selector/entityBrowseTypes';

const value = (ID: number) => ({ ID, Name: `Item ${ID}` });
const page = (ID = 1): BrowsePage => ({ items: [{ value: value(ID), html: '' }], page: 1, hasNext: true, styles: [], warnings: [] });
const browse: EntityBrowseMetadata = { entity: 'group', multiple: true, parameters: () => ({}), excludedKeys: () => [] };
const source: EntityBrowseSource = { search: async () => page(), resolve: async ({ ids }) => ids.map(value) };
const flush = async () => { await Promise.resolve(); await Promise.resolve(); };
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(r => { resolve = r; }); return { promise, resolve }; }

describe('picker session', () => {
 it('adds only pending values, explicitly, preserving choices across pages and filters', async () => {
  const onConfirm = vi.fn(() => true), session = createPickerSession(source);
  session.open({ browse, existing: [value(1)], onConfirm }); await flush();
  session.toggle(value(1)); session.toggle(value(2)); session.setPage(2); session.setFilter('Name=another'); await flush();
  expect(session.snapshot().steps[0].pending).toEqual([value(2)]);
  expect(onConfirm).not.toHaveBeenCalled();
  expect(await session.confirm()).toBe(true);
  expect(onConfirm).toHaveBeenCalledWith([value(2)]);
  expect(session.snapshot().isOpen).toBe(false);
 });
 it('replaces a single choice only on confirm and respects zero/remaining capacity', async () => {
  const onConfirm = vi.fn(() => true), session = createPickerSession(source);
  session.open({ browse: { ...browse, multiple: false }, existing: [value(1)], onConfirm });
  session.toggle(value(2)); session.toggle(value(3)); await session.confirm();
  expect(onConfirm).toHaveBeenCalledWith([value(3)]);
  session.open({ browse: { ...browse, maximum: 2 }, existing: [value(1)], onConfirm });
  session.toggle(value(2));session.toggle(value(3));
  expect(session.snapshot().steps[0].pending).toEqual([value(2)]);
  session.open({ browse: { ...browse, maximum: 0 }, existing: [], onConfirm });session.toggle(value(2));
  expect(session.snapshot().steps[0].pending).toEqual([]);
 });
 it('ignores old responses after query, push, back, cancel and reopen even without transport abortion', async () => {
  const reads: ReturnType<typeof deferred<BrowsePage>>[] = [];
  const session = createPickerSession({ ...source, search: () => { const r = deferred<BrowsePage>(); reads.push(r); return r.promise; } });
  const options = { browse, existing: [], onConfirm: () => true };
  session.open(options);session.setFilter('Name=new');
  reads[0].resolve(page(10));await flush();expect(session.snapshot().steps[0].status).toBe('loading');
  reads[1].resolve(page(20));await flush();
  session.push(options);session.back();reads[2].resolve(page(30));await flush();
  expect(session.snapshot().steps[0].items[0].value.ID).toBe(20);
  session.setFilter('Name=cancelled');session.cancel();session.open(options);
  reads[3].resolve(page(40));await flush();expect(session.snapshot().steps[0].status).toBe('loading');
  reads[4].resolve(page(50));await flush();expect(session.snapshot().steps[0].items[0].value.ID).toBe(50);
 });
 it('keeps the parent alive and targets its filter by identity during child confirmation', async () => {
  const session = createPickerSession(source), onDispose = vi.fn();
  session.open({ browse, existing: [], onConfirm: () => true, onDispose });await flush();
  const parent = session.snapshot().steps[0].id;
  session.toggle(value(2));session.setPage(2);await flush();
  session.push({ browse, existing: [], onConfirm: () => { session.setFilter('Categories=3', parent);return true; } });
  session.toggle(value(3));await session.confirm();await flush();
  expect(session.snapshot().steps).toHaveLength(1);
  expect(session.snapshot().steps[0]).toMatchObject({ filter: 'Categories=3', page: 1, pending: [value(2)] });
  expect(onDispose).not.toHaveBeenCalled();session.cancel();expect(onDispose).toHaveBeenCalledOnce();
 });
 it('retains choices on refusal/error, disallows duplicate confirmation and ignores stale completion', async () => {
  const session = createPickerSession(source), pending = deferred<boolean>();
  const onConfirm = vi.fn(() => pending.promise);
  session.open({ browse, existing: [], onConfirm });session.toggle(value(2));
  const first = session.confirm();expect(await session.confirm()).toBe(false);expect(onConfirm).toHaveBeenCalledOnce();
  pending.resolve(false);expect(await first).toBe(false);expect(session.snapshot().steps[0].pending).toEqual([value(2)]);
  session.open({ browse, existing: [], onConfirm: () => {throw new Error('Refresh needed');} });session.toggle(value(2));
  expect(await session.confirm()).toBe(false);expect(session.snapshot().steps[0].error).toContain('Refresh needed');
  const stale = deferred<boolean>();
  session.open({ browse, existing: [], onConfirm: () => stale.promise });session.toggle(value(2));const second = session.confirm();
  session.cancel();session.open({ browse, existing: [], onConfirm: () => true });stale.resolve(true);await second;
  expect(session.snapshot().isOpen).toBe(true);
 });
 it('publishes immutable snapshots and cannot reopen after destruction', () => {
  const session = createPickerSession(source);session.open({ browse, existing: [], onConfirm: () => true });
  expect(Object.isFrozen(session.snapshot().steps)).toBe(true);
  session.destroy();session.open({ browse, existing: [], onConfirm: () => true });expect(session.snapshot().isOpen).toBe(false);
 });
});
