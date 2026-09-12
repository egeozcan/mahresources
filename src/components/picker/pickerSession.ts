import { encodeBrowseParameters, type BrowseResult, type BrowseStyle, type BrowseValue, type EntityBrowseMetadata, type EntityBrowseSource } from '../../selector/entityBrowseTypes';

export interface PickerOpenOptions {
 browse: EntityBrowseMetadata;
 existing: readonly BrowseValue[];
 onConfirm: (values: readonly BrowseValue[]) => Promise<boolean> | boolean;
 /** Releases a confirmation bridge when its step closes, including cancellation. */
 onDispose?: () => void;
}
export interface PickerStepSnapshot {
 id: number;
 options: PickerOpenOptions;
 filter: string;
 page: number;
 items: readonly BrowseResult[];
 styles: readonly BrowseStyle[];
 warnings: readonly string[];
 pending: readonly BrowseValue[];
 hasNext: boolean;
 status: 'idle' | 'loading' | 'ready' | 'error' | 'confirming';
 error: string | null;
}
export interface PickerSnapshot { steps: readonly PickerStepSnapshot[]; isOpen: boolean }
export interface PickerSession {
 snapshot(): PickerSnapshot;
 subscribe(listener: (state: PickerSnapshot) => void): () => void;
 open(options: PickerOpenOptions): void;
 push(options: PickerOpenOptions): void;
 back(): void;
 cancel(): void;
 setFilter(filter: string, stepId?: number): void;
 setPage(page: number): void;
 toggle(value: BrowseValue): void;
 retry(): void;
 confirm(): Promise<boolean>;
 destroy(): void;
}

type Step = { state: PickerStepSnapshot; generation: number; controller: AbortController | null };
const key = (value: BrowseValue) => String(value.ID);

export function createPickerSession(source: EntityBrowseSource): PickerSession {
 let steps: Step[] = [], serial = 0, nextID = 0, destroyed = false;
 const subscribers = new Set<(state: PickerSnapshot) => void>();
 let snapshot: PickerSnapshot = Object.freeze({ steps: Object.freeze([]), isOpen: false });
 const top = () => steps.at(-1);
 function publish() {
  snapshot = Object.freeze({ isOpen: steps.length > 0, steps: Object.freeze(steps.map(({ state }) => Object.freeze({
   ...state,
   items: Object.freeze([...state.items]), styles: Object.freeze([...state.styles]),
   warnings: Object.freeze([...state.warnings]), pending: Object.freeze([...state.pending]),
  }))) });
  subscribers.forEach(listener => listener(snapshot));
 }
 function stop(step: Step) {
  step.controller?.abort();step.controller = null;step.generation++;
  if (step.state.status === 'loading') step.state.status = 'idle';
 }
 function dispose(step: Step) {
  stop(step);
  try { step.state.options.onDispose?.(); } catch { /* Cleanup cannot keep a closed dialog alive. */ }
 }
 function live(step: Step, generation: number, session: number) {
  return !destroyed && serial === session && steps.includes(step) && step.generation === generation;
 }
 async function load(step: Step) {
  stop(step);
  const generation = step.generation, session = serial, controller = new AbortController();
  step.controller = controller;step.state.status = 'loading';step.state.error = null;publish();
  try {
   const { browse } = step.state.options;
   const result = await source.search({ entity: browse.entity, constraints: encodeBrowseParameters(browse.parameters()), filter: step.state.filter, page: step.state.page }, controller.signal);
   if (!live(step, generation, session)) return;
   const excluded = new Set(browse.excludedKeys().map(String));
   step.state.items = result.items.filter(item => !excluded.has(key(item.value)));
   step.state.styles = result.styles;step.state.warnings = result.warnings;
   step.state.hasNext = result.hasNext;step.state.status = 'ready';
  } catch (error) {
   if (!live(step, generation, session)) return;
   step.state.status = 'error';step.state.error = error instanceof Error ? error.message : 'Could not load results';
  }
  if (live(step, generation, session)) {step.controller = null;publish();}
 }
 function add(options: PickerOpenOptions) {
  const step: Step = { generation: 0, controller: null, state: {
   id: ++nextID, options: Object.freeze({ ...options, existing: Object.freeze([...options.existing]) }),
   filter: '', page: 1, items: [], styles: [], warnings: [], pending: [], hasNext: false, status: 'idle', error: null,
  } };
  steps.push(step);void load(step);
 }
 function cancel() {
  serial++;const previous = steps;steps = [];previous.forEach(dispose);publish();
 }
 function back() {
  const child = steps.pop();if (!child) return;dispose(child);
  const parent = top();
  if (parent?.state.status === 'idle') void load(parent);else publish();
 }
 return {
  snapshot: () => snapshot,
  subscribe(listener) {subscribers.add(listener);return () => {subscribers.delete(listener);};},
  open(options) {if (destroyed) return;cancel();add(options);},
  push(options) {if (destroyed || top()?.state.status === 'confirming') return;const parent = top();if (parent) stop(parent);add(options);},
  back, cancel,
  setFilter(filter, stepID) {
   const step = stepID === undefined ? top() : steps.find(step => step.state.id === stepID);
   if (!step || step.state.status === 'confirming' || step.state.filter === filter) return;
   step.state.filter = filter;step.state.page = 1;step.state.items = [];void load(step);
  },
  setPage(page) {
   const step = top();
   if (!step || step.state.status === 'confirming' || !Number.isInteger(page) || page < 1 || page === step.state.page) return;
   step.state.page = page;step.state.items = [];void load(step);
  },
  toggle(value) {
   const step = top();if (!step || step.state.status === 'confirming') return;
   const state = step.state, { browse, existing } = state.options;
   if (existing.some(item => key(item) === key(value)) || browse.excludedKeys().map(String).includes(key(value))) return;
   if (state.pending.some(item => key(item) === key(value))) state.pending = state.pending.filter(item => key(item) !== key(value));
   else if (!browse.multiple) state.pending = [value];
   else {
    const count = new Set([...existing, ...state.pending].map(key)).size;
    if (browse.maximum !== undefined && count >= browse.maximum) return;
    state.pending = [...state.pending, value];
   }
   state.error = null;publish();
  },
  retry() {const step = top();if (step && step.state.status !== 'confirming') void load(step);},
  async confirm() {
   const step = top();if (!step || step.state.status === 'confirming' || step.state.pending.length === 0) return false;
   stop(step);
   const generation = step.generation, session = serial;
   step.state.status = 'confirming';step.state.error = null;publish();
   try {
    const accepted = await step.state.options.onConfirm([...step.state.pending]);
    if (!live(step, generation, session)) return false;
    if (accepted) {back();return true;}
    step.state.error = 'The selection could not be applied. Refresh and try again.';
   } catch (error) {
    if (!live(step, generation, session)) return false;
    step.state.error = error instanceof Error ? error.message : 'The selection could not be applied';
   }
   step.state.status = 'ready';publish();return false;
  },
  destroy() {if (destroyed) return;cancel();destroyed = true;subscribers.clear();},
 };
}
