# Plugin command staging

Plugin-declared commands are trusted host processes, not a sandbox: they run as
the server service account with unrestricted process networking and the OS
account's filesystem reach. `PLUGIN_COMMAND_PATH` is therefore a startup trust
boundary. Pin it to the smallest set of absolute directories containing the
declared executables and helpers instead of inheriting a broad service `PATH`.

Command and import bytes live in an OS staging root even when resources use
MemoryFS; in that mode staging and imported resource copies consume RAM. Quotas
are sampled, so brief overshoot is possible, and the per-run quota must cover
merge peak (roughly twice final size while separate audio/video and mux output
coexist). Import streams the admitted source into AddResource's one scratch copy,
so staging peaks near twice the source; the global sample is refreshed outside
admission at startup and after sweeps, and a failed refresh preserves the last
complete sample. Exactly one process may own a staging root. Busy lease
acquisition uses short capped backoff; recovery blockers retain the lease and
retry every five minutes, and a blocker waiting for a live or uninspectable
process group is re-checked every five seconds and recovered as soon as that
group is provably dead. Callers are told the time to that next check.
Quarantine keeps rows nonterminal, withholds command mutations, reports to
`/logs`, and may be healed by terminating the named abandoned process group.

The positive PGID and host-unique boot-session UUID are persisted together; that
identity is host-internal and never enters Lua or administrator views. On Linux
and Darwin an all-zombie group is dead, and signal zero is only conservative
existence evidence. Recovery gets one verified signal attempt per process; a
live creator gets at most two signals and then slow polling. A surviving group
keeps its row nonterminal and its command slot occupied. Sweeps page to a fixed
finish/id boundary in bounded unswept terminal batches so sustained expiry cannot
prevent wraparound and pinned rows cannot starve later work, then stamp
`exchange_removed_at` after deletion or confirmed absence and clear successful
imports' cleanup marker; output pruning retains durable run/import history.

Pending commands/imports stay in dispatcher-owned per-plugin queues; only active
work enters the bounded managed live lane, whose occupancy is derived from the
job registry. Lua callbacks are at-most-once and only list, enqueue or discard;
resource bytes move outside the VM lock. The durable import map is the recovery
source of truth: interrupted claims re-drive with the same id, while actor and
plugin generation are revalidated before work starts. Resource import fields are
not persisted on the claim; a later resubmission supplies them again and worker
admission validates its current series target. Successful imports persist the
resource before unlinking the source; cleanup trouble sets
`source_delete_pending` and leaves the successful import's `error` empty.
Exchange access stays flat, descriptor-relative and no-follow; intentional
actorlessness is stored separately from an actor nulled by deletion.

`mah.fs.set_resource_thumbnail(run_id, name, resource_id)` stays at that exchange
seam rather than requiring a throwaway image Resource. It uses the authorized,
leased, descriptor-safe bounded read, then the existing custom-thumbnail
validation/resize/JPEG transaction under the freshly resolved acting principal.
It needs `commands` plus `db:write`, leaves the exchange source intact, and every
command/filesystem call remains refused inside `mah.db.transaction`.
