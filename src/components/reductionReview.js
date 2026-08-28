import Alpine from 'alpinejs';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';

// The Clusters container, addressed by its own attribute rather than through
// findListContainer: that helper takes the first `.items-container` on the page,
// and this page will grow more sections.
const CLUSTERS = '[data-reduction-clusters]';

// The footer pagination, addressed by its landmark label. It lives in the base
// layout, outside the body block, so a `.body` fragment never carries it — the
// refresh below fetches the full page for exactly this reason.
const PAGINATION = 'nav[aria-label="Pagination"]';

/**
 * Force the live cluster checkboxes onto the server's verdict.
 *
 * The checkbox state is server-rendered as an *attribute*, while a reviewer's
 * click sets the DOM *property* — and Alpine.morph's attribute patching never
 * resets a property the user has touched. Any click whose action did not stick
 * (a click swallowed by the busy window, a POST that failed, one cut off by
 * navigating away) therefore leaves a card advertising a decision the server
 * never recorded, and no amount of refreshing repairs it. bfcache and browser
 * form-state restoration then carry the phantom across back-navigation.
 *
 * Called after every morph: the live articles' data-cluster-id attributes have
 * just been patched to the fresh render's values, so matching by cluster id
 * pairs each surviving checkbox with its server truth.
 */
function repairClusterCheckboxes(from, fresh) {
  const truth = new Map();
  for (const box of fresh.querySelectorAll('[data-testid="cluster-checkbox"]')) {
    const card = box.closest('[data-cluster-id]');
    if (card) truth.set(card.getAttribute('data-cluster-id'), box.checked);
  }
  if (truth.size === 0) return;
  for (const box of from.querySelectorAll('[data-testid="cluster-checkbox"]')) {
    const card = box.closest('[data-cluster-id]');
    const serverChecked = card && truth.get(card.getAttribute('data-cluster-id'));
    if (serverChecked !== undefined && box.checked !== serverChecked) {
      box.checked = serverChecked;
    }
  }
}

/**
 * The review surface of a Resource Reduction, as a store.
 *
 * The page is two columns — the Clusters in the body, the controls in the
 * sidebar — and the two are separate Alpine roots, so the state they share (the
 * version, the checked counts, the apply result) cannot live in one component.
 * A store is what both roots can reach, and it is the app's established pattern
 * for state that crosses the body/sidebar split (downloads, confirmDialog,
 * lightbox).
 *
 * Every decision is one POST carrying the version the page last saw, followed by
 * a re-render of the Clusters from the server. Re-rendering rather than patching
 * the DOM in place is deliberate: a promote can eject a member, change the
 * deciding criterion and change the curation warning all at once, and keeping a
 * second copy of that reasoning in JavaScript is how the two would drift.
 *
 * A refused write is shown, not swallowed. The one refusal that matters is the
 * version conflict — something else wrote the plan since this page loaded — and
 * the only correct response to it is to reload, because a decision made against a
 * plan that no longer exists is not a decision about anything.
 */
export function registerReductionReviewStore(Alpine) {
  Alpine.store('reductionReview', {
    reductionId: 0,
    version: 0,
    // Counted by the server over the whole plan. The page can only see its own
    // Clusters, and apply acts on every page — a confirm that counts checkboxes
    // in this DOM understates the blast radius by however many pages the reviewer
    // has not opened.
    checkedCount: 0,
    checkedLoserCount: 0,
    busy: false,
    error: '',
    applyResult: null,
    // Oversized Near-Identical Clusters must be expanded before they can be acted
    // on, so a chained match cannot delete three hundred files behind one
    // checkbox. Keyed by cluster id; nothing is remembered across a reload,
    // deliberately — the gesture is meant to be made each time.
    expanded: {},

    /**
     * Seed the store from the server-rendered page. Called from the body root's
     * x-init, which is the first root in the document; the sidebar's bindings
     * pick the values up through reactivity before the first paint.
     *
     * Not named `init`: Alpine treats an `init` method on a store as its own
     * registration lifecycle hook and calls it with no arguments, which would
     * throw on the required `initial`.
     */
    seed(initial) {
      this.reductionId = initial.reductionId;
      this.version = initial.version;
      this.checkedCount = initial.checkedCount;
      this.checkedLoserCount = initial.checkedLoserCount;
    },

    isExpanded(clusterId, oversized) {
      return !oversized || this.expanded[clusterId] === true;
    },

    expand(clusterId) {
      this.expanded[clusterId] = true;
      window.mahAnnounce?.('Cluster expanded. Its controls are now available.');
    },

    /**
     * Checking an oversized Near-Identical Cluster carries an explicit
     * acknowledgement, which the server requires. The flag says "this reviewer
     * expanded it first" — it is not proof they looked, but it is what stops a
     * caller that is not this page checking three hundred files in one request.
     *
     * The native click toggles the checkbox before anything here runs; when the
     * action is swallowed, revert the control too, or it keeps showing a
     * decision that was never recorded — a state no later refresh can repair
     * (see repairClusterCheckboxes for the other half of that rule).
     */
    check(clusterId, checked, oversized, event) {
      if (this.busy) {
        if (event?.target) event.target.checked = !checked;
        window.mahAnnounce?.('One moment — the previous action is still running.');
        return;
      }
      return this.act(clusterId, checked ? 'check' : 'uncheck', 0, { acknowledgeOversized: oversized && this.expanded[clusterId] === true });
    },

    // The report of the last apply: what merged, and what was refused and why.
    // Held here rather than re-rendered from the row, because a stale Cluster's
    // reason is about the batch that just ran and the row only says the Cluster is
    // stale.

    async apply() {
      if (this.busy) return;
      const count = this.checkedCount;
      if (count === 0) {
        this.error = 'Nothing is checked.';
        return;
      }
      const clusters = `${count} Cluster${count === 1 ? '' : 's'}`;
      const losers = `${this.checkedLoserCount} Resource${this.checkedLoserCount === 1 ? '' : 's'}`;
      const confirmed = await Alpine.store('confirmDialog').ask(
        `${clusters} will be merged and their Losers deleted — ${losers} across every page of this Reduction, not just this one. This cannot be undone.`,
        { title: 'Apply this Resource Reduction?', confirmLabel: 'Apply' },
      );
      if (!confirmed) return;

      this.busy = true;
      this.error = '';
      try {
        const response = await fetch('/v1/reduction/apply', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ id: this.reductionId, version: this.version }),
        });
        if (!response.ok) {
          const message = await window.errorMessageFromResponse(response);
          throw new Error(message);
        }
        this.applyResult = await response.json();
        const applied = this.applyResult.applied?.length || 0;
        const stale = this.applyResult.stale?.length || 0;
        window.mahAnnounce?.(
          `${applied} Cluster${applied === 1 ? '' : 's'} applied, ${this.applyResult.destroyed} Resources deleted.` +
          (stale ? ` ${stale} refused and kept for you to look at.` : ''),
          { assertive: true },
        );
        // Announced before the re-render, and its failure caught separately. The
        // merge has already happened by this point; a shared catch would report
        // "Nothing was applied" over Resources that no longer exist, which is the
        // one thing a destructive surface must never say.
        try {
          await this.refresh();
        } catch (refreshErr) {
          this.error = `Applied, but the page could not be refreshed: ${refreshErr.message}. Reload to see the result.`;
        }
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`Nothing was applied: ${err.message}`, { assertive: true });
      } finally {
        this.busy = false;
      }
    },

    async act(clusterId, action, resourceId = 0, extra = {}) {
      if (this.busy) return;
      this.busy = true;
      this.error = '';
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
        await this.refresh();
        window.mahAnnounce?.(ANNOUNCEMENTS[action] || 'Cluster updated.');
      } catch (err) {
        this.error = err.message;
        window.mahAnnounce?.(`That did not happen: ${err.message}`, { assertive: true });
        // The clicked checkbox still paints the state the server just refused.
        // Re-render from server truth (best effort) so the page never shows a
        // decision that will not be applied.
        try {
          await this.refresh();
        } catch {
          // The action's own error stays on screen if the repair fetch fails too.
        }
      } finally {
        this.busy = false;
      }
    },

    /**
     * Re-render the Clusters from the server.
     *
     * The full page, not the `.body` fragment: the footer pagination lives in
     * the base layout, and an action under a filter can change how many Clusters
     * match, so the pagination has to come back with everything else. The server
     * render cost is identical either way — the context is built fully for a
     * `.body` request too — and only the response's chrome is extra.
     *
     * Morphing rather than replacing keeps focus where the reviewer left it,
     * which matters a great deal here: the whole review is meant to be operable
     * from the keyboard.
     */
    async refresh() {
      const response = await fetch(window.location.href, { headers: { Accept: 'text/html' } });
      if (!response.ok) throw new Error(`Could not refresh the Clusters: ${response.status}`);

      // An out-of-range page — one an action just emptied — is 302'd to the last
      // valid page, and the redirected response IS that page. Morph it in and
      // adopt its URL rather than unloading: a reload would clear the apply
      // report this page exists to show.
      const adopted = new URL(response.url);
      const refreshed = new DOMParser().parseFromString(await response.text(), 'text/html');

      const current = document.querySelector(CLUSTERS);
      const next = refreshed.querySelector(CLUSTERS);
      if (!current || !next) throw new Error('Could not find the refreshed Clusters');
      Alpine.morph(current, next, morphOptionsWithShortcodeElements());
      repairClusterCheckboxes(current, next);

      // The version travels in the page too, so a refresh from any other cause
      // leaves this store agreeing with what was rendered.
      const marker = refreshed.querySelector('[data-reduction-version]');
      if (!marker) throw new Error('Could not find the refreshed page state');
      this.version = Number(marker.dataset.reductionVersion);
      this.checkedCount = Number(marker.dataset.reductionChecked);
      this.checkedLoserCount = Number(marker.dataset.reductionCheckedLosers);

      // The heading's Cluster count is outside the morphed container, and a
      // filter-affecting action (skip, reopen, apply) can change what matches.
      // The refreshed page has already counted the filtered set; copy its
      // figure in, visible text and raw number together.
      const countEl = document.querySelector('[data-reduction-count]');
      const refreshedCount = refreshed.querySelector('[data-reduction-count]');
      if (!countEl || !refreshedCount) throw new Error('Could not find the refreshed Cluster count');
      countEl.textContent = refreshedCount.textContent;
      countEl.dataset.reductionCount = refreshedCount.dataset.reductionCount;

      // The footer pagination, which the base layout renders and the review
      // action just made stale. Three transitions are possible: morph the new
      // nav in over the old one, remove a nav the refreshed page no longer has
      // (the match set dropped to a single page), or — the case a filter can
      // also produce — insert a nav where there was none (Apply moved Clusters
      // into the filtered status, creating a multi-page result). The include
      // sits first in the footer, so that is where an inserted nav belongs.
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
      // is rendered. replaceState so nothing unloads — the apply report and the
      // reviewer's scroll survive.
      if (adopted.pathname + adopted.search !== window.location.pathname + window.location.search) {
        history.replaceState(null, '', adopted.pathname + adopted.search);
      }
    },
  });
}

const ANNOUNCEMENTS = {
  promote: 'Winner changed. Any member with no stored pair to the new Winner has been ejected.',
  eject: 'Member ejected. That Resource is left untouched.',
  restore: 'Member restored to the Cluster.',
  skip: 'Cluster skipped. It will not be applied and will not be re-clustered.',
  reopen: 'Cluster reopened.',
  check: 'Cluster checked. It will be applied.',
  uncheck: 'Cluster unchecked. It will not be applied.',
};
