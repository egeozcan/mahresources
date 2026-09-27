// How every Job surface names a state. The table is server/jobview/job_states.json,
// which the server-rendered Job Center list reads too, so the drawer, the Job page
// and a /jobs card cannot say different things about one Job.
import table from '../../server/jobview/job_states.json';
import { formatDuration } from './jobProgress.js';

export const JOB_STATES = Object.freeze(Object.keys(table.states));

function stateName(jobOrState) {
    const raw = typeof jobOrState === 'string' ? jobOrState : jobOrState?.state;
    return String(raw || 'unknown').toLowerCase();
}

function isPartial(job) {
    return typeof job === 'object' && stateName(job) === 'succeeded' && job?.phase === 'partial';
}

/**
 * One state's entry: label, drawer group, tone and whether it is work being done.
 * A succeeded Job its Kind recorded as partial reads as partially completed.
 */
export function presentState(jobOrState) {
    const entry = table.states[stateName(jobOrState)];
    if (!entry) return table.unknown;
    if (isPartial(jobOrState)) return { ...entry, label: table.partial.label, tone: table.partial.tone };
    return entry;
}

/** The states the table puts in one drawer group: attention, active or finished. */
export function statesInGroup(group) {
    return JOB_STATES.filter(state => table.states[state].group === group);
}

/** Every terminal state: what the Job Center's Finished filter lists. */
export function terminalStates() {
    return JOB_STATES.filter(state => table.states[state].terminal);
}

/** Whether an executor is doing the work, the one state a moving bar may claim. */
export function isWorking(jobOrState) {
    return presentState(jobOrState).working === true;
}

function defaultInstantText(date) {
    return date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
}

/**
 * When scheduled work starts, for a Job still waiting for that time: "Starts
 * 28 Sep 2026, 14:17 (in 1 h 30 min)". Empty for any other Job, including one
 * whose scheduled time has come and gone, which is about to start.
 */
export function scheduledStartText(job, now = Date.now(), instantText = defaultInstantText) {
    if (stateName(job) !== 'scheduled' || !job?.scheduledFor) return '';
    const at = new Date(job.scheduledFor);
    if (Number.isNaN(at.getTime())) return '';
    const remaining = (at.getTime() - now) / 1000;
    const when = instantText(at);
    return remaining > 0 ? `Starts ${when} (in ${formatDuration(remaining)})` : `Starts ${when} (due now)`;
}
