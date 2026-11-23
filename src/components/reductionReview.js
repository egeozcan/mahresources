import Alpine from 'alpinejs';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';
import { hoistCustomCSS } from '../utils/customCssHoist.js';
import { focusedElement, restoreFocus } from '../utils/focus.js';
import { makeModal } from './reductionModal.js';
import { computeMethods } from './reductionCompute.js';
import { shortcutMethods, readShortcutPreference } from './reductionShortcuts.js';

// The Clusters container, addressed by its own attribute rather than through
// findListContainer: that helper takes the first `.items-container` on the page,
// and this page will grow more sections.
const CLUSTERS = '[data-reduction-clusters]';
const CARD = 'article[data-cluster-id]';

// The regions the server re-renders after each action, matched by position. The
// apply bar, About and the dialogs are not among them: their open state is the
// reviewer's, and the bar reads the store.
const LIVE = '[data-reduction-live]';

// The footer pagination, addressed by its landmark label. It lives in the base
// layout, outside the body block, so a `.body` fragment never carries it — the
// refresh below fetches the full page for exactly this reason.
const PAGINATION = 'nav[aria-label="Pagination"]';

/**
 * Force the approval buttons onto the server's verdict.
 *
 * The approval toggle is a button whose state is the `aria-pressed` attribute, and
 * the server renders it; nothing here sets it optimistically. Morph patches the
 * attribute from the fresh render, so the one divergence left is a button that
 * morph matched to the wrong card, and pairing by cluster id after every morph
 * closes that too. A card advertising a decision the server never recorded is the
 * one state a review of destructive work must not show, and bfcache and form
 * restoration cannot carry it back, because the state is in no form control.
 */
function repairApprovalButtons(from, fresh) {
  const truth = new Map();
  for (const button of fresh.querySelectorAll('[data-testid="cluster-approve"]')) {
    const card = button.closest('[data-cluster-id]');
    if (card) truth.set(card.getAttribute('data-cluster-id'), button.getAttribute('aria-pressed'));
  }
  if (truth.size === 0) return;
  for (const button of from.querySelectorAll('[data-testid="cluster-approve"]')) {
    const card = button.closest('[data-cluster-id]');
    const pressed = card && truth.get(card.getAttribute('data-cluster-id'));
    if (pressed != null && button.getAttribute('aria-pressed') !== pressed) {
      button.setAttribute('aria-pressed', pressed);
    }
  }
}

/** Where keyboard focus is among the Clusters, so a refresh can put it back. */
function cardFocus() {
  const region = document.querySelector(CLUSTERS);
  const active = document.activeElement;
  if (!region || !active || !region.contains(active)) return null;
  const list = [...region.querySelectorAll(CARD)];
  const card = active.closest(CARD);
  return { index: list.indexOf(card), id: card?.dataset.clusterId, onCard: active === card };
}

/**
 * After a refresh: a card that left the view (approved under an Approved filter,
 * skipped from Open) takes focus with it. Focus goes to the card that took its
 * place, or to the previous one at the end of the page.
 */
function restoreCardFocus(before) {
  const region = document.querySelector(CLUSTERS);
  if (!before || !region || region.contains(document.activeElement)) return;
  const list = [...region.querySelectorAll(CARD)];
  const same = list.find((card) => card.dataset.clusterId === before.id);
  const target = same || list[Math.min(Math.max(before.index, 0), list.length - 1)];
  if (target) target.focus();
  else document.getElementById('reduction-clusters')?.focus();
}

/**
 * The review surface of a Resource Reduction, as a store.
 *
 * The page is two columns, the Clusters in the body and the settings in the
 * sidebar, and the two are separate Alpine roots, so the state they share (the
 * version, the counts, the apply result) cannot live in one component. A store is
 * what both roots can reach, and it is the app's established pattern for state
 * that crosses the body/sidebar split (downloads, confirmDialog, lightbox).
 *
 * Every decision is one POST carrying the version the page last saw, followed by
 * a re-render of the live regions from the server. Re-rendering rather than
 * patching the DOM in place is deliberate: a promote can eject a member, change
 * the deciding criterion and change the keeps-smaller warning all at once, and
 * keeping a second copy of that reasoning in JavaScript is how the two would
 * drift. The bulk actions are what remove the per-click cost.
 *
 * A refused write is shown, not swallowed. The one refusal that matters is the
 * version conflict (something else wrote the plan since this page loaded), and
 * the only correct response to it is to reload, because a decision made against a
 * plan that no longer exists is not a decision about anything.
 *
 * Compute, keyboard review and the dialogs' modality live in their own modules
 * and are spread in below.
 */
export function registerReductionReviewStore(Alpine) {
  Alpine.store('reductionReview', {
    reductionId: 0,
    version: 0,
    status: '',
    jobId: '',
    // Counted by the server over the whole plan. The page can only see its own
    // Clusters, and apply acts on every page: a confirm that counts cards in this
    // DOM understates the blast radius by however many pages the reviewer has not
    // opened.
    checkedCount: 0,
    checkedLoserCount: 0,
    openCount: 0,
    figures: { identical: { clusters: 0, losers: 0, keepAsVersion: false }, near: { clusters: 0, losers: 0, keepAsVersion: false }, keepsSmaller: 0 },
    bulkFigures: { identicalApprovable: 0, identicalOpen: 0 },
    busy: false,
    // An apply running here, and one the plan says is running (another tab's, or a
    // lost response's). Either blocks every write.
    localApplying: false,
    serverApplying: false,
    applyingCount: 0,
    error: '',
    applyResult: null,
    bulkNotice: null,
    // The compute-start failure the create flow left in sessionStorage: a message,
    // or true when there is one but no reason to give.
    startError: '',
    progress: { text: 'Starting…', count: 0, total: 0 },
    dialog: '',
    moreOpen: false,
    shortcutsOn: true,
    // Oversized Near-Identical Clusters must be expanded before they can be acted
    // on, so a chained match cannot delete three hundred files behind one
    // approval. Keyed by cluster id; nothing is remembered across a reload,
    // deliberately: the gesture is meant to be made each time.
    expanded: {},

    get applying() {
      return this.localApplying || this.serverApplying;
    },

    ...computeMethods,
    ...shortcutMethods,

    /**
     * Seed the store from the server-rendered page. Called from the body root's
     * x-init, which is the first root in the document.
     *
     * Not named `init`: Alpine treats an `init` method on a store as its own
     * registration lifecycle hook and calls it with no arguments.
     */
    seed() {
      this.shortcutsOn = readShortcutPreference();
      this._readState(document);
      try {
        const key = `reduction-compute-error-${this.reductionId}`;
        const message = window.sessionStorage.getItem(key);
        if (message !== null) {
          window.sessionStorage.removeItem(key);
          this.startError = message || true;
        }
      } catch {
        // Without storage the sentence has no reason to give, and nothing says there was one.
      }
      if (this.status === 'computing') this.watchCompute();
    },

    /** Take the numbers the server rendered, from the page or from a refreshed copy of it. */
    _readState(doc) {
      const node = doc.querySelector('[data-reduction-state]');
      if (!node) throw new Error('Could not find the refreshed page state');
      const state = JSON.parse(node.textContent);
      this.reductionId = state.id;
      this.version = state.version;
      this.status = state.status;
      this.jobId = state.jobId;
      this.checkedCount = state.checked;
      this.checkedLoserCount = state.checkedLosers;
      this.openCount = state.open;
      this.figures = state.figures;
      this.bulkFigures = state.bulk;
      // From the plan: another tab's apply blocks this one's, and a finished one
      // does not.
      this.serverApplying = state.applying;
    },

    isExpanded(clusterId, oversized) {
      return !oversized || this.expanded[clusterId] === true;
    },

    expand(clusterId) {
      this.expanded[clusterId] = true;
      window.mahAnnounce?.('Cluster expanded.');
    },

    /**
     * Toggle a Cluster's approval from its button.
     *
     * Approving an oversized Near-Identical Cluster carries an explicit
     * acknowledgement, which the server requires. The flag says "this reviewer
     * expanded it first": it is not proof they looked, but it is what stops a
     * caller that is not this page approving three hundred files in one request.
     */
    toggleApproval(clusterId, oversized, event) {
      if (this.busy) return;
      const pressed = event?.currentTarget?.getAttribute('aria-pressed') === 'true';
      return this.act(clusterId, pressed ? 'uncheck' : 'check', 0, {
        acknowledgeOversized: oversized && this.expanded[clusterId] === true,
      });
    },

    async act(clusterId, action, resourceId = 0, extra = {}) {
      if (this.busy) return;
      this.busy = true;
      this.error = '';
      this.bulkNotice = null;
      const focus = cardFocus();
      try {
        const response = await fetch('/v1/reduction/cluster', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({
            id: this.reductionId,
            version: this.version,
            clusterId,
            action,
            resourceId,
            ...extra,
          }),
        });
        if (!response.ok) {
          const message = await window.errorMessageFromResponse(response);
          throw new Error(message);
        }
        const json = await response.json();
        this.version = json.version;
        await this.refresh(focus);
        window.mahAnnounce?.(ANNOUNCEMENTS[action] || 'Cluster updated.');
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Could not update Cluster: ${err.message}`, { assertive: true });
        // Re-render from server truth (best effort) so the page never shows a
        // decision that will not be applied.
        try {
          await this.refresh(focus);
        } catch {
          // The action's own error stays on screen if the repair fetch fails too.
        }
      } finally {
        this.busy = false;
      }
    },

    /**
     * Approve or clear Clusters in bulk. `scope` is 'page' (the open Clusters on
     * screen, any tier, because the reviewer has them in front of them) or
     * 'identical' (every open Identical Cluster in the plan, which carries the
     * number on the button so the server can refuse a count that has moved).
     * The server never bulk-approves a Cluster that needs attention.
     */
    async bulk(action, scope) {
      if (this.busy || this.applying) return;
      const body = { id: this.reductionId, version: this.version, action };
      if (scope === 'page') {
        body.clusterIds = [...document.querySelectorAll(`${CLUSTERS} ${CARD}[data-cluster-state="open"]`)]
          .map((card) => card.dataset.clusterId);
        if (body.clusterIds.length === 0) {
          this.bulkNotice = { text: 'There are no open Clusters on this page.', needsReview: false };
          window.mahAnnounce?.(this.bulkNotice.text);
          return;
        }
      } else {
        body.allIdentical = true;
        body.expectedCount = action === 'check' ? this.bulkFigures.identicalApprovable : this.bulkFigures.identicalOpen;
      }

      this.busy = true;
      this.error = '';
      this.bulkNotice = null;
      this.moreOpen = false;
      try {
        const response = await fetch('/v1/reduction/clusters', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify(body),
        });
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        const json = await response.json();
        this.version = json.version;
        const needs = json.needsReview || 0;
        const text = action === 'check'
          ? `Approved ${json.changed}.${needs ? ` ${needs} need individual review.` : ''}`
          : `Cleared ${json.changed}.`;
        const focus = cardFocus();
        await this.refresh(focus);
        this.bulkNotice = { text, needsReview: needs > 0 };
        window.mahAnnounce?.(text);
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Could not update Clusters: ${err.message}`, { assertive: true });
        // A count that has moved, or a plan that has: show the truth.
        try { await this.refresh(); } catch { /* the action's own error stays */ }
      } finally {
        this.busy = false;
      }
    },

    toggleMore() {
      this.moreOpen = !this.moreOpen;
    },

    closeMore() {
      if (!this.moreOpen) return;
      this.moreOpen = false;
      document.querySelector('[data-testid="reduction-more"]')?.focus();
    },

    focusSettings() {
      // Below 900px the sidebar is a collapsed disclosure; open it first.
      const disclosure = document.getElementById('sidebar-disclosure');
      if (disclosure && !disclosure.open) disclosure.open = true;
      const field = document.querySelector('#reduction-settings input[name="name"]');
      if (!field) return;
      field.scrollIntoView({ block: 'center' });
      field.focus();
    },

    // --- Dialogs -----------------------------------------------------------

    openDialog(kind, event) {
      if (this.dialog) return;
      this._opener = event?.currentTarget?.nodeType === 1 ? event.currentTarget : focusedElement();
      this.dialog = kind;
    },

    closeDialog() {
      if (!this.dialog) return;
      const opener = this._opener;
      this.dialog = '';
      this._opener = null;
      this._release?.();
      this._release = null;
      // Restore focus only once Alpine has torn the dialog down: moved sooner it
      // is pulled back inside, and then the subtree is removed and focus falls to
      // <body>.
      setTimeout(() => {
        restoreFocus(opener, document.getElementById('reduction-clusters'));
      }, 0);
    },

    escapeDialog() {
      // A running apply cannot be dismissed; the server finishes it either way.
      if (this.applying) return;
      this.closeDialog();
    },

    _mountDialog(root) {
      this._release?.();
      this._release = makeModal(root);
      // After the inner x-if has rendered its buttons.
      setTimeout(() => (root.querySelector('[autofocus]') || root).focus(), 0);
    },

    /** The per-tier lines of the apply dialog, from the server's figures. */
    tierSummaries() {
      const out = [];
      for (const [key, label, same] of [['identical', 'Identical', true], ['near', 'Near-Identical', false]]) {
        const tier = this.figures[key];
        if (!tier || tier.clusters === 0) continue;
        const clusters = `${tier.clusters} Cluster${tier.clusters === 1 ? '' : 's'}`;
        const copies = `${tier.losers} cop${tier.losers === 1 ? 'y' : 'ies'} removed`;
        const files = tier.keepAsVersion
          ? 'Their files are added as versions of the kept copy.'
          : 'Their files move to the deleted-files backup unless a version still uses them.';
        out.push({
          key,
          headline: `${label}: ${clusters}, ${copies}.`,
          detail: `${same ? 'They have the same content as the kept copy. ' : ''}${files}`,
        });
      }
      return out;
    },

    keepsSmallerHref() {
      return `/reduction?id=${this.reductionId}&status=open&approval=approved&attention=true`;
    },

    // --- Apply -------------------------------------------------------------

    apply(event) {
      if (this.busy || this.applying) return;
      if (this.checkedCount === 0) {
        this.error = 'Approve at least one Cluster to apply.';
        return;
      }
      this.openDialog('apply', event);
    },

    async confirmApply() {
      if (this.busy || this.applying) return;
      this.busy = true;
      this.localApplying = true;
      this.applyingCount = this.checkedCount;
      this.error = '';
      this.bulkNotice = null;
      // The dialog's buttons are about to be replaced; keep focus on the dialog.
      setTimeout(() => document.querySelector('[data-testid="reduction-dialog"]')?.focus(), 0);
      let outcome = null;
      try {
        const response = await fetch('/v1/reduction/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ id: this.reductionId, version: this.version }),
        });
        if (!response.ok) throw new Error(await window.errorMessageFromResponse(response));
        outcome = await response.json();
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Nothing was applied: ${err.message}`, { assertive: true });
      }
      this.localApplying = false;

      if (outcome) {
        this.applyResult = outcome;
        const applied = outcome.applied?.length || 0;
        const stale = outcome.stale?.length || 0;
        window.mahAnnounce?.(
          `${applied} Cluster${applied === 1 ? '' : 's'} applied, ${outcome.destroyed} cop${outcome.destroyed === 1 ? 'y' : 'ies'} removed.` +
          (stale ? ` ${stale} could not be applied. See the report.` : ''),
          { assertive: true },
        );
        // Then the Open tab: what is left to do.
        window.history.replaceState(window.history.state, '', `/reduction?id=${this.reductionId}`);
      }
      // Announced before the re-render, and its failure caught separately. The
      // merge has already happened by this point; a shared catch would report
      // "Nothing was applied" over Resources that no longer exist, which is the
      // one thing a destructive surface must never say.
      try {
        await this.refresh();
      } catch (refreshErr) {
        this.error = outcome
          ? `Applied, but the page could not be refreshed: ${refreshErr.message}. Reload to see the result.`
          : this.error;
      }
      this.busy = false;
      // The dialog closes onto the report, which is where the reviewer's next
      // decision is.
      const report = document.getElementById('reduction-apply-result');
      this.closeDialog();
      if (outcome && report) {
        this._opener = null;
        setTimeout(() => report.focus(), 0);
      }
    },

    /**
     * Re-render the live regions from the server.
     *
     * The full page, not the `.body` fragment: the footer pagination lives in
     * the base layout, and an action under a filter can change how many Clusters
     * match, so the pagination has to come back with everything else. The server
     * render cost is identical either way: the context is built fully for a
     * `.body` request too.
     *
     * Morphing rather than replacing keeps focus where the reviewer left it,
     * which matters a great deal here: the whole review is meant to be operable
     * from the keyboard.
     */
    async refresh(focus = null) {
      const response = await fetch(window.location.href, { headers: { Accept: 'text/html' } });
      if (!response.ok) throw new Error(`Could not refresh the Clusters: ${response.status}`);

      // An out-of-range page (one an action just emptied) is 302'd to the last
      // valid page, and the redirected response IS that page. Morph it in and
      // adopt its URL rather than unloading: a reload would clear the apply
      // report this page exists to show.
      const adopted = new URL(response.url);
      const refreshed = new DOMParser().parseFromString(await response.text(), 'text/html');

      const current = [...document.querySelectorAll(LIVE)];
      const next = [...refreshed.querySelectorAll(LIVE)];
      if (current.length === 0 || current.length !== next.length) throw new Error('Could not find the refreshed Clusters');
      const before = focus || cardFocus();
      current.forEach((region, i) => {
        hoistCustomCSS(next[i]);
        Alpine.morph(region, next[i], morphOptionsWithShortcodeElements());
        repairApprovalButtons(region, next[i]);
      });

      // The version travels in the page too, so a refresh from any other cause
      // leaves this store agreeing with what was rendered.
      this._readState(refreshed);

      // The footer pagination, which the base layout renders and the review
      // action just made stale. Three transitions are possible: morph the new
      // nav in over the old one, remove a nav the refreshed page no longer has
      // (the match set dropped to a single page), or insert a nav where there was
      // none (Apply moved Clusters into the filtered status, creating a
      // multi-page result). The include sits first in the footer, so that is
      // where an inserted nav belongs.
      const footer = document.querySelector('footer.footer');
      const nav = document.querySelector(PAGINATION);
      const nextNav = refreshed.querySelector(PAGINATION);
      if (nav && nextNav) {
        Alpine.morph(nav, nextNav, morphOptionsWithShortcodeElements());
      } else if (nav && !nextNav) {
        nav.remove();
      } else if (!nav && nextNav && footer) {
        footer.insertBefore(nextNav, footer.firstChild);
      }

      // Adopt the server's verdict on which page is current: an out-of-range
      // page was redirected to the last valid one, and the URL must match what
      // is rendered. replaceState so nothing unloads: the apply report and the
      // reviewer's scroll survive. The entry's state is kept: the media viewer's
      // marker may be on it, and without that marker closing the viewer would
      // leave its entry behind for Back to step through.
      if (adopted.pathname + adopted.search !== window.location.pathname + window.location.search) {
        history.replaceState(history.state, '', adopted.pathname + adopted.search);
      }

      restoreCardFocus(before);
    },
  });
}

const ANNOUNCEMENTS = {
  promote: 'Kept copy changed. Copies that do not match it were left out.',
  eject: 'Copy left out of the Cluster. It will be kept.',
  restore: 'Copy included again.',
  skip: 'Cluster skipped. Excluded from applying and recomputing.',
  reopen: 'Cluster reopened.',
  check: 'Cluster approved for the next apply.',
  uncheck: 'Cluster approval cleared.',
};
