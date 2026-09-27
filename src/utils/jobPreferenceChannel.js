// Dismissing, pinning and forgetting a Job change what this viewer's Job lists
// show without any Job event, so the live stream never tells the viewer's other
// tabs. Whichever page ran such a command says so on a channel every page of
// the site listens to (the Jobs panel's, in this tab and others), and each
// panel reads its lists again.
const CHANNEL = 'mahresources-job-preferences';
// The commands that keep a Job's record rather than act on its work.
const PREFERENCE_COMMANDS = new Set(['dismiss', 'pin', 'unpin', 'pin-lineage', 'forget']);
const COMMAND_PATH = /^\/v1\/jobs\/(?:([^/?]+)\/)?commands\/([^/?]+)/;

export function openJobPreferenceChannel(onMessage) {
    if (typeof BroadcastChannel === 'undefined') return null;
    try {
        const channel = new BroadcastChannel(CHANNEL);
        if (onMessage) channel.onmessage = event => onMessage(event.data);
        return channel;
    } catch {
        return null;
    }
}

// The preference command a request ran, or null when it ran none: its key, and
// the Jobs it applied to. A single Job's command names its Job in the path; a
// bulk one names its Jobs in its body, and its answer says which of them it
// applied to, so a refused Job is not reported changed.
export function preferenceCommand(url, init = {}, payload = null) {
    if (String(init.method || 'GET').toUpperCase() !== 'POST') return null;
    const path = new URL(String(url), 'http://localhost').pathname;
    const match = COMMAND_PATH.exec(path);
    const command = match ? decodeURIComponent(match[2]) : '';
    if (!PREFERENCE_COMMANDS.has(command)) return null;
    if (match[1]) return { command, jobIds: [decodeURIComponent(match[1])] };
    if (Array.isArray(payload?.results)) {
        return {
            command,
            jobIds: payload.results
                .filter(result => result?.status === 'succeeded' || result?.code === 'applied')
                .map(result => String(result.jobId)),
        };
    }
    try {
        const ids = JSON.parse(init.body || '{}').jobIds;
        return { command, jobIds: Array.isArray(ids) ? ids.map(String) : [] };
    } catch {
        return { command, jobIds: [] };
    }
}

// The Jobs a request changed a viewer preference of, or null when it was not
// such a command.
export function preferenceCommandJobIDs(url, init = {}, payload = null) {
    return preferenceCommand(url, init, payload)?.jobIds ?? null;
}

let sender = null;

// Says, after a request succeeded, which Jobs' preferences it changed and how.
// A channel does not hear its own messages, so a page that also listens passes
// the channel it listens on.
export function announcePreferenceCommand(url, init = {}, payload = null, channel = null) {
    const change = preferenceCommand(url, init, payload);
    if (!change) return;
    const target = channel || (sender ??= openJobPreferenceChannel());
    try {
        target?.postMessage(change);
    } catch {
        // A closed channel has nobody to tell.
    }
}
