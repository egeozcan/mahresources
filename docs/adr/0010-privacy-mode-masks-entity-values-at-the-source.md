---
status: accepted
---

# Privacy mode masks Entity values at the source, and a guard fails the run closed

Privacy mode replaces every Entity value with a Placeholder before it reaches an AI provider, and it is on by default for every Generation run. Template source, field names, MetaSchema, taxonomy names and the user's own request stay readable, because the model cannot write a template against fields it cannot name. Masking happens where entities are read: the Seed, every Tool result, and the entity (with everything loaded alongside it) that a preview renders against, including `[mrql]` results. A single guard on the outbound transport then scans each request for the run's real values and fails the run if one is present. Placeholders only ever go one way: nothing maps them back into a Draft, a preview or a query, and `submit` refuses a Draft that contains one.

## Considered options

- **Scrub at the boundary only.** Rejected: templates transform values (truncation, `upper`, markdown rendering, `format`, date formatting), so a filter that looks for known strings misses the transformed ones and leaks silently. The boundary is kept as a guard, not as the mechanism.
- **Mask field names, schema text and Category names too.** Rejected for now: the model would write `[meta path="⟦…⟧"]` and every Draft would need reverse substitution. Field names are the user's deliberate vocabulary, not their content.
- **Realistic fake values instead of Placeholders.** Rejected: the model cannot tell a fake from real data and may hard-code one, and the Transcript could no longer show what was masked.
- **Allow plugin previews under Privacy mode.** Rejected: plugin Lua returns arbitrary text that cannot be masked at the source, and the guard catches only exact matches.

## Consequences

Privacy mode still sends template source, partial source, revision content and the user's request as written, so literal text a person typed into a template slot is not protected. An administrator can lock Privacy mode on (`always`), and the server then refuses any template generation request that asks for it off, in harness and classic mode alike. Values that equal a MetaSchema enum or const value of their own field stay readable. The guard skips values of fewer than four characters and values that also appear verbatim in the run's readable text.
