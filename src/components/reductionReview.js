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
export function reductionReview({ reductionId, version }) {
  return {
    reductionId,
    version,
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

    async act(clusterId, action, resourceId = 0) {
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
      if (marker) this.version = Number(marker.dataset.reductionVersion);
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
