// Who says a Job's state change on a page. The Jobs drawer is on every page and
// announces the Jobs it follows: every Job the viewer may see, or, when an
// administrator's drawer lists only their own, those. A page's own Job view
// (the Job Center list, a Job's page) announces a change only when the drawer
// does not follow that Job, so each change is said once, by one region.

let follows = null;

/**
 * Registers the drawer's answer to "do you announce this Job?". The returned
 * function takes it back, and does nothing once another answer replaced it.
 */
export function followJobAnnouncements(predicate) {
    follows = predicate;
    return () => {
        if (follows === predicate) follows = null;
    };
}

/** Whether the drawer announces this Job's changes, so a page must not. */
export function drawerAnnouncesJob(job) {
    try {
        return typeof follows === 'function' && follows(job) === true;
    } catch {
        return false;
    }
}
