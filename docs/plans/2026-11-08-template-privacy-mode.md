# Template generation Privacy mode

Status: planned 2026-11-08. Grilled the same day, decisions Q1 to Q19. Terms are in
`GLOSSARY.md` (Privacy mode, Entity value, Placeholder). The decision record is
`docs/adr/0010-privacy-mode-masks-entity-values-at-the-source.md`. Builds on
`docs/architecture/template-generation-runs.md` and ADR 0009.

## Goal and success criterion

With Privacy mode on, no Entity value read from the database leaves the server in
any request to an AI provider, in harness or classic mode. It is on by default for
every run. **Done** means all of the following:

1. An API test seeds entities with two kinds of fixture text.
   - **What it seeds.**
     - Protected markers in every Entity value: names, descriptions, meta values,
       tags, owners, relation targets, note blocks, hash, location, URL, share
       token, partial descriptions, revision labels, and a ContentType carrying
       parameters.
     - Separate readable markers in vocabulary: category names, meta keys, template
       source, partial names and the user's request.
   - **What it drives.** A scripted harness run through every Tool, using the real
     wire adapters behind a capturing `http.RoundTripper`.
     - `preview` with `[property]` reads of Hash, Location, OriginalName, URL and
       ShareToken.
     - `[meta]`, `[each]` and `[conditional]`.
     - An `[mrql]` preview with a nested CustomMRQLResult and an aggregate bucket.
     - `[mrql saved=…]`.
     - Full-detail `get_revision`.
   - **What it asserts.**
     - No protected marker appears in any body, in raw or JSON-escaped form.
     - Every readable marker does.
     - Every scripted Tool call succeeded rather than hiding a failure behind a
       fixed error.
     - Placeholders appear, and the legend maps each one back to its marker.
   - **A determinism test.** Two fresh maskers, given the same data loaded in
     shuffled association order, build the same Seed bytes.
2. The same holds for a classic-mode request, including the repair call.
3. A leak injected below the masking layer stops the run before that request is
   sent.
   - **The leak.** The injected value contains a quote, a newline and non-ASCII
     text, and it reaches the request inside a JSON Tool result.
   - **Harness mode.** The stop reason is `privacy_guard`, and the Transcript marks
     that Turn's records `sent:false`.
   - **Classic mode.** The leak is caught in the first call, in the repair call, and
     in one fan-out section while a sibling section succeeds. Each case gives
     HTTP 500 with no Draft.
   - **No false positive.** A readable Replay leaf and a model identifier that equal
     a masked value do not trip the guard.
4. `ai_template_privacy_mode=always` refuses a request that asks for Privacy mode
   off, dry run included. Privacy mode plus `previewPlugins` is refused.
5. In the editor's Disclosure the Privacy mode box is ticked by default, the Seed
   shows Placeholders, unticking re-runs the dry run and shows real values, and the
   locked state disables the box. All of this is checked by an E2E test against a
   real server, with no provider involved.
6. Every suite is green: Go unit tests on SQLite, the Postgres Go tests
   (`./mrql/... ./server/api_tests/...`), browser E2E, CLI E2E and a11y. Privacy
   mode off produces byte-identical Seeds, which the existing Seed and sample tests
   prove.

## Settled decisions (grilling)

Q1. Entity values are masked. Field names, MetaSchema, taxonomy names, template
source and the user's request stay readable. Q2 and Q16. Privacy mode is a per-run
checkbox in the Disclosure. The admin setting `ai_template_privacy_mode` is
`per-run` (the default) or `always`. Q15. The checkbox is ticked by default on every
run, and nothing is remembered between runs. Q3. Values are masked at the source,
and a guard on the transport fails the run closed. Q4. A Placeholder is a
shape-hinted token, and equal values share one. Numbers, booleans and IDs pass
through. Q5. The whole loaded entity graph is masked. Q6. `[mrql]` previews stay
available with their results masked. Plugin previews are refused under Privacy
mode. Q7. Dates are masked. Q8 and Q17. A Draft that contains a Placeholder the run
issued is refused, and nothing is ever mapped back. Q9. The Transcript shows masked
text, and a browser-only legend gives the real values. Q10. Classic mode gets the
same masking. Q11. MRQL generation is out of scope. Q12. The guard matches values
of 4 or more characters, case-insensitively, as substrings. A value that also
appears in readable text is exempt. Q13. A meta value equal to an enum or const
value of its own field stays readable. Q14. The Disclosure shows the masked Seed,
and the hash covers the flag. Q18. The eval gets a privacy flag, and no live report
is required. Q19. ADR 0010.

## Design decisions made while planning

These follow from the grilling, but the grilling did not spell them out. They are
listed so a reviewer can contest them.

- **P1. Request field `privacyOff`.** A boolean that defaults to false, so a client
  that omits it gets Privacy mode. A negative name is the only way a zero value can
  mean "on".
- **P2. Dates in a rendered preview are a stand-in instant.** A `time.Time` field
  cannot hold a Placeholder string. In the masked entity copy every `time.Time`
  becomes `2001-02-03T04:05:06Z`, and the Seed's privacy note says so. In JSON
  projections (sample `StartDate`/`EndDate`) and in date-shaped meta strings, dates
  are `⟦tag·date N⟧` Placeholders, as Q7 decided.
- **P3. The Placeholder format.** It is `⟦<tag>·<kind> <n> · <lines> · ~<len> chars⟧`,
  for example `⟦k7·text 3 · 1 line · ~50 chars⟧`.
  - `kind` is one of `text`, `url`, `email` or `date`. A date drops the lines and
    length parts: `⟦k7·date 2⟧`.
  - `lines` is `1 line` or `multi-line`.
  - `len` is rounded up to one of 10, 20, 50, 100, 200, 500 or 1000. Longer values
    show `1000+`.
  - `n` counts from 1 in order of first masking.
  - The empty string stays empty.
  - The brackets are U+27E6 and U+27E7. Markdown, HTML escaping and Go's JSON
    encoder all leave them alone.
- **P4. The tag is deterministic.** The dry run and the real request each build
  their own Seed, and the hash covers the Seed text, so the Placeholders must come
  out identical. The tag is two base32 characters of
  `HMAC-SHA256(process key, accountKey|entityType|carrierID)`, where the key is
  random per process. A restart between the dry run and Send gives a 409, which the
  UI already handles by re-running the dry run. Masking walks map keys in sorted
  order, so numbering is deterministic too.
- **P5. The vocabulary that stays readable when it sits inside an entity.**
  - The names, `Custom*` fields, `MetaSchema` and `SectionConfig` of Category,
    ResourceCategory and NoteType.
  - The name of a GroupRelationType.
  - `Resource.ContentType`, restricted to its structure. The model needs it to
    choose between image and video markup, but a stored value can come straight
    from an import (`groupio/apply_import.go`). So it is kept only as lower-case
    `type/subtype` matching `^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`,
    with any parameters dropped. Anything else is masked.
  - A note block's `Type`.
  - Meta keys and JSON keys.
  - Partial names. A partial's description is an Entity value and is masked, in
    the Seed's partials block and in `get_partial` alike.

  Every other string is masked, Descriptions of taxonomy types included. Masking a
  field by mistake only lowers quality. Keeping one by mistake would be a leak, so
  the list is an allow-list.
- **P6. URL fields.** A `*types.URL` becomes `https://placeholder.invalid/<Placeholder>`.
  It is masked as one value rather than field by field.
- **P7. The guard's readable corpus.**
  - **What goes in.** Each run keeps a corpus of text it knows is readable:
    - every string passed through `Keep`;
    - the system prompt;
    - the Seed sections that carry no entity data;
    - the results of `get_template`, `get_revision` (its slot content only),
      `diff_revisions`, `get_partial` (name and source only),
      `get_shortcode_docs`, `lint`, `check_mrql` and `submit`;
    - the model's own output. That is assistant text, plus every decoded string
      leaf of the call arguments and of Replay. The live Replay bytes are never
      changed.
    - the run's fixed instructions;
    - the trusted request skeleton: the model identifier from the profile, every
      string in the Tool catalogue (names, descriptions, parameter schemas), and
      each wire adapter's fixed protocol strings (`roleUser`-style constants,
      `type` values, `tool_choice` values). Each adapter declares that skeleton
      list next to the code that writes it. A test builds a request for each adapter
      with empty message content and asserts that every string leaf it sends is in
      the corpus. The payload is never registered wholesale.
  - **How the guard checks.** A masked value found in an outgoing body counts as a
    leak only if it is not also in the corpus.
  - **What a miss costs.** A source of readable text that nobody registered can
    only cause a false positive, so the run fails closed, never open.
  - **Forms of a value.** Tool results are JSON that `MarshalToolJSON` encodes and
    that then sits inside a message string. So the guard looks for each masked value
    in three spellings in every string leaf: raw, JSON-string-escaped, and
    JSON-escaped with HTML escaping (`<` etc.). Corpus entries are stored in the
    same three spellings. All matching is on `strings.ToLower` forms.
  - **The algorithm.** One Aho-Corasick automaton over the lower-cased spellings of
    every masked value of 4 or more runes. It is rebuilt only when a value has been
    added since the last check, and it scans each string leaf once. Each candidate
    hit is then checked against the corpus. The automaton is written in the package
    (about 150 lines, tested), not added as a dependency. A benchmark with 20,000
    values against a 2 MB payload must stay under 50 ms.
- **P8. Where the guard runs.** In `generationProviderClient.postPayload`, reached
  through the request context (`WithTemplatePrivacy`).
  - **What it checks.** It decodes the payload and walks every string leaf.
  - **What it reports.** `ErrTemplatePrivacyLeak` names the JSON path of the leaf
    (for example `messages[7].content`), never the value.
  - **Keeping its identity.** Every wrap between the transport and the handler uses
    `%w`, so `errors.Is(err, ErrTemplatePrivacyLeak)` holds at the top. This
    includes the conversion in `defaultTemplateGenerator.callProvider`, which today
    uses `%v`.
  - **Classic mode.** The error is fatal and is never absorbed:
    - The repair fallback (which returns the original Draft when repair fails)
      re-raises it.
    - Fan-out cancels its sibling sections and returns no Draft.
  - **How a harness run ends.** It stops with the new stop reason `privacy_guard`,
    returns no Draft and returns the transcript as it stood. The request that
    tripped the guard is not sent.
  - **Transcript bookkeeping.** `admit` commits a Turn's tentative records (and its
    stubs) before `callProvider`. On a guard failure those records go back to
    `sent:false`, and a stub first written in that Turn is reverted, so the
    Transcript never claims a blocked request was sent.
  - **Status codes.** `classifyTemplateGenerationError` maps the error to HTTP 500
    with the fixed text "Privacy mode stopped a request that contained an entity
    value. Nothing was sent". On the event stream it is an `error` event with
    status 500.
- **P9. Accessibility of the legend.** Hover-only reveal fails keyboard and
  touch users, and making every Placeholder focusable would add hundreds of Tab
  stops to a dialog whose focus trap enumerates them on each Tab
  (`src/utils/containFocus.js`).
  - **Placeholder markup.** Each Placeholder is a non-focusable `<span>` carrying a
    `title`, which serves mouse users.
  - **The toggle.** The accessible path for everyone is one "Show real values"
    toggle button (`aria-pressed`) per surface, one in the Disclosure and one in the
    Transcript. It swaps every Placeholder for `Placeholder = value` inline.
  - **Locked and plugin explanations.** The text for the locked box and for the
    disabled plugin box is associated through `aria-describedby`.
- **P10. Where the legend appears, and where it doesn't.**
  - **Where.** In the dry-run response (`TemplateDisclosure.Legend`, never hashed),
    in `TemplateGenerationResult.Legend`, and in the error payload alongside the
    transcript.
  - **Where not.** It is dropped exactly when the transcript is dropped on an access
    change. It is never stored, logged or sent to a provider.
- **P11. A Placeholder in a Draft.** It is an error issue in
  `ValidateTemplateRunContent`'s caller: `submit`, the final Draft, and the classic
  validator that feeds repair. The message is
  `contains the Placeholder ⟦…⟧; reference the field with [meta]/[property] instead`.
- **P12. Enum and const exemption.** Meta is walked alongside the schema's
  `properties` and `items`, the branches of `allOf`, `anyOf` and `oneOf`, and local
  `#/$defs` or `#/definitions` references, one level deep. A value matching an
  `enum` or `const` of the schema node at its own path stays readable. Any path the
  walk cannot resolve is masked.

  Which schema applies:
  - Seed samples use the request's MetaSchema, which is the editor's value.
  - Tool reads use the entity's own category's saved schema.
  - `[mrql]` items use `QueryResultItem.MetaSchema`.
- **P13. Concurrency.** Tools of a Turn run in parallel, so the masker is protected
  by a mutex. Numbering order inside a Turn may then vary between runs. Only the
  Seed needs to be deterministic.
- **P14. Idempotence.** `Text` on an exact Placeholder this masker issued returns it
  unchanged and records nothing. Text that merely looks like a Placeholder is
  ordinary text, so the rule never weakens masking. This lets a masked entity pass
  through `BuildMetaContextForEntity`, whose presentation names derive from the
  entity graph, and then through `MetaContext` without double-masking.
- **P15. MRQL in previews under Privacy mode.**
  - **Saved queries are refused.** `[mrql saved=…]` (and the `saved` forms of
    `[using]`, `[conditional]` and `[each]`) renders the fixed text "saved queries
    are not available in Privacy mode". A saved query's text and its scope can
    carry stored values, and its errors echo them
    (`template_filters/shortcode_query_executor.go`, `mrql/scope.go`).
  - **Inline-query errors become fixed text.** "the query failed; details are
    hidden in Privacy mode; use check_mrql to check its syntax". The exception is
    `ErrQueryUnavailable`, which keeps its inert rendering.
  - **Results are masked field by field** (`privacy.QueryResult`):
    - flat and bucketed `Items`: `Entity` through `Entity`, `Meta` through `Meta`
      with that item's `MetaSchema`, the `Scope`/`Parent`/`RootGroupName`
      fields through `Text`, the `*CategoryName` fields through `Keep`, and
      `CustomMRQLResult`/`CustomCSS` through `Keep`;
    - aggregate `Rows`: keys kept, strings through `Text`, `time.Time` values
      through `Date`, numbers passed through;
    - bucket `Key` values: the same rules as rows;
    - `EffectiveQuery`: the model's own inline text, kept.
- **P16. Bounds.**
  - **The masked copy.** It sets every `[]byte` field that is not `types.JSON` to
    nil (preview thumbnails, for example). A render never needs them, and copying
    them is wasted work.
  - **The masker's limits.** At most 20,000 distinct masked values, and at most
    4 MiB of real value bytes in the legend. Past either limit, the next `Text`
    returns an error sentinel. The calling Tool turns that into an error result
    ("too much entity data for Privacy mode"), and the Seed builder turns it into
    a 400 at the dry run. Masking is never skipped.
  - **Cancellation.** The deep copy checks the context every 1,000 nodes.

## Architecture

### `application_context/template_privacy.go` (new)

`TemplatePrivacy` is one per request, and nil means off.

```go
func NewTemplatePrivacy(tag string) *TemplatePrivacy
func TemplatePrivacyTag(accountKey, entityType string, carrierID uint) string
func (p *TemplatePrivacy) Text(s string) string          // masks; kind from shape (url/email/date/text)
func (p *TemplatePrivacy) Date(t time.Time) string       // ⟦tag·date N⟧, keyed by the instant
func (p *TemplatePrivacy) Keep(s string) string          // readable; adds to the guard corpus
func (p *TemplatePrivacy) Meta(raw []byte, schema string) []byte   // string leaves masked (P12), keys kept
func (p *TemplatePrivacy) Value(v any, schema any) any   // the same, over a decoded JSON value
func (p *TemplatePrivacy) Entity(e any) any               // deep copy with P2, P5, P6 applied (reflection)
func (p *TemplatePrivacy) MetaContext(c *shortcodes.MetaShortcodeContext) // masks group names, keeps category names
func (p *TemplatePrivacy) QueryResult(r *shortcodes.QueryResult) *shortcodes.QueryResult
func (p *TemplatePrivacy) ContainsIssued(s string) (string, bool)  // P11
func (p *TemplatePrivacy) Legend() []TemplatePrivacyLegendEntry    // [{placeholder, value}] in issue order
func (p *TemplatePrivacy) CheckOutbound(payload []byte) error      // P7, P8
func WithTemplatePrivacy(ctx context.Context, p *TemplatePrivacy) context.Context
func TemplatePrivacyFrom(ctx context.Context) *TemplatePrivacy
```

**Rules for `Entity`.** It reflects over the struct and returns a deep copy, never
touching the original. Every exported settable field is handled by kind, with the
P5 allow-list consulted by struct type name and field name:

- A string is passed through `Text`.
- `time.Time`, `*time.Time` and `gorm.DeletedAt` become the stand-in instant.
- `types.JSON` goes through `Meta`, with the schema from the entity's category
  where one is loaded.
- `*types.URL` gets the P6 form.
- Pointers, slices and structs are recursed. The visited map is keyed by the
  original pointer and returns the masked copy already allocated for it, never the
  original, so aliases stay aliases and cycles end.
- Nullable `*string` fields are masked behind a new pointer, and nil stays nil.
- Numbers and booleans are copied. Other `[]byte` fields get the P16 rule.

**Tests.**

- A test iterates the exported fields of every model reachable from
  Group/Resource/Note preloads. Each string field of a type not on the allow-list
  must come out masked, so a model field added later is covered without anyone
  editing a list.
- Explicit cases cover:
  - JSON string leaves in `Meta`, `OwnMeta`, and note block `Content`/`State`;
  - the URL wrapper;
  - nullable pointers (`GUID`, `StorageLocation`);
  - a repeated alias and a cycle;
  - an original graph that is unchanged after the copy (deep-equal against a copy
    taken before), and numbers that are unchanged.

### Seed and projections

`ProjectTemplateSample` is split into project, then mask (only when privacy is
non-nil), then clip. Clipping runs after masking, so a long value and its clipped
copy share one Placeholder, and with privacy off the output does not change by a
byte. Every caller of the projection passes the request's privacy: Seed samples,
`get_entity`, and `fullSampleMeta`.

The other Seed sections:

- **identity:** the name is passed through `Keep`, the description through `Text`.
- **starting-points:** names go through `Text`.
- **samples:** the "values seen" summary is derived from the masked samples, so it
  is masked automatically.
- **request:** goes through `Keep`.
- **current, siblings, schema, layout and the reference prefix:** each is passed
  through `Keep`. The one exception is in the partials block: each partial's
  description goes through `Text` before the block is formatted (P5).

A new readable section, `privacy`, is added only when privacy is on. It explains
Placeholders, says never to copy one, gives the stand-in date and says to reference
fields instead.

`TemplateGenerationInput` gains `Privacy *TemplatePrivacy`. The classic builder
(`buildTemplatePromptSections`) and the harness builder (`buildTemplateRunSections`)
both read it through the shared helpers.

### Tools (`server/api_handlers/template_run_tools.go`)

The executor config carries the privacy.

| Tool | Under Privacy mode |
| --- | --- |
| `get_entity` | The projection is masked with the entity's own schema. The carrier name goes through `Keep`. |
| `list_related`, `list_members` | Entity names go through `Text`. Carrier names go through `Keep`. |
| `list_carriers` | Names go through `Keep`, descriptions through `Text`. |
| `list_revisions` | Labels go through `Text`. Everything else is IDs, times and enums. createdAt goes through `Date`. |
| `get_revision`, `diff_revisions`, `get_template` | Slot content and source are readable, and only that text is added to the corpus. The revision label goes through `Text`, and `createdAt`, in both the full-detail and the field form, goes through `Date`. |
| `get_partial` | The name and source are readable and are added to the corpus. The description goes through `Text`. |
| `preview` | Load the entity, then build the MetaContext from `privacy.Entity(entity)`, then apply `privacy.MetaContext`, which P14 makes idempotent. The `[mrql]` executor is wrapped as P15 describes. The carrier-mode preview masks a copy of the carrier, so the P5 vocabulary stays and its Description is masked. Plugins are refused earlier, at the request. |
| `lint`, `check_mrql`, `get_shortcode_docs`, `submit` | Readable, and added to the corpus. `submit` adds P11. |

### Run loop (`template_run.go`)

- `runHarness` puts the privacy on the run context, and the guard reads it in
  `postPayload`.
- `handleReply` passes assistant text, call arguments and Replay through `Keep`.
  The final-Turn and nudge instructions are passed through `Keep` once.
- An `ErrTemplatePrivacyLeak` from `callProvider` becomes stop `privacy_guard`,
  with no Draft. Even a run that already has submissions ends in error. Releasing a
  Draft built before the leak would be safe, but it would hide a bug, so the run
  fails loudly.
- `buildResult` and `TemplateRunError` carry `Legend`.

### Request, settings, Disclosure

- **The request field.** `templateGenerateRequest.PrivacyOff` (`privacyOff`). The
  handler resolves `privacyOn = !req.PrivacyOff`, then applies two refusals, both
  400 and both before any grounding read:
  - with the setting at `always` and `privacyOff` set: "Privacy mode is required on
    this server";
  - with `privacyOn` and `previewPlugins` both set: "plugin previews are not
    available in Privacy mode".
- **The masker.** When privacy is on, the handler builds the `TemplatePrivacy`
  before `loadTemplateSamples`.
- **The setting.** `ai_template_privacy_mode`, `per-run` or `always`, lives in
  `addAIHarnessSettingSpecs` with a validator. It is read in the same settings
  snapshot as the mode (`PrepareTemplateGeneration`), so it is exposed on the
  preparation. It applies in both modes.
- **The Disclosure.** `TemplateDisclosure.Privacy *TemplateDisclosedPrivacy{On, Locked bool}`
  is present in both modes and hashed. `Legend` is shown and never hashed.
  `harnessEnvelope` gains a sentence when privacy is on, along the lines of
  "Entity values are replaced by Placeholders before anything is sent; template
  source, field names, the schema and your request are sent as written". The
  classic continuation rule gains the same sentence.

### Browser

The Disclosure is `generationDisclosure.js` and `generationDisclosureDialog.tpl`.
Its Privacy mode checkbox (`generation-optin-privacy`) sits first, ticked:

- **Toggling it** re-runs the dry run, the same as `setOptIn` does today, and the
  request carries `privacyOff`.
- **When the admin lock is on**, the Disclosure reports `privacy.locked`. The box is
  then disabled and ticked, with the text "Required by the administrator".
- **While privacy is on**, the plugin preview checkbox is disabled, with a line
  explaining why. Ticking privacy while plugins are ticked unticks plugins.
- **Placeholders** in the sections and in the run Transcript
  (`templateRun.js`/`templateRunProgress.tpl`) are rendered from text segments
  (`x-text`, never `x-html`) as described in P9.

**Mode independence.** Privacy mode works in both modes, but the existing opt-in
flow is harness-only. Today the redisclosure callbacks and the approved options
are gated on harness mode (`templateGeneration.js` and `templateBundle.js` around
the `harness` checks), and the opt-ins live inside the harness-only subtree of
`generationDisclosureDialog.tpl`. So:

- The Privacy mode box and its redisclosure callback sit outside that subtree.
- `privacyOff` is sent from the approved Disclosure in both modes, by both hosts
  (`templateGeneration.js`, `templateBundle.js`).
- The host checks that the server's echoed `privacy.on` matches the box before it
  enables Send.

**Cleanup of real values in the browser.**

- **The store.** The Disclosure store's payload, its legend and the reveal state
  are cleared when the dialog settles (Send, Cancel, Escape), when a redisclosure
  replaces them, and on an access-denied response, JSON or SSE.
- **The run panel.** The run panel's legend lives with its transcript, and is
  cleared when the transcript is.
- **Stale responses.** Both hosts check the request identity again after decoding
  a response body and before adopting it, so a superseded response's legend is
  never shown.

### Classic mode

- **Masked Seed.** The same input. Privacy feeds the same section helpers, so the
  repair and fan-out calls resend masked sections.
- **The model's output.** The model's previous draft and the repair issue lines are
  the model's own output, so they go to the corpus through `Keep` before the repair
  call.
- **The guard.** It runs in the handler's request context.
- **P11.** It feeds the classic validator, so a Placeholder triggers a repair, and
  a draft that still contains one comes back `valid:false` with the issue.
- **The legend.** It is on the result.

### Eval

`internal/templateeval` gets a privacy option on the runner. It builds a
`TemplatePrivacy` for each case and records whether privacy was on in the report.
Its fixtures cover masked runs. Running the eval live is not required.

## Lanes

Both lanes run in worktrees off `feat/template-privacy-mode`, one after the other.
B starts after A merges, because B's E2E needs A's API.

**Lane A, backend.** In order, red then green:

1. Unit tests for `TemplatePrivacy`:
   - the format, the determinism, and equal values sharing one Placeholder;
   - the kinds, the date and URL forms, and the enum and const exemption;
   - an `Entity` deep copy that leaves the original unchanged, plus the
     reflection-driven field coverage test;
   - the guard: thresholds, case, corpus exemption, JSON paths, and an error that
     never contains the value.

   Then the implementation.
2. The projection split: the existing sample tests stay green and byte-identical,
   and new tests cover privacy on.
3. The Seed (classic and harness), with tests.
4. The Tools and preview, with an API test: the leak test from criterion 1, using a
   capturing `http.RoundTripper` installed through `InstallAIGeneration` and a
   scripted conversation provider.
5. The guard in `postPayload`, the run loop, `privacy_guard`, the legend, and P11
   in submit, the Draft and the classic validator. Tests for criteria 2 and 3.
6. The request field, the setting, the refusals, and the Disclosure and its hash.
   Tests for criterion 4, plus a hash test: approving with privacy on and sending
   with it off gives 409.
7. The eval option.
8. Docs:
   - `docs/architecture/template-generation-runs.md`: a "Privacy mode" section,
     the settings table, the stop reasons table and the opt-ins text;
   - `docs/architecture/ai-generation.md`: the classic mode note;
   - the OpenAPI spec, regenerated for the new request field.

**Lane B, browser, E2E and user docs.**

1. An E2E test for criterion 5, against the real server, dry run only. It also
   checks that the plugin box is disabled under privacy, that the "Show real
   values" toggle shows the seeded name, and the lock set through the settings API.
   It covers both hosts: the per-slot generator and the whole-template bundle
   generator. It also checks classic mode: turning privacy off, re-running the dry
   run and sending, with a routed generate response.
2. The UI changes, then `npm run build-js`.
3. A Transcript legend test. It extends the golden replay fixture
   (`e2e/fixtures/template-run-golden.json`) with a recording that has privacy on
   and a legend, if the recorder can produce one. Otherwise it uses a routed
   response built from the same JSON shape as the API test.
4. An a11y spec, run with realistic content that is heavy with Placeholders
   (200 or more). It covers:
   - axe on the Disclosure and on the expanded Transcript, with the legend toggled
     off and on;
   - the toggle reached and operated by keyboard, with its `aria-pressed` state
     announced;
   - Send and Cancel reachable at 320×256 by searching the focus order, not by a
     fixed Tab count;
   - focus kept across a redisclosure;
   - the locked and plugin-disabled explanations exposed through
     `aria-describedby`.
5. `docs-site/docs/features/custom-templates.md`: a "Privacy mode" subsection
   stating precisely what is and isn't masked, the stand-in date, the lock, and the
   plugin preview restriction.

## Verification

Each lane finishes with its gates green and pi-reviewed until a round has no P0 or
P1. After both are merged, the integration step runs:

- the full Go unit suite (`go test --tags 'json1 fts5' ./...`);
- the Postgres Go tests;
- `cd e2e && npm run test:with-server:all`;
- the Postgres E2E;
- a11y.

It also shows a behaviour diff against master: privacy off gives the same Seed
bytes, which the existing golden and Seed tests pin.

## Out of scope

- MRQL generation (Q11).
- Masking field names (Q1, a possible later opt-in).
- Plugin previews under Privacy mode.
- Protecting literal text a person typed into a template slot, which ADR 0010
  records as a limitation.

## Review

(to be filled after execution)
