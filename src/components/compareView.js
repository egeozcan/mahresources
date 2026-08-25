/**
 * Alpine.js component for managing version comparison UI state.
 * Handles URL state for resource and version selection parameters.
 */
export function compareView(initialState) {
  return {
    r1: initialState.r1,
    v1: initialState.v1,
    r2: initialState.r2,
    v2: initialState.v2,

    /**
     * Updates the URL with current comparison state and navigates to it.
     * This triggers a full page reload to fetch new comparison data.
     */
    updateUrl() {
      const url = new URL(window.location);
      url.searchParams.set('r1', this.r1);
      url.searchParams.set('v1', this.v1);
      url.searchParams.set('r2', this.r2);
      url.searchParams.set('v2', this.v2);
      window.location.href = url.toString();
    },

    /**
     * Handles a selection in a resource picker.
     *
     * The new side's version is left at 0 rather than resolved here: the server
     * fills a missing version in and redirects, and it picks the version matching
     * `CurrentVersionID` rather than the highest number, because a merge can
     * transfer versions whose numbers are higher but describe different files.
     * Resolving it a second time in the browser is the same decision made twice,
     * and the two would drift — which is how a picker could land on a version the
     * server would not have chosen.
     *
     * Only the side that changed moves. Changing one side of a same-resource
     * comparison turns it into a cross-resource one, which is deliberate: it is
     * how you start one from a version panel, and the page now says so — the
     * heading, the breadcrumb and both pane labels name the two resources, and
     * the summary carries the cross-resource badge.
     *
     * @param {'left'|'right'} side - which picker fired
     * @param {object} change - the selector's change record
     */
    onSideSelected(side, change) {
      const added = change && change.added && change.added[0];
      if (!added || !added.raw) return;

      const resourceId = added.raw.ID;
      const currentId = side === 'left' ? this.r1 : this.r2;
      if (String(resourceId) === String(currentId)) return;

      if (side === 'left') {
        this.r1 = resourceId;
        this.v1 = 0;
      } else {
        this.r2 = resourceId;
        this.v2 = 0;
      }

      // A version number only means something relative to its own resource. If
      // the pick has made both sides the same resource, the number still sitting
      // on the other side was counted against a different one — at best it names
      // a different file, at worst no file at all.
      if (String(this.r1) === String(this.r2)) {
        this.v1 = 0;
        this.v2 = 0;
      }

      this.updateUrl();
    },

    onResource1Selected(change) {
      this.onSideSelected('left', change);
    },

    onResource2Selected(change) {
      this.onSideSelected('right', change);
    },

    swapSides() {
      [this.r1, this.r2] = [this.r2, this.r1];
      [this.v1, this.v2] = [this.v2, this.v1];
      this.updateUrl();
    },

    /** Best-effort clipboard write. Reports whether it landed. */
    async copyText(value) {
      try {
        await navigator.clipboard.writeText(value);
        return true;
      } catch (e) {
        return false;
      }
    },
  };
}
