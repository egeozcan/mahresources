# Plugin package format: manifest, `api_version`, permission grants

Package 2 of the plugin roadmap, planned in
`docs/plans/2026-08-15-plugin-invocation-and-hook-integrity.md` §7 (item G in the
capability report). Package 1 shipped in `bac7fd3f`.

It is second because it closes a live security hole and because it provides the
grant mechanism that lets package 3's deny-lift land per plugin rather than
globally.

---

## 0. Scope

**In:**

1. A manifest declared on the existing `plugin` global: `api_version`,
   `capabilities`, `network`, `allow_private_hosts`, `dependencies`,
   `min_app_version`.
2. Capability grants enforced **at load**, by installing only the granted subset
   of the `mah` table into the `LState`.
3. Consent persisted per plugin, so editing `plugin.lua` cannot widen a grant
   silently.
4. Egress control on **every plugin-initiated fetch** (sharp edge #4): per-plugin
   host allowlist checked before the request and re-checked on every redirect
   hop, plus a global dial-time deny of loopback / link-local / private
   addresses. "Every" includes the host-side downloader reached through
   `mah.db.create_resource_from_url` and `add_resource_version_from_url` — see
   the co-requisite rule in §4.
5. The server-side `action.Filters` re-check on submitted `entity_ids`, and the
   `ResourcesMatching` category gap it exposed.
6. Dependencies enforced as "the named plugin must be enabled", including boot
   ordering.
7. Manifests for the six bundled plugins, the manage UI, docs, e2e.

**Out, deliberately:**

- **Distribution.** Signed tarballs, `mr plugin install`, an index. Separable
  second half, per the report.
- **Install-then-rescan-without-restart.** The discovered-plugin list is
  immutable after construction (`manager.go:869-877` documents this as what lets
  two readers run lock-free). Making it mutable is a named refactor.
- **`min_app_version` enforcement.** There is no application version constant to
  compare against — `package.json`'s `1.0.0` is inert and nothing reads it.
  Inventing one here would be enforcement against a number nobody maintains. The
  field is parsed, stored, displayed and **warned about, never enforced**, and
  this plan says so rather than implying a check exists.
- **Lifting the group-confined plugin deny.** That is package 3, and it is the
  thing this package exists to make safe.
- **The app's own remote-fetch surface, reached from anywhere other than a
  plugin.** `/v1/resource/remote` and the download queue keep their present
  behaviour; this package constrains the plugin's use of that downloader, not
  the operator's. Fixing the endpoint itself belongs with the app's
  remote-download surface.

---

## 1. The manifest

**Location: the `plugin` global**, extended. Discovery already executes
top-level Lua and reads `name`, `version`, `description` from that table
(`manager.go:249-260`); a second file would drift from it and would need its own
parser. The cost is that a malformed manifest is a Lua error, which discovery
already handles by skipping the plugin with a warning.

```lua
plugin = {
  name = "fal-ai",
  version = "1.1.0",
  description = "...",

  api_version = 1,                       -- required for a manifest to exist
  capabilities = { "db:read", "db:write", "http", "actions", "jobs",
                   "pages", "image" },   -- exactly what it uses: no "render",
                                         -- because it registers no shortcode,
                                         -- block type or display type
  network = { "fal.run", "*.fal.ai" },   -- optional; absent = any public host
  allow_private_hosts = false,           -- optional; default false
  dependencies = { "other-plugin" },     -- optional
  min_app_version = "1.2.0",             -- optional, warn-only
}
```

**A manifest exists iff `api_version` is present.** That is the single
discriminator; `capabilities = {}` with an `api_version` is a legal manifest
declaring nothing, and is not legacy mode.

**`api_version`** is compared against a host constant
`plugin_system.PluginAPIVersion = 1`. A plugin declaring a *higher* version is
refused at load with a clear message (it wants a host it does not have). A
*lower* version is accepted for now — there is only one version, so a
lower-version branch cannot be tested honestly; the acceptance rule is written
down here so the first real bump has to make a decision rather than inherit one.

**Validation** happens at discovery: unknown capability names are an error (a
typo'd capability that silently grants nothing produces a plugin that fails
deep inside `init()` with `attempt to call a nil value`), non-string entries are
an error, and a `network` entry that is not a hostname, a `*.suffix` pattern, an
IP literal or a CIDR is an error. A plugin that fails manifest validation is
skipped with a logged warning, exactly as a plugin that fails to parse is today.

### Legacy plugins

No `api_version` → **legacy**: the full `mah` surface, a loud warning at load,
and a badge in the manage UI.

**Legacy is not an exemption from the egress fix.** The dial-time deny of
private, loopback and link-local addresses applies to every plugin, manifest or
not. Sharp edge #4 is a vulnerability, and exempting the plugins that exist
today would leave it open for exactly the population that has it. A legacy
plugin that genuinely needs a LAN host must gain a manifest with
`allow_private_hosts` — the migration is one table, and the six bundled plugins
get theirs in this package.

The loophole needs a closing deadline, and this is it: **legacy mode is removed
when the first `api_version` bump lands.** Written here, at the point it is
opened, not after.

---

## 2. Consent, and where it is stored

The manifest alone cannot be the grant. If the live `plugin.lua` is the source
of truth, adding `db:write` and restarting grants it, and operator consent is
theatre.

**`PluginState.GrantsJSON`**, a new text column beside `SettingsJSON`
(`models/plugin_state_model.go`), holds what the operator consented to:

```json
{"api_version":1,"capabilities":["db:read","http"],"network":["fal.run"],"allow_private_hosts":false}
```

- **Enable** (`SetPluginEnabled(name, true)`, `plugin_state_context.go:81`)
  writes the currently-declared set as the consented set, then loads. Enabling
  *is* the consent gesture; the manage UI shows what is about to be granted next
  to the button.
- **Load** (boot activation and enable alike) compares consented against the
  **load-time** manifest — the one the grants were built from — never against
  `DiscoveredPlugin.Manifest`. The two can differ without the file changing: the
  header VM runs once at discovery and again at load, and a plugin can vary its
  declaration between runs (`tostring({})` is a heap address, `math.random` is
  auto-seeded). Comparing consent against the discovered copy would check one
  manifest and enforce another. `declared ⊆ consented` loads. Anything else — a new capability, a
  new host, `allow_private_hosts` flipped on, a raised `api_version` — refuses
  to load with `ErrGrantsChanged`, and the manage UI shows "requires
  re-consent: <the delta>" with a re-enable button.
- **Narrowing is not a re-consent event.** A plugin that drops a capability
  loads with the smaller declared set (the *declared* set is what is installed;
  the consented set is only ever a ceiling).
- A **legacy** plugin's consented set records `{"legacy":true}`.

### `⊆` is not the same relation on every field

Three of the five fields invert or special-case, and getting one backwards
grants silently. Written out because a single `for` loop over "is every declared
entry in the consented list" is wrong for two of them:

| Field | Widening (refuse) | Narrowing (load) |
|---|---|---|
| `capabilities` | declared has one consented lacks | declared has fewer |
| `network` | **consented restricted, declared unrestricted** | declared list ⊆ consented list |
| `allow_private_hosts` | `false` → `true` | `true` → `false` |
| `api_version` | declared > consented | declared ≤ consented |
| `legacy` | manifest → legacy | legacy → manifest (see below) |

**`network` is the inverted one.** An absent or empty `network` means *any
public host* — the broadest policy, not the narrowest. So a plugin consented at
`network = {"fal.run"}` that drops the field entirely has widened to the whole
public internet, and must refuse. Plain subset logic reads an empty declared
list as trivially contained and would load it.

**Capabilities compare as the *effective* set, not the declared list**, because
the effective set is what gets installed: `Capabilities()` already adds
`db:read` whenever `db:write` is present, so a plugin that later spells out both
has changed nothing an operator would recognise, and must not prompt.

**Legacy → manifest loads; manifest → legacy refuses.** Legacy is the maximum
grant, so gaining a manifest is strictly a narrowing, and the plan's earlier
"a change the operator sees" would have punished exactly the migration this
package wants — every bundled plugin gained a manifest in batch 1, so requiring
re-consent for that would leave them disabled on upgrade for no security gain.
The one exception is `allow_private_hosts = true`, which legacy never had: that
is a widening even out of legacy, and re-consents. Losing a manifest goes the
other way and always refuses.

### The upgrade: a NULL `GrantsJSON` is grandfathered, once

Every existing `plugin_states` row predates the column, so on the first boot
after upgrade every enabled plugin has no consented set. Treating that as "never
consented" would refuse to load every plugin in every deployment — a
self-inflicted outage on the release that is supposed to *tighten* security.

**NULL/empty consent is backfilled from the load-time manifest and loads**, with
one log line per plugin naming what was recorded. This is sound rather than
merely convenient: before this release those plugins ran with the entire `mah`
surface and no egress control, so grandfathering them to their *declared* set is
strictly a reduction in privilege. The gesture being trusted is the operator's
original decision to install and enable the plugin.

It happens exactly once per plugin — after the backfill the row has a consented
set, and every subsequent widening prompts normally. It is **not** a fallback
for a row whose consent was written and then failed to parse: that is corruption
or tampering, and it refuses with `ErrGrantsChanged` rather than silently
re-granting whatever the file now asks for.

---

## 3. Capability taxonomy, and enforcement at load

Enforcement is by construction: `registerMahModule` (`manager.go:351`) builds
the `mah` table key by key, so an ungranted key is simply never set. No guard at
62 call sites, and no way to reach a host function whose key is absent.

| Capability | Grants | Registered in |
|---|---|---|
| `db:read` | the `mah.db` readers (every function built on `querierFor`), `mrql_query`, `get_resource_data` | `db_api.go` |
| `db:write` | the `mah.db` writers (every function built on `writerFor`), including resource creation and association. **Except the two URL-fetching writers, which additionally require `http`** — see §4 | `db_api.go` |
| `http` | `mah.http` | `http_api.go` |
| `kv` | `mah.kv` | `kv_api.go` |
| `image` | `mah.image` | `image_api.go` |
| `hooks` | `mah.on` | `manager.go` |
| `inject` | `mah.inject` | `manager.go` |
| `render` | `mah.shortcode`, `mah.block_type`, `mah.display_type` | `manager.go` |
| `pages` | `mah.page`, `mah.menu` | `manager.go` |
| `api` | `mah.api` | `manager.go` |
| `actions` | `mah.action`, and the `job_*` reporters (an async action is handed a `job_id` and is expected to report on it, so requiring `jobs` for that would hand a plugin an id it cannot use) | `manager.go` |
| `jobs` | `mah.start_job`, and the `job_*` reporters | `manager.go` |

**Always installed, ungated:** `mah.json`, `mah.util`, `mah.log`,
`mah.html_escape`, `mah.sleep`, `mah.abort`, `mah.doc`, `mah.get_setting`. None
of them reads or writes anything outside the plugin itself. `abort` is grouped
here rather than under `hooks` because it is inert without a hook to raise from,
and `get_setting` reads only that plugin's own settings.

The `db:read` / `db:write` split follows the line the code already draws:
`querierFor(L)` versus `writerFor(L)` (`db_api.go:209-232`). The registrar
helpers (`registerGetter`, `registerLister`, `registerCounter`,
`registerCreator`, …) make this mechanical — each helper is told its kind once,
not each of the 63 functions.

**`inject` is separate from `render`** because it is not author-invoked: six of
its slots live in the base layout, so an injection runs on every page, while a
shortcode or block runs only where a template author placed it. An operator
consenting to "this plugin may render blocks" has not consented to "this plugin
emits HTML and `<script>` on every page in the app."

**Absent, not stubbed.** An ungranted key is nil, so `if mah.kv then` works and
a plugin can degrade. To keep the resulting `attempt to index a nil value`
diagnosable, load logs one line per withheld module: `[plugin] fal-ai: mah.kv
not installed (capability "kv" not granted)`.

**Guard:** an architecture test walks the `mah` table registration sites and
fails the build when a new `RawSetString` on the root `mah` table or a new
`register*Module` appears without a capability decision — the same shape as
`internal/arch/plugin_db_chokepoint_test.go` and
`internal/arch/plugin_render_gate_test.go`.

---

## 4. Egress control (sharp edge #4)

Measured today: `validateScheme` (`http_api.go:284-291`) is a case-insensitive
`http://` / `https://` prefix test; the URL is never parsed for a host.
`newHttpClient` (`http_api.go:37-49`) leaves `Transport` nil, so the default
transport dials anything, and `CheckRedirect` counts hops without looking at the
target. Under the default no-auth deployment every request is an implicit
administrator, so the admin gate on plugin paths is effectively absent.

Three layers, because no one of them is sufficient:

**(a) Request-level allowlist, per plugin.** One chokepoint applied before
`Do` in both `executeHttpRequest` and `executeSyncHttpRequest`: parse the URL,
match the host against the plugin's `network` list (exact host, `*.suffix`, IP
literal, CIDR). No list → any public host. This is the per-plugin gate.

**(b) Redirect re-validation.** The same match inside `CheckRedirect`, per hop.
This is the half that is easy to forget and where the current code is weakest: a
request to an allowed public host can otherwise be redirected to an internal one
for ten hops with no re-check.

**(c) Dial-time deny, global.** A `Transport.DialContext` wrapping a
`net.Dialer` whose `Control` inspects the **resolved** address and rejects
loopback, link-local (including `169.254.169.254`), unique-local, private and
unspecified ranges. This is the DNS-rebinding defense: (a) and (b) see a
hostname, and a hostname that resolves to `127.0.0.1` passes them both. `Control`
runs per candidate IP after resolution, so it cannot be fooled by a second
answer.

**The host's own downloader is a fourth door, and it takes the same three
layers.** `mah.db.create_resource_from_url` and `add_resource_version_from_url`
do not use `mah.http` at all — they hand the URL to the application's remote
downloader, the same path `/v1/resource/remote` uses. Gated on `db:write`
alone they would leave a plugin a full server-side fetch primitive that the
declared `network` list cannot see, which is the hole this package exists to
close, reached through a different door.

**Decision: they require `http` as a co-requisite capability, and their URL is
subject to (a), (b) and (c) exactly as `mah.http` is.** A plugin that declares
`db:write` without `http` gets the other writers and not these two — the
`registerCreator`-style helper is told this once, not at each call site. The
rule is what makes the capability model honest: a function that opens a socket
is a network function no matter which module it is filed under, and after this
a plugin's `network` list *is* its egress surface rather than most of it.

Two consequences, both deliberate:

- **`plugins/fal-ai/plugin.lua` must declare every host it downloads results
  from**, not just `queue.fal.run`. Its declared list is not its real egress
  surface today, which is the proof the rule is needed. Batch 5 widens its
  manifest; if the result CDN is not a stable, enumerable host set, that is a
  finding about fal-ai's manifest and it gets recorded rather than papered over
  with a wildcard.
- **A third-party `db:write`-only plugin that fetches URLs breaks on upgrade**
  until it adds `http` and its hosts. Same shape as the private-host break, same
  treatment: release note, not a silent behaviour change.

Enforcement lives at the downloader call, not at the Lua boundary, so a future
caller of the same host function inherits it. Batch 3 adds an architecture guard
in the shape of `plugin_db_chokepoint_test.go`: a new `mah.db` function reaching
the remote downloader without passing through the egress chokepoint fails the
build.

**The private-host opt-in is address-based, not name-based.** `allow_private_hosts`
lifts layer (c) only for a request whose *resolved address* matches an address
rule (an IP literal or a CIDR) in the plugin's own list. It must never be lifted
because the request matched a *name* rule: a public name resolves to whatever
the plugin author's DNS says, so `network = {"attacker-owned.example"}` with an
A record pointing at `169.254.169.254` would otherwise satisfy every validation
rule and reach the metadata endpoint. Names remain declarable for reachability;
reaching a private address means naming the address.

**Private hosts are reachable only by explicit opt-in.** `allow_private_hosts =
true`, consented like any other grant and rendered in the UI as its own line,
relaxes (c) **for hosts that already matched (a)**. This is not optional
nice-to-have: this app's users legitimately point plugins at LAN services (an
Immich or Nextcloud importer is RFC1918 by nature), so a blanket private-deny
with no escape hatch would break the real use case and get switched off wholesale.

**One client per distinct policy, not one shared client.** `http.Transport`
pools connections keyed on scheme+host, not on our policy, so a shared client
lets plugin B reuse a connection plugin A opened to a host B may not reach —
with no dial for B, layer (c) never runs. `pm.httpClientFor(policy)` returns a
client cached by policy fingerprint (sorted allowlist + private flag), so two
plugins share a pool only when their policy is identical. Legacy and
no-`network` plugins share the single public-only client, which is correct
because their policy *is* identical.

A blocked request returns the same error shape a DNS failure does — `{error =
"..."}` for sync, the error callback for async — with a message naming the host
and the rule, and one warning in the application log (entity type `plugin`).

---

## 5. The `action.Filters` re-check

`actionMatchesFilters` (`actions.go:532`) decides which actions the UI
*offers* for an entity (`actions.go:493`, via `GetActions`). Nothing re-checks
it against the submitted `entity_ids` at execute time, so an action restricted
to `content_types = {"image/png"}` runs happily on a PDF via a direct POST.

**Re-check with the same function, not a parallel query.** A new reader returns
the filter-relevant fields for the submitted ids:

```go
// plugin_system
type ActionEntityDataReader interface {
    EntityFilterData(entity string, ids []uint) (map[uint]map[string]any, error)
}
```

implemented in `application_context` beside `actionEntityRefReader`, selecting
only what the filters read (`content_type` + `resource_category_id` for
resources, `note_type_id` for notes, `category_id` for groups), chunked at the
existing `entityRefChunkSize`. The handler then runs `actionMatchesFilters` per
id. Offer and execute cannot drift, because they are the same predicate over the
same keys.

**A mismatch rejects the whole batch**, HTTP 400 in the existing
`{"errors": [...]}` shape, naming the offending ids. This is package 1's written
rule ("a veto covers the whole batch") applied to the same question, and the UI
never offers a mismatched action anyway, so the only caller that hits it is a
direct POST.

It goes at the existing chokepoint in `GetActionRunHandler`
(`server/api_handlers/action_handlers.go:178-197`), where the comment already
declares that DB-backed validation happens exactly once, before the async/sync
fork — so bulk fan-out pays one extra read for the batch, not one per entity.

**The gap this exposed:** `actionEntityRefReader.ResourcesMatching`
(`application_context/action_entity_ref_reader.go:39`) never applies
`filter.CategoryIDs`, so an `entity_ref` param declaring a resource-category
filter accepts a resource of any category. `ResourceSearchQuery` carries a
single `ResourceCategoryId`, not a list, which is why it was skipped. Fixed with
its own test.

---

## 6. Dependencies and boot order

The only honest semantic without a Lua module loader is **"the named plugin must
be enabled"**:

- Enabling refuses when a dependency is not enabled, naming it
  (`ErrDependencyNotEnabled`).
- Disabling refuses when an enabled plugin depends on it, naming that plugin
  (`ErrDependencyInUse`). Refuse, not cascade: disabling one plugin must not
  silently disable another. The operator asked about *this* plugin, and a
  dependent going dark as a side effect is a change they did not make and will
  not go looking for.
- A dependency on an *undiscovered* plugin refuses at enable and is shown in the
  UI.
- Cycles are rejected at discovery.

**The checks live on `EnablePlugin`/`DisablePlugin`, not on
`SetPluginEnabled`.** The plan first put them at the `application_context`
layer; the manager is the right place because the `mr` CLI, the API and the
template UI all converge there, and a rule enforced one layer up is a rule the
other two callers do not get. `DisablePlugin` — the public entry — is also the
only one that checks: `Close` tears every plugin down through `finishTeardown`
directly, and a shutdown that refused to stop a plugin because something
depended on it would never finish.

**Only the members of a cycle are dropped, not everything that reaches one.**
"Can reach a cycle" and "is on a cycle" are different questions, and a plugin
that merely *depends* on a cycle member is not itself broken packaging. Dropping
it would make it vanish from the manage UI with no explanation; leaving it lets
the enable refusal name the dependency it cannot get. `pluginsOnADependencyCycle`
gets this with two prunings — drop nodes with no outgoing edge until none
remain (everything that can reach a cycle), then drop nodes with no incoming
edge (leaving what is also reachable *from* a cycle, which for a node on a path
both to and from a cycle means the cycle itself). A diamond survives both.

**Boot order.** `ActivateEnabledPlugins` (`plugin_state_context.go:193`)
iterates states in name order, so a dependent can be enabled before its
dependency. Replaced with a repeated pass: enable everything whose dependencies
are already loaded, repeat while progress is made, then log a warning naming
whatever is left (a cycle among enabled rows, or a dependency that failed to
load). No topological sort library, and the leftover set is the diagnosis.

---

## 7. UI

`templates/managePlugins.tpl`, per card:

- **Capabilities**, always rendered, as a list of human sentences rather than
  slugs ("Read your library", "Write to your library", "Reach the network:
  fal.run, *.fal.ai"). Before enabling, headed "Enabling grants:".
- **`allow_private_hosts`** as its own line, worded as the risk it is ("May
  reach private network addresses, including services on this machine").
- **Legacy badge** with the "no manifest — full access" warning, linking to the
  docs section on adding one.
- **Needs re-consent** state: the button becomes "Review & enable", the delta is
  listed, and the plugin stays unloaded until it is pressed.
- `dependencies`, and any unmet ones.
- `min_app_version` when declared, labelled as informational.

`GET /v1/plugins/manage` grows the same fields (`pluginListItem`), so the CLI
and API see what the page sees.

---

## 8. Sequencing

Five batches. Each is TDD (a failing test first).

**Review cadence (decided 2026-08-16, after batch 1).** Batch 1 ran ten
alternating rounds. The manifest and capability code — the batch's actual
subject — was quiet for the last five; every finding after round 5 was in VM
teardown and lifecycle, a subsystem this plan never scoped and that the review
loop itself kept destabilising. The loop had started finding bugs it had
introduced. So:

- **Batch 3 (egress) keeps the full alternating loop** — Opus and `pi` in turn
  until both report clean on the same code state. It is the security payload,
  and it is the batch where a missed case is a live vulnerability rather than a
  regression.
- **Batches 2, 4 and 5 get one review each** from both reviewers, in parallel,
  on the same snapshot. Findings are fixed; a second round happens only if a
  finding is rated high or above.

The rule that a clean pass must be **on the same code state** survives
unchanged — a clean report on code the other reviewer then changed does not
count, in either mode.

1. **Manifest + capabilities.** Parsing, validation, `PluginAPIVersion`, the
   grant-filtered `mah` table, the arch guard. Legacy path unchanged in
   behaviour.
2. **Consent + lifecycle.** `GrantsJSON`, enable/load comparison, re-consent,
   dependencies, boot ordering, the `/v1/plugins/manage` payload.
3. **Egress.** The three layers, the per-policy client cache, the address
   classifier, redirect re-validation.
4. **Filters re-check.** The entity-data reader, the handler chokepoint, the
   `ResourcesMatching` category gap.
5. **Bundled manifests, UI, docs, e2e**, then the full gate run.

## 9. Risks

- **Every existing deployment's plugins lose private-network access** unless
  they declare `allow_private_hosts`. That is the intended fix, and it is a
  behaviour change on upgrade. The six bundled plugins are handled in batch 5;
  a third-party plugin pointed at a LAN service breaks until its manifest gains
  the flag. Stated in the release note, not buried.
- **Grants are a breaking change** for any plugin that gains a manifest and gets
  it wrong: an under-declared capability is a nil call deep in `init()`. The
  per-module load log line is the mitigation, and manifest validation catches
  the typo case.
- **`discoverPlugin` runs top-level Lua with no deadline** at boot
  (`manager.go:221-264`). Pre-existing, and the manifest makes that top-level
  code slightly more load-bearing. Not fixed here; noted so the next reader
  does not assume it was considered and dismissed.
- **`db:write`-only plugins that fetch URLs break on upgrade.** Closing the
  fourth door (§4) means `create_resource_from_url` and
  `add_resource_version_from_url` need `http` and a matching `network` entry.
  Release note, same as the private-host break.

## 9a. Residuals carried out of batch 1

Recorded rather than fixed, so they are not rediscovered as new:

1. **A disable cannot abort an operation already inside the DB layer.** Every
   `mah.db`, `mah.kv` and `mah.http` accessor refuses a revoked VM, so no *new*
   operation starts after `DisablePlugin` returns. One already blocked inside
   the backend when revocation landed runs to completion. Bounding it means a
   cancellable context threaded through the host-function surface — a larger
   change than this package, and one with its own correctness questions about
   half-applied writes.
2. **`pm.actionInFlight` is keyed by plugin name, not by generation.** A
   teardown racing a re-enable can drain the wrong generation's WaitGroup. No
   safety consequence found — the registrations themselves are state-scoped, so
   nothing is served from the dead VM either way — but the accompanying comment
   claims more than the key delivers, and the comment should be corrected when
   the key is.
3. **`parseSettingsFromLua` is production-dead.** `discoverPlugin` does not
   route through it. Either delete it or route discovery through it; leaving a
   second settings parser that nothing calls invites a future fix to be applied
   to the wrong one.

## 10. Gates

`go build ./...`, `go vet`, `go test --tags 'json1 fts5' ./...`,
`npm run build`, `cd e2e && npm run test:with-server:all` (browser + CLI),
`go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...`,
`./mr docs lint`. `./mahresources` rebuilt before every e2e run.
