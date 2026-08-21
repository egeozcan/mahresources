/**
 * The "Run now" control on each plugin schedule row.
 *
 * It fetches rather than letting the native form navigate, and that is the whole
 * reason the component exists. A full-page POST-and-redirect loses three things
 * here, none of which it loses on the enable/disable forms above it:
 *
 *   - The row would come back byte-identical. `runs` and `lastStatus` move only
 *     when the run *finishes*, and a manual run deliberately does not move
 *     `nextDueAt`, so the page an operator lands on looks exactly like the one
 *     they left and the button reads as broken.
 *   - A refusal would replace the manage page with a full-page error document.
 *     Every refusal here is ordinary — already running, no longer declared — and
 *     none of them should cost the operator the page they were working on.
 *   - The navigation tears down the jobs panel's EventSource, so the "Action
 *     started" announcement is never delivered; on reconnect the init handler
 *     replays the job silently.
 *
 * The form keeps its method and action, so with JavaScript off the native POST
 * still works and the server's redirect carries a `started` banner instead.
 *
 * Announcement follows the same rule as pluginSettings on this page (Finding 4):
 * the status region stays in the tree and only its text changes. A region that is
 * display:none until it has something to say is not reliably announced.
 */
export function pluginScheduleRun(pluginName, scheduleId) {
    return {
        pluginName,
        scheduleId,
        started: false,
        error: '',
        busy: false,

        async run() {
            if (this.busy) return;
            this.busy = true;
            this.started = false;
            this.error = '';

            try {
                const body = new URLSearchParams();
                body.set('name', this.pluginName);
                body.set('scheduleId', this.scheduleId);

                const response = await fetch('/v1/plugin/schedule/run', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/x-www-form-urlencoded',
                        // Without this the server answers the redirect branch, which
                        // is the no-JS fallback rather than this one.
                        Accept: 'application/json',
                    },
                    body: body.toString(),
                });

                if (!response.ok) {
                    this.error = await refusalMessage(response);
                    return;
                }

                // "Started", never "completed". The server answers once the run has
                // begun, because a handler may take the full async job allowance;
                // whether it succeeded is the jobs panel's announcement and the
                // row's own `runs` and `lastStatus` after a reload.
                this.started = true;
            } catch (err) {
                this.error = err.message;
            } finally {
                this.busy = false;
            }
        },
    };
}

/**
 * The server's own words for a refusal, falling back to the status code.
 *
 * Every refusal on this path is a sentence an operator can act on — the plugin no
 * longer declares this schedule, the schedule has stopped because it has no
 * owner, it is already running — so a generic "request failed" would throw away
 * the only useful part of the answer.
 */
async function refusalMessage(response) {
    const contentType = response.headers.get('Content-Type') || '';
    if (contentType.includes('application/json')) {
        try {
            const data = await response.json();
            if (data && typeof data.error === 'string' && data.error) {
                return data.error;
            }
            if (data && typeof data.message === 'string' && data.message) {
                return data.message;
            }
        } catch {
            // Fall through to the status code below.
        }
    }
    return `Could not start the schedule (${response.status})`;
}
