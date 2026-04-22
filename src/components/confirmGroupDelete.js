/**
 * BH-014: Alpine data component used by the group bulk-delete form.
 *
 * On submit:
 *   1. Reads selected group IDs from $store.bulkSelection.selectedIds.
 *   2. Fetches each group's metadata in parallel via GET /v1/group?id=<id>.
 *   3. Aggregates OwnGroups / OwnNotes / OwnResources counts (up to the
 *      API's page limit of 50 per association — accurate enough for the
 *      orphan-warning message; very large groups get "50+" treatment).
 *   4. Shows a confirm() dialog: "Delete N groups? This will orphan X
 *      child groups and M notes/resources (they'll move to top level)."
 *   5. Leaf-only selections see a simple "Delete N groups?" with no orphan
 *      language.
 *   6. If confirmed, submits the form natively.
 *
 * Escape hatch: holding Shift while submitting bypasses this dialog
 * entirely (same contract as src/components/confirmAction.js). This is
 * intentional — power users who have already verified what they're
 * about to delete can skip the extra round-trip. Do not remove without
 * coordinating with users of that workflow.
 *
 * Fallback: on fetch failure, we still prompt with a generic confirm so
 * delete keeps working — the user just doesn't get the count breakdown.
 */
const PAGE_LIMIT = 50;

function isAtLimit(n) {
    return n >= PAGE_LIMIT;
}

function formatOrphanCount(n, singular, plural) {
    const label = n === 1 ? singular : plural;
    return isAtLimit(n) ? `${PAGE_LIMIT}+ ${label}` : `${n} ${label}`;
}

export function confirmGroupDelete() {
    return {
        _shiftHeld: false,
        _inFlight: false,
        init() {
            this._onKeyDown = (e) => { if (e.key === 'Shift') this._shiftHeld = true; };
            this._onKeyUp   = (e) => { if (e.key === 'Shift') this._shiftHeld = false; };
            document.addEventListener('keydown', this._onKeyDown);
            document.addEventListener('keyup', this._onKeyUp);
        },
        destroy() {
            document.removeEventListener('keydown', this._onKeyDown);
            document.removeEventListener('keyup', this._onKeyUp);
        },
        events: {
            ["@submit"](e) {
                // Shift bypass: power-user escape hatch (see component comment).
                if (this._shiftHeld) return;

                // Re-entry guard: after we call form.submit() below the native
                // submit fires again and re-enters this handler. Let it through.
                if (this._inFlight) return;

                // Prevent the native submit while we fetch counts.
                e.preventDefault();

                const form = e.target;
                const ids = [...(window.Alpine?.store('bulkSelection')?.selectedIds || [])];
                if (ids.length === 0) return;

                this._askAndSubmit(form, ids);
            },
        },
        async _askAndSubmit(form, ids) {
            let childGroups = 0;
            let notes = 0;
            let resources = 0;
            let fetchFailed = false;

            try {
                const results = await Promise.all(ids.map(id =>
                    fetch('/v1/group?id=' + encodeURIComponent(id), {
                        headers: { 'Accept': 'application/json' },
                    }).then(r => r.ok ? r.json() : null).catch(() => null)
                ));
                for (const g of results) {
                    if (!g) { fetchFailed = true; continue; }
                    // Response fields are PascalCase per Go default JSON encoding.
                    childGroups += (g.OwnGroups?.length || 0);
                    notes       += (g.OwnNotes?.length || 0);
                    resources   += (g.OwnResources?.length || 0);
                }
            } catch (_) {
                fetchFailed = true;
            }

            let message;
            const nGroups = `${ids.length} group${ids.length !== 1 ? 's' : ''}`;

            if (fetchFailed) {
                message = `Delete ${nGroups}? (counts unavailable — children will be moved to top level.)`;
            } else if (childGroups === 0 && notes === 0 && resources === 0) {
                message = `Delete ${nGroups}?`;
            } else {
                const parts = [];
                if (childGroups > 0) parts.push(formatOrphanCount(childGroups, 'child group', 'child groups'));
                const items = notes + resources;
                if (items > 0) {
                    // Say "note" vs "note/resource" depending on whether
                    // resources are involved — keeps the message readable.
                    if (resources === 0) {
                        parts.push(formatOrphanCount(notes, 'note', 'notes'));
                    } else if (notes === 0) {
                        parts.push(formatOrphanCount(resources, 'resource', 'resources'));
                    } else {
                        // Cap display at the lower of page limits; combined
                        // count may still be "50+" if either hit the limit.
                        const combined = items;
                        const label = combined === 1 ? 'note/resource' : 'notes/resources';
                        parts.push((isAtLimit(notes) || isAtLimit(resources)) ? `${PAGE_LIMIT}+ ${label}` : `${combined} ${label}`);
                    }
                }
                message = `Delete ${nGroups}? This will orphan ${parts.join(' and ')} (they'll move to top level).`;
            }

            if (window.confirm(message)) {
                // User confirmed — submit natively.
                this._inFlight = true;
                form.submit();
            }
        },
    };
}
