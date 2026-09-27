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

// The Jobs a request changed a viewer preference of, or null when it was not
// such a command: a single Job's command names its Job in the path, a bulk one
// in its body.
export function preferenceCommandJobIDs(url, init = {}) {
    if (String(init.method || 'GET').toUpperCase() !== 'POST') return null;
    const path = new URL(String(url), 'http://localhost').pathname;
    const match = COMMAND_PATH.exec(path);
    if (!match || !PREFERENCE_COMMANDS.has(decodeURIComponent(match[2]))) return null;
    if (match[1]) return [decodeURIComponent(match[1])];
    try {
        const ids = JSON.parse(init.body || '{}').jobIds;
        return Array.isArray(ids) ? ids.map(String) : [];
    } catch {
        return [];
    }
}

let sender = null;

// Says, after a request succeeded, that it changed these Jobs' preferences.
// A channel does not hear its own messages, so a page that also listens passes
// the channel it listens on.
export function announcePreferenceCommand(url, init = {}, channel = null) {
    const jobIds = preferenceCommandJobIDs(url, init);
    if (!jobIds) return;
    const target = channel || (sender ??= openJobPreferenceChannel());
    try {
        target?.postMessage({ jobIds });
    } catch {
        // A closed channel has nobody to tell.
    }
}
