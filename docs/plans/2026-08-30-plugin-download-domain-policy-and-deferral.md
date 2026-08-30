# Plugin download domain policy and deferred downloads

## Context

`mah.download.submit` let a plugin hand one URL to the host download queue and
return immediately, but a plugin that discovered many URLs from one site could
only submit them all at once. The deployment-wide worker budget limited total
background work, not politeness to that source. The same API also had no way to
say "download this later" without keeping plugin code alive.

## Decisions

1. **Declare pacing in the manifest.** `download_limits` sits beside `network`
   because both are static facts about host downloads and both are resolved live
   by plugin name. It is not a Lua registration: there is no handler to keep in
   memory, no teardown to run on disable, and no consent to widen.
2. **Throttle only the plugin's own jobs.** A rule can set `concurrency`,
   `min_interval` and `backoff` for hosts using the same grammar as `network`.
   The first matching rule wins. Limits grant no power and do not affect other
   principals or plugins, so changing them never requires re-consent.
3. **Gate before the global semaphore.** Backoff, per-domain slots and
   min-interval sleeps all happen before a job occupies the shared background
   budget. Otherwise one polite plugin could starve unrelated downloads while it
   is waiting to start.
4. **Reserve starts, don't check-then-sleep.** `min_interval` claims the next
   start time under the gate lock and sleeps outside it. A cancelled sleeper has
   consumed a conservative reservation; that is cheaper than two jobs reading
   the same old `lastStart` and starting together.
5. **Backoff follows the submitted URL.** `429` and `503` feedback is charged to
   the host the plugin submitted, not a redirected final host, because that is
   the key later queued jobs will match. `Retry-After` is honored when present
   and clamped to the declared backoff ceiling.
6. **Deferred downloads are rows, not pending queue jobs.** The in-memory queue
   is capped and never evicts pending jobs, so future work belongs in
   `ScheduledDownload`. A scheduler tick claims due rows and submits ordinary
   download jobs when they are ready.
7. **Deferred downloads remain `db:write`.** They fetch one URL into the library
   under the same authority as `create_resource_from_url` and immediate
   `mah.download.submit`; the new shape only waits. The delay is capped at 30
   days to keep it from becoming recurring unattended plugin work.
8. **Fire-time checks are fresh.** The stored payload is not standing
   permission. Firing resolves the stored actor, checks write role and scope,
   confirms the plugin is still available, and then submits under the stored
   plugin name. Deleting the submitter NULLs ownership, and ownerless pending
   rows are never claimed.
9. **Duplicate prevention is fail-closed.** A due row is reserved as
   `submitted` before the queue side effect so a stale claim cannot submit it
   twice. A crash in that narrow window can strand a submitted row without a job
   id; the accepted trade-off is operational repair rather than two concurrent
   transfers.

## Non-goals

- Host-wide rate limits. A plugin may narrow its own traffic, not control other
  users' downloads to the same domain.
- HLS segment pacing. The gate is job-level; one HLS job still uses the
  deployment's segment concurrency and retry logic.
- Generic jobs. Exports, imports and plugin action jobs have no submitted remote
  URL to key on.
