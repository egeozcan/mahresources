# b4-kinds: scope notes and routed follow-ups

- Follow-up (with K5, from batch 1): "Download in background" navigates away, so a download that finishes fast can land as history the page never announces. Submit with fetch and stay, or carry a per-tab cursor, whichever fits K5's fix.
- Follow-up (QA L8-21.8): `/v1/jobs/queue` plugin-command entries carry no `canonicalJobId`, and their timestamps mix `Z` and `+02:00` offsets. Make them match the other entries.
- Follow-up (QA L8-21.7): the router's HEAD and 405 behaviour on Job routes. Check what the report says and make it consistent with the rest of the API.
- Follow-up (from batch 1 and 3): `scheduled_download_context.go` (around :888) resolves the deferred-row sweep's actor through `principalForPluginActor`, whose outage log says "plugin" when a download calls it. Name the caller correctly.
- Batch 3 fact: a deleted account at dispatch fails the Job as principal-missing; an account read that fails gives the Job back to the queue (`checks-unanswered`).
