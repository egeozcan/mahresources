# Template generation Privacy mode

Status: planned 2026-11-08. Grilled the same day, decisions Q1 to Q19. Terms are in
`GLOSSARY.md` (Privacy mode, Entity value, Placeholder). The decision record is
`docs/adr/0010-privacy-mode-masks-entity-values-at-the-source.md`. Builds on
`docs/architecture/template-generation-runs.md` and ADR 0009.

## Goal and success criterion

With Privacy mode on, no Entity value read from the database leaves the server in
any request to an AI provider, in harness or classic mode. It is on by default for
every run. **Done** means all of the following:

1. An API test seeds entities whose every string field, meta value, tag, owner,
   relation, note block, hash, location and URL carries a distinctive marker. It
   drives a harness run through every Tool, including `preview` with `[property]`
   reads of Hash, Location, OriginalName, URL and ShareToken and an `[mrql]`
   preview. It captures every outgoing HTTP body at the transport. No marker
   appears in any body, and Placeholders do.
2. The same holds for a classic-mode request, including the repair call.
3. A leak injected below the masking layer stops the run with stop reason
   `privacy_guard` before that request is sent.
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
  - `Resource.ContentType`, which the model needs in order to choose between image
    and video markup.
  - A note block's `Type`.
  - Meta keys and JSON keys.

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
    - the results of `get_template`, `get_revision` (its slot content), `diff_revisions`,
      `get_partial`, `get_shortcode_docs`, `lint`, `check_mrql` and `submit`;
    - the model's own output: assistant text, call arguments and Replay;
    - the run's fixed instructions.
  - **How the guard checks.** A masked value found in an outgoing body counts as a
    leak only if it is not also in the corpus.
  - **What a miss costs.** A source of readable text that nobody registered can
    only cause a false positive, so the run fails closed, never open.
- **P8. Where the guard runs.** In `generationProviderClient.postPayload`, reached
  through the request context (`WithTemplatePrivacy`).
  - **What it checks.** It decodes the payload and walks every string leaf.
  - **What it reports.** `ErrTemplatePrivacyLeak` names the JSON path of the leaf
    (for example `messages[7].content`), never the value.
  - **How the run ends.** A harness run stops with the new stop reason
    `privacy_guard` and returns the transcript as it stood. The request that tripped
    the guard is not sent.
  - **Status codes.** HTTP 500 in JSON delivery. On the event stream, an `error`
    event with status 500.
- **P9. Accessibility of the legend.** Hover-only reveal fails keyboard and
  touch users. Each Placeholder in the Disclosure sections and the Transcript is a
  focusable element whose `title` and accessible description give the value. A
  "Show real values" toggle button swaps every Placeholder for
  `Placeholder = value` inline.
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
- Pointers, slices and structs are recursed. A visited map stops cycles.
- Numbers and booleans are copied.

A test iterates the exported fields of every model reachable from
Group/Resource/Note preloads. Each string field of a type not on the allow-list must
come out masked, so a model field added later is covered without anyone editing a
list.

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
  through `Keep`.

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
| `get_revision`, `diff_revisions`, `get_template`, `get_partial` | Readable. Their result text is added to the corpus. Revision labels inside `get_revision` go through `Text`. |
| `preview` | Load the entity, then build the MetaContext from `privacy.Entity(entity)`, then apply `privacy.MetaContext`. The `[mrql]` executor is wrapped so each result passes through `privacy.QueryResult`. The carrier-mode preview masks a copy of the carrier, so the P5 vocabulary stays and its Description is masked. Plugins are refused earlier, at the request. |
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

The request bodies in `templateGeneration.js` and `templateBundle.js` carry
`privacyOff`.

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
2. The UI changes, then `npm run build-js`.
3. A Transcript legend test. It extends the golden replay fixture
   (`e2e/fixtures/template-run-golden.json`) with a recording that has privacy on
   and a legend, if the recorder can produce one. Otherwise it uses a routed
   response built from the same JSON shape as the API test.
4. An a11y spec covering the dialog with privacy on and with the legend toggled.
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
