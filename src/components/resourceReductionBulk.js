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
 */
export function reductionBulkAction({ entity = 'resource' } = {}) {
  return {
    open: false,
    mode: 'new',
    name: '',
    existingId: '',
    existing: [],
    loadedExisting: false,
    busy: false,
    error: '',

    selectedIds() {
      return [...this.$store.bulkSelection.selectedIds];
    },

    async toggle() {
      this.open = !this.open;
      this.error = '';
      if (this.open && !this.loadedExisting) {
        await this.loadExisting();
      }
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
        this.error = `Could not load your Resource Reductions: ${err.message}`;
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
      const ids = this.selectedIds();
      if (!ids.length) {
        this.error = 'Select something first.';
        return;
      }
      const body = entity === 'group' ? { groupIds: ids } : { resourceIds: ids };
      if (this.mode === 'existing') {
        if (!this.existingId) {
          this.error = 'Choose a Resource Reduction to add to.';
          return;
        }
        body.id = Number(this.existingId);
      } else {
        body.name = this.name.trim();
      }
      void this.post(body);
    },

    async post(body) {
      this.busy = true;
      this.error = '';
      try {
        const response = await fetch('/v1/reduction', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify(body),
        });
        if (!response.ok) {
          const message = await window.errorMessageFromResponse(response);
          throw new Error(message);
        }
        const json = await response.json();
        // The explicit navigation the bulk bar's own submit path would not do.
        window.location.href = json.url;
      } catch (err) {
        this.error = err.message;
        this.busy = false;
      }
    },
  };
}
