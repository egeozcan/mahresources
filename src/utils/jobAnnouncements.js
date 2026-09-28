// Who says a Job's state change on a page. The Jobs drawer is on every page and
// keeps the one record of what the reader has been told about each Job (its
// ledger). A page's own Job view (the Job Center list, a Job's page) sees
// changes on its cards that the drawer may never read, since the drawer's lists
// are capped, so it hands each change to the drawer. The drawer says the ones it
// follows unless its ledger has already heard them, and hands back the rest for
// the page to say. Each change is then said once, by one region, whichever of
// the two saw it first.

let drawer = null;

/**
 * Registers the drawer: `hear(changes)` takes a page's changes, each
 * `{ previous, next }` (the Job as the page showed it before and after), and
 * returns the ones the drawer leaves to the page. The returned function takes
 * the registration back, and does nothing once another replaced it.
 */
export function followJobAnnouncements(announcer) {
    drawer = announcer;
    return () => {
        if (drawer === announcer) drawer = null;
    };
}

/**
 * Hands a page's state changes to the drawer and returns the ones the page must
 * say itself: all of them when no drawer is on the page, or when it fails.
 */
export function tellDrawerOfJobs(changes) {
    if (!changes.length || typeof drawer?.hear !== 'function') return changes;
    try {
        const left = drawer.hear(changes);
        return Array.isArray(left) ? left : changes;
    } catch {
        return changes;
    }
}
