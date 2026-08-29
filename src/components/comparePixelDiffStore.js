/**
 * The compare page's summary banner is server-rendered above every comparator
 * and outside the image comparator's Alpine scope, so the pixel-diff
 * percentage reaches it through this store instead of through a scope.
 *
 * The image comparator dispatches a `compare-pixel-diff` window event whenever
 * its computed number changes (a percent, "no overlap", or null for hidden);
 * this store subscribes to that event in JS — not in template markup, where
 * the compare pages deliberately keep no `@…​.window` listeners — and the
 * banner reads the store reactively. One transport, one reader.
 *
 * The listener writes through `Alpine.store(...)` — the registered reactive
 * proxy — and never through the raw object it was declared with: a write to
 * the raw target updates every later read but triggers no reactive effect,
 * so the banner would silently keep the last rendered value.
 */
export function registerComparePixelDiffStore(Alpine) {
  Alpine.store('comparePixelDiff', {
    percent: null,
    overlapEmpty: false,
  });
  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    window.addEventListener('compare-pixel-diff', (e) => {
      const store = Alpine.store('comparePixelDiff');
      store.percent = e.detail.percent;
      store.overlapEmpty = e.detail.overlapEmpty;
    });
  }
}
