import { blockingModal, refuseOverModal } from '../utils/modality.js';
import { restoreFocus } from '../utils/focus.js';

/**
 * The template generation disclosure (decision 11 of
 * docs/plans/2026-10-24-template-generation-quality.md): before a generation
 * request is sent, this dialog lists exactly what goes to the provider, from a
 * dry run of the same request. Send resolves true and the caller sends the
 * request with the disclosure's groundingHash; Cancel and Escape resolve false
 * and nothing is sent.
 *
 * It follows the confirm dialog's pattern (templates/partials/confirmDialog.tpl):
 * `.overlays`, the confirm overlay's z-index, the rest of the page made inert,
 * and focus returned by the store one macrotask after the dialog is torn down,
 * because `x-trap` pulls focus back while it is still armed. It never opens over
 * another painted modal; the reader is told so inside that modal instead.
 */
export const DISCLOSURE_REFUSED_OVER_DIALOG = 'Close this dialog first to review what template generation will send.';

export function registerGenerationDisclosureStore(Alpine) {
  Alpine.store('generationDisclosure', {
    isOpen: false,
    disclosure: null,
    _resolve: null,
    _opener: null,
    _inerted: [],

    get host() {
      return this.disclosure?.destination?.host || 'the provider';
    },

    // One line per kind of call the request can make.
    get profiles() {
      return (this.disclosure?.destination?.profiles || []).map((p) =>
        `${p.kind}: ${p.model}, thinking ${p.thinking ? 'on' : 'off'}, at most ${p.maxCalls} call${p.maxCalls === 1 ? '' : 's'}`,
      );
    },

    get sections() {
      return this.disclosure?.sections || [];
    },

    /**
     * Show `disclosure` and resolve true only if the reader presses Send.
     * `opener` gets focus back on Cancel; on Send the caller moves focus, since
     * its Generate button is disabled while the request runs.
     */
    open(disclosure, opener = null) {
      if (this.isOpen) return Promise.resolve(false);
      const blocker = blockingModal();
      if (blocker) {
        refuseOverModal(blocker, DISCLOSURE_REFUSED_OVER_DIALOG);
        return Promise.resolve(false);
      }
      this.disclosure = disclosure;
      this._opener = opener;
      this.isOpen = true;
      return new Promise((resolve) => {
        this._resolve = resolve;
      });
    },

    send() {
      this._settle(true);
    },

    cancel() {
      this._settle(false);
    },

    _settle(value) {
      if (!this.isOpen) return;
      const resolve = this._resolve;
      const opener = this._opener;
      this.isOpen = false;
      this._resolve = null;
      this._opener = null;
      this._releaseInert();
      if (!value) {
        setTimeout(() => restoreFocus(opener), 0);
      }
      if (resolve) resolve(value);
    },

    // Make everything except this dialog inert; see confirmDialog's _applyInert.
    _applyInert(dialogRoot) {
      if (!dialogRoot) return;
      this._releaseInert();
      const ancestors = new Set();
      for (let el = dialogRoot; el && el !== document.body; el = el.parentElement) ancestors.add(el);
      const mark = (el) => {
        if (ancestors.has(el) || el.inert) return;
        el.inert = true;
        this._inerted.push(el);
      };
      for (const child of document.body.children) {
        if (ancestors.has(child)) {
          for (const grandchild of child.children) mark(grandchild);
        } else {
          mark(child);
        }
      }
    },

    _releaseInert() {
      for (const el of this._inerted) el.inert = false;
      this._inerted = [];
    },
  });
}

/**
 * Ask the reader to approve a disclosure, from a plain module. Fails closed:
 * without the store nothing is sent.
 */
export function reviewDisclosure(disclosure, opener) {
  const store = globalThis.Alpine?.store('generationDisclosure');
  if (!store) {
    console.error('generationDisclosure store is not registered; refusing to send');
    return Promise.resolve(false);
  }
  return store.open(disclosure, opener);
}

/** One macrotask, so a dialog's x-trap has released and its subtree is gone. */
export function afterDialogTeardown() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

/**
 * Return focus to the Generate button once a request has settled. The button
 * is disabled while the request runs, so this waits for Alpine to apply the
 * re-enabled binding ($nextTick, itself a macrotask, then one more), and moves
 * focus only if the reader is still on the status region, or nowhere (a
 * disabled button that had focus drops it to <body>). A reader who went
 * elsewhere meanwhile is left there.
 */
export async function returnFocusToGenerate(component, region, button) {
  await component.$nextTick();
  await afterDialogTeardown();
  const active = document.activeElement;
  const nowhere = !active || active === document.body || active === document.documentElement;
  if ((nowhere || active === region) && button && !button.disabled) button.focus();
}
