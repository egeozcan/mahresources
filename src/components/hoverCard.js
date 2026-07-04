// hoverCard — a popover preview shown when hovering or focusing an entity link
// (group / resource / note). It fetches the /hovercard fragment (which reuses the
// list-card CustomAvatar + CustomSummary machinery) and positions a popover next
// to the trigger.
//
// Accessibility (WCAG 1.4.13, content on hover or focus):
//   - Dismissible: Escape closes the popover without moving the pointer.
//   - Hoverable:   the popover itself can be hovered without it disappearing.
//   - Persistent:  it stays until hover/focus leaves BOTH trigger and popover.
// The trigger gets role/aria wiring (aria-describedby → the tooltip) while open.
// The appear transition is suppressed under prefers-reduced-motion.

import { abortableFetch } from '../index.js';

const TRIGGER_SELECTOR =
  'a[href^="/group?id="], a[href^="/resource?id="], a[href^="/note?id="]';
const HREF_RE = /^\/(group|resource|note)\?id=(\d+)/;

const HOVER_DELAY = 500; // hover-intent delay (ms)
const FOCUS_DELAY = 120; // shorter delay for keyboard focus
const CLOSE_DELAY = 200; // grace period to move pointer trigger → popover
const POPOVER_ID = 'hovercard-popover';
const GAP = 8; // px between trigger and popover

/**
 * Whether hover previews are enabled. Reads the server-backed "showHoverPreviews"
 * UI setting, defaulting to on (including before the settings store hydrates).
 */
function previewsEnabled() {
  try {
    const store = window.Alpine && window.Alpine.store('savedSetting');
    const v = store && store.localSettings ? store.localSettings.showHoverPreviews : undefined;
    // Stored as boolean or "true"/"false" string; undefined → default on.
    if (v === undefined || v === null) return true;
    return v === true || v === 'true';
  } catch (e) {
    return true;
  }
}

function parseTrigger(el) {
  const href = el.getAttribute('href') || '';
  const m = href.match(HREF_RE);
  if (!m) return null;
  return { type: m[1], id: m[2] };
}

// Exclude links that are part of the lightbox, or that live inside the popover
// itself (its own title link), from triggering a preview.
function isExcluded(el) {
  return (
    el.hasAttribute('data-lightbox-item') ||
    el.closest('[data-lightbox-item]') !== null ||
    el.closest('#' + POPOVER_ID) !== null ||
    el.closest('#shared-lightbox') !== null
  );
}

export function setupHoverCard() {
  const reducedMotion =
    window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  const cache = new Map(); // "type:id" -> html string
  let popover = null;
  let openTimer = null;
  let closeTimer = null;
  let aborter = null;
  let currentTrigger = null; // the trigger the popover is (or is being) shown for
  let currentKey = null;

  function ensurePopover() {
    if (popover) return popover;
    popover = document.createElement('div');
    popover.id = POPOVER_ID;
    popover.className = 'hovercard-popover';
    popover.setAttribute('role', 'tooltip');
    // Base styles kept inline so the feature works without a CSS build step.
    Object.assign(popover.style, {
      position: 'fixed',
      zIndex: '9998',
      maxWidth: '320px',
      width: 'max-content',
      background: '#fff',
      border: '1px solid #e7e5e4',
      borderRadius: '0.5rem',
      boxShadow: '0 10px 25px -5px rgba(0,0,0,0.15), 0 8px 10px -6px rgba(0,0,0,0.1)',
      padding: '0.75rem',
      pointerEvents: 'auto',
      display: 'none',
      opacity: reducedMotion ? '1' : '0',
      transition: reducedMotion ? 'none' : 'opacity 120ms ease-out',
    });
    // Hoverable + persistent: hovering the popover keeps it open.
    popover.addEventListener('mouseenter', cancelClose);
    popover.addEventListener('mouseleave', scheduleClose);
    document.body.appendChild(popover);
    return popover;
  }

  function positionPopover(trigger) {
    const el = ensurePopover();
    const r = trigger.getBoundingClientRect();
    // Measure after content is set.
    const pw = el.offsetWidth;
    const ph = el.offsetHeight;
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    // Prefer below; flip above if it would overflow the bottom.
    let top = r.bottom + GAP;
    if (top + ph > vh && r.top - GAP - ph >= 0) {
      top = r.top - GAP - ph;
    }
    // Left-align to the trigger; shift left if it would overflow the right edge.
    let left = r.left;
    if (left + pw > vw - 4) {
      left = Math.max(4, vw - 4 - pw);
    }
    if (left < 4) left = 4;

    el.style.top = Math.max(4, top) + 'px';
    el.style.left = left + 'px';
  }

  function show(trigger, html) {
    const el = ensurePopover();
    el.innerHTML = html;
    el.style.display = 'block';
    // Hydrate Alpine directives / custom elements in the injected fragment.
    if (window.Alpine && typeof window.Alpine.initTree === 'function') {
      try {
        window.Alpine.initTree(el);
      } catch (e) {
        /* non-fatal */
      }
    }
    positionPopover(trigger);
    // Fade in on the next frame so the transition applies.
    if (!reducedMotion) {
      requestAnimationFrame(() => {
        el.style.opacity = '1';
      });
    }
    // A11y: associate the trigger with the tooltip while open.
    trigger.setAttribute('aria-describedby', POPOVER_ID);
    currentTrigger = trigger;
  }

  function fetchAndShow(trigger, key, type, id) {
    if (cache.has(key)) {
      show(trigger, cache.get(key));
      return;
    }
    if (aborter) aborter();
    const { abort, ready } = abortableFetch(
      `/hovercard?type=${encodeURIComponent(type)}&id=${encodeURIComponent(id)}`,
    );
    aborter = abort;
    ready
      .then((r) => (r.ok ? r.text() : null))
      .then((html) => {
        aborter = null;
        if (html == null) return;
        cache.set(key, html);
        // Only show if this is still the intended trigger (no newer hover won).
        if (currentKey === key) show(trigger, html);
      })
      .catch((err) => {
        if (err && err.name !== 'AbortError') {
          /* swallow: a failed preview must never disrupt the page */
        }
      });
  }

  function scheduleOpen(trigger, delay) {
    if (!previewsEnabled()) return;
    const info = parseTrigger(trigger);
    if (!info) return;
    const key = info.type + ':' + info.id;
    // Already open for this trigger — keep it.
    if (currentKey === key && popover && popover.style.display === 'block') {
      cancelClose();
      return;
    }
    cancelOpen();
    cancelClose();
    currentKey = key;
    openTimer = setTimeout(() => {
      openTimer = null;
      fetchAndShow(trigger, key, info.type, info.id);
    }, delay);
  }

  function cancelOpen() {
    if (openTimer) {
      clearTimeout(openTimer);
      openTimer = null;
    }
  }

  function cancelClose() {
    if (closeTimer) {
      clearTimeout(closeTimer);
      closeTimer = null;
    }
  }

  function close() {
    cancelOpen();
    cancelClose();
    if (aborter) {
      aborter();
      aborter = null;
    }
    currentKey = null;
    if (currentTrigger) {
      currentTrigger.removeAttribute('aria-describedby');
      currentTrigger = null;
    }
    if (popover) {
      popover.style.display = 'none';
      popover.style.opacity = reducedMotion ? '1' : '0';
      popover.innerHTML = '';
    }
  }

  function scheduleClose() {
    cancelClose();
    closeTimer = setTimeout(close, CLOSE_DELAY);
  }

  // --- Delegated listeners -------------------------------------------------

  function triggerFrom(target) {
    if (!target || typeof target.closest !== 'function') return null;
    const el = target.closest(TRIGGER_SELECTOR);
    if (!el || isExcluded(el)) return null;
    return el;
  }

  document.addEventListener('mouseover', (e) => {
    const t = triggerFrom(e.target);
    if (t) scheduleOpen(t, HOVER_DELAY);
  });

  document.addEventListener('mouseout', (e) => {
    const t = triggerFrom(e.target);
    if (!t) return;
    // Ignore moves between the trigger's own child nodes (mouseout bubbles and
    // fires on every internal boundary) — only act on a real exit.
    if (e.relatedTarget && t.contains(e.relatedTarget)) return;
    // Leaving the trigger: if the pointer is heading into the popover, the
    // popover's own mouseenter cancels the close (hoverable). Otherwise close.
    if (openTimer) cancelOpen();
    scheduleClose();
  });

  // Keyboard focus opens (shorter delay); blur closes with the same grace.
  document.addEventListener(
    'focusin',
    (e) => {
      const t = triggerFrom(e.target);
      if (t) scheduleOpen(t, FOCUS_DELAY);
    },
    true,
  );
  document.addEventListener(
    'focusout',
    (e) => {
      const t = triggerFrom(e.target);
      if (t) scheduleClose();
    },
    true,
  );

  // Dismissible: Escape closes without moving the pointer.
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && currentKey) close();
  });

  // Detached popovers are worse than none: close on scroll and on navigation.
  window.addEventListener('scroll', () => currentKey && close(), true);
  document.addEventListener('click', (e) => {
    // A click on the trigger navigates; a click elsewhere dismisses.
    if (!e.target.closest || !e.target.closest('#' + POPOVER_ID)) close();
  });
}
