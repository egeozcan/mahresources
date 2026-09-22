# Use one durable Job control plane with specialized executors

Every user- or operator-facing background execution is represented by the same durable Job control plane, which owns identity, normalized lifecycle, visibility, lineage, events, outputs, commands, claims, and retention. Job Kinds keep specialized executors for work such as downloads, plugin VMs, commands, imports, and Resource Reductions; the shared service coordinates them rather than replacing them with one universal worker. This makes the Job Center complete and restart-safe without flattening materially different safety, concurrency, and recovery models into download-shaped behavior.

## Considered options

- **Keep independent registries and aggregate them in the UI.** This preserves each implementation but leaves durability, authorization, command eligibility, retention, and event delivery inconsistent. The current live panel already demonstrates that presentation-level aggregation does not create one reliable Job system.
- **Move every workload into one universal queue and executor.** This creates one apparent source of truth, but plugin VM serialization, process recovery, resumable downloads, imports, and domain-owned computation do not share one safe execution contract. The abstraction would either leak Kind checks everywhere or erase required guarantees.
- **Use a durable control plane with adapters.** The common service owns cross-Kind invariants while adapters retain specialized execution and reconciliation. A new Kind joins by implementing one explicit contract rather than editing central source/status conditionals.

## Consequences

No accepted user-facing Job record may exist only in memory. Restorable work uses at-least-once dispatch with durable claims and Kind-specific reconciliation, never blind re-execution. Legacy closure-backed `mah.start_job` work is non-restorable: proven originating-runtime loss interrupts queued/running records without Retry. A missing adapter blocks pending work instead of falling back to another executor.

The Job Service fences lifecycle, progress, and output publication by execution token. The first release permits only one fenced plugin-command/import runtime per database and its single staging namespace; other Kinds may execute across processes. Only that owner may recover or clean up command work. Neither lease expiry nor a cross-host boot mismatch proves external work stopped: unresolved work stays blocked with its claims and capacity held until quiescence is proven.

Nonterminal Jobs, including blocked work, and their execution-required inputs, keys, and recovery records do not expire as ordinary history. History and optional replay retention start at terminal completion. Kind-specific domain records remain authoritative for their own business state and details.

Cutover retires legacy plaintext replay copies, not just their readers: verify canonical encrypted input, drain and fence old writers, switch execution/replay to canonical envelopes, then scrub legacy payloads and raw secret-bearing URLs before exposing the new surfaces. Resumable migration preserves required nonterminal input. Forget/expiry atomically purges all replay copies with a durable marker that prevents backfill resurrection; compatibility never falls back to plaintext.
