import { hoistCustomCSS } from './customCssHoist.js';
import { focusedElement, focusOn, restoreFocus } from './focus.js';
import { captureTabFocus, captureTabSelections, restoreTabSelections } from './tabSelections.js';
// Elements whose children are CLIENT-owned: the server only ever sends a
// placeholder inside them, so morph must patch their attributes but never walk
// into their subtree, or live content is overwritten with that placeholder.
const CLIENT_OWNED_CHILDREN_TAGS = new Set([
  'LAZY-SHORTCODE',
  'DETAILS-SHORTCODE',
  'META-SHORTCODE',
  // <schema-editor> is the metadata panel on every group, note and resource
  // detail page. In every mode but `edit` it renders into light DOM, and the
  // server sends the element empty -- exactly the condition described above.
  // Without this, paste-upload's whole-`.main` morph swapped Lit's part markers
  // for the server's whitespace and removed the rendered subtree, so the panel
  // vanished on any detail page that also carries data-paste-context.
  'SCHEMA-EDITOR',
]);

// Elements that want to re-derive state once morph has finished patching them.
// That is everything above, plus server-owned subtrees that need to re-check
// the button name ([reload]), restore tab state ([tabs]), or repaint a busy
// button ([action]) after the incoming markup lands.
const MORPH_AWARE_TAGS = new Set([...CLIENT_OWNED_CHILDREN_TAGS, 'RELOAD-SHORTCODE', 'TABS-SHORTCODE', 'ACTION-SHORTCODE']);

function hasClientOwnedChildren(el) {
  return el?.nodeType === 1 && (CLIENT_OWNED_CHILDREN_TAGS.has(el.tagName) || el.hasAttribute?.('data-morph-client-owned'));
}

function isMorphAware(el) {
  return el?.nodeType === 1 && (MORPH_AWARE_TAGS.has(el.tagName) || el.hasAttribute?.('data-morph-client-owned'));
}

function actionPayloadIdentity(host) {
  const { plugin, action, entityType, entityId, params } = host.dataset;
  // The server marshals params with stable key order.
  return JSON.stringify([plugin, action, entityType, entityId, params]);
}

function captureActionFocus(container, active) {
  const host = active?.parentElement;
  if (!host?.matches('action-shortcode') || !active.matches('button') || !container.contains(host)) return null;
  const identity = actionPayloadIdentity(host);
  return (now) => {
    if (now === active && active.parentElement === host && actionPayloadIdentity(host) === identity) return;
    const matches = Array.from(container.querySelectorAll('action-shortcode'))
      .filter((candidate) => actionPayloadIdentity(candidate) === identity);
    if (matches.length === 1 && focusOn(matches[0].querySelector(':scope > button'))) return;

    // Park on the list, not on another control. A boxless list may refuse
    // focus; try its ancestors instead of letting Enter activate a new target.
    const list = container === host ? host.parentElement : container;
    for (let region = list; region && region !== document.body; region = region.parentElement) {
      if (focusOn(region)) return;
    }
    now?.blur();
  };
}

const pendingMorphRestores = [];

function restoreMorphedShortcodes() {
  for (const restore of pendingMorphRestores.splice(0)) restore();
}

export function morphOptionsWithShortcodeElements(options = {}) {
  const { updating, updated, ...rest } = options;
  let captured = false;

  return {
    ...rest,
    updating(el, toEl, childrenOnly, skip, skipChildren) {
      if (!captured && typeof el.querySelectorAll === 'function') {
        // The first callback is the morph root. Reconcile every root patched in
        // this turn before the shared tab-host flush re-applies their state.
        captured = true;
        const selections = captureTabSelections(el);
        const active = focusedElement();
        const findTab = captureTabFocus(el, active);
        const restoreActionFocus = captureActionFocus(el, active);
        if (!pendingMorphRestores.length) queueMicrotask(restoreMorphedShortcodes);
        pendingMorphRestores.push(() => {
          captured = false;
          if (!el.isConnected) return;
          restoreTabSelections(el, selections);
          if (findTab || restoreActionFocus) queueMicrotask(() => {
            // The tab hosts' flush has restored panel visibility. A surviving
            // DOM button can still target something different after a positional patch.
            const now = focusedElement();
            if (now && now !== active) return;
            if (restoreActionFocus) restoreActionFocus(now);
            else restoreFocus(findTab(), el);
          });
        });
      }
      updating?.(el, toEl, childrenOnly, skip, skipChildren);

      if (hasClientOwnedChildren(el) && hasClientOwnedChildren(toEl)) {
        if (typeof skipChildren !== 'function') {
          skip?.();
          return;
        }
        skipChildren();
      }
    },
    updated(el, toEl) {
      if (isMorphAware(el) && typeof el.refreshFromMorph === 'function') {
        el.refreshFromMorph(toEl);
      }

      updated?.(el, toEl);
    },
  };
}

export const morphOptionsWithDeferredShortcodes = morphOptionsWithShortcodeElements;

/**
 * Morph `from` into `to`, then repair the Alpine components the morph left holding stale
 * state.
 *
 * Alpine's morph copies `_x_dataStack` across matched elements and then patches the `x-data`
 * *attribute*, but it never re-reads that attribute — so an element whose x-data expression
 * changed keeps running the component built from the old one. Destroying and re-initialising
 * exactly those elements is the repair. Doing it for the whole tree instead would throw away
 * the live state the morph exists to preserve.
 *
 * Every "re-fetch the page and morph the list back in" path needs this, not just the first
 * one that hit the problem.
 */
export function morphAndReinitChangedComponents(from, to, options = {}) {
  // The live side's category stylesheets already sit in the head; take the incoming ones out
  // too, or morph's positional matching would replace unchanged cards around them.
  hoistCustomCSS(to);
  const before = new Map();
  from.querySelectorAll('[x-data]').forEach((el) => {
    before.set(el, el.getAttribute('x-data'));
  });

  window.Alpine.morph(from, to, morphOptionsWithShortcodeElements(options));

  before.forEach((oldValue, el) => {
    if (!el.isConnected) return; // removed by the morph
    if (el.getAttribute('x-data') === oldValue) return;
    window.Alpine.destroyTree(el);
    window.Alpine.initTree(el);
  });
}
