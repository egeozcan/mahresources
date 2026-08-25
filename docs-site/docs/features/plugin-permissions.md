---
sidebar_position: 14
title: Plugin Permissions
---

# Plugin Permissions

A plugin declares what it needs, an operator consents to it once, and the host installs exactly that much. This page covers the manifest, the capability list, the consent record, and the network rules.

## The manifest

The manifest lives on the `plugin` global that already carries `name`, `version` and `description`:

```lua
plugin = {
    name = "fal-ai",
    version = "1.1.0",
    description = "Image generation",

    api_version = 1,
    capabilities = { "db:read", "db:write", "http", "image", "actions", "jobs", "pages" },
    network = { "queue.fal.run", "*.fal.run" },
    allow_private_hosts = false,
    dependencies = { "other-plugin" },
    min_app_version = "1.2.0",
}
```

**A manifest exists if and only if `api_version` is present.** That is the single discriminator. `capabilities = {}` alongside an `api_version` is a legal manifest declaring nothing; it is not legacy mode. Declaring any other manifest field without `api_version` is an error, not a silent grant of everything.

| Field | Required | Meaning |
|---|---|---|
| `api_version` | Yes, for a manifest to exist | Compared against the host's `PluginAPIVersion`. A higher value is refused at load. |
| `capabilities` | No | The `mah` modules to install. Unknown names are an error. |
| `network` | No | Host allowlist for outbound requests. **Absent means any public host.** |
| `allow_private_hosts` | No | Permission to reach private addresses named in `network`. Defaults to false. |
| `dependencies` | No | Plugin names that must be enabled first. |
| `min_app_version` | No | **Never enforced.** Parsed, stored and displayed only. |

`min_app_version` is recorded rather than checked because there is no application version constant to compare against. Rather than enforce against a number nobody maintains, the field is documented as inert.

The `plugin` table must not have a metatable. The manifest is read field by field with raw access, so an inherited field would read as absent while the file appears to declare it, and an `__index` function could answer the parser and the reader differently.

**The declaration must be the same every time it runs.** `plugin.lua` is executed twice — once at discovery, once at load — and the two manifests must be identical or the load is refused. Otherwise a plugin could read narrow when it is inspected and wide when it is installed, and neither the metatable rule nor anything else would catch it.

**A misspelled field is an error, not a shrug.** `apiVersion`, `API_VERSION`, `netowrk`, or any key within one edit of a real field is refused, as is any key containing non-ASCII characters. Without that, a typo means "no manifest" — which means the full surface — and the plugin author sees nothing wrong.

Some values are refused at parse time for being broader than they look: `network = {}` (omit the field entirely if you mean "any public host"), a CIDR with host bits set (`10.0.0.5/8` must be written `10.0.0.0/8`), a default route, a hostname that is really an address (`0x7f000001`, `2130706433`, or any name whose last label is all digits), and a plugin depending on itself.

## Capabilities

An ungranted module is **absent**, not stubbed, so `if mah.kv then` works and a plugin can degrade. Each withheld module is named in one log line at load, so the resulting `attempt to index a nil value` is diagnosable.

| Capability | Grants |
|---|---|
| `db:read` | the `mah.db` readers, `mrql_query`, `get_resource_data` |
| `db:write` | the `mah.db` writers, including creation and association |
| `http` | `mah.http` |
| `kv` | `mah.kv` |
| `image` | `mah.image` |
| `hooks` | `mah.on` |
| `inject` | `mah.inject` |
| `render` | `mah.shortcode`, `mah.block_type`, `mah.display_type` |
| `pages` | `mah.page`, `mah.menu` |
| `api` | `mah.api` |
| `actions` | `mah.action`, and the `job_*` reporters |
| `jobs` | `mah.start_job`, and the `job_*` reporters |
| `schedule` | `mah.schedule` |
| `job_events` | `mah.on` for the `after_job_*` events |

Always installed, no capability required: `mah.json`, `mah.util`, `mah.log`, `mah.html_escape`, `mah.sleep`, `mah.abort`, `mah.doc`, `mah.get_setting`. None reads or writes anything outside the plugin itself.

**`db:write` implies `db:read`.** The writers return the entity they wrote — `patch_note(id, {})` changes nothing and hands back the whole note — so a write-only grant was already a read of anything by id. The implication is made explicit so the label an operator consents to is what they actually grant. The reverse never holds.

**`job_events` is separate from `hooks`** for the same reason `schedule` is separate from `jobs`. An entity hook fires on a write the caller just made, so a plugin holding `hooks` observes what its own users are doing. A job event fires when *any* background job in the deployment finishes, whoever submitted it, including work the plugin had nothing to do with. That is unattended observation of other people's activity, so it gets its own name and `CompareGrants` reports the widening. A plugin holding only `hooks` that registers `mah.on("after_job_completed", ...)` is refused at load, and the error names the capability it needs rather than reporting an unknown event.

**`schedule` is separate from `jobs`** for the same kind of reason. `jobs` runs work a user just asked for; `schedule` runs work nobody asked for, on a timer, including while nobody is logged in. Folding it into `jobs` would have silently widened every plugin an operator had already consented to. Because it is its own name, a plugin adding `mah.schedule` refuses to load until the operator re-enables it and sees the new line.

**`inject` is separate from `render`** because it is not author-invoked. Six injection slots live in the base layout, so an injection runs on every page, while a shortcode or block runs only where a template author placed it. Consenting to "may render blocks" is not consenting to "emits HTML and `<script>` on every page".

### `inject`, `render` and `pages` are browser-side code execution

The HTML a plugin emits is rendered unescaped, in this application's own origin, in the session of whoever is looking at the page. That is deliberate — it is what makes these capabilities useful — but it has a consequence worth stating plainly:

**A plugin holding `inject`, `render` or `pages` can do anything the viewing user can do.** Its JavaScript runs with their session, reads the CSRF token from the page, and can call any endpoint they are allowed to call. Granting one of these to a plugin you do not trust is equivalent to granting it that user's authority, whatever its other capabilities say.

The network rules do not constrain this and are not intended to: they govern requests the *server* makes on the plugin's behalf, not requests the *browser* makes because the plugin asked it to. Withholding the caller's credentials from the plugin's Lua closes the server-side route; it does not change what the plugin's own JavaScript can do in a browser that has already authenticated.

Treat these three as the high-trust capabilities. `db:read` and `http` are narrower grants than any of them.

**Two writers need `http` as well as `db:write`.** `mah.db.create_resource_from_url` and `mah.db.add_resource_version_from_url` do not use `mah.http` — they hand a URL to the application's own downloader. Gated on `db:write` alone they would be a server-side fetch primitive the `network` list could not describe. They require both capabilities, and their URL goes through the same network rules as `mah.http`.

### Legacy plugins

A plugin with no `api_version` is **legacy**: it gets the full `mah` surface, a warning at load, and a badge in the management UI.

Legacy is **not** an exemption from the network rules. The dial-time deny of private addresses applies to every plugin, manifest or not — exempting the plugins that predate manifests would exempt exactly the population that has the problem. A legacy plugin that genuinely needs a LAN host must gain a manifest with `allow_private_hosts`.

**Legacy mode is intended to be removed when the first `api_version` bump lands.** Nothing enforces or schedules that today — there is only one API version, so a lower-version branch cannot be tested honestly. The intention is written down here, and at the point the loophole was opened, so the first bump has to decide deliberately rather than inherit it.

## Consent

The manifest alone cannot be the grant. If the live `plugin.lua` were the source of truth, adding `db:write` and restarting would grant it.

- **Enabling is the consent gesture.** The declared set is recorded against the plugin before it loads.
- **Loading compares** the recorded set against what the plugin declares at load time. Declaring the same or less loads. Declaring more refuses with a message naming the difference. Re-enabling the plugin from the management page consents to the new set; there is no separate re-consent control, because enabling is already the gesture.
- **Narrowing is not a re-consent event.** A plugin that drops a capability loads with the smaller set. The recorded set is a ceiling, never a floor — what is installed is always what is currently declared.

`⊆` is not the same relation on every field:

| Field | Needs re-consent | Loads quietly |
|---|---|---|
| `capabilities` | a capability appears | fewer capabilities |
| `network` | a host is added, or **the list is dropped** (widening to any public host) | a shorter list |
| `allow_private_hosts` | `false` → `true` | `true` → `false` |
| `api_version` | raised | unchanged or lower |
| legacy | manifest → legacy | legacy → manifest |

**`network` inverts**: an absent list means *any public host*, the broadest policy, so dropping the list is a widening even though the list got shorter. **Legacy is the maximum grant**, so gaining a manifest is a narrowing and loads quietly — except `allow_private_hosts`, which legacy never had.

### Upgrading

Plugins enabled before this release have no consent on record. On the first load after upgrade each one is **grandfathered**: whatever it currently declares is recorded, and it loads. This happens once per plugin; every later widening prompts normally.

One consequence worth knowing when a bundled plugin's manifest changes in a release: a deployment that already has consent on record does **not** silently pick up the new declaration. If an upgrade widens what a plugin needs — a new host, say — it stays refused until an operator re-enables it. That is the mechanism working as designed, but it means such a fix does not fully land on upgrade without that step.

That is sound rather than merely convenient — before this release those plugins ran with the entire `mah` surface and no network rules, so recording their declared set is strictly a reduction. A consent record that exists but cannot be read is *not* grandfathered: that is corruption or tampering, and it refuses.

## Who may reach a plugin

Capabilities answer what a plugin may do. This answers who may ask it to.

With authentication enabled, a **group-limited** account — a user confined to one
group's subtree, or any guest — is refused every plugin surface by default: the
plugin's pages, its JSON endpoints, its shortcodes, the slots it injects into and
the blocks and metadata displays it renders. Unscoped roles (admin, editor, and a
user with no scope group) are unaffected, and with authentication off every
request is an implicit administrator, so none of this is visible.

An operator opens a plugin to those accounts one plugin at a time, with **Allow
limited users** on `/plugins/manage` or `mr plugin scoped-access <name>
--allowed=true` from the command line. It is off for every plugin until then,
including plugins that were installed before the control existed.

What it does *not* do is widen the plugin. A confined caller's `mah.db` calls stay
bound to that caller's own subtree, and taxonomy operations still require that
caller's own role, exactly as they do everywhere else. The toggle is about the
door, not about the room.

Two consequences worth knowing:

- **A refused shortcode renders as `<!-- mr:plugin unavailable in this context -->`**,
  which is what a page renders when there is no plugin renderer at all. It is
  deliberately indistinguishable: otherwise a page would report which plugins
  exist, and which ones a given account may not use.
- **Actions are covered.** A group-limited account running a plugin action from
  a card or the bulk bar gets `403` unless that plugin is open to it. This is
  the one place the setting *narrows* what such an account could do before, and
  it is deliberate: an action is the most direct way to make a plugin's Lua run.
  A **guest** is refused whatever this setting says, because running an action is
  a write and a guest is read-only. Opening a plugin to guests gives them its
  pages, shortcodes and slots, never its actions.
  The buttons follow the same rule, so an action that would be refused is not
  drawn: the detail sidebar, the card menu and the bulk bar each list only the
  actions that account could run, and `GET /v1/plugin/actions` answers the same
  way. That is tidiness rather than protection. Drawing a button runs no plugin
  code, and the `403` is what actually holds.
- **Hooks are not covered by this.** They fire from ordinary writes a confined
  user is entitled to make, not from a plugin URL, so a plugin's hooks run for
  every account whatever this setting says. That is why the protection that
  matters is the binding of `mah.db` to the acting principal rather than the
  door.

## Network rules

Three layers apply to every plugin-initiated request, whether through `mah.http` or through the two URL-fetching writers.

1. **The allowlist**, checked before the request. The request host must match an entry in `network`: an exact hostname, a `*.suffix` wildcard, an IP literal, or a CIDR block. No list means any public host.
2. **Redirect re-validation**, on every hop. Without it, a permitted public host is a doorway: it can redirect to an internal one and be followed.
3. **A dial-time deny**, applied to the **resolved** address. Layers 1 and 2 see a hostname, and a hostname that resolves to `127.0.0.1` satisfies both; this one runs per candidate address after resolution. It refuses the unspecified address, loopback, link-local (unicast and multicast), interface-local multicast, private (RFC1918 and IPv6 unique-local), carrier-grade NAT (`100.64.0.0/10`), multicast, the IPv4 broadcast address, and seven ranges no `net.IP` predicate covers: `0.0.0.0/8`, `198.18.0.0/15`, `240.0.0.0/4`, NAT64's `64:ff9b::/96` and `64:ff9b:1::/48`, deprecated site-local `fec0::/10`, and `168.63.129.16/32` — Azure's platform-agent endpoint, which is host-internal on every Azure VM despite being numbered out of public address space.

Ports are deliberately not part of a rule. A host is either reachable or it is not, and a port-level allowlist invites the belief that it confines a plugin more than it does.

**One client per policy.** Connection pools are keyed by scheme and host, not by our rules, so two plugins share an HTTP client only when their declared policies are identical. Otherwise one plugin could reuse a connection another opened — and a reused connection is never dialled, so the dial-time deny would never run for it.

### Reaching private addresses

`allow_private_hosts = true` relaxes layer 3 — but only for an address matching an **address** rule (an IP literal or a CIDR) in that plugin's own `network` list.

A name rule never lifts it. A public name resolves to whatever the plugin author's DNS says, so `network = { "looks-fine.example" }` with an A record pointing at `169.254.169.254` would otherwise satisfy every layer and reach the cloud metadata endpoint. Names stay declarable for reachability; **reaching a private address means naming the address**.

```lua
-- Works: the address is named, and private access is consented.
network = { "192.168.1.50", "10.0.0.0/8" },
allow_private_hosts = true,

-- Refused at the dial: a name cannot lift the deny, however it resolves.
network = { "nas.local" },
allow_private_hosts = true,
```

A rule combined with `allow_private_hosts` must also be specific: wildcards are refused outright (wildcard-DNS services resolve anything to anything), and a CIDR broader than `/8` is refused for IPv4 — `/32` for native IPv6, so `fd00::/8` is refused where `10.0.0.0/8` is accepted. `/8` itself is allowed; the test is "broader than". An IPv4-mapped IPv6 block is measured as IPv4. A default route (`/0`) is refused earlier and for a different reason: at parse time, as a rule that says what omitting the field already says.

### What a plugin is told

A refused request never reports the address a host resolved to. Otherwise the refusal is an oracle: a plugin granted nothing but `http` could loop over a wordlist of internal names and read the private network out of the error messages, without ever being permitted to connect.

An allowlist refusal names the host that was asked for, which the plugin already knows. A dial-time refusal names neither the host nor the address — the check that produced it knows only the address, and that is the thing to withhold.

The full detail — host, resolved address and reason — is written to the application log, so a host a plugin legitimately needs is diagnosable from the operator's side.

### Proxies

Plugin egress does **not** use `HTTP_PROXY`/`HTTPS_PROXY`. Through a proxy the dialer connects to the proxy, so layer 3 would inspect the proxy's address and never see the request's real destination — every request would pass, including one aimed at the metadata endpoint. A deployment that requires a proxy for outbound traffic will find plugin HTTP blocked at the firewall rather than silently unpoliced.

### What this does not cover

A plugin never receives its caller's credentials. The `Authorization` header, session cookie, `X-CSRF-Token` and `Proxy-Authorization` are withheld from `mah.api` handlers and plugin pages; so is `csrf_token` as a query parameter or form field, since this application accepts the token in all three spellings. Without that, a plugin could call this server's own API as its caller and have the *application* fetch a URL of the plugin's choosing.

Credential-bearing entity fields are withheld too. The entity handed to a shortcode is built by walking every exported field, so a credential added to a model would be exposed by default; `ShareToken`, `TokenHash`, `CsrfToken` and `PasswordHash` are withheld by name. A note's share token is a bearer credential granting anonymous read of that note, and `render` — the least privileged capability that sees an entity at all — would otherwise be handed one.

The application's own remote-fetch endpoints (`POST /v1/resource/remote`, the download queue) apply the same dial-time deny, governed by `-allow-private-fetch` rather than by any plugin's manifest. What they do not apply is an allowlist: a plugin's `network` list bounds only that plugin's own requests. They are operator paths, reachable by any authenticated user with the rights to use them, and which public hosts they may reach belongs with the application's download surface rather than with the plugin system.

## Dependencies

A dependency means **the named plugin must be enabled**, which is the only honest semantic without a Lua module loader.

- Enabling refuses when a dependency is not enabled, naming it.
- Disabling refuses when an enabled plugin depends on it. It refuses rather than cascades: disabling one plugin must not silently disable another.
- A dependency on a plugin that is not installed refuses at enable.
- **Cycles are rejected at discovery** and the members are dropped, because a dependency is satisfied only by an enabled plugin and no member of a cycle can ever be enabled. A plugin that merely *depends* on a cycle member survives and is refused by name, which is a better diagnosis than vanishing from the list.

At startup, plugins are enabled in repeated passes rather than in name order, so a plugin whose dependency sorts later still loads. Whatever is left after a pass makes no progress is reported in one log entry naming what each plugin is waiting for.
