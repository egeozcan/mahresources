import { blockingModal, refuseOverModal } from '../utils/modality.js';
import { containFocus, focusFirstAvailable } from '../utils/containFocus.js';

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
 * and focus returned by the store one macrotask after the dialog is torn down.
 * It keeps focus in with inert and containFocus rather than x-trap (see
 * src/utils/containFocus.js). It never opens over another painted modal; the reader is
 * told so inside that modal instead.
 */
export const DISCLOSURE_REFUSED_OVER_DIALOG = 'Close this dialog first to review what template generation will send.';

export function registerGenerationDisclosureStore(Alpine) {
  Alpine.store('generationDisclosure', {
    isOpen: false,
    disclosure: null,
    _resolve: null,
    // Where focus goes back on Cancel: the first of these that can take it.
    _returnTo: [],
    _containment: null,

    _inerted: [],

    get host() {
      return this.disclosure?.destination?.host || 'the provider';
    },

    get endpoint() {
      return this.disclosure?.destination?.endpoint || this.host;
    },

    // One line per kind of call the request can make.
    get profiles() {
      return (this.disclosure?.destination?.profiles || []).map((p) =>
        `${p.kind}: ${p.model}, thinking ${p.thinking ? 'on' : 'off'}, up to ${p.maxTokens} output tokens, at most ${p.maxCalls} call${p.maxCalls === 1 ? '' : 's'}`,
      );
    },

    // How every call is sent: the fixed transport settings.
    get transport() {
      const t = this.disclosure?.transport;
      if (!t) return [];
      return [
        `POST ${this.endpoint}`,
        `Content-Type: ${t.contentType}`,
        `Authorization: ${t.authorization}`,
        `stream: ${t.stream ? 'true' : 'false'}, response_format: ${t.responseFormat}`,
      ];
    },

    get continuationRule() {
      return this.disclosure?.continuationRule?.text || '';
    },

    get sections() {
      return this.disclosure?.sections || [];
    },

    /**
     * Show `disclosure` and resolve true only if the reader presses Send.
     * On Cancel focus goes to the first of `returnTo` that can take it (the
     * Generate button, else its status region, else its prompt); on Send the
     * caller moves focus, since its Generate button is disabled while the
     * request runs.
     */
    open(disclosure, returnTo = []) {
      if (this.isOpen) return Promise.resolve(false);
      const blocker = blockingModal();
      if (blocker) {
        refuseOverModal(blocker, DISCLOSURE_REFUSED_OVER_DIALOG);
        return Promise.resolve(false);
      }
      this.disclosure = disclosure;
      this._returnTo = Array.isArray(returnTo) ? returnTo : [returnTo];
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
      const returnTo = this._returnTo;
      this.isOpen = false;
      this._resolve = null;
      this._returnTo = [];
      this._containment?.stop();
      this._containment = null;
      this._releaseInert();
      if (!value) {
        setTimeout(() => focusFirstAvailable(returnTo), 0);
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

    // Mounting: inert page, focus kept inside, then focus on Cancel, the safe
    // default.
    _mount(dialogRoot) {
      this._applyInert(dialogRoot);
      this._containment?.stop();
      this._containment = containFocus(dialogRoot);
      dialogRoot.querySelector('[autofocus]')?.focus();
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
export function reviewDisclosure(disclosure, returnTo) {
  const store = globalThis.Alpine?.store('generationDisclosure');
  if (!store) {
    console.error('generationDisclosure store is not registered; refusing to send');
    return Promise.resolve(false);
  }
  return store.open(disclosure, returnTo);
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
 * elsewhere meanwhile is left there. When Generate cannot take focus (still
 * disabled, or gone), it falls back to `fallbacks` in order.
 */
export async function returnFocusToGenerate(component, region, button, fallbacks = []) {
  await component.$nextTick?.();
  await afterDialogTeardown();
  const active = document.activeElement;
  const nowhere = !active || active === document.body || active === document.documentElement;
  if (nowhere || (region && active === region)) focusFirstAvailable([button, ...fallbacks]);
}
