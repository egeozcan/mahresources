import { afterEach, expect, it, vi } from 'vitest';
import { mountSingleEntitySelector } from './mountEntitySelector.js';

afterEach(() => vi.unstubAllGlobals());

it('standalone mounts share browser confirmation, native disabled state and teardown', async () => {
 function node() {
  return {
   style: {}, parentNode: null, parentElement: null, isConnected: true, disabled: false,
   setAttribute: vi.fn(), getAttribute: () => null,
   append(child) {child.parentElement = this;child.parentNode = this;},
   appendChild(child) {this.append(child);}, removeChild(child) {child.parentNode = null;},
   replaceChildren(child) {this.append(child);}, remove() {this.isConnected = false;},
   closest(query) {return query === 'fieldset[disabled]' && this.parentElement?.disabled ? this.parentElement : null;},
   matches: () => true, querySelector: () => null,
  };
 }
 const field = node(), container = node(), openField = vi.fn(), onChange = vi.fn();
 let adapter, factory;
 const Alpine = {
  store: () => ({ openField }),
  addScopeToNode: (_field, scope) => {factory = scope.mountedEntitySelector;return vi.fn();},
  mutateDom: fn => fn(),
  initTree: () => {
   adapter = factory();Object.assign(adapter, { $el: field, $refs: {}, $watch: vi.fn(), $dispatch: vi.fn(), $nextTick: fn => fn?.() });adapter.init();
  },
  destroyTree: () => adapter.destroy(),
 };
 vi.stubGlobal('Alpine', Alpine);
 vi.stubGlobal('window', { Alpine, addEventListener: vi.fn(), removeEventListener: vi.fn() });
 vi.stubGlobal('document', { body: node(), querySelectorAll: () => [], createElement: name => name === 'template' ? { content: { firstElementChild: field } } : node() });
 vi.stubGlobal('fetch', vi.fn(async url => String(url).startsWith('/partials/')
  ? new Response('<div data-selector-profile="single"></div>')
  : new Response(JSON.stringify({ items: [{ ID: 2, Name: 'Group' }] }))));
 const handle = await mountSingleEntitySelector(container, { entity: 'group', title: 'Owner', onChange });
 handle.setDisabled(true);expect(adapter.openEntityBrowser()).toBe(false);
 handle.setDisabled(false);expect(adapter.openEntityBrowser()).toBe(true);
 expect(await openField.mock.calls[0][0].onConfirm([{ ID: 2, Name: 'Group' }])).toBe(true);
 expect(handle.getRawValues()).toEqual([{ ID: 2, Name: 'Group' }]);expect(onChange).toHaveBeenCalledOnce();
 adapter.openEntityBrowser();const latest = openField.mock.calls[1][0];handle.destroy();
 expect(await latest.onConfirm([{ ID: 3, Name: 'Other' }])).toBe(false);
});
