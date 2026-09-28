# b4-numbers: scope notes and routed follow-ups

- Follow-up (from batch 1, a11y): progress bars vanish in forced colors (Windows High Contrast). Give each bar a forced-colors rendering (for example a border or `forced-color-adjust` with system colours) on every surface.
- Follow-up (from batch 3): the drawer does not show why a Job is blocked. The detail page does; the drawer row should say it in one short line.
- Follow-up (from batch 3): the drawer orders scheduled Jobs by when they were scheduled (state entry), not by when they start. Scheduled Jobs should lead with the soonest start.
- N4 overlaps the shared state table: state labels come from `server/jobview/job_states.json` (read by Go and by `src/components/jobStates.js`). Change vocabulary there, once, not per surface.
- Batch 3 facts you will meet: `paused` is a real state; a running Job with a pause or cancel intent reads "Pausing" or "Cancelling" (`runningIntents`); progress shows only for a report (a total alone is not one; metrics are).
