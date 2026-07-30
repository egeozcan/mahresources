import { captureTrigger, restoreFocus } from '../utils/focus.js';

/**
 * The navbar's state, including the mobile panel.
 *
 * UI bug hunt 2026-07-29, finding 3: the mobile menu could not be closed. Escape
 * was a no-op, the panel is `position: fixed; inset: 0` and — being a descendant
 * of the `z-index: 40` header — painted above the hamburger, so the toggle click
 * was intercepted by the panel rather than reaching the button. And the panel
 * contained zero buttons, so there was no close affordance at all. Measured at
 * 390x844: after Escape, `aria-expanded` was still "true" and the panel still
 * `display: block; visibility: visible; opacity: 1` at the full 390x844, with
 * `elementFromPoint` over the hamburger returning `navbar-mobile-panel`. The only
 * way out was to follow a nav link.
 *
 * This was `x-data="{ mobileOpen: false, adminOpen: false, pluginsOpen: false,
 * currentPath: '' }"` inline in the template. It is a module now because the
 * close path needs the shared focus helpers and a deferred restore, and neither
 * belongs in an inline expression.
 */
export function mobileNav() {
  return {
    mobileOpen: false,
    adminOpen: false,
    pluginsOpen: false,
    currentPath: '',
    /**
     * The nav section the current URL belongs to (findings 116/121), which is not
     * the same as currentPath: /log?id=5 belongs to the Logs entry. The Admin
     * dropdown button compares against this so it stays lit on a detail page,
     * matching the links inside it, which the server marks.
     */
    activeNav: '',

    /** The control that opened the panel, so focus can go back to it. */
    _trigger: null,
    /** The component root, captured once — `$el` in a method is the caller. */
    _root: null,

    initMobileNav() {
      this._root = this.$el;
      this.currentPath = this.$el.dataset.currentPath || '';
      this.activeNav = this.$el.dataset.activeNav || '';

      this.$watch('mobileOpen', (open) => {
        if (open) return;
        // Deferred two frames, not $nextTick.
        //
        // Setting the flag only *schedules* Alpine's work, so a restore on the
        // next line runs while the panel is still mounted and its x-trap still
        // active — and the trap pulls focus straight back, leaving the reader on
        // <body> once the panel finally goes. WS4 hit this on three separate
        // dialogs before the pattern was understood; see docs/todo.md.
        requestAnimationFrame(() =>
          requestAnimationFrame(() => {
            const fallback = this._root?.querySelector('.navbar-toggle');
            restoreFocus(this._trigger, null) || restoreFocus(fallback, null);
            this._trigger = null;
          }),
        );
      });
    },

    /**
     * Toggle the panel, remembering what opened it.
     *
     * captureTrigger reads `currentTarget`, which is only valid during dispatch —
     * hence at open time rather than at close time. It resolves to the button and
     * not the <svg> inside it, which is the whole reason the helper exists.
     */
    toggleMobileNav(event) {
      if (!this.mobileOpen) this._trigger = captureTrigger(event);
      this.mobileOpen = !this.mobileOpen;
    },

    closeMobileNav() {
      this.mobileOpen = false;
    },
  };
}
