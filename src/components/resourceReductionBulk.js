import { createAndOpen } from './reductionStart.js';
import { defaultReductionName } from './reductionText.js';

/**
 * The bulk-bar handoff into a Resource Reduction.
 *
 * Two arms over one endpoint: start a new Reduction from the current selection,
 * or add the selection to one that already exists.
 *
 * This is deliberately NOT a <form> inside the bulk bar. `bulkSelectionForms`
 * delegates a submit listener on the bar's container that intercepts every form
 * it registered, POSTs it and then morphs the *list* back in place — which is
 * right for "add a tag to these forty" and wrong here, where the whole point is
 * to leave the list and land on the Reduction. The buttons below issue their own
 * request and navigate explicitly.
 *
 * `entity` selects which half of the Extent the selection fills: the resources
 * list sends Resource ids, the groups list sends Group ids, and the server
 * expands a Group through its descendants at compute time rather than here.
 * On group details, `ownerId` instead selects all currently owned Resources on
 * the server, optionally including descendants, without needing a bulk store.
 */
export function reductionBulkAction({ entity = 'resource', ownerId = 0 } = {}) {
  return {
    open: false,
    mode: 'new',
    name: '',
    existingId: '',
    existing: [],
    loadedExisting: false,
    busy: false,
    error: '',
    includeDescendants: false,
    excludeExternalResources: false,
    computeNow: true,

    /** The panel heading: names what is about to be searched. */
    heading() {
      if (ownerId) return "Find duplicates in this group's Resources";
      const count = this.selectedIds().length;
      const noun = entity === 'group' ? 'Group' : 'Resource';
      return `Find duplicates in ${count} selected ${noun}${count === 1 ? '' : 's'}`;
    },

    /** What the Reduction is called when the field is left empty. */
    defaultName() {
      if (ownerId) {
        // The group's own heading: the page this panel sits on is that group.
        const heading = document.getElementById('page-title');
        const groupName = (heading?.querySelector('inline-edit')?.textContent || heading?.textContent || '').replace(/\s+/g, ' ').trim();
        return defaultReductionName({ groupName });
      }
      const count = this.selectedIds().length;
      return defaultReductionName(entity === 'group' ? { groups: count } : { resources: count });
    },

    hasSelection() {
      return ownerId !== 0 || this.selectedIds().length > 0;
    },

    selectedIds() {
      return [...(this.$selection || this.$store.bulkSelection).selectedIds];
    },

    async toggle() {
      this.open = !this.open;
      this.error = '';
      if (this.open) this.revealPanel();
      if (this.open && !this.loadedExisting) {
        await this.loadExisting();
      }
    },

    /**
     * The panel opens beneath its button, which on a group page is often near the
     * bottom of the screen. Once it has finished expanding, bring it into view.
     */
    revealPanel() {
      setTimeout(() => {
        const panel = this.$refs?.panel;
        if (!this.open || !panel) return;
        const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
        panel.scrollIntoView({ block: 'nearest', behavior: reduce ? 'auto' : 'smooth' });
      }, 300);
    },

    /**
     * The list is fetched rather than server-rendered into the bar because the
     * bar is on every list page and most visits never open this control. It is
     * also the caller's own list — the endpoint applies the owner predicate — so
     * "add to an existing one" can only ever offer Reductions the caller may
     * write to.
     */
    async loadExisting() {
      try {
        const response = await fetch('/v1/reductions?sortBy=created_at+desc', {
          headers: { Accept: 'application/json' },
        });
        if (!response.ok) throw new Error(`Server error: ${response.status}`);
        const json = await response.json();
        this.existing = json.reductions || [];
        this.loadedExisting = true;
        if (this.existing.length && !this.existingId) {
          this.existingId = String(this.existing[0].id);
        }
      } catch (err) {
        this.error = `Could not load Resource Reductions: ${err.message}`;
      }
    },

    /** A label that tells two similarly named Reductions apart. */
    optionLabel(reduction) {
      const created = reduction.createdAt ? new Date(reduction.createdAt) : null;
      const when = created && !Number.isNaN(created.valueOf())
        ? created.toLocaleString()
        : '';
      return when ? `${reduction.name} — ${when}` : reduction.name;
    },

    submit() {
      if (this.busy) return;
      if (!this.hasSelection()) {
        this.error = 'Select at least one resource or group.';
        return;
      }
      const body = ownerId
        ? { ownerId, includeDescendants: this.includeDescendants }
        : entity === 'group' ? { groupIds: this.selectedIds() } : { resourceIds: this.selectedIds() };
      if (this.mode === 'existing') {
        if (!this.existingId) {
          this.error = 'Choose a Resource Reduction to add to.';
          return;
        }
        body.id = Number(this.existingId);
      } else {
        body.name = this.name.trim() || this.defaultName();
        body.excludeExternalResources = this.excludeExternalResources;
      }
      void this.post(body);
    },

    async post(body) {
      this.busy = true;
      this.error = '';
      try {
        // The explicit navigation the bulk bar's own submit path would not do.
        await createAndOpen(body, this.computeNow);
      } catch (err) {
        this.error = err.message;
        this.busy = false;
      }
    },
  };
}
