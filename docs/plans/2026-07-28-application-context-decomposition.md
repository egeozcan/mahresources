# Decomposing `application_context`

Status: proposed, not started. Planning only — no code changes yet.

Every number below was measured against the tree at `849a6e2a`, not taken from
the brief.

**Revision 2** — an adversarial review found four defects in revision 1, all
confirmed against the code and all corrected: (1) the alias list was missing
five types that `cmd/mr` constructs directly, so the build would have broken;
(2) the seam inventory counted only `ctx.*` calls and missed five package-level
helpers in `associations.go`; (3) the transaction test asserted a nesting
scenario GORM rejects outright and production never executes; (4) the new
security tests were placed in a package that cannot import what they need.

**Revision 3** — a second review found three more, again all confirmed:
(5) the §3.1 fix to (2) was itself wrong — three of those five helpers are also
used by `group_crud_context.go`, so *moving* them breaks the package; `groupio`
must copy them instead; (6) the test-move step named the wrong helper —
the moved tests call `createTestContext` (50 sites, `resource_context_test.go`),
never the `setupTestContext` the plan proposed to port; (7) **the scoped-import
test described a security contract that does not exist** — all five import
routes are wrapped in `denyScopedPrincipal`, so scoped import is denied
outright rather than confined.

**Revision 4** — a third review found two more, both confirmed:
(8) the white-box inventory was still wrong — the moved tests also use
`RegisterAltFs`, `Config`, `DefaultResourceCategoryID`, `altFileSystems`,
`NewMahresourcesContext`, and 13 `*MahresourcesContext`-typed helpers, and a
**ninth** test file (`guid_test.go`, calling the moved `ensureGUID`) was missed
because it does not match the `*import*`/`*export*` filename pattern;
(9) **scoped export is untested at the output level** — the existing test
compares status codes from the *estimate* endpoint only, so the §2 failure mode
could leak cross-subtree data into an archive with every test still green.

**Revision 5** — finding (9) was then verified empirically rather than assumed,
and it turned up a **real pre-existing bug on `master`**: the typed
`GroupRelation` BFS is not subtree-confined, because `group_relations` is not in
`scopeColumn()`. Full reproduction, measurements, and impact analysis in §5a.
It is not caused by this refactor and must not be bundled into it, but the
slice's acceptance test has to be written against the fixed behaviour. Note the
first probe of this passed and proved nothing — the BFS is gated on
`Scope.RelatedM2M`/`GroupRelations`, which the probe left false. **A control
that proves the code under test actually ran is not optional.**

**Revision 6** — a fourth review found three more, all confirmed:
(10) the (2b) acceptance test specified only `relatedDepth > 0`, which leaves
both BFS branches gated off — **the same vacuous-test trap revision 5 had just
documented and failed to apply to its own test spec**; the request body and a
positive-traversal control are now spelled out; (11) §2a said the §5a fix could
land "before or after" and then that it must be established first — since (2b)
cannot pass on today's `HEAD`, "after" is not a real option, so the fix is now
a hard prerequisite with an explicit four-step ordering; (12) §3.5's expected
outcome put the new tests in `groupio`, contradicting §3.4 and reintroducing
defect (4), and the count was stale at 4 — now a per-package table of 5.

**Revision 7** — the §5a bug is **fixed** on branch
`fix/scoped-export-group-relation-confinement` (TDD: red first, green after).
A fifth review then found two defects in the fix itself, both confirmed and
both corrected: (13) filtering the BFS target was not enough — the still-readable
`group_relations` row produced a relationship payload with neither `ToRef` nor
`DanglingRef`, invalid per the archive contract and silently dropped on import
(proved by parsing the tar, not by grepping bytes); (14) the first
`visibleGroupIDs` implementation issued a `WHERE id IN (?)` query that the scope
callback would extend with the full allow-list, risking a parameter-limit abort
on large subtrees — it now intersects in memory and issues no query at all.
A sixth review then found two more, both confirmed: (15) `visibleGroupIDs` was
calling `subtreeScopeIDs()`, re-running the recursive CTE once per BFS level
against a caller-controlled uncapped `RelatedDepth` — an amplification path
reachable from the estimate endpoint, and a *different* snapshot from the one
the callbacks use; it now reuses the materialized allow-list and issues no
query; (16) the positive traversal assertion was vacuous, because `Subtree:
true` made Phase A pre-seed the target before BFS ran — proved by disabling
traversal entirely and watching the assertion still pass, now fixed and
confirmed by mutation.

**Revision 8** — §2a step 2 done: all baselines re-measured at `dabbfbc6` and
recorded in §3.4 alongside the `849a6e2a` figures. Two corrections fell out.
(17) The old "64 tests across 8 files" **double-counted two files** through
overlapping globs; the true pre-fix figure is 62. (18) `export_scope_test.go`,
added by the fix itself, **straddles the seam** — two of its five tests use
`buildExportPlan`/`exportPlan` (moving) *and* `WithPrincipal` (staying), so
after the move neither package can host them. They must be rewritten against
the exported API before code moves. That is the `guid_test.go` lesson arriving
from the other direction: a file can reference symbols on both sides of a seam.

**Revision 9 — slice 1 is DONE** (commits `0fa18344`, `db76229d`, `f8270f8f`).
Executing it surfaced four more defects in this plan, all confirmed against the
code, and all four are the same shape the plan already warned about: an
inventory taken from a proxy rather than from a search.

(19) **The `ctx.*` fan-out was still one short.** §3.2 names three external
helpers; there are four. `visibleGroupIDs` (added by the §5a fix, in
`scoping.go`, which stays) is called from `bfsCollectGroupRelations`, which
moves. `ScopeResolver` therefore needs two methods, not one. The plan's own
fan-out table was measured before the §5a fix landed and never re-run against
`dabbfbc6`, which §2a step 2 was supposed to catch.

(20) **`guid_test.go` straddles the seam in a second way.** §3.4 flagged it for
`ensureGUID`. It also calls `CreateGroup`, which does not move. Both tests
overwrite the GUID immediately afterwards and assert only on `ensureGUID`, so
they now insert the row directly. Two revisions found this file; neither read it.

(21) **The scoped-import denial test cannot live in `application_context`.**
§3.4 insists all five new tests belong there because they need `WithPrincipal`
and friends. That reasoning holds for four of them. It does not hold for the
import denial, which is a property of `denyScopedPrincipal` applied at the
router in `server/routes.go`: `application_context` has no router and cannot
observe it. It is in `server/api_tests`, so the per-package expectation in §3.5
is wrong. Corrected counts below.

(22) **Test (4) passes on the baseline.** §3.4 anticipated that `Begin()`
inheriting the scoped context might expose a pre-existing latent bug. It does
not. `Begin()` carries both the scope filter and the acting user, confirmed
behaviourally as well as by inspection. Nothing to fix.

Corrected §3.5 outcome, measured:

| Package | Plan said | Actual |
|---|---|---|
| `application_context` | 465 | **464** (4 new, not 5) |
| `groupio` | 64 | **64** |
| `server/api_tests` | 627 | **628** (the import-denial test) |
| `archive` | 15 | **15** |
| `internal/arch` | 3 | **4** (the `groupio` rule was adopted) |

464 + 64 = 528 = 524 + 4, and `api_tests` gains the fifth new test. No test was
deleted, 64 relocated, 5 added.

The one methodological thing this slice did that the plan did not ask for, and
which is worth carrying into slice 2: **every new and rewritten test was checked
by mutation, not by observing green.** That is what caught the handle-propagation
test being insensitive to the very defect it existed to catch. Its fixture used a
`GroupRelation` edge, and the relation BFS is confined through `Deps.Scope`, which
resolves against `ctx.db` and stays correct even when `Deps.DB` is wrong. Only an
M2M edge, guarded by the GORM callbacks that read `Deps.DB`, actually detects a
captured handle. A test written specifically to catch §2's failure mode did not
catch it, and only mutation revealed that. This is the fourth time in this
document that a test proved less than it appeared to.

**Revision 10 — slice 2 is DONE** (commits `f8a2de5e`, `b3c90fa1`, `cfef99f2`),
and with it the re-measurement §4 makes the decision point. **The measurement
says stop.**

Slice 2 moved `search_context.go` and `search_cache.go` into `search/` behind a
facade. The seam was as advertised: five fields and exactly one external method
(`principalForcedScope`). One correction to §4's characterisation: the risk here
is not primarily the db handle. Search owns two pieces of state that must be
*shared* rather than copied, and duplicating either fails **silently** —
a per-call result cache never hits and never invalidates, so all 31
`InvalidateSearchCacheByType` call sites quietly become no-ops; a per-call FTS
flag reads false and downgrades every search to the LIKE fallback. Results stay
correct in both cases, just stale and slow, so nothing surfaces as an error.
That is a different and nastier failure mode than §2's, and it is why the two
tests guarding it assert through deliberate *controls* (a stale read, an
InitFTS-then-derive check) rather than through their headline assertions.

(23) **A third vacuous test, found the same way as (9).**
`TestScopedUser_SearchAndMRQLConfined` requested `/v1/search?query=sf-`, but the
handler reads `q`. The term never arrived: every response was
`{"query":"","total":0,"results":[]}`, so the confinement assertion held for
free. **Scoped global search had never been tested.** The behaviour is correct
(4 in-subtree results against an admin's 7), so this was a dead test rather than
a bug — but it was the only coverage of the property slice 2 puts at risk. It is
fixed, given the positive control its MRQL sibling already had, and joined by
`TestScopedUser_SearchCacheNotSharedAcrossScopes`, which covers both directions
of the shared-cache leak that nothing covered before.

### The re-measurement, and what it says

| Measure | `dabbfbc6` | after slices 1+2 | delta |
|---|---|---|---|
| `application_context` non-test LOC | 29,612 | **23,285** | −6,327 (−21%) |
| `.go` files | 143 | **131** | −12 |
| `MahresourcesContext` fields | 26 | **25** | **−1** |
| methods | 453 | **418** | −35 |
| **exported methods** | 329 | **329** | **0** |
| `application_context` test wall-clock | 13.1s | 10.6s (+1.1 +0.7) | ~2s |

**Two slices moved 21% of the package and the god object lost one field and zero
exported methods.** That is not a failure of execution; it is the direct
consequence of the facade strategy, which is also the thing that made both
slices safe enough to land with no call-site churn. But it means the headline
complaint the brief opened with — 452 methods, 329 exported — is completely
unimproved, and slices 3 and 4 would improve it by the same amount: nothing.

§1 predicted this ("the payoff from decomposition is lower than the method count
suggests"). The measurement now confirms it rather than merely arguing it.

**Recommendation: do not proceed to slices 3 or 4. Do slice 5.** The DI ratio is
where the real coupling is, and it is measured, not asserted:

| Package | concrete `*MahresourcesContext` | `contracts.*` |
|---|---|---|
| `server/api_handlers/` | 74 | 173 (70%) |
| `server/template_handlers/template_context_providers/` | **60** | **2 (3%)** |

Sixty concrete dependencies on the god struct sit in one directory that nobody
has migrated. Slice 5 changes parameter types at a call boundary, carries no
extraction risk, needs no facade, and is the only remaining work that would
actually shrink what `server/` can reach for.

**Revision 11 — slice 5 is DONE** (commits `56d0662d`, `badc7c77`). All 61
template context providers took `*MahresourcesContext` and could reach any of
its 329 exported methods. **None do now.**

| Measure | before | after |
|---|---|---|
| provider fns taking the concrete type | 61 | **0** |
| concrete references in the package | 60 | **21** (20 compile-time assertions + 1 comment) |

§4 called this "pure `contracts/` work, no extraction" and predicted "near-zero
risk, because it changes only parameter types at the call boundary." The first
half held. The second half was wrong twice, and the second miss was dangerous.

(24) **The route table forbids changing a parameter type.** All 69 providers are
stored in one map whose field is
`func(*MahresourcesContext) func(*http.Request) pongo2.Context`. Go has no
function-parameter contravariance, so narrowing any provider's parameter breaks
the map. This is not a detail — it is the reason the migration stalled at 2 of
61 while `api_handlers/` reached 70%: handlers are wired individually, providers
are wired through a uniform table. **A DI migration is only "just parameter
types" when each call site is independent.**

(25) **The obvious fix is a silent security regression.** The natural way to get
past (24) is to apply `appContext` when the map is built, turning the field into
an already-bound `func(*http.Request) pongo2.Context`. That is wrong:
`routes.go` calls `info.contextFn(sc)` **per request** with a principal-scoped
context, and binding at wiring time hands every page the unscoped singleton.
Every HTML page would leak across subtrees while the entire `/v1` API — and
every existing scoping test — stayed correct. The fix is `adaptTemplate[T]`,
which preserves the per-request call and mirrors the existing `scopedAPI[T]`.

**Template-page confinement was covered only by e2e, in a browser.** It is now
pinned in Go by `TestScopedUser_TemplatePagesConfined`, written *before* the
migration and verified by mutation: binding to the singleton leaks
out-of-subtree groups, notes and resources through `/groups.json`,
`/notes.json` and `/resources.json`, and every other `TestScopedUser_*` test
still passes. That is the third time in this document a security property turned
out to rest on a test that did not test it.

(26) **Interfaces cannot describe struct fields.** Four providers read
`Config`/`DefaultResourceCategoryID` directly. They get four narrow accessors
(`DatabaseType`, `ShareEnabled`, `AltFileSystems`, `IsDefaultResourceCategory`)
rather than an escape hatch back to the whole config.

The interfaces live in the providers package, not `contracts/`, for the reason
`api_handlers.GroupImporter` already documents: `contracts/` may import only
`models/` and `constants/`, and several of these methods return
`application_context`-owned types (`RuntimeSettings`, `PopularTag`,
`ActivityEntry`, `MRQLFilterError`) or `plugin_system.PluginManager`. **Moving
those four DTOs into `contracts/` is the obvious follow-up** and would let most
of these interfaces move with them.

### Where this leaves the effort

| Slice | Verdict |
|---|---|
| 1 `groupio` | done; 5,865 LOC out, no call-site churn |
| 2 `search` | done; 651 LOC out, no call-site churn |
| 3 `media`, 4 `mrql` | **not recommended** — see revision 10 |
| 5 template DI | done; 61 concrete params to 0 |

Slice 5 was the one worth doing, and for the reason revision 10 predicted: it
is the only one of the five that reduced what `server/` can reach for. Slices 1
and 2 moved 21% of the package's LOC and left the god struct's exported surface
at exactly 329 methods; slice 5 moved no LOC and removed 61 unrestricted
references to it.

**Revision 12 — `api_handlers/` is done too** (commit `2ce92ad4`), along with two
hygiene items: the FTS state is now atomic rather than race-free-by-convention
(`7dfb4eb9`), and two empty tracked test databases are gone (`8b3df8da`).

| Package | concrete refs before | after | non-assertion |
|---|---|---|---|
| `server/api_handlers/` | 74 | **16** | **1** |
| `template_context_providers/` | 60 | **21** | **0** |
| `template_filters/` | 20 | **13** | 10 (unmigrated internals) |

The single real one left in `api_handlers` is `principalBinder.WithPrincipal`,
which returns `*MahresourcesContext` by definition.

(27) **"Only parameter types" was wrong a third time, and this one was a live
bug.** Narrowing a parameter from `*MahresourcesContext` to an interface changes
nil semantics: **a nil pointer assigned to an interface produces a NON-NIL
interface holding a nil pointer.** Every `appCtx == nil` guard inside the
narrowed helpers silently stopped firing, and the first method call panicked on
a nil receiver. It reached two call sites; the existing custom-CSS test caught
one, and the other (`buildPageRenderContext`) was found only by auditing the
rest — it guards `appCtx != nil` two lines below the widening call, which is
proof nil is expected there.

The regression test for it was itself vacuous on the first attempt: asserting
the returned context is non-nil passes with a widened typed nil, because only
*invoking* the resolver panics. **That is the fifth vacuous test in this
document, and the second one written specifically to catch a defect it could
not detect.** Both were found by mutation, not by review.

Migrating `api_handlers` also required narrowing three `template_filters` entry
points and the six internal helpers behind them: a handler holding an interface
cannot call a function demanding the concrete type, so the dependency chain has
to narrow together or not at all.

**§2a is now fully satisfied. The decomposition may begin.**

The seam choice itself has survived all four reviews unchallenged. Every defect
has been in execution detail, and **five of the nine were inventory errors of
the same shape**: a convenient proxy (a `ctx.*` grep, a filename glob, a
hand-written shortlist) substituted for an exhaustive search. Lessons, recorded
inline: **a `ctx.*` fan-out count is not a dependency inventory** (§3.1);
**forward references are only half of one — run the reverse search before
moving a symbol** (§3.1); **select files by symbol reference, not by
filename** (§3.4); **verify what production actually permits before writing a
test that asserts a security property** (§3.4); and **a status-code assertion
is not a data-confinement assertion** (§3.4).

## 1. What the measurement actually says

The brief's figures are all correct:

| Claim | Verified |
|---|---|
| 142 `.go` files (69 non-test, 73 test) | yes |
| 29,514 non-test LOC | yes |
| `MahresourcesContext` has 26 fields | yes (`context.go:308`) |
| 452 methods, 329 exported | yes |
| Top-10 file sizes | yes, exactly |
| `contract_checks.go` holds 25 assertions | yes |
| ~89–99 external call sites | 99 files: api_tests 30, template providers 26, api_handlers 19, server 10, template_filters 5, mrqlbench 2, cmd/mr 2, openapi 1, main 1, importExisting 1 |
| import/export = 5,828 LOC | yes (2567 + 2112 + 866 + 283) |

But the conclusion the brief draws from them does not survive contact with the
code. **The struct is wide, not deeply entangled.** Measuring how many of the
26 fields each non-test file ≥200 LOC actually touches:

| Fields touched | Files |
|---|---|
| 0 | 5 |
| 1 | 14 |
| 2 | 9 |
| 3 | 4 |
| 4 | 2 |
| 5 | 4 |
| **17** | **1 — `context.go`** |

Every file except the constructor touches ≤5 fields, and 28 of 39 touch ≤2.
All the wide coupling lives in `context.go`, which is the wiring file and is
*supposed* to see everything. `MahresourcesContext` is a service locator whose
fields are consumed in **disjoint slices**, not a ball of mud where everything
reaches everything.

The 452-method count overstates the problem for the same reason: the methods are
mostly not coupled to one another. It is a large namespace, not a large object.

**Consequence for this plan:** the payoff from decomposition is lower than the
method count suggests, and the risk is concentrated in exactly one mechanism
(section 2). This is not a rewrite candidate. It is a candidate for two or three
surgical lifts of genuinely separable subsystems, after which the remainder
should probably be left alone.

Field count alone is also not a sufficient seam test. `plugin_db_adapter.go` is
1,476 LOC and touches only **one** field (`Config`, twice) — which looks like an
ideal seam until you count its method fan-out: it calls **~60 distinct
`ctx.*` methods**. It is a *consumer* of the god struct, not a component of it,
and extracting it would move the entanglement rather than remove it. A seam
needs **low field coupling AND low method fan-out**.

## 2. Constraint 1: how derived contexts reach extracted code

This is the crux, and the existing design already answers it — the answer is
just undocumented.

### The mechanism

Scope, actor identity, and transaction membership **all ride inside the
`*gorm.DB` handle**, not in the struct fields around it:

- `scoping.go:149` `applyPrincipalScope` ends with
  `dst.db = base.db.WithContext(ctx)`, where `ctx` carries `scopeCtxKey{}`
  (the flattened subtree allow-list) and `actingUserCtxKey{}` (the acting user
  id for `CreatedByUserId`).
- `scoping.go:270` `registerScopeCallbacks` installs GORM `Query`/`Update`/
  `Delete`/`Create` callbacks that read those values off
  `db.Statement.Context`. Registered once on the root handle, so every derived
  session and transaction inherits them.
- `context.go:785` `WithTransaction` swaps `txCtx.db = tx`.

So a bare `*gorm.DB` is a **complete carrier of (transaction, subtree scope,
actor)**. Nothing else on the struct participates.

### The rule this implies

> Extracted services must be **stateless with respect to the database handle**.
> They receive `*gorm.DB` as a **call parameter**, never store it at
> construction.

### The failure mode if done the obvious way

The obvious refactor is to make the extracted service a field:

```go
type MahresourcesContext struct {
    // ...
    importer *groupio.Importer   // constructed once in NewMahresourcesContext
}
```

`groupio.Importer` would capture `db` at construction. Then:

- `WithTransaction` shallow-copies the struct and sets `txCtx.db = tx`. The
  `importer` pointer is copied verbatim and still holds the **pre-transaction**
  handle. Every write the importer performs runs outside the transaction and
  **is not rolled back** when the outer transaction aborts. A failed import
  leaves partial rows behind, silently.
- `WithPrincipal` / `WithRequest` set `cp.db = base.db.WithContext(scoped)`.
  The `importer` still holds the **unscoped** handle, so its GORM callbacks see
  no `scopeFilter`. A group-limited user's import writes escape their subtree
  and their rows stamp `CreatedByUserId = 0`. **Both failures are silent and
  security-relevant.**

Neither would be caught by any existing test, because no current test asserts
that a scoped principal's *import* stays scoped.

### The shape that is already correct

`apply_import.go` is most of the way there already. `applyState` threads the
handle explicitly:

```go
func (s *applyState) applyOneResource(tx *gorm.DB, ...) error
func (s *applyState) mergeResource(tx *gorm.DB, ...) error
func (s *applyState) replaceResource(tx *gorm.DB, ...) error
func (s *applyState) applyOneNote(tx *gorm.DB, ...) error
```

and opens its batch transactions with `s.ctx.db.Begin()` (`apply_import.go:1241,
1773). Only the `s.ctx` back-pointer needs replacing.

### The one thing the db does *not* carry

`principal` is a struct field, not a db context value. Raw-SQL paths that bypass
the GORM callbacks read it directly (`principalForcedScope`, `subtreeScopeIDs`,
`isScopedPrincipal`). Import/export touches this once, via
`collectSubtreeGroupIDs`. So the extracted package needs a **second**
per-call input alongside the handle — a small scope-resolver interface — not a
copy of the principal.

## 3. The first slice: `groupio`

### 3.1 The seam

Move four files out of `application_context/` into a new top-level package
`groupio/` (sibling to the existing `archive/`, which is already standalone —
its non-test imports are stdlib only, no `mahresources/*` at all):

| File | LOC |
|---|---|
| `apply_import.go` | 2,567 |
| `export_context.go` | 2,112 |
| `import_context.go` | 866 |
| `import_plan.go` | 283 |
| **total** | **5,828** |

Plus **five package-level helpers** from `application_context/associations.go:105-143`
which the four files call: `BuildAssociationSlicePtr` (10 call sites),
`TagPtrFromID` (3), `GroupPtrFromID` (3), `NotePtrFromID` (2),
`ResourcePtrFromID` (2). All are model-only (`uint` → `*models.X`).

**Do not move them.** Three of the five —
`BuildAssociationSlicePtr`, `GroupPtrFromID`, `TagPtrFromID` — are also used by
`application_context/group_crud_context.go:191-192`, which is **not** moving.
Relocating them leaves `application_context` uncompilable. `groupio` instead
defines its own unexported copies; they are three-line constructors and a
one-line generic, so duplication is cheaper than a shared leaf package or than
dragging `group_crud_context.go` into the blast radius.

Two inventories are needed here, and revision 1 ran only the first:
*forward* ("what do the moved files reference?") and **reverse** ("who else
references what I am about to move?"). A forward-only inventory produces
exactly this class of build break.

Why this seam and not another — measured, not assumed:

- **Field coupling: 2 of 26.** `db` (105 references) and `fs` (14). Plus
  `altFileSystems` reached transitively through one helper. Nothing else —
  no `Config`, no `settings`, no `locks`, no `principal`, no caches, no queues.
- **Method fan-out: 3.** The only `ctx.*` calls leaving these four files are
  `collectSubtreeGroupIDs` (1 call), `GetFsForStorageLocation` (4),
  `GetSeriesBySlug` (1). All three are ≤20-line bodies depending solely on
  `db` / `fs` / `altFileSystems`. **Method fan-out is not the whole story** —
  see the five package-level helpers above, which a `ctx.*` count misses.
- **Blast radius: 2 files for the methods, 6 for the types.** All 9 external
  *call sites* live in `server/api_handlers/import_api_handlers.go` and
  `export_api_handlers.go`. But the seam's exported *types* are referenced from
  four further files: `cmd/mr/commands/group_import.go`,
  `cmd/mr/commands/group_export.go`, `server/routes_openapi.go`, and
  `server/api_handlers/import_api_handlers_test.go`. Type aliases (§3.3) cover
  all six, but only if the alias list is complete.
- **Already interface-mediated.** Those handlers do *not* take the concrete
  type. They take locally-declared `GroupImporter`, `GroupExporter`,
  `GroupExporterWithManager`. The DI migration is already done here; the
  interfaces are just declared in the wrong package.
- **Destination exists.** `archive/` (4 non-test files, 882 LOC) already owns
  the manifest/reader/writer format and imports nothing from the module.

Contrast with the alternatives, which is why this one goes first:

| Candidate | LOC | Fields | Method fan-out | Verdict |
|---|---|---|---|---|
| import/export | 5,828 | 2 | 3 | **take it** |
| `plugin_db_adapter.go` | 1,476 | 1 | ~60 | consumer, not a seam |
| mrql (`mrql_context` + 2) | ~2,400 | 4 | high (fts, settings, scope) | later, harder |
| `resource_media_context.go` | 2,018 | 5 | medium | later |
| `search_context.go` | 497 | 5 | low | good but small |

### 3.2 What the new type owns

```go
package groupio

// Service is stateless with respect to the database. Every entry point takes
// the caller's *gorm.DB so that transaction membership, subtree scope, and
// actor attribution — all of which ride on the handle — are inherited rather
// than captured. See docs/plans/2026-07-28-application-context-decomposition.md §2.
type Service struct {
    fs  afero.Fs                    // main filesystem, immutable for process life
    alt map[string]afero.Fs         // alternative filesystems, likewise
}

type Deps struct {
    DB    *gorm.DB                  // per-call: carries tx + scope + actor
    Scope ScopeResolver             // per-call: the principal-derived bits the db cannot carry
}

// ScopeResolver is the sliver of principal state that raw SQL needs, since the
// GORM scope callbacks do not fire on it.
type ScopeResolver interface {
    SubtreeGroupIDs(rootID uint) ([]uint, error)
}

func (s *Service) ParseImport(d Deps, cancelCtx context.Context, jobID, tarPath string) (*ImportPlan, error)
func (s *Service) ApplyImport(d Deps, cancelCtx context.Context, parseJobID string, dec *ImportDecisions, sink download_queue.ProgressSink) (*ImportApplyResult, error)
func (s *Service) LoadImportPlan(d Deps, jobID string) (*ImportPlan, error)
func (s *Service) DeleteImportFiles(d Deps, jobID string) error
func (s *Service) EstimateExport(d Deps, req *ExportRequest) (*ExportEstimate, error)
func (s *Service) StreamExport(d Deps, cancelCtx context.Context, req *ExportRequest, dst io.Writer, report ReporterFn) error
```

`fs` and `alt` are safe as fields: they are set once in
`NewMahresourcesContext` and **never rebound by any of the five derivation
methods**. `db` is the only field a shallow copy mutates, which is exactly why
it must not be one.

The five `associations.go` helpers (§3.1) move into `groupio` as unexported
functions, or into a shared leaf package if another slice later needs them.
They depend only on `models/`, so either placement is layering-clean.

The three external helpers come along:

- `GetFsForStorageLocation` → a `Service` method over `s.alt` / `s.fs`.
- `GetSeriesBySlug` → inlined as an unexported `groupio` helper (one query).
- `collectSubtreeGroupIDs` → stays in `application_context`, reached through
  `Deps.Scope`. `MahresourcesContext` satisfies `ScopeResolver` with a
  one-line method.

Dependency direction: `groupio` → {`archive`, `models`, `download_queue`,
`constants`}. It does **not** import `application_context`, `server`, or
`contracts`. `download_queue` does not import `application_context`, so
`ProgressSink` creates no cycle.

### 3.3 Facade strategy — zero changes outside the seam

Two devices keep all 99 external files compiling untouched.

**(a) Type aliases.** The six DTOs move to `groupio`; `application_context`
keeps aliases:

```go
// application_context/groupio_facade.go
// All TWELVE externally-referenced seam types. Verified by searching
// `application_context.<symbol>` across the repo for every exported symbol
// defined in the four moved files. An incomplete list breaks the build in
// cmd/mr — which is exactly what a first draft of this plan did.
type ImportPlan        = groupio.ImportPlan
type ImportDecisions   = groupio.ImportDecisions
type ImportApplyResult = groupio.ImportApplyResult
type ExportRequest     = groupio.ExportRequest
type ExportEstimate    = groupio.ExportEstimate
type ReporterFn        = groupio.ReporterFn
type ProgressEvent     = groupio.ProgressEvent
// Consumed by cmd/mr/commands/group_import.go (and the handler unit tests),
// which construct these directly rather than only receiving them:
type ImportPlanItem    = groupio.ImportPlanItem
type MappingEntry      = groupio.MappingEntry
type MappingAction     = groupio.MappingAction
type DanglingAction    = groupio.DanglingAction
type ShellGroupAction  = groupio.ShellGroupAction
```

Seven further exported seam symbols (`ConflictSummary`, `DanglingRefPlan`,
`DecisionKeyFor`, `ImportMappings`, `ImportPlanCounts`, `MappingAlternative`,
`SeriesMapping`) have **no** external references and need no alias. Re-run the
search before implementing; do not trust this list to have aged well.

Aliases (`=`, not a defined type) are *identical* types, so
`*application_context.ImportPlan` and `*groupio.ImportPlan` are
interchangeable. Every existing reference in the two handler files, in
`api_tests`, and in the local `GroupImporter`/`GroupExporter` interface
declarations keeps working with no edit.

**(b) Delegating methods.** All six exported methods stay on
`MahresourcesContext` with unchanged signatures:

```go
func (ctx *MahresourcesContext) ApplyImport(
    cancelCtx context.Context, parseJobID string,
    decisions *ImportDecisions, sink download_queue.ProgressSink,
) (*ImportApplyResult, error) {
    return ctx.groupio.ApplyImport(ctx.groupioDeps(), cancelCtx, parseJobID, decisions, sink)
}

// groupioDeps rebuilds per call, so it always reflects THIS context's db —
// the transactional and/or subtree-scoped one on a derived copy. Never cache it.
func (ctx *MahresourcesContext) groupioDeps() groupio.Deps {
    return groupio.Deps{DB: ctx.db, Scope: ctx}
}
```

`ctx.groupio` may be a struct field (it holds only `fs`/`alt`, never `db`), and
`groupioDeps()` reads `ctx.db` **at call time** — so a shallow copy that swapped
in a transaction or a scoped handle is picked up correctly. This is the whole
trick, and it is why §2's failure mode does not apply.

**Layering:** no change to `internal/arch/layering_test.go` is required, and
none should be made. `contracts/` may import only `models/` and `constants/`
(`TestContractsStayBelowTheLayersThatUseThem`), so the `GroupImporter` /
`GroupExporter` interfaces **cannot** move there while they mention
`groupio.ImportPlan`. Leave them where they are. Do fix their now-stale doc
comments, which still say "not in `server/interfaces`" — that package is now
`contracts/`.

Optionally add a fourth architecture rule asserting `groupio` does not import
`application_context` or `server`. Cheap, and it pins the seam.

### 3.4 Test strategy

**Baseline to preserve — re-measured at `dabbfbc6`** (the §5a fix commit, which
§2a step 2 makes the real baseline), all green, exit 0. The `849a6e2a` column is
kept so the delta is auditable:

| Suite | `849a6e2a` | **`dabbfbc6`** | Delta |
|---|---|---|---|
| `application_context` test funcs | 519 | **524** | +5 (the §5a regression tests) |
| — in the 9 seam-glob test files | 64 | **67** | +5 new, −2 accounting¹ |
| — `guid_test.go` (10th file) | 2 | **2** | — |
| `server/api_tests` test funcs | 627 | **627** | — |
| — of which import/export API | 7 | **7** | — |
| `archive` test funcs | 15 | **15** | — |
| `internal/arch` test funcs | 3 | **3** | — |
| `go test ./...` | 35 ok, 13 no-test, 0 FAIL | **same** | — |
| e2e spec files | 274 | **274** | — |
| e2e results | — | **1724 passed, 5 skipped** | — |
| Postgres (`mrql` + `api_tests`) | — | **both ok** | — |

¹ The earlier "64 across 8 files" double-counted two files through overlapping
globs. The corrected pre-fix figure is 62 across 8 files; `export_scope_test.go`
adds 5 and is the 9th. Recount from `ls ... | sort -u`, not from a raw glob.

**Package measurements also shifted** (§1's figures were taken at `849a6e2a`):

| Measure | `849a6e2a` | **`dabbfbc6`** |
|---|---|---|
| non-test LOC | 29,514 | **29,612** |
| methods | 452 | **453** (`visibleGroupIDs`) |
| exported methods | 329 | **329** (unchanged — the new one is unexported) |
| seam LOC (4 files) | 5,828 | **5,865** |
| `export_context.go` | 2,112 | **2,149** |
| `scoping.go` | 421 | **482** |

None of this changes the seam analysis: `export_context.go` still touches only
`db`, and `scoping.go` still only `db` + `principal`.

**What happens to the white-box tests.** The 64 import/export tests
(5,051 LOC across 8 files, all `package application_context`) touch this
context surface — established by scanning for *every* `ctx.*` reference, not a
hand-written shortlist:

| Symbol | Occurrences | Needed in the `groupio` fixture |
|---|---|---|
| `createTestContext` | 50 | port it (see step 2) |
| `ctx.db` | 50 | `*gorm.DB` |
| `ctx.fs` | 37 | `afero.Fs` |
| `*MahresourcesContext` (typed helpers) | 13 | rewrite to `*Service` / fixture |
| `NewMahresourcesContext` | 5 | fixture constructor |
| `altFileSystems` / `ctx.RegisterAltFs` | 3 / 2 | alt-FS map + registration |
| `ctx.buildExportPlan` | 3 | unexported, moves along |
| `ctx.Config` | 2 | whatever fields the tests read |
| `ctx.DefaultResourceCategoryID` | 1 | seeded default category |

So the fixture must carry `Service`, `Deps`, DB, fs, alt filesystems, `Config`,
and default-category state — not just a handle and a filesystem, as an earlier
revision claimed.

**A tenth file moves too.** `guid_test.go` calls `ensureGUID`, which is defined
in `apply_import.go` and therefore moves. It does not match the
`*import*_test.go` / `*export*_test.go` filename pattern, which is exactly why
two revisions missed it. Select test files by **which moved symbols they
reference**, not by filename. (`export_overhead_test.go` uses `exportPlan` and
`estimateJSONOverhead`, both unexported in `export_context.go`; it already
matches the pattern and moves with them.)

**One file straddles the seam and must be split first.**
`export_scope_test.go` (added by the §5a fix) cannot live in either package as
written, because it uses symbols from both sides:

| Test | Uses | Destination |
|---|---|---|
| `GroupRelationEdgeStaysInSubtree` | `buildExportPlan`, `exportPlan.groupIDs`/`.shellGroupIDs` **and** `WithPrincipal` | **blocked** |
| `OwnerBackfillStaysInSubtree` | same | **blocked** |
| `EstimateDoesNotCountOutOfSubtreeGroups` | `EstimateExport` + `WithPrincipal` — exported facade only | stays in `application_context` |
| `ArchiveExcludesOutOfSubtreeGroup` | `StreamExport` + `WithPrincipal` | stays |
| `NoTargetlessRelationshipPayload` | `StreamExport` + `WithPrincipal` | stays |

The two blocked tests reach into `exportPlan` (moving to `groupio`) while also
needing `WithPrincipal` (staying, and unreachable from `groupio` without an
import cycle). **Resolve before moving code**, by rewriting both against the
exported API: `EstimateExport` already reports `Counts.Groups` and
`Counts.ShellGroups`, which is exactly what their plan-level assertions check.
Do the rewrite on the current baseline and confirm it still fails under the
mutation (force `visibleGroupIDs` to reject everything) — an API-level rewrite
is the easiest place to reintroduce a vacuous assertion.

This is the same lesson as `guid_test.go`, arriving from the other direction:
the moving set is defined by symbol references, and a file can reference symbols
on **both** sides of a seam.

The tests split into two groups, and the split is load-bearing:

- **(a) white-box service tests** — the 64 existing ones. Move to
  `package groupio`. They need only a handle and a filesystem.
- **(b) facade / security tests** — the 4 new ones in §"transaction and scoping"
  below. Stay in `package application_context`. They need `WithPrincipal`,
  `WithTransaction`, and the registered callbacks, none of which `groupio` can
  reach without an import cycle.

Plan for group (a):

1. Move all 8 test files to `package groupio` alongside the code. `ctx.db` /
   `ctx.fs` become the local handle and filesystem the test already constructs;
   `buildExportPlan` stays unexported in the same package and keeps working.
   This is safe to do with a bare `*gorm.DB` that lacks the scope/stamp
   callbacks: **none** of the 64 tests reference `CreatedByUserId`,
   `WithPrincipal`, or `principal`, so none depend on `registerScopeCallbacks`.
   Verify that claim still holds before moving; if a test has since grown such a
   dependency, it belongs in group (b) below instead.
2. Port **`createTestContext`** (`resource_context_test.go:25`) into a `groupio`
   test helper returning `(*Service, *gorm.DB)`. This is the helper the moved
   tests actually call — **50 call sites** across the three largest files
   (`apply_import_test.go` 21, `export_context_test.go` 21,
   `import_context_test.go` 8). It is *not* `setupTestContext`
   (`dashboard_context_test.go:16`), which those files never call, and
   `resource_context_test.go` is not moving — so without this port the moved
   tests fail on an undefined symbol.
   Note also the differing DSNs: `createTestContext` uses
   `file::memory:?cache=shared` while `setupTestContext` uses a per-test
   `cache=private` name. Preserve `cache=shared`; some cross-entity paths
   depend on it.
   Note its hand-maintained `AutoMigrate` list — the new helper needs its own
   copy, and it must include every model the import path writes (`Series`,
   `ResourceVersion`, `TemplatePartial`, `SavedMRQLQuery` are **not** in the
   current list; verify against what the import actually creates before
   assuming parity).
   Sweep for any other cross-file test helper the moved files reference before
   starting; this one was missed by inspecting the wrong file.
3. The 7 black-box `api_tests` (`import_api_test.go`, `export_api_test.go`)
   need **no change** — they drive HTTP and the aliases keep types identical.

**What proves behaviour is unchanged:** the 64 moved tests plus 7 API tests must
pass with identical counts. The move is mechanical; any diff in behaviour is a
bug in the move.

**What proves the transaction and scoping semantics survived** — this is new
coverage that does not exist today and is the real deliverable of the slice.

**These four tests must live in `package application_context`, not `groupio`.**
They need `WithPrincipal` / `WithTransaction` and the callbacks
`registerScopeCallbacks` installs (`context.go:481`), and `application_context`
imports `groupio`, so a `package groupio` test importing back is an import
cycle. A bare `*gorm.DB` would not install the callbacks either, so the
assertions would silently pass against a context that never enforced anything.
Build the context through `NewMahresourcesContext` for all four.

1. **Handle propagation, not nesting.** ~~Run `ApplyImport` inside
   `WithTransaction`~~ — this was wrong and is deliberately not the test.
   GORM's `Begin` (`finisher_api.go`) switches on `Statement.ConnPool`; inside
   `WithTransaction` that pool is a `*sql.Tx`, which satisfies neither
   `TxBeginner` nor `ConnPoolBeginner`, so it falls through to
   `default: err = ErrInvalidTransaction`. Since `apply_import.go` opens its
   batches with `s.ctx.db.Begin()` (lines 1241, 1773), a nested run would abort
   at the first batch and "zero rows persisted" would prove nothing.
   **`ApplyImport` is not nested anywhere in production** — all 13
   `WithTransaction` call sites are in bulk/series/crud contexts, and the sole
   `ApplyImport` call site is the handler on a `WithPrincipal`-derived context.
   So do not add savepoint machinery for a path nothing exercises. Instead
   assert that `groupioDeps()` hands the service **the current `ctx.db`**: call
   through a derived context and verify the service observes the derived
   handle, not the singleton. Against the field-capture design of §2 this
   fails; against `groupioDeps()` it passes.
2. **Scoped import is denied, not confined.** ~~Import under a group-limited
   principal and assert the writes are confined to the subtree~~ — that would
   codify a guarantee the system does not make. **All five import routes are
   wrapped in `denyScopedPrincipal`** (`server/routes.go:702-706`), because, as
   the comment there states, import creates new top-level groups that a
   group-limited principal could not place inside its subtree; the whole import
   surface is fail-closed for scoped users and guests. So
   `ctx.WithPrincipal(p).ApplyImport(...)` is **not a supported request path**,
   and a test asserting confined-import semantics would either pass vacuously
   or invent a contract.

   Test the real invariant instead: a group-limited principal receives the
   denial at the import routes, and the extraction does not quietly open that
   door. Concretely — assert the 403/deny boundary still holds after the move,
   and treat any new `groupio` entry point reachable by a scoped principal as
   out of scope for this slice.

   **This is a boundary to preserve, not a feature to add.** If someone later
   wants scoped import to work, it is its own project: caller-controlled
   `ShellGroupAction` and `DanglingAction` destination IDs are accepted without
   visibility checks and drive association and `GroupRelation` writes that the
   GORM scope callbacks do not guard, so an owner-only check would pass while
   cross-subtree relation writes stayed possible. Do not let a refactor be the
   thing that enables it.

   **Export is the opposite case and is genuinely scoped:** `/v1/groups/export`
   and the download route run through `scopedCtx` (`routes.go:691,695`), the
   estimate route through `scopedAPI` (`routes.go:687`), and
   `ensureGroupsVisible` rejects out-of-subtree roots. Scope assertions belong
   on the **export** path, where they describe something real.

2a. **PRE-EXISTING BUG FOUND — do not bundle it into this slice.** Probing the
   question raised by (9) turned up a real defect on `master`, unrelated to the
   refactor. Reproduction and evidence are in §5a. Summary: the M2M BFS is
   correctly confined, but the **typed `GroupRelation` BFS is not**, because
   `bfsCollectGroupRelations` queries the `group_relations` table and
   `scopeColumn()` only handles `groups` / `resources` / `notes` — so the scope
   callback is a no-op there and `ToGroupId` is consumed with no visibility
   check.

   **Fix it on its own branch, and land it FIRST. This is a hard prerequisite,
   not a suggestion.** It is a separate commit with a separate rationale — it
   must not be bundled *into* the decomposition commit — but the decomposition
   cannot start until it has landed. Sequencing:

   1. ~~Land the `bfsCollectGroupRelations` visibility fix (§5a) on its own
      branch, with its own regression test.~~ **DONE** — branch
      `fix/scoped-export-group-relation-confinement`, 4 regression tests.
   2. **That commit becomes the decomposition baseline.** Re-measure §3.4's
      baseline numbers against it, not against `849a6e2a`.
   3. ~~Confirm the (2b) archive-confinement test passes on that baseline.~~
      **DONE** — `TestScopedExport_ArchiveExcludesOutOfSubtreeGroup` passes on
      the fix branch and fails on `849a6e2a`.
   4. Only then move code. ← the decomposition may now start, once step 2's
      re-measurement is recorded.

   The reason ordering is not free: (2b) is the slice's only archive-content
   confinement test, and it **cannot pass on today's `HEAD`** — scoped
   `StreamExport` aborts with `load group N: record not found` before producing
   a tar to inspect. Deferring the fix to "after" therefore means shipping the
   refactor with its central security test either absent or rewritten to accept
   the defect as baseline. Neither is acceptable.

2b. **Scoped export must be asserted on archive contents, not status codes.**
   This is the single highest-risk path in the slice and it is **currently
   untested at the output level**. `TestScopedUser_ExportConfined`
   (`server/api_tests/scoping_http_test.go:145-163`) only POSTs to
   `/v1/groups/export/estimate` and compares status codes — it never submits
   the export, downloads the tar, or parses it. So if the extraction handed the
   export service a singleton handle instead of the scoped one — precisely the
   §2 failure mode — `ensureGroupsVisible` would still reject an out-of-subtree
   *root* and this test would stay green, while the BFS quietly followed
   related groups, resources, notes, and `GroupRelation` edges out of the
   subtree and wrote them into the archive. Green tests, leaked tenant data.

   Required acceptance test — the request body must be spelled out, because
   `relatedDepth > 0` **alone is not sufficient**. Both traversal branches are
   independently gated (`export_context.go:291,299`) on `Scope.RelatedM2M` and
   `Scope.GroupRelations`, whose JSON zero value is `false`. A request that sets
   only `relatedDepth` exercises **neither** vulnerable path and excludes every
   outside entity trivially — the exact vacuous pass described in revision 5.

   ```json
   {
     "rootGroupIds": [<in-subtree root>],
     "relatedDepth": 2,
     "scope": {
       "subtree": true, "owned_resources": true, "owned_notes": true,
       "related_m2m": true, "group_relations": true
     }
   }
   ```

   Fixture: cross-boundary edges of **both** kinds (M2M `RelatedGroups` /
   `RelatedResources` / `RelatedNotes`, and a typed `GroupRelation`), plus at
   least one **in-subtree** related target of each kind.

   Assert three things, not one:
   1. **Positive traversal** — the in-subtree related targets **are** present.
      This is the control that proves the BFS ran; without it the negative
      assertion proves nothing.
   2. **Negative confinement** — no out-of-subtree IDs, names, GUIDs, metadata,
      or blobs appear anywhere in the tar, **including the manifest** (§5a
      showed the manifest is written from plan state before payload load, so
      checking only the payload files would miss it).
   3. **Unscoped control** — the same fixture exported by an admin **does**
      contain the outside entities, proving the edges are followable and the
      scoped run's absence is enforcement rather than a no-op.

   Run it against the §5a-fixed baseline, not today's `master` (see 2a).
3. **Actor attribution.** Import under `WithPrincipal(p)`; assert
   `CreatedByUserId == p.UserID` on created resources, notes, and groups. This
   remains valid despite (2): the route denial targets *group-limited*
   principals, while attribution applies to the unscoped users, editors, and
   admins who do import.
4. **`Begin()` inherits the scoped context.** `apply_import.go` opens batch
   transactions with `s.ctx.db.Begin()` rather than `WithTransaction`. Assert
   directly that a handle derived by `WithPrincipal` still carries its
   `scopeFilter` and acting-user values through `Begin()`. If this fails, it is
   a **pre-existing latent bug**, not one the refactor introduced — write it
   against `master` first to establish which.

Write tests 1–4 **before** moving any code (red against a deliberately
field-captured prototype, green against the real design). That is the TDD order
CLAUDE.md asks for and it is what makes the slice safe.

### 3.5 Verification recipe

Capture the baseline on `master` first, then compare after.

```bash
# 0. Baseline, on master, before touching anything
go test --tags 'json1 fts5' ./... 2>&1 | tee /tmp/base-go.txt
grep -c '^ok' /tmp/base-go.txt    # expect 35
grep -c FAIL /tmp/base-go.txt     # expect 0

# 1. Build (json1 = SQLite JSON, fts5 = full-text search)
npm run build
go build --tags 'json1 fts5' ./...

# 2. Go unit tests, incl. internal/arch layering rules
go test --tags 'json1 fts5' ./... 2>&1 | tee /tmp/after-go.txt
diff <(grep -oE '^(ok|FAIL|\?)\s+\S+' /tmp/base-go.txt) \
     <(grep -oE '^(ok|FAIL|\?)\s+\S+' /tmp/after-go.txt)
# expect: only the added groupio package line

# 3. Layering rules explicitly (3 tests must stay green)
go test --tags 'json1 fts5' ./internal/arch/... -v -count=1

# 4. Import/export focus, no cache
go test --tags 'json1 fts5' ./groupio/... ./archive/... -count=1 -v
go test --tags 'json1 fts5' ./server/api_tests/... -run 'Import|Export' -count=1 -v

# 5. Postgres (needs Docker) — the import path is transaction- and
#    FOR UPDATE-sensitive, so SQLite alone is not sufficient evidence
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1

# 6. E2E, browser + CLI, both against ephemeral servers
cd e2e && npm run test:with-server:all
cd e2e && npm run test:with-server:postgres
```

Two process notes, both learned the hard way on this repo:

- **Rebuild `./mahresources` before running e2e.** The e2e harness reuses the
  prebuilt binary; skipping `npm run build` after a Go change produces phantom
  failures that look like regressions.
- **Do not read suite results through a pipe to `tail`** — you get `tail`'s exit
  code. Redirect to a log and read `.last-run.json`.

Expected outcome, by package — the split matters, and an earlier revision got
this backwards by putting the security tests in `groupio`:

Starting from `dabbfbc6`'s **524** `application_context` tests:

| Package | Change |
|---|---|
| `groupio` (new) | **64** moved white-box tests: 62 from the 8 original seam files + 2 from `guid_test.go`. Passing at identical counts. |
| `application_context` | **−64** moved out. **+5** new tests, all of which must stay because they need `WithPrincipal` / `WithTransaction` / `registerScopeCallbacks`: (1) handle propagation, (2) scoped-import route denial, (2b) scoped-export archive confinement, (3) actor attribution, (4) `Begin()` scope inheritance. **Retains** `export_scope_test.go`'s 5, two of which must first be rewritten against the exported API (see §3.4). Net: 524 − 64 + 5 = **465**. |
| `server/api_tests` | unchanged — **627**, including the 7 import/export API tests |
| `archive` | unchanged — **15** |
| `internal/arch` | **3**, plus one new `groupio` rule if adopted |
| e2e | unchanged — **1724 passed, 5 skipped** |

Net: no test is deleted, 64 relocate, 5 are added; 465 + 64 = 529 = 524 + 5.
If the arithmetic does not come out, something was dropped — do not proceed on
a hand-wave.

### 3.6 Revertibility

One commit, one new directory, one facade file. `git revert` restores the prior
state with no schema, HTTP, or archive-format implications. The archive format
(`archive/manifest.go`, schema version 1) is a stable public contract and is
**not touched** — no field renames, no semantic changes, no version bump.

## 4. Remaining slices

Sequenced by decreasing ratio of benefit to risk. Each is independently
shippable and revertible; none is a prerequisite for another except where noted.

| # | Slice | LOC | Fields | Notes |
|---|---|---|---|---|
| 1 | `groupio` (this plan) | 5,828 | 2 | already interface-mediated; 2 call-site files |
| 2 | `search` — `search_context.go` | 497 | 5 | fts/cache/config cluster; small, clean, good second proof |
| 3 | `media` — `resource_media_context.go` (+ upload) | ~2,965 | 5 | fs/locks/alt-fs heavy; thumbnails already have workers |
| 4 | `mrql` — `mrql_context.go`, `mrql_render_data.go`, `plugin_mrql_adapter.go` | ~2,410 | 4 | hardest: raw SQL + its own scope CTE + `WithMRQLPrincipal`; must move the principal, not just the handle |
| 5 | Template-provider DI catch-up | — | — | 58 concrete vs 2 interface params; pure `contracts/` work, no extraction |

Slice 5 is the one worth arguing for on its own merits. The DI migration is
lopsided in a way the brief undersells: `server/api_handlers/` is 117 interface
params vs 68 concrete (63% migrated), but
`server/template_handlers/template_context_providers/` is **2 vs 58** (3%). If
the goal is decoupling rather than file-shuffling, finishing that migration buys
more than slices 2–4 combined and carries near-zero risk, because it changes
only parameter types at the call boundary.

**Stop after slice 2 and re-measure.** Given §1 — 28 of 39 files touch ≤2 fields
— the remaining LOC in `application_context` is not obviously better off in
smaller packages. If slices 1 and 2 do not measurably improve test time,
reviewability, or the concrete-vs-interface ratio, the honest call is to do
slice 5 and stop. Splitting a package that is already internally
loosely-coupled is motion, not progress.

## 5a. Pre-existing bug: scoped export follows GroupRelation edges out of subtree

**STATUS: FIXED.** Branch `fix/scoped-export-group-relation-confinement`, ahead
of the decomposition and separate from it, exactly as the prerequisite in §2a
requires. That branch is now the decomposition baseline; re-measure §3.4's
numbers against it rather than against `849a6e2a`.

Found while verifying finding (9). Not caused by this refactor. The record below
is kept because it explains the fix and because the slice's acceptance test
(2b) depends on the fixed behaviour.

**The fix, in two parts.**

1. **BFS confinement.** `bfsCollectGroupRelations` collects candidate
   `ToGroupId`s and filters them through a new `visibleGroupIDs` helper
   (`scoping.go`) before they enter the plan. Filtering happens at BFS time,
   not at payload-load time, which is what avoids the trap below.

   `visibleGroupIDs` reads the allow-list `applyPrincipalScope` already
   materialized onto the db context and intersects in memory — **no query, and
   the same snapshot the GORM callbacks enforce**, so planning and payload
   loading cannot disagree about the tree mid-export. Two rejected
   alternatives, both of which the first draft used:
   - `WHERE id IN (?)`: the scope callback appends the whole allow-list as a
     second `IN` clause, so a large fan-out over a large subtree can exceed
     SQLite's `SQLITE_MAX_VARIABLE_NUMBER` or Postgres's 65535-parameter
     ceiling and abort a valid export.
   - `subtreeScopeIDs()`: re-runs the recursive group-tree CTE on **every** call,
     and this is called once per BFS level with a caller-controlled, uncapped
     `RelatedDepth` — turning a long relation chain into O(depth × subtree-size)
     work reachable from the estimate endpoint.

   Fail-closed in both directions: an empty allow-list denies all (matching
   `scopeReadCallback`), and a scoped principal whose db context somehow carries
   no filter also denies all rather than defaulting to unrestricted.
2. **No targetless relationships.** Filtering the target out is not sufficient
   on its own. The `group_relations` row remains readable (the table is not
   scope-mapped), so `loadGroupPayload` still saw the relation — while
   `collectDanglingRefs` skipped it, because its `Preload("ToGroup")` came back
   nil under scope. The result was a `GroupRelationPayload` with **neither**
   `ToRef` nor `DanglingRef`: invalid per the archive contract and silently
   dropped by the importer. Such relations are now omitted.
   **Emitting a dangling stub instead would have been wrong** — the stub
   carries `ToStub.Name`, the target group's name, which is precisely what the
   scope boundary must withhold. The safe representation is absence.

**Regression tests** (`application_context/export_scope_test.go`, 5 tests):
confinement with a positive in-subtree assertion *and* an unscoped control;
estimate counts; end-to-end archive bytes including the manifest; tar-parsing
round-trip asserting every relationship resolves to exactly one target
reference; plus a pin on `bfsEnsureOwners`, which reads scope-mapped tables and
was already safe (it passed before the fix, and is kept so a future
`scopeColumn` change cannot silently open a second path).

The positive assertion uses `relationOnlyRequest`, which leaves `Subtree` off.
That matters: with `Subtree: true` Phase A pre-seeds every in-subtree group, so
the assertion passes even with relation traversal **entirely disabled** — the
first version of this test was vacuous in exactly that way. Verified by
mutation: forcing `visibleGroupIDs` to reject every candidate makes the test
fail, which is the property a positive assertion has to have.

**Mechanism.** `bfsCollectGroupRelations` (`export_context.go:374-398`) queries
`models.GroupRelation`. `scopeColumn()` (`scoping.go:69-80`) maps only `groups`,
`resources`, and `notes`; `group_relations` falls to the `default` branch, so
`scopeReadCallback` returns early and applies no filter. The loop then takes
`rel.ToGroupId` and appends it to `plan.groupIDs` / `plan.shellGroupIDs` with no
visibility check.

**Measured on `master`**, group-limited guest scoped to `inside`, one typed
relation `inside → outside`:

| Path | Result |
|---|---|
| M2M BFS (`RelatedGroups`/`Resources`/`Notes`) | **correctly confined** — control proves the edges are followed when unscoped (`groupIDs=[1 2]`), scoped yields `groupIDs=[1]` |
| `GroupRelation` BFS | **not confined** — scoped yields `groupIDs=[1 2]`, `shellGroupIDs={2:true}` |
| `EstimateExport` (`POST /v1/groups/export/estimate`) | **succeeds**, returns `Groups=2 ShellGroups=1` |
| `StreamExport` | **fails**: `load group 2: record not found` — `loadGroupPayload` *is* scoped, so it cannot read the group the BFS just added |
| Manifest bytes written before that failure | contain the outside group's `name`, `guid`, and `source_id` |
| Partial tar in production | **deleted** — `buildExportRunFn` does `fs.Remove(tarPath)` on `streamErr` |

**Actual impact, stated precisely:**

1. **Reachable information disclosure (low).** `/v1/groups/export/estimate`
   returns 200 and discloses the *count* of out-of-subtree groups a scoped
   caller's subtree has typed relations to. Names are not disclosed by this
   endpoint. The existing `TestScopedUser_ExportConfined` cannot catch it: it
   only compares status codes, and the status is legitimately 200.
2. **Functional break (moderate).** A scoped principal whose subtree has any
   outgoing `GroupRelation` to an outside group **cannot export at all** — the
   stream always aborts. This is probably the more user-visible half.
3. **Not currently a content leak.** The manifest naming the outside group never
   reaches a user, because the partial tar is removed on error.

**The trap.** The obvious fix for (2) — make `loadGroupPayload` skip a missing
shell group and emit a warning — would convert (3) from "not reachable" into a
genuine cross-tenant content leak, because the manifest entry is written from
plan state, not from the loaded row. **Fix it at BFS time instead**: filter
`ToGroupId` through a visibility check in `bfsCollectGroupRelations`, so the
out-of-subtree group never enters the plan. That fixes (1), (2), and (3) at
once and keeps `loadGroupPayload`'s strictness as a backstop.

Worth checking in the same pass: `bfsEnsureOwners` (`export_context.go:405`)
writes `shellGroupIDs[*row.OwnerID]` at lines 427 and 452 from owner IDs. Its
source rows come from scoped queries, so it is likely fine, but it was not
probed.

## 5. Explicit non-goals

- No behaviour, HTTP contract, or schema changes.
- No archive-format changes (`schema_version` stays 1).
- No touching the `routes.go` / `routes_openapi.go` double declaration.
- No renames for taste.
- No big-bang rewrite. The evidence does not support one, and §1 argues it
  would not pay for itself.
