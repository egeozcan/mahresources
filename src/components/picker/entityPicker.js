import { getEntityConfig } from './entityConfigs.js';
import { createEntityPickerDialog } from './entityPickerDialog.js';
import { createPickerSession } from './pickerSession.ts';
import { createHttpEntityBrowseSource } from '../../selector/httpEntityBrowseSource.ts';
import { createEntityBrowseConfirmation } from '../../selector/entityBrowseIntegration.ts';
import { encodeBrowseParameters } from '../../selector/entityBrowseTypes.ts';
import { generateParamNameForMeta } from '../freeFields.js';

function waitForInput(signal, delay) {
  if (!delay) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(new DOMException('Aborted', 'AbortError')); };
    const timer = setTimeout(() => { signal.removeEventListener('abort', abort); resolve(); }, delay);
    signal.addEventListener('abort', abort, { once: true });
    if (signal.aborted) abort();
  });
}

export function registerEntityPickerStore(Alpine, { source = createHttpEntityBrowseSource(), debounceMs = 200 } = {}) {
  const session = createPickerSession({ ...source, search: async (input, signal) => {
    await waitForInput(signal, debounceMs);
    return source.search(input, signal);
  } });
  const openers = new Map(), styleNodes = new Map();
  let dialog = null;
  const nextTick = callback => Alpine.nextTick ? Alpine.nextTick(callback) : Promise.resolve().then(callback);
  const store = {
    isOpen: false, steps: [], views: [],
    attachDialog(root) { dialog?.destroy();dialog = createEntityPickerDialog(root, { afterRender: nextTick });dialog.setOpen(this.isOpen); },
    detachDialog() { this.close();dialog?.destroy();dialog = null; },
    get currentStep() { return this.steps.at(-1) || null; },
    get currentView() { return this.views.at(-1) || null; },
    get config() { return this.currentStep ? getEntityConfig(this.currentStep.options.browse.entity) : null; },
    get title() { return this.currentStep?.options.title || `Browse ${this.config?.entityLabel || 'Entities'}`; },
    get loading() { return this.currentStep?.status === 'loading'; },
    get confirming() { return this.currentStep?.status === 'confirming'; },
    get error() { return this.currentStep?.error; },
    get selectionCount() { return this.currentStep?.pending.length || 0; },
    get multiSelect() { return this.currentStep?.options.browse.multiple ?? true; },
    get displayResults() { return this.currentStep?.items || []; },
    get activeTab() { return this.currentView?.legacy?.mode.tab || 'all'; },
    get noteId() { return this.currentView?.legacy?.noteId; },
    get filterValues() { return this.currentView?.filters || {}; },
    get searchQuery() { return this.filterValues.Name || ''; },
    set searchQuery(value) { if (this.currentView) this.currentView.filters.Name = value; },
    get capacityReached() {
      const step = this.currentStep;
      return step && step.options.browse.multiple && step.options.browse.maximum !== undefined
        && new Set([...step.options.existing, ...step.pending].map(v => String(v.ID))).size >= step.options.browse.maximum;
    },
    renderResult(host, html) {
      // The ignored child is never initialized as Alpine code. Keep the DOM
      // observer suspended while inserting author HTML, including on refresh.
      Alpine.mutateDom(() => { host.firstElementChild.innerHTML = html; });
    },
    filtersFor(entity) { return getEntityConfig(entity).filters; },
    stepFor(id) { return this.steps.find(step => step.id === id); },
    viewFor(id) { return this.views.find(view => view.id === id); },

    openField(options, opener) {
      const parentID = Number(opener?.closest?.('[data-picker-step]')?.getAttribute('data-picker-step'));
      if (parentID && parentID !== this.currentStep?.id) return false;
      if (this.isOpen && parentID) session.push(options);else session.open(options);
      if (this.currentStep) openers.set(this.currentStep.id, opener);
      return true;
    },

    // Compatibility with blocks and plugin action parameters. These consumers
    // still receive IDs, but eligibility and pagination use the same session.
    open({ entityType, noteId = null, existingIds = [], lockedFilters = {}, multiSelect = true, onConfirm }) {
      getEntityConfig(entityType);
      const mode = { tab: entityType === 'resource' && noteId ? 'note' : 'all' };
      const existing = existingIds.map(ID => ({ ID: Number(ID), Name: `#${ID}` }));
      const metadata = { entity: entityType, multiple: multiSelect, excludedKeys: () => [], parameters: () => ({
        ...(lockedFilters.content_types?.length ? { ContentTypes: lockedFilters.content_types } : {}),
        ...(lockedFilters.category_ids?.length ? { Categories: lockedFilters.category_ids } : {}),
        ...(lockedFilters.note_type_ids?.length ? { NoteTypeIds: lockedFilters.note_type_ids } : {}),
        ...(mode.tab === 'note' ? { Notes: [noteId] } : {}),
      }) };
      let callbackResult;
      const bridge = createEntityBrowseConfirmation({
        metadata, getValues: () => existing,
        isAvailable: () => session.snapshot().steps.some(step => step.options.browse === metadata),
        replace: values => {
          const previous = new Set(existing.map(v => String(v.ID)));
          callbackResult = onConfirm?.(values.filter(value => !multiSelect || !previous.has(String(value.ID))).map(value => value.ID));
        },
      }, source);
      this.openField({ browse: metadata, existing,
        title: `Select ${getEntityConfig(entityType).entityLabel}`, legacy: { noteId, mode },
        onConfirm: async values => (await bridge.confirm(values)) && (await callbackResult) !== false,
        onDispose: bridge.destroy,
      }, typeof document === 'undefined' ? null : document.activeElement);
    },
    close() { session.cancel(); },
    back() { session.back(); },
    escape() { if (this.steps.length > 1) this.back();else this.close(); },
    confirm() { return session.confirm(); },
    retry() { session.retry(); },
    nextPage() { if (this.currentStep?.hasNext) session.setPage(this.currentStep.page + 1); },
    previousPage() { if (this.currentStep?.page > 1) session.setPage(this.currentStep.page - 1); },
    toggleSelection(value) {
      const row = typeof value === 'object' ? value : this.currentStep?.items.find(item => String(item.value.ID) === String(value))?.value;
      if (row) session.toggle(row);
    },
    isSelected(id) { return Boolean(this.currentStep?.pending.some(value => String(value.ID) === String(id))); },
    isAlreadyAdded(id) { return Boolean(this.currentStep?.options.existing.some(value => String(value.ID) === String(id))); },
    rowDisabled(id) { return this.confirming || this.isAlreadyAdded(id) || (this.capacityReached && !this.isSelected(id)); },
    setActiveTab(tab) {
      const view = this.currentView;
      if (!view?.legacy || (tab === 'note' && !view.legacy.noteId)) return;
      view.legacy.mode.tab = tab;
      if (this.currentStep.page !== 1) session.setPage(1);else session.retry();
    },
    setFilter(key, value, stepID = this.currentStep?.id) {
      const view = this.viewFor(stepID);if (!view) return;
      if (value === '' || value === null || value === undefined || value === false || (Array.isArray(value) && value.length === 0)) delete view.filters[key];
      else view.filters[key] = value;
      this.loadResults(stepID);
    },
    applyFilterChange(key, multiple, change, stepID = this.currentStep?.id) {
      this.setFilter(key, multiple ? change.current.map(option => option.raw.ID) : (change.current[0]?.raw.ID ?? null), stepID);
    },
    onSearchInput(stepID = this.currentStep?.id) { this.loadResults(stepID); },
    loadResults(stepID = this.currentStep?.id) {
      const view = this.viewFor(stepID);if (!view) return;
      session.setFilter(encodeBrowseParameters(view.filters), stepID);
    },
    applyMetaFilter(stepID) {
      const view = this.viewFor(stepID);if (!view) return;
      for (const key of Object.keys(view.filters)) if (key.startsWith('MetaQuery.')) delete view.filters[key];
      view.metaFields.filter(field => field.name && field.value !== '').forEach((field, i) => {
        view.filters[`MetaQuery.${i}`] = generateParamNameForMeta(field);
      });
      this.loadResults(stepID);
    },
    destroy() { session.destroy(); },
  };
  Alpine.store('entityPicker', store);
  const reactive = Alpine.store('entityPicker') || store;
  session.subscribe(snapshot => {
    const previous = reactive.views, oldIDs = new Set(previous.map(view => view.id));
    const nextIDs = new Set(snapshot.steps.map(step => step.id));
    const removed = previous.filter(view => !nextIDs.has(view.id));
    const returning = snapshot.isOpen ? removed.at(-1) : removed[0];
    const returnTo = returning && openers.get(returning.id);
    reactive.steps = snapshot.steps;reactive.isOpen = snapshot.isOpen;
    dialog?.setOpen(snapshot.isOpen);
    reactive.views = snapshot.steps.map(step => previous.find(view => view.id === step.id) || {
      id: step.id, entity: step.options.browse.entity, filters: {}, more: false, metaFields: [], legacy: step.options.legacy,
    });
    removed.forEach(view => openers.delete(view.id));
    if (returnTo) nextTick(() => { if (returnTo.isConnected !== false) returnTo.focus?.(); });
    const latest = snapshot.steps.at(-1);
    if (latest && !oldIDs.has(latest.id)) nextTick(() => {
      if (typeof document !== 'undefined') document.querySelector(`[data-picker-step="${latest.id}"] input[name="Name"]`)?.focus();
    });
    if (typeof document !== 'undefined' && document.head) {
      const styles = latest?.styles || [], keys = new Set(styles.map(style => style.key));
      for (const [key, node] of styleNodes) if (!keys.has(key)) {node.remove();styleNodes.delete(key);}
      for (const style of styles) {
        let node = styleNodes.get(style.key);
        if (!node) {node = document.createElement('style');node.setAttribute('data-entity-picker-style', style.key);document.head.appendChild(node);styleNodes.set(style.key, node);}
        if (node.textContent !== style.css) node.textContent = style.css;
      }
    }
    if (previous.length && !snapshot.isOpen && typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('entity-picker-closed'));
  });
}
