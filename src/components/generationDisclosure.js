import { blockingModal, refuseOverModal } from '../utils/modality.js';
import { containFocus, focusFirstAvailable } from '../utils/containFocus.js';

/**
 * The template generation disclosure, shown before a generation request is sent,
 * from a dry run of the same request. A classic disclosure lists exactly what goes
 * to the provider (decision 11 of docs/plans/2026-10-24-template-generation-quality.md).
 * A harness disclosure approves the Seed messages, the Tools, the caps, the opt-ins
 * and the Envelope (everything the account can read); what the Tools return is read
 * live and shown in the transcript afterwards (docs/adr/0009-generation-runs-approve-an-envelope.md).
 * Send resolves the approval tuple the caller sends with the
 * request; Cancel and Escape resolve null and nothing is sent.
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
    _owner: null,
    _redisclose: null,
    _seq: 0,
    _controller: null,
    _confirmed: { previewMRQL: false, previewPlugins: false },
    previewMRQL: false,
    previewPlugins: false,
    redisclosing: false,
    redisclosureError: '',
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
      return (this.disclosure?.destination?.profiles || []).map((p) => {
        const reasoning = p.reasoningMode === 'provider_default'
          ? 'reasoning: provider default'
          : `thinking ${p.thinking ? 'on' : 'off'}`;
        const timeout = p.timeout ? `, timeout ${p.timeout}` : '';
        return `${p.kind}: ${p.model}, ${reasoning}, up to ${p.maxTokens} output tokens, at most ${p.maxCalls} call${p.maxCalls === 1 ? '' : 's'}${timeout}`;
      });
    },

    // How every call is sent: the fixed transport settings.
    get transport() {
      const t = this.disclosure?.transport;
      if (!t) return [];
      const lines = [
        `POST ${this.endpoint}`,
        `Content-Type: ${t.contentType}`,
        `Authorization: ${t.authorization}`,
        `stream: ${t.stream ? 'true' : 'false'}, ${t.responseFormat === 'prompt_json' ? 'JSON requested in prompt' : `response_format: ${t.responseFormat}`}`,
      ];
      if (t.apiVersion) lines.push(`Anthropic-Version: ${t.apiVersion}`);
      if (t.options) lines.push(`Options: ${t.options}`);
      const deadline = this.disclosure?.destination?.bundleDeadline;
      if (deadline) lines.push(`Whole-template deadline: ${deadline}`);
      return lines;
    },

    get continuationRule() {
      return this.disclosure?.continuationRule?.text || '';
    },

    get sections() {
      return this.disclosure?.sections || [];
    },

    // Send is allowed only when no re-disclosure is in flight or failed: the shown
    // hash must belong to the opt-ins ticked.
    get canSend() {
      return this.isOpen && !this.redisclosing && !this.redisclosureError;
    },

    // Harness disclosures carry the opt-ins; classic ones render as before.
    get harness() {
      return !!this.disclosure?.optIns;
    },

    get tools() {
      return this.disclosure?.tools || [];
    },

    get caps() {
      return this.disclosure?.caps || null;
    },

    get envelope() {
      return this.disclosure?.envelope || '';
    },

    // Each Tool as the model sees it: name, description and parameter schema, as text.
    get toolRows() {
      return this.tools.map((t) => ({
        name: t.name,
        description: t.description || '',
        parameters: t.parameters === undefined ? '' : JSON.stringify(t.parameters, null, 2),
      }));
    },

    // The run's limits, one line each, with what the token budget counts.
    get capLines() {
      const c = this.caps;
      if (!c) return [];
      const lines = [
        `Turns: up to ${c.maxTurns}`,
        `Run deadline: ${c.deadline}`,
        `Token budget: ${c.tokenBudget}${c.budgetNote ? `. ${c.budgetNote}` : ''}`,
        `Tool calls per Turn: up to ${c.callsPerTurn}`,
        `Tool timeout: ${c.toolTimeout}; preview timeout: ${c.previewTimeout}`,
      ];
      for (const r of c.resultCaps || []) {
        lines.push(`${r.tool} result: at most ${r.bytes} bytes${r.entries ? `, ${r.entries} entries` : ''}`);
      }
      return lines;
    },

    /**
     * Show `disclosure`. Resolves an immutable { groundingHash, previewMRQL,
     * previewPlugins } when the reader presses Send, else null (Cancel, Escape,
     * cancel(owner)). `owner` is the token cancel(owner) matches; `redisclose`
     * (requested, signal) => Promise<disclosure> repeats the dry run with the
     * frozen request body when an opt-in is toggled. On Cancel focus goes to the
     * first of `returnTo` that can take it; on Send the caller moves focus.
     */
    open(disclosure, returnTo = [], { owner = null, redisclose = null } = {}) {
      if (this.isOpen) return Promise.resolve(null);
      const blocker = blockingModal();
      if (blocker) {
        refuseOverModal(blocker, DISCLOSURE_REFUSED_OVER_DIALOG);
        return Promise.resolve(null);
      }
      this.disclosure = disclosure;
      this._returnTo = Array.isArray(returnTo) ? returnTo : [returnTo];
      this._owner = owner;
      this._redisclose = redisclose;
      this.previewMRQL = !!disclosure?.optIns?.previewMRQL;
      this.previewPlugins = !!disclosure?.optIns?.previewPlugins;
      this._confirmed = { previewMRQL: this.previewMRQL, previewPlugins: this.previewPlugins };
      this.redisclosing = false;
      this.redisclosureError = '';
      this.isOpen = true;
      return new Promise((resolve) => {
        this._resolve = resolve;
      });
    },

    // Toggle an opt-in and repeat the dry run. Only the newest request may change
    // the shown disclosure; older ones are aborted and their answers ignored.
    setOptIn(name, value) {
      if (!this.isOpen || !this._redisclose) return;
      if (name !== 'previewMRQL' && name !== 'previewPlugins') return;
      this[name] = !!value;
      const seq = ++this._seq;
      this._controller?.abort();
      const controller = new AbortController();
      this._controller = controller;
      const requested = { previewMRQL: this.previewMRQL, previewPlugins: this.previewPlugins };
      this.redisclosing = true;
      this.redisclosureError = '';
      const stale = () => seq !== this._seq || !this.isOpen;
      let request;
      try {
        request = Promise.resolve(this._redisclose(requested, controller.signal));
      } catch (err) {
        request = Promise.reject(err);
      }
      request
        .then((next) => {
          if (stale()) return;
          if (!next?.groundingHash) throw new Error('The server returned no disclosure.');
          const echoed = next.optIns;
          if (echoed && (!!echoed.previewMRQL !== requested.previewMRQL || !!echoed.previewPlugins !== requested.previewPlugins)) {
            throw new Error('The server did not apply the options you chose.');
          }
          this.disclosure = next;
          this._confirmed = requested;
          this.redisclosing = false;
        })
        .catch((err) => {
          if (stale()) return;
          this.redisclosing = false;
          this.redisclosureError = err?.message || 'Could not update what will be sent.';
        });
    },

    send() {
      if (!this.canSend) return;
      this._settle(Object.freeze({
        groundingHash: this.disclosure.groundingHash,
        previewMRQL: this._confirmed.previewMRQL,
        previewPlugins: this._confirmed.previewPlugins,
      }));
    },

    // The dialog's own Cancel and Escape.
    dismiss() {
      this._settle(null);
    },

    // A host tearing down or superseding its run: settles only the review it opened.
    cancel(owner) {
      if (owner == null || owner !== this._owner) return;
      this._settle(null);
    },

    _settle(value) {
      if (!this.isOpen) return;
      const resolve = this._resolve;
      const returnTo = this._returnTo;
      this.isOpen = false;
      this._resolve = null;
      this._returnTo = [];
      this._owner = null;
      this._redisclose = null;
      this._seq++;
      this._controller?.abort();
      this._controller = null;
      this.redisclosing = false;
      this.redisclosureError = '';
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
      const first = dialogRoot.querySelector('[autofocus]');
      first?.focus();
      // On a very short viewport the whole panel scrolls (see the dialog template), and
      // focus alone does not always bring a button at its end into view.
      // The panel's own sections render after this runs, so wait for them to lay out.
      setTimeout(() => {
        if (this.isOpen && first && document.activeElement === first) first.scrollIntoView?.({ block: 'nearest' });
      }, 0);
    },

    _releaseInert() {
      for (const el of this._inerted) el.inert = false;
      this._inerted = [];
    },
  });
}

/**
 * Ask the reader to approve a disclosure, from a plain module. Resolves the
 * approval tuple or null. Fails closed: without the store nothing is sent.
 */
export function reviewDisclosure(disclosure, returnTo, options = {}) {
  const store = globalThis.Alpine?.store('generationDisclosure');
  if (!store) {
    console.error('generationDisclosure store is not registered; refusing to send');
    return Promise.resolve(null);
  }
  return store.open(disclosure, returnTo, options);
}

/** Settle the review `owner` opened, if it is still the open one. */
export function cancelReview(owner) {
  globalThis.Alpine?.store('generationDisclosure')?.cancel(owner);
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
