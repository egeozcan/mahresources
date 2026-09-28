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

    /**
     * Whether the links are folded behind the menu button although the window
     * is desktop-wide, because they do not fit beside the header's other
     * controls. How wide they are depends on the deployment (plugins add menus,
     * an account adds its name, the Jobs button gains badges), so it is
     * measured rather than set as a breakpoint: pushed past the right edge, the
     * Jobs button and the settings could not be reached at all.
     */
    linksCollapsed: false,

    /** The control that opened the panel, so focus can go back to it. */
    _trigger: null,
    /** The component root, captured once — `$el` in a method is the caller. */
    _root: null,
    /** The links' own width, measured whenever they show. */
    _linksWidth: 0,
    _linksObserver: null,

    initMobileNav() {
      this._root = this.$el;
      this.currentPath = this.$el.dataset.currentPath || '';
      this.activeNav = this.$el.dataset.activeNav || '';

      const header = this._root.closest?.('header');
      if (header && typeof ResizeObserver !== 'undefined') {
        this._linksObserver = new ResizeObserver(() => this.fitLinks());
        // The header itself, what shares it (the Jobs button's badges come and
        // go), and the links, whose width settles once the web font loads.
        this._linksObserver.observe(header);
        for (const child of header.children) {
          if (child !== this._root) this._linksObserver.observe(child);
        }
        const links = this._root.querySelector('.navbar-links');
        if (links) this._linksObserver.observe(links);
        this.fitLinks();
      }

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

    destroy() {
      this._linksObserver?.disconnect();
    },

    // Folds the links behind the menu button when they are wider than the
    // header leaves them, and unfolds them once they fit again. Below the
    // mobile breakpoint the stylesheet folds them anyway, and they measure 0.
    fitLinks() {
      const header = this._root?.closest?.('header');
      const links = this._root?.querySelector('.navbar-links');
      if (!header || !links) return;
      if (!this.linksCollapsed && links.offsetWidth > 0) this._linksWidth = links.scrollWidth;
      if (!this._linksWidth) return;
      const style = getComputedStyle(header);
      const gap = parseFloat(style.columnGap) || 0;
      let available = header.clientWidth - (parseFloat(style.paddingLeft) || 0) - (parseFloat(style.paddingRight) || 0);
      for (const child of header.children) {
        if (child === this._root || child.offsetWidth === 0) continue;
        available -= child.offsetWidth + gap;
      }
      const collapsed = this._linksWidth > available;
      if (!collapsed && this.linksCollapsed) this.mobileOpen = false;
      this.linksCollapsed = collapsed;
    },
  };
}
