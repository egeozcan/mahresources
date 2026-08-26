import Alpine from 'alpinejs';
import { morphOptionsWithShortcodeElements } from '../utils/shortcodeElementMorph.js';

// The Clusters container, addressed by its own attribute rather than through
// findListContainer: that helper takes the first `.items-container` on the page,
// and this page will grow more sections.
const CLUSTERS = '[data-reduction-clusters]';

/**
 * The review surface of a Resource Reduction.
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
export function reductionReview({ reductionId, version, checkedCount, checkedLoserCount }) {
  return {
    reductionId,
    version,
    // Counted by the server over the whole plan. The page can only see its own
    // Clusters, and apply acts on every page — a confirm that counts checkboxes
    // in this DOM understates the blast radius by however many pages the reviewer
    // has not opened.
    checkedCount,
    checkedLoserCount,
    busy: false,
    error: '',
    // Oversized Near-Identical Clusters must be expanded before they can be acted
    // on, so a chained match cannot delete three hundred files behind one
    // checkbox. Keyed by cluster id; nothing is remembered across a reload,
    // deliberately — the gesture is meant to be made each time.
    expanded: {},

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
     */
    check(clusterId, checked, oversized) {
      const action = checked ? 'check' : 'uncheck';
      return this.act(clusterId, action, 0, { acknowledgeOversized: oversized && this.expanded[clusterId] === true });
    },

    // The report of the last apply: what merged, and what was refused and why.
    // Held here rather than re-rendered from the row, because a stale Cluster's
    // reason is about the batch that just ran and the row only says the Cluster is
    // stale.
    applyResult: null,

    async apply() {
      if (this.busy) return;
      const count = this.checkedCount;
      if (count === 0) {
        this.error = 'Nothing is checked.';
        return;
      }
      const clusters = `${count} Cluster${count === 1 ? '' : 's'}`;
      const losers = `${this.checkedLoserCount} Resource${this.checkedLoserCount === 1 ? '' : 's'}`;
      const confirmed = await this.$store.confirmDialog.ask(
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
      } finally {
        this.busy = false;
      }
    },

    /**
     * Re-render the Clusters from the server.
     *
     * The `.body` fragment is the same page without its chrome, which is how the
     * bulk bar refreshes a list. Morphing rather than replacing keeps focus where
     * the reviewer left it, which matters a great deal here: the whole review is
     * meant to be operable from the keyboard.
     */
    async refresh() {
      const url = new URL(window.location);
      url.pathname = url.pathname + '.body';
      const response = await fetch(url.toString());
      if (!response.ok) throw new Error(`Could not refresh the Clusters: ${response.status}`);
      const refreshed = new DOMParser().parseFromString(await response.text(), 'text/html');

      const current = document.querySelector(CLUSTERS);
      const next = refreshed.querySelector(CLUSTERS);
      if (!current || !next) throw new Error('Could not find the refreshed Clusters');
      Alpine.morph(current, next, morphOptionsWithShortcodeElements());

      // The version travels in the page too, so a refresh from any other cause
      // leaves this component agreeing with what was rendered.
      const marker = refreshed.querySelector('[data-reduction-version]');
      if (marker) {
        this.version = Number(marker.dataset.reductionVersion);
        this.checkedCount = Number(marker.dataset.reductionChecked);
        this.checkedLoserCount = Number(marker.dataset.reductionCheckedLosers);
      }
    },
  };
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
