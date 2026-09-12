import { describe, expect, it, vi } from 'vitest';
import { createEntityBrowseConfirmation, type EntityBrowseOrigin } from './entityBrowseIntegration';
import { createMultiEntityFieldProfile } from './entityFieldProfiles';
import { createTagEditorProfile } from './tagEditorProfile';
import type { BrowseValue, EntityBrowseSource } from './entityBrowseTypes';
const value = (ID: number): BrowseValue => ({ ID, Name: `Item ${ID}` });
const source: EntityBrowseSource = { search: vi.fn(), resolve: async ({ ids }) => ids.map(value) };
function fixture(multiple = true) {
 let values = [value(1)], available = true, params = {}, exclusions: number[] = [], maximum: number | undefined;
 const replace = vi.fn((next: readonly BrowseValue[]) => { values = [...next]; });
 const origin: EntityBrowseOrigin = {
  metadata: { entity: 'group', multiple, get maximum() { return maximum; }, parameters: () => params, excludedKeys: () => exclusions },
  getValues: () => values, isAvailable: () => available, replace,
 };
 return { origin, replace, setValues: (next: BrowseValue[]) => { values = next; }, disable: () => { available = false; }, changeFilter: () => { params = { Categories: 7 }; }, exclude: () => { exclusions = [2]; }, reduce: () => { maximum = 1; } };
}
describe('entity browse confirmation', () => {
 it('appends atomically to latest values, deduplicates and reconfirms without a change', async () => {
  const f = fixture(), bridge = createEntityBrowseConfirmation(f.origin, source);
  expect(await bridge.confirm([value(2), value(2)])).toBe(true);
  expect(f.replace).toHaveBeenCalledOnce();expect(f.origin.getValues()).toEqual([value(1), value(2)]);
  expect(await bridge.confirm([value(2)])).toBe(true);expect(f.replace).toHaveBeenCalledOnce();
 });
 it('replaces a single selection and publishes exactly one non-silent core change', async () => {
  const f = fixture(false);await createEntityBrowseConfirmation(f.origin, source).confirm([value(2)]);
  expect(f.origin.getValues()).toEqual([value(2)]);
  const profile = createMultiEntityFieldProfile({ entity: 'group', selected: [value(1)] });
  const changed = vi.fn();profile.selector.subscribe((_state, change) => { if (change) changed(change); });
  const bridge = createEntityBrowseConfirmation({ metadata: profile.browse, isAvailable: () => true,
   getValues: () => profile.selector.getSnapshot().selected.map(v => v.raw),
   replace: values => { profile.selector.dispatch({ type: 'replace-selection', silent: false, options: values.map(raw => ({ key: raw.ID, label: raw.Name, raw })) }); },
  }, source);
  await bridge.confirm([value(2), value(3)]);expect(changed).toHaveBeenCalledOnce();profile.selector.destroy();
 });
 it.each(['disable','changeFilter','exclude','reduce','destroy'])('never commits if %s occurs during resolution', async action => {
  const f = fixture();let finish!: (values: BrowseValue[]) => void;
  const bridge = createEntityBrowseConfirmation(f.origin, { ...source, resolve: () => new Promise(resolve => { finish = resolve; }) });
  const pending = bridge.confirm([value(2)]);
  if (action === 'destroy') bridge.destroy();else f[action]();
  finish([value(2)]);await pending.catch(() => false);expect(f.replace).not.toHaveBeenCalled();
 });
 it('appends against changes made externally while resolving', async () => {
  const f = fixture();let finish!: (values: BrowseValue[]) => void;
  const bridge = createEntityBrowseConfirmation(f.origin, { ...source, resolve: () => new Promise(resolve => { finish = resolve; }) });
  const pending = bridge.confirm([value(2)]);f.setValues([value(3)]);finish([value(2)]);expect(await pending).toBe(true);
  expect(f.origin.getValues()).toEqual([value(3), value(2)]);
 });
 it('preserves pending tag writes when a browser adds another tag', async () => {
  const add = vi.fn((_tag: BrowseValue, _signal: AbortSignal) => new Promise<void>(() => {}));
  const profile = createTagEditorProfile({ usage: 'group', association: { add, remove: async () => {} } });
  profile.selector.dispatch({ type: 'select-option', option: { key: 1, label: 'Item 1', raw: value(1) } });
  const signal = add.mock.calls[0][1];
  const bridge = createEntityBrowseConfirmation({ metadata: profile.browse, isAvailable: () => true,
   getValues: () => profile.selector.getSnapshot().selected.map(v => v.raw),
   replace: values => {profile.selector.dispatch({ type: 'replace-selection', reason: 'reset', silent: false, options: values.map(raw => ({key: raw.ID, label: raw.Name, raw})) });},
  }, source);
  await bridge.confirm([value(2)]);
  expect(signal.aborted).toBe(false);
  expect(profile.getSnapshot().pendingKeys).toEqual(['1','2']);
  profile.destroy();
 });
 it('refuses missing rows and transport failures without publishing partial changes', async () => {
  const f = fixture();
  await expect(createEntityBrowseConfirmation(f.origin, { ...source, resolve: async () => [] }).confirm([value(2)])).rejects.toThrow();
  await expect(createEntityBrowseConfirmation(f.origin, { ...source, resolve: async () => { throw new Error('HTTP 500'); } }).confirm([value(2)])).rejects.toThrow('HTTP 500');
  expect(f.replace).not.toHaveBeenCalled();
 });
});
