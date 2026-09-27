// How every Job surface names a Job's Kind, where it was started from, what class
// of failure ended it and why it is blocked. The table is
// server/jobview/job_vocabulary.json, which the server-rendered Job Center list
// reads too, so the drawer, the Job page and a /jobs card say the same words.
import table from '../../server/jobview/job_vocabulary.json';

function lookup(map, key) {
    const raw = String(key ?? '');
    return Object.hasOwn(map, raw) ? map[raw] : raw;
}

/** A Kind as a person reads it; a Kind the table does not know keeps its identifier. */
export function kindLabel(kind) {
    return lookup(table.kinds, kind);
}

/** Where a Job was started from. */
export function originLabel(origin) {
    return lookup(table.origins, origin);
}

/** The class of a Job's failure. */
export function failureClassLabel(failureClass) {
    return lookup(table.failureClasses, failureClass);
}

/**
 * Why a Job is blocked, from the reason its blocked event recorded. A reason the
 * table does not know is read from its code rather than shown as nothing.
 */
export function blockedReasonText(reason) {
    const raw = String(reason ?? '').trim();
    if (!raw) return '';
    if (Object.hasOwn(table.blockedReasons, raw)) return table.blockedReasons[raw];
    const words = raw.replaceAll('-', ' ').trim();
    return words ? `${words[0].toUpperCase()}${words.slice(1)}.` : '';
}
