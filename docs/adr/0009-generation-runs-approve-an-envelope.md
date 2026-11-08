---
status: accepted
---

# A Generation run approves an Envelope, not every message

Template generation becomes a Generation run of up to a capped number of Turns, in which the model asks the server for Tools (entity reads, related-entity navigation, template and revision reads, lint, HTML previews of its own Draft) before it submits. Decision 11 of `docs/plans/2026-10-24-template-generation-quality.md` ("what you approve is what is sent") cannot hold for such a run, because what a Tool returns depends on what the model asks for. We replace it: the Disclosure now states the Envelope, which is everything the account can read plus the opt-ins the user ticks for that run, and the provider, model, Tool list and caps. The `groundingHash` covers those, not the values the Tools will return. Values are read live, and the review panel shows the full transcript of what was sent once the run ends. The account's subtree scope, enforced by the scoped request context, is the only boundary on reads. Previews execute `[mrql]` and plugin shortcodes only when the user ticks the matching opt-in, off by default, because each one reads beyond the entities the Tools would otherwise reach.

## Considered options

- **Precompute every Tool result in the dry run.** Rejected: the reachable set is large, and a preview of HTML the model has not written yet cannot be computed in advance.
- **Approve each Tool call during the run.** Rejected as too tedious for a run of up to 20 Turns.
- **An enumerated set of entity IDs, with a hop cap.** Rejected: the model must be able to walk the graph freely, up to owners and their Categories' templates as well as down, and an ID list without a hop cap bounds nothing.
- **Drop the Disclosure for runs.** Rejected: the user still approves the destination and the opt-ins.

## Consequences

The Disclosure no longer proves which entity values left the server. The transcript after the run is the record, and it is not stored. The continuation rule's version is bumped, since Tool results now carry entity data. The explicit Apply step (decision 10 of the same plan) remains the only boundary between injected entity text and running code, as before.
