# Plugin invocation and hook integrity

Status: **implemented** (see `docs/todo.md` for the build record and the mutation
tests). Written as a plan against `ebdb29b6`; the body below is the plan as
approved, and "What shipped differently" at the end records where the build
diverged from it.

Every line reference below was measured against `ebdb29b6`, not taken from the
capability report. Where the report's own numbers differ, the measured number is
used and the difference is noted.

This is the next package after the twelve low-hanging-fruit items (`docs/todo.md`),
of which eleven shipped and **item 01 did not**. Item 01 is the gate: the report
names it the prerequisite for scope-aware plugin data, and part four's ordering is
"**A** first", where A is item 01 plus a pooled LState and the lifted deny.

The package is item 01 **plus the two open sharp edges that live in the same code**,
because item 01's headline payoff is attributed writes from hooks, and the hook
surface currently both hangs (§2) and silently skips (§3).

---

## 0. Scope

**In:**

1. **The invocation** — every `mah.db.*` call runs as the principal that triggered
   it, and `mah.start_job` produces a job its submitter can see. (Item 01.)
2. **The self-hook deadlock** — a plugin that hooks an event and writes that same
   entity type wedges its VM forever. (Sharp edge #2.)
3. **Hook coverage on bulk paths** — resource bulk-delete and merge fire no delete
   hooks at all; note and tag bulk paths fire their after-hook inside the still-open
   transaction. (Sharp edge #3.)

**Explicitly out, and why:**

| Deferred | To | Reason |
|---|---|---|
| Lifting the group-confined plugin deny | Package 3 | The report is explicit that the deny-lift widens the blast radius of any plugin bug to confined users and "should land behind a per-plugin grant (item G) rather than flipping on globally". |
| The LState pool | Package 3 | It is a real semantic break — plugin-global Lua tables stop being process singletons — and it belongs with the still-open enable/disable ABA question and the failed-`init()` defect. |
| `mah.db.transaction(fn)` | Package 3 | Needs the per-call adapter this package builds, but is a separate API. |
| Egress control on `mah.http` (sharp edge #4) | Package 2 (item G) | It is the same change as the per-plugin host allowlist. |
| Server-side re-check of `action.Filters` | Package 2 | Authorization-shaped, and G is the authorization package. |
| Async after-hook delivery | Item B | Explicitly a scheduler/event-bus concern. §2 is careful not to pre-empt it. |

**The load-bearing property of this scope:** nothing in it widens what any principal
can reach. Attribution and hook correctness are strictly safety-improving, so this
package can ship *before* item G without violating the report's own caution about
ordering. That is what dissolves the A-before-G / G-before-A tension.

---

## 1. The invocation (item 01)

### 1.1 What is broken, measured

One adapter is constructed at wiring time and shared by every plugin call, forever:

```go
// application_context/context.go:623
adapter := &pluginDBAdapter{ctx: ctx}
pm.SetEntityQuerier(adapter)
pm.SetEntityWriter(adapter)
```

`pluginDBAdapter` is a one-field struct (`plugin_db_adapter.go:23-25`) holding the
singleton `*MahresourcesContext`, whose `principal` is nil. The two chokepoints
`getDbProvider()` and `getDbWriter()` (`plugin_system/db_api.go:154`, `:170`) return
that instance to **19 call sites, all of them inside `db_api.go`** — 10 querier, 9
writer — and nowhere else in the package. (The report said 27; the measured number
is 19, because several registrations share one lookup.)

Three consequences, all confirmed:

- Under `-auth`, every entity a plugin creates gets a NULL `CreatedByUserId`. The
  stamp callback reads the acting user off the db context, and this db context has
  none.
- `mah.start_job` builds its `ActionJob` with no owner (`manager.go:764-777`), and
  the jobs panel hides ownerless jobs from every non-admin — so a user who triggers
  plugin work cannot see it running. Note that the *async action* path already
  solved this: `ActionJob.ownerUserID` exists and `RunActionAsyncForOwner`
  (`action_jobs.go:40`, `:108`) sets it. `start_job` simply never got the same
  treatment.
- Hooks fire with no actor at all, so a hook that writes through `mah.db` on behalf
  of a user's action produces orphan rows.

### 1.2 Shape of the change

**Do not add a `context.Context` parameter to the interface methods.** `EntityQuerier`
has 21 methods and `EntityWriter` has 41 (measured; distinct from the report's 63,
which counts Lua-side registrations). Threading a context through 62 signatures is
churn with no payoff, because the adapter is a one-field struct wrapping the
context and `WithPrincipal` rebinds that whole surface at once.

Add **one** binder instead:

```go
// plugin_system

// Invocation identifies who is calling and which VMs are executing on this call
// chain. states is a set, not a single VM: see §2.1 for the two-plugin cycle that
// a single-VM field does not close. Both fields beyond the actor are unexported —
// application_context receives an *Invocation from the binder and hands it back
// unread, so it never needs to import gopher-lua.
type Invocation struct {
    ActorUserID uint
    states      []*lua.LState
}

type PrincipalBinder interface {
    BindInvocation(inv *Invocation) (EntityQuerier, EntityWriter)
}

func (pm *PluginManager) SetPrincipalBinder(b PrincipalBinder)
```

```go
// application_context
func (a *pluginDBAdapter) BindInvocation(inv *plugin_system.Invocation) (plugin_system.EntityQuerier, plugin_system.EntityWriter) {
    bound := &pluginDBAdapter{ctx: a.ctx, inv: inv}
    if id := inv.ActorUserID; id != 0 {
        bound.ctx = a.ctx.WithPrincipal(&auth.Principal{UserID: id})
    }
    return bound, bound
}
```

**Actor 0 must stay the unbound context, explicitly.** A context-less path (auth
off, or a worker with no submitter) yields no actor, and the existing no-auth
default-actor stamping already produces the right answer — root — from the *unbound*
singleton. `applyPrincipalScope` happens to early-return when the actor is 0, so
binding a zero principal would be harmless today, but it would leave a non-nil
`cp.principal` with no role on the context, which is a new state nothing else in the
tree produces. Skip the bind instead of relying on that early return.

`getDbProvider()` / `getDbWriter()` become `pm.querierFor(L)` / `pm.writerFor(L)`,
which build the `Invocation` from `L` plus the actor on `L.Context()` and call the
binder. The 19 call sites change mechanically; nothing else in `db_api.go` moves.

**Why the role-less principal is safe.** `applyPrincipalScope` (`scoping.go:149-176`)
computes `mustScope := p != nil && !p.IsAdmin() && (p.IsScoped() || p.RequiresScope())`.
A `&auth.Principal{UserID: id}` has no role, so `IsScoped()` and `RequiresScope()`
are both false and `mustScope` is false: the only effect is attaching the actor to
the db context for the stamp callback. No subtree flattening, no `collectSubtreeGroupIDs`
query, no scope filter. This is exactly the shape `WithActorUserID` (`context.go:782-787`)
already uses for background download workers.

**A deliberate non-effect.** `applyPrincipalScope` parents its context on
`context.Background()`, *not* on the request — the comment there records that this
preserves historical detached-write behaviour so admin writes are not killed by
request cancellation. Binding the actor therefore does **not** make plugin writes
cancellable, and must not be "fixed" to. Item 07's cancellation applies to the Lua
call, not to a write already in flight.

### 1.3 Where the actor comes from

The actor always rides on `L.Context()`. Item 07 already installed a context at
every request-serving entry point via `vmParentContext` (`manager.go:1193`), so for
those the request's principal is *already reachable* and no signature changes at all.
There are 13 `LockVM` sites (`manager.go:1220`); this is all of them:

| Entry point | Site | Actor source | Work |
|---|---|---|---|
| Shortcode render (inline, block) | `shortcodes.go:211,375` | `reqCtx` | none — already carried |
| Injection | `injections.go:33` | `reqCtx` | none |
| Plugin page | `pages.go:49` | `reqCtx` | none |
| Plugin JSON endpoint | `api_endpoints.go:65` | `reqCtx` | none |
| Block render | `block_render.go:65` | `reqCtx` | none |
| Display render | `display_render.go:57` | `reqCtx` | none |
| Sync action | `action_executor.go:470` | `ctx` | none |
| Async action job | `action_jobs.go:245` | `ActionJob.ownerUserID` — already exists | put it on the timeout context (`:252`) instead of bare `Background` |
| `start_job` callback | `action_jobs.go:314` | inherit from the submitting call | as above, plus §1.4 |
| Before hook | `hooks.go:240` | new parameter | §1.5 |
| After hook | `hooks.go:297` | new parameter | §1.5 |
| Async HTTP callback | `http_api.go:436` | capture at `mah.http.get`/`post` registration | stash the actor with the callback |

For the four `Background`-parented sites the pattern is the one `vmParentContext`
already documents: build the context, put the actor value on it, hang the timeout
off it. No new mechanism.

### 1.4 `mah.start_job` ownership

`mah.start_job` is called from Lua while some entry point holds the VM lock, so the
actor is on `L.Context()` at the moment the `ActionJob` is constructed
(`manager.go:764-777`). Set `ownerUserID` from it. This is a three-line change that
closes the "a user cannot see the plugin work they triggered" half of item 01, and it
needs nothing but §1.3.

### 1.5 Hook signatures

`RunBeforeHooks(event string, data map[string]any)` and `RunAfterHooks(event, data)`
(`hooks.go:229`, `:286`) take no context. Both are called from exactly one place each
— the `MahresourcesContext` wrappers at `context.go:656` and `:666`. Give both
dispatchers an `*Invocation` parameter, built by the wrappers from two fields of
their own receiver: `ctx.principal.UserID` for the actor and `ctx.pluginInvocation`
(§2.3) for the accumulated VM set, nil on an ordinary request.

This is uniform across both origins, which is why it is cheap. An ordinary request's
context carries the requesting principal and no invocation; a plugin-originated
write runs on the clone `BindInvocation` produced, whose `principal` *is* the actor
and whose `pluginInvocation` is the chain so far. The wrappers do not need to know
which case they are in.

There are 33 call sites of the wrappers across the CRUD files; none of them changes.

### 1.6 The one new package edge

`plugin_system` does not currently import `auth`. Reading the actor off `L.Context()`
requires `auth.PrincipalFromContext` (`auth/context.go:29`). The edge is legal —
`auth` imports only `models`, so this is downward — and `internal/arch/layering_test.go`
has no rule against it.

**Constrain it deliberately.** Put the extraction in one new file,
`plugin_system/actor.go`, exposing a single `actorFromLState(L *lua.LState) uint`
that reads the principal and returns *only* `p.UserID`. The `*auth.Principal` must
never travel further into the package: the moment it does, the deny-lift starts
creeping into this package by accident. Add an arch rule pinning `mahresources/auth`
to that one file, in the shape `internal/arch/plugin_render_gate_test.go` already
uses for the `auth.PluginCodeAllowed` gate.

### 1.7 Acceptance

- Under `-auth`, an action run by a non-admin creates an entity whose
  `CreatedByUserId` is that user. Assert against a re-read row, not against the
  `*uint` handed in — the stamp callback writes *through* the pointer.
- A hook fired by user X's create, writing a note through `mah.db`, produces a note
  created by X.
- `mah.start_job` invoked by a non-admin produces a job that non-admin sees at
  `/jobs` and that a *different* non-admin does not.
- With auth off, everything above attributes to root (the existing
  `RootAdminPrincipal` default), i.e. no behaviour change.
- A test that pins the 19 chokepoint call sites to the bound adapter, so a new
  `mah.db` function added later cannot quietly reach for the unbound one.

---

## 2. The hook re-entry deadlock (sharp edge #2)

### 2.1 Mechanism

`mah.db.create_note` runs with the VM lock held. It calls into
`application_context`, which calls `RunAfterPluginHooks` → `RunAfterHooks`, which
calls `pm.LockVM(L)` for each registered hook (`hooks.go:297`). If the plugin that
is *currently executing* also holds a hook for that event, `LockVM` blocks on a
non-reentrant `sync.Mutex` already held by this same goroutine. It never returns.

The 5s timeout cannot preempt it: the block is inside a Go call, below the only
place gopher-lua polls the context. The lock is never released, so every later entry
into that plugin — including page-wide injections — blocks behind it permanently,
one leaked goroutine per attempt, with no panic and nothing in the log.

If no plugin hooks the event the dispatcher early-exits (`hooks.go:234`, `:291`),
so the common case is unaffected.

**The report's characterisation is too narrow, and this matters for the fix.** It
states that the trigger "requires the *writing plugin itself* to hold the hook",
because a different plugin "has its own state and mutex and the write completes".
That is only true at depth 1 — it holds when the other plugin's hook does not itself
write. Hooks are dispatched **synchronously on the caller's goroutine** (the loop at
`hooks.go:240`/`:297` spawns nothing), so two plugins wedge each other just as
permanently:

1. Plugin P's action holds `L_P`'s lock and writes entity A. P does not hook A.
2. Q hooks A, so Q's hook runs on the same goroutine and takes `L_Q`'s lock.
3. Q's hook writes entity B.
4. P hooks B → `LockVM(L_P)` → held by this same goroutine's outer frame → wedged,
   with the identical leaked-goroutine, nothing-in-the-log signature.

This is reachable on `master` today. The empirical repro behind the report tested
depth 1 only, which is why it read as narrower than it is. **The fix must therefore
key on the whole call chain, not on one state** — a design that only compares
against the immediately-executing VM closes case 1 and leaves this one open, and no
test written against the report's framing would notice.

### 2.2 The decision: skip any hook whose VM is already on this call chain

Three candidates were considered.

- **Reentrant execution** (allow the nested call on the same LState). Gopher-lua
  supports calling back into a state from a Go function, so this is not obviously
  unsafe, but it needs a recursion depth cap and it needs `L.SetContext` /
  `RemoveContext` to become save-and-restore rather than remove — the nested call
  currently clobbers the outer call's deadline. It is a real semantic commitment.
- **Refuse the write.** Turns a hang into an error, but breaks a write the plugin
  legitimately asked for, and the failure is confusing (the plugin did nothing wrong).
- **Skip the hook.** ← chosen.

**Skip any hook whose VM is already held on the current call chain, log the skip,
and let every other plugin's hook fire normally.** Justification:
it is the only option that neither hangs nor changes what the write does. It
generalises a rule the dispatcher already implements — "no hook registered for this
plugin, move on" — to "this plugin is already inside a call on this chain, move on".
And it composes with a later reentrancy or pool decision without a second migration,
because nothing comes to depend on the skip except the absence of a hang.

The semantic to write down in the plugin docs, in one sentence: **a plugin is not
notified of a write made while it is already running**, whether it made that write
itself or a hook of its own triggered it.

### 2.3 Mechanism, reusing §1

No goroutine-id tracking and no new plumbing. The `*Invocation` from §1.2 already
travels the exact path the fix needs — it just has to carry a **set** of states
rather than one:

- `BindInvocation` stores the invocation on the cloned `*MahresourcesContext` as an
  opaque `*plugin_system.Invocation` field.
- `WithTransaction` (`context.go:873-881`) does `txCtx := *ctx`, so the shallow copy
  carries it into a transaction for free.
- `RunBeforePluginHooks` / `RunAfterPluginHooks` pass `ctx.pluginInvocation` down
  (§1.5).
- `RunBeforeHooks` / `RunAfterHooks` skip any `hookEntry` whose `state` is **in**
  `inv.states`.
- For a hook that does run, the dispatcher installs `inv.states ∪ {L_hook}` on the
  context it sets on `L_hook`. So when that hook calls `mah.db`, `querierFor(L_hook)`
  reads the accumulated set off `L_hook.Context()` and the next level down sees the
  whole chain.

That last bullet is the one that closes §2.1's two-plugin cycle, and it costs one
union per hook dispatch. The set is small by construction — bounded by the number of
enabled plugins — so a slice with linear scan is the right representation.

The actor rides the same accumulation: a hook fired by user X's write runs as X, and
so does anything that hook triggers in turn. One threading, three payoffs.

The comparison happens inside `plugin_system`, on unexported fields, so
`application_context` stores and forwards an opaque pointer and never imports
gopher-lua.

### 2.4 What this deliberately does not do

It does not make after-hooks asynchronous. That is item B's "make entity after-hooks
deliverable asynchronously", and it would be the wrong place to introduce it: async
delivery changes ordering guarantees for every plugin, not just the reentrant one.

Note also that §3's fix — deferring after-hooks past commit — does **not** fix this
deadlock. The commit happens while Lua still holds the VM lock, so a deferred
dispatch still re-locks the same mutex on the same goroutine. The two changes are
independent and both are needed.

### 2.5 Acceptance

The report's empirical repro is the starting point, extended to the depth-2 case it
missed. Every one of these must be written with a timeout that **fails** rather than
hangs, or a regression re-wedges the suite instead of reporting.

- **Self.** P hooks `after_note_create` and its action writes a note → completes,
  hook skipped, one warning logged. (Today: hangs.)
- **Mutual.** P hooks `after_note_create`, Q hooks `after_resource_create`; Q's hook
  writes a note → completes, P's hook skipped, warning logged. (Today: hangs. This is
  the case §2.1 adds, and the one a single-state design would still fail.)
- **Cross-plugin, no cycle.** P hooks `after_note_create`, Q's action writes a note
  → P's hook fires. (Must not regress — this is the case the skip must not swallow.)
- **Depth-2, no cycle.** P's action writes A, Q hooks A and writes B, R hooks B → R's
  hook fires. Proves the set skips only what is genuinely on the chain.
- **No hook registered** → unchanged early exit.

Plus a guard that the skip is keyed on the accumulated state set and not on a global
"we are inside a plugin" flag, which would silently disable cross-plugin hooks
wholesale while passing the first two tests.

---

## 3. Hook coverage on bulk paths (sharp edge #3)

### 3.1 Three distinct defects, measured

**Resources fire nothing.** `BulkDeleteResources` (`resource_bulk_context.go:542`)
and `MergeResources` (`:756`) both route through `deleteResourceDBOnly` (`:462`),
which contains no hook calls. Single-item `DeleteResource` (`:23`) brackets its work
with `before_resource_delete` (`:24`) and `after_resource_delete` (`:143`). So a
plugin that mirrors resources to an external system, or vetoes deletion of protected
ones, works for one resource and is silently bypassed for fifty. The gap is invisible
until it costs data.

**Notes and tags fire inside the transaction.** `BulkDeleteNotes`
(`note_bulk_context.go:161`) and `BulkDeleteTags` (`tag_bulk_context.go:147`) call
the single-item `DeleteNote` / `DeleteTag` inside `WithTransaction`. Those fire
`after_note_delete` (`note_context.go:308`) and `after_tag_delete`
(`tags_context.go:213`) while the transaction is still open, so a plugin is told an
entity was deleted before a commit that may roll back.

**Groups are correct, and are the template.** `BulkDeleteGroups`
(`group_bulk_context.go:404-428`) splits the work: `prepareGroupDelete` runs the
before-hook inside the transaction, `deleteGroupInTransaction` returns a
`groupDeleteEffect` value, and `emitGroupDeleteEffects` runs the after-hooks, the log
line and the cache invalidation **after** the transaction commits
(`group_crud_context.go:464-497`).

### 3.2 The change

Generalise the group shape to resources, notes and tags. For each: a `prepare*`
that runs the before-hook, a DB-only body that returns an effect value, and an
`emit*Effects` after commit. Resources already have the hard half of this —
`deleteResourceDBOnly` already returns a `FileCleanupAction` that `BulkDeleteResources`
applies after commit — so the effect-collection pattern is present and only the hook
payload has to join it.

Both resource callers need the before-hook too, and `MergeResources` needs a decision
recorded: a vetoed loser must fail the whole merge, because a merge that silently
keeps one loser alive leaves the winner holding half its associations.

### 3.3 Ordering semantics to write down

The plugin docs currently do not state when a hook runs relative to the transaction.
After this package one half of that becomes uniform and should be documented:
**an after-hook runs after the commit that made the change durable.** That is already
true of the group path and becomes true of everything.

The before-hook half is deliberately *not* claimed as uniform, because it is not.
Single-item `DeleteResource` fires `before_resource_delete` at
`resource_bulk_context.go:24`, before it opens its transaction at `:84`; the group
bulk path fires `prepareGroupDelete` from *inside* `WithTransaction`
(`group_bulk_context.go:414`). Both are defensible — a bulk veto has to be inside the
transaction to roll the batch back — so document the guarantee that actually holds
(**a before-hook can veto, and a veto means the change does not happen**) and leave
transaction membership unstated rather than stating it wrongly.

There is a reason not to force uniformity here beyond the doc wording: a before-hook
that *writes* through `mah.db` while the caller's transaction is open contends with
that transaction for the SQLite writer lock. That is a `busy_timeout` stall rather
than a deadlock, so it does not belong to §2, but moving more before-hooks inside
transactions would widen the window for it. Revisit with `mah.db.transaction(fn)` in
package 3, where the plugin can join the caller's transaction instead of fighting it.

### 3.4 Acceptance

- Bulk-deleting three resources fires three `before_resource_delete` and three
  `after_resource_delete`.
- `mah.abort` from `before_resource_delete` refuses a bulk delete and leaves all
  three resources present.
- A merge fires delete hooks for each loser, and a vetoed loser rolls the merge back
  whole.
- A bulk note delete whose transaction rolls back fires **no** `after_note_delete`.
  This is the test the current code fails.
- The group path keeps passing unchanged — it is the reference.

---

## 4. Sequencing

The three parts are ordered by dependency, and §2 genuinely depends on §1 (it reuses
the `Invocation`), so they cannot be reordered.

1. `Invocation` + `PrincipalBinder` + the two chokepoints; request paths only.
2. The four `Background`-parented entry points (async action, `start_job`, hooks,
   HTTP callback), plus `start_job` ownership.
3. The arch rule confining the `auth` import.
4. §2: the reentrancy skip, on top of the threading from 1–2.
5. §3a: resource bulk-delete and merge hooks — the half that costs data.
6. §3b: notes and tags after-hooks past commit.

**Split point if it balloons:** step 6. It is a cross-entity refactor of code that
currently works (it fires the hook, just too early), whereas step 5 is a missing call
that loses data. Ship 1–5 and let 6 follow; do not ship 6 without 5.

Sizing: item 01 alone was billed Effort M. With §2 and §3 this is M+. The report's
own retrospective is the warning worth heeding — every estimate that slipped was
"right about the change and wrong about the surface it touched", and all three
misses were about which caller actually mattered.

---

## 5. Risks

**Attribution becomes visible.** Rows that were NULL become attributed, so
creator-filtered views and the audit surface start showing plugin output. This is the
point of the change, but it is a visible behaviour change on any deployment already
running `-auth` with plugins.

**Hook writes inherit the triggering user.** A plugin whose hook writes a companion
record now attributes that record to whoever triggered the hook, not to nobody. For a
multi-user deployment this spreads plugin output across users' creations, which is
correct and may still surprise.

**Job visibility widens by exactly one principal.** `jobVisibleToPrincipal`
(`server/api_handlers/download_queue_handlers.go:47-52`) is `owner != nil && *owner ==
p.UserID`, so an ownerless job is visible only to admins today. Naming an owner makes
it visible to its submitter as well; every other non-admin still cannot see it, and the
admin view is unchanged. This is the only direction in the package in which anything
becomes visible to anyone, and it is the correct one. Worth an e2e check rather than a
unit test.

**The skip is a silent behaviour change** from "hangs forever" to "hook does not
fire". Nothing can depend on the hang, so the risk is only that an author expected the
hook and now needs the log line to find out. Log it at warning level with the plugin
and event named, so it lands in `/logs`.

**Per-call binding cost.** `WithPrincipal` on a role-less principal is a struct copy
plus `db.WithContext` — negligible per `mah.db` call. This stops being true in package
3, where binding a *scoped* principal runs `collectSubtreeGroupIDs` on every bind.
That needs a cache, and it is a package-3 problem, not one to pre-solve here.

---

## 6. Gates

Per `CLAUDE.md`, and read from a redirected log rather than through a pipe:

- `go build ./...`, `go vet ./...`
- `go test --tags 'json1 fts5' ./...`
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
- `cd e2e && npm run test:with-server:all` (browser + CLI)
- Browser E2E is **not** optional for this package despite it being backend-only: the
  jobs-panel visibility change is user-visible.
- Rebuild `./mahresources` before the e2e run — the suite reuses the prebuilt binary.
- `pi` review each of the six steps as it lands, against a pinned worktree.

---

## 7. Package 2 — item G: manifest, `api_version`, permission grants

Second because it closes a live security hole and because it provides the grant
mechanism that lets package 3's deny-lift land per plugin rather than globally.

**What is there now.** Discovery reads exactly three keys from the `plugin` global —
name, version, description (`manager.go:234-244`, `:297-307`). `version` is stored,
displayed and compared to nothing. There is no api_version, no min-app-version, no
dependency declaration and no permission model.

**The egress hole (sharp edge #4), measured.** The only pre-flight check is
`validateScheme` (`http_api.go:279-285`), a case-insensitive `http://` / `https://`
string prefix test. The URL is never parsed for a host. `newHttpClient`
(`http_api.go:34-43`) leaves `Transport` nil, so the default transport dials anything,
and `CheckRedirect` counts hops without looking at the target. A plugin reaches
loopback, link-local (including cloud metadata) and RFC1918 directly, and a request to
a public host can be redirected to an internal one for up to ten hops with no re-check.
Under the default no-auth deployment every request is an implicit administrator, so
the admin gate on plugin paths is effectively absent.

**Contents:**

1. A manifest with `api_version`, `min_app_version`, dependencies and a capability
   grant list.
2. Grants enforced at load by installing only the granted subset of the `mah` table
   into the LState. This is a natural fit: the table is already built key by key in
   `registerXModule` calls, so an ungranted module is simply absent rather than
   guarded at 62 call sites.
3. A `DialContext` control on the shared client rejecting private, loopback and
   link-local addresses, **plus re-validation inside `CheckRedirect`** — the redirect
   half is the one that is easy to forget and is where the current code is weakest.
4. The grant surfaced in the management UI at enable time.
5. **The server-side `action.Filters` re-check.** `actionMatchesFilters`
   (`actions.go:532`) already exists and is applied when deciding which actions to
   *offer* (`actions.go:503`); nothing re-checks it against the submitted
   `entity_ids` at execute time, so an action restricted to `content_types =
   {"image/png"}` runs happily on a PDF via a direct POST. It needs a per-entity load
   and a policy for a partially-valid selection, which is why it is a change rather
   than a patch — and why it belongs with the authorization package.

**Out of v1:** distribution (signed tarball, `mr plugin install`, an index), and
install-then-rescan-without-restart. The discovered-plugin list is immutable by
design, which is what lets two readers run lock-free; making it mutable is a named
refactor.

**The migration loophole:** an absent manifest has to mean "legacy, full access, with
a loud warning", or every existing plugin breaks. That loophole needs a closing
deadline written down when it is opened, not after.

---

## 8. Package 3 — the remainder of item A

1. **The LState pool.** Replace the single state with a checkout pool sized to the job
   concurrency budget, removing the coarse per-plugin serialization where a 120s sync
   HTTP call blocks every other surface of that plugin. The sharp edge: an
   `LFunction` is bound to the state that created it, so N pooled states each run
   `plugin.lua` and `init()` independently. Plugin-global Lua tables used as caches
   stop being process singletons — a real semantic break, with `mah.kv` as the
   migration path — and registration side effects fire N times and must be deduped to
   the first state.
2. **Fold in the failed-`init()` defect.** A plugin that registers a display or block
   type and then errors during `init()` has its VM closed while its registrations and
   its `vmLocks` entry survive, so the next render of that type panics inside
   gopher-lua. It sits in the same lifecycle code as the pool and the still-open
   enable/disable ABA question; fixing it separately risks regressing the VM races
   just closed.
3. **Lift the deny.** Replace the blanket `auth.PluginCodeAllowed` refusal with
   ordinary capability checks, behind package 2's per-plugin grant. Note the scoped
   binding cost from §5: this is where `collectSubtreeGroupIDs` starts running per
   `mah.db` call and needs a cache.
4. **`mah.db.transaction(fn)`**, expressible once the adapter is per-call.
5. Revisit §2's skip. With a pool, a reentrant call could check out a *different*
   state — which removes the hang but changes hook identity, since the hook would no
   longer see its own plugin's globals. Decide it there, with the pool's semantics in
   hand, not here.

After that the report's ordering resumes: **B** and **D** in parallel, **E** and **F**
planned as a pair, **C** and **H** when the appetite is there.

---

## What shipped differently

Three divergences, all found while building rather than while planning.

**A fourth path fired its after-hook inside a transaction.** §3.1 enumerated the
bulk-delete paths and missed `MergeTags` (`tag_bulk_context.go`), which deletes
its losers through the single-item `DeleteTag` from inside its own transaction —
the same defect as notes and tags bulk delete. Left alone it would have made the
rule §3.3 documents false the day it was written, so it got the same
prepare/delete/emit split. Found by grepping for remaining callers of the
single-item deletes, not by the plan.

**The tests could not observe before-hooks by counting rows.** The first draft
had every hook record itself by creating a tag. After-hooks worked; before-hooks
reported zero fires. They *were* firing — a before-hook runs inside the caller's
open transaction, plugin `mah.db` writes are issued on a separate connection, and
on SQLite they lose the writer lock and silently vanish. That is the contention
§3.3 predicts, met in practice. The tests now use a Lua-side counter for "did it
fire" and keep the tag write as the assertion for "did it run outside a
transaction" — which turns out to be the sharper of the two, since it is what
catches an after-hook that has been moved back inside the transaction.

**`hookEntry` gained a plugin name.** The re-entry skip logs which plugin it
skipped and for which event, and the entry carried only a state and a function.
`mah.on` is reachable only from `init()`, which runs after the name is populated,
so capturing it at registration is safe.


## Review rounds (pi, `openai-codex/gpt-5.6-sol:high`)

Nine rounds on a pinned worktree, re-snapshotted after each round's fixes,
stopping at two consecutive clean rounds. 28 confirmed findings in total; the
full record is in `docs/todo.md`. A round that comes back clean means that
angle is exhausted, not that the change is: round 6 was clean and round 7, asked
about a different half of the diff, found twelve. The pattern worth keeping: every round found
something real, and twice the defect was inside the previous round's fix — the
bounded lock wait added in round 1 made before-hook vetoes fail open, and the
registration check added in round 4 covered only the timeout half of the window
it was written for. A single pass would have shipped both.

Round 1's five findings, all confirmed against the code and all addressed. Two
were real defects:

**The async HTTP callback lost its actor.** It read the actor with
`actorFromContext(L.Context())`, which sees only an `auth.Principal` — but hooks,
async jobs and drained callbacks carry their actor on an *Invocation*, not a
principal. So `mah.http.get` called from inside a hook queued a callback with
actor 0, and everything that callback wrote was un-attributed. The very case §1
exists to fix, missed on the one path that reads the actor twice. Now
`pm.actorFor(L)`, which goes through `invocationFor`. Mutation-tested.

**A lock cycle across goroutines is still possible, and was still permanent.**
The invocation chain is per-call-stack, so it cannot see goroutine A holding
plugin P and waiting for Q while B holds Q and waits for P. Both waits were
unbounded — on `master` too; this is pre-existing rather than introduced. Fixed
by bounding the wait *only* on the nested path (`TryLockVMWithin`, `hookLockWait`):
a dispatch holding no VM lock cannot be part of such a cycle, so it still waits
as long as it takes and the common case is unchanged. A nested dispatch that
cannot get the lock in 5s skips the hook, which is the same policy the re-entry
guard already applies.

Bounding the nested wait then opened a gap of its own, caught on review of the
fix rather than by pi: **it made a before-hook veto fail open.** A plugin's VM
being busy is ordinary — a 120s sync HTTP call holds it — so a nested dispatch
that timed out would skip the hook, and if that hook was the veto, the write it
was protecting against proceeded. Contention silently disabling a guard is worse
than the deadlock it was fixing. The two dispatchers are now deliberately
asymmetric: an after-hook timeout is a missed notification of something already
committed and is skipped and logged, while a before-hook timeout fails the
operation with `ErrHookVMBusy`. Both directions are pinned by tests, because
collapsing the two onto one code path is the obvious future simplification and
it reintroduces the bypass silently.

The other three were smaller: the skip warning went to `log.Printf` rather than
the `PluginLogger`, so it never reached `/logs` as this plan promised (now
routed, with a process-log fallback); the auth-disabled system principal was
being recorded as a real actor, which disagreed with `principalOwnerID` in the
HTTP layer and with the job-ownership documentation (`actorFromContext` now
yields 0 for a super-user, so plugin jobs are ownerless under auth-off exactly
like async actions, and attribution still resolves root through the stamp
callback's own default); and the no-principal test asserted only "not the other
test's actor", which almost anything would satisfy — it now compares against
what a direct non-plugin create produces in the same context.


## Appendix: measured inventory

| Thing | Count | Source |
|---|---|---|
| `EntityQuerier` methods | 21 | `db_api.go:19-50` |
| `EntityWriter` methods | 41 | `db_api.go:56-98` |
| Chokepoint call sites | 19 (10 querier, 9 writer) | `db_api.go`, and nowhere else in the package |
| `LockVM` sites | 13 | across 9 files |
| Sites already carrying a request context | 8 of 13 | `vmParentContext` callers |
| Hook wrapper call sites in CRUD code | 33 | `RunBefore/AfterPluginHooks` |
| Bulk-delete paths | 4 | resources, notes, tags, groups |
| — of those firing hooks correctly | 1 | groups |
| — firing them inside the transaction | 2 | notes, tags |
| — firing none at all | 1 | resources (plus merge) |
