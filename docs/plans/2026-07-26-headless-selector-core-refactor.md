# Headless Selector Core Refactor Implementation Plan

**Status:** Proposed

**Date:** 2026-07-26

**Scope:** Frontend selector behavior, integrations, profiles, and tests

**Delivery style:** Incremental refactor; every commit leaves the application buildable and the affected behavior working

## Problem Statement

The shared autocompleter module has become the selection engine for tags, groups, categories, notes, resources, relation types, series, and queries. It now mixes selection rules, asynchronous search, entity creation, Alpine reactivity, form serialization, popover positioning, focus behavior, accessibility announcements, and immediate persistence callbacks in one implementation.

The module's effective interface is much larger than its function arguments. Callers also depend on internal Alpine properties, a particular DOM declaration string, mutable selected arrays, callback ordering, custom window events, hidden-input behavior, and endpoint URLs. This makes apparently local changes application-wide and has repeatedly produced regressions involving duplicate creation, stale search results, keyboard handling, focus, form submission, and lightbox synchronization.

The goal is to establish a deep headless selector module: callers choose an explicit profile and provide domain inputs, while selection, search, creation, concurrency, and change semantics remain behind a small interface. Alpine and Pongo remain rendering adapters during this refactor.

## Solution

Build a framework-independent selector core in TypeScript, backed by a selector-source port with HTTP and in-memory adapters. Put application-facing behavior behind explicit profiles for single entity fields, multi-entity fields, creatable tag fields, and immediate tag editors.

Keep the current autocompleter interface as a temporary compatibility adapter. Before delegating it to the core, remove the most dangerous implicit integrations: direct selected-array mutation and MRQL's inspection of Alpine internals. Migrate callers in increasing order of behavioral complexity, then remove the legacy options and state aliases only after no caller depends on them.

The refactor must not redesign the selector UI, change server payloads, replace Alpine, or introduce Lit rendering. Existing behavior remains the compatibility contract unless an intentional correction is called out in this document.

## Target Architecture

### External module interface

Ordinary callers use explicit profile constructors rather than configuring search URLs, creation URLs, maximum counts, callbacks, and standalone flags independently.

The target profile families are:

- **Single entity field:** zero-or-one selection serialized into a form.
- **Multi-entity field:** chip-based multiple selection serialized into a form.
- **Creatable entity field:** form field that may create an entity before selecting it; initially needed by tags and series.
- **Tag field:** tag-specific creatable multi-select with the lean tag suggestion source, tag creation, delimiter behavior, and usage-aware sorting.
- **Tag editor:** immediate tag association editor with optimistic pending/failure presentation and persistence supplied by its owning domain module.
- **Dynamic entity selector:** lower-level escape hatch for runtime-defined sources such as entity-picker filters. It still accepts a profile object rather than the legacy collection of unrelated flags.

Profiles return the same small selector handle to rendering and integration adapters. Application callers do not receive the source adapter or internal state machine directly.

### Headless selector interface

The headless interface consists of:

- A read-only state snapshot.
- A subscription method returning an unsubscribe function.
- A semantic command entry point.
- A destroy method that cancels outstanding work and releases subscribers.

The command vocabulary covers:

- Updating the query.
- Opening and closing the option list.
- Moving the active option forward or backward.
- Committing the active option.
- Committing a typed token.
- Selecting an explicit option.
- Removing a selected option.
- Replacing the complete selection, optionally silently.
- Requesting the legacy create-confirmation state.
- Confirming creation.
- Cancelling create confirmation.

The create-confirmation commands are required because the existing “Add X?” interface is a real interaction state. It must not remain as Alpine-only state alongside a headless selection state.

### Option and key model

The core uses normalized selector options containing:

- A stable key.
- A display label.
- The unconstrained raw domain value.

Keys may be strings or numbers at the interface. Internally, the core canonicalizes keys for maps and sets so a numeric identifier and its serialized form cannot create duplicate selections. The original key remains available on the option.

The core places no structural constraint on the raw value. Entity-specific mapping from server objects to normalized options belongs to the source/profile.

### Selector state

The state snapshot describes externally observable behavior rather than Alpine or DOM details. It includes:

- The current query.
- Available options.
- Selected options.
- Active option index.
- Whether the option list is open.
- Search lifecycle state.
- Entity-creation lifecycle state.
- The current create candidate, if search results for the current query are complete.
- The current create-confirmation candidate, if any.
- A typed error state.

Search and creation use separate lifecycle fields. A single loading boolean cannot accurately represent a completed search with queued creation or other overlapping operations.

Association-persistence pending and failed keys belong to the tag-editor profile layer, not the generic selector core. The core owns search and entity-creation operations; the tag-editor profile owns the separate operation of associating an already selected tag with a resource, note, or group.

### Selection change contract

Every non-silent selection transition produces one atomic change containing:

- The previous selection.
- The current selection.
- Added options.
- Removed options.
- A reason such as select, remove, create, replace, or reset.

For a single selector, replacing A with B is one change containing A in removed and B in added. Compatibility callbacks are derived from this event in a documented order: removals first, additions second, followed by the legacy aggregate event. New profiles consume only the atomic change.

Silent replacement updates state and form serialization but does not invoke domain callbacks, custom selection events, or user-action accessibility announcements.

### Selector source port

The source port owns remote entity lookup and optional entity creation. Search accepts an abort signal. Creation also accepts an abort signal and is absent for non-creatable profiles.

The production HTTP adapter owns:

- Endpoint construction.
- Query-string encoding.
- HTTP status validation.
- JSON response validation.
- Mapping raw server values to normalized options.
- Dynamic request parameters supplied through a callback.

The dynamic-parameter callback is how current relation-field dependencies are preserved. The compatibility Alpine adapter may construct a callback that reads the dependent form controls, but neither the core nor the reusable HTTP adapter directly queries the document.

Debouncing is implemented as a source decorator. The core receives every query immediately, remains the single source of truth for the query and create candidate, and invokes the decorated source. Production profiles use the debounce decorator; deterministic core tests use an immediate in-memory source without fake rendering state.

### Search concurrency contract

On every query transition the core:

1. Cancels the previous search.
2. Advances an internal request generation.
3. Starts a search for the new query through the source.
4. Accepts results only when both the generation and query still match.
5. Treats cancellation as a normal transition, not a user-visible error.
6. Marks the query as completed before evaluating whether creation may be offered.

The create candidate is available only after the search for the current trimmed query has completed and neither available nor selected options have an exact matching label. Initial matching semantics remain behavior-compatible; case-folding or server-side canonicalization changes are out of scope.

### Creation concurrency contract

All creation entry points use one queue: typed-token commit, virtual create-option commit, and confirmed legacy creation. No path may bypass the queue or use a shared loading boolean as a silent mutex.

The queue:

- Clears/consumes the committed query before awaiting the network.
- Deduplicates against selections and earlier queued results.
- Processes entries in order.
- Selects each successfully created option through the normal selection transition.
- Produces a typed error for failure without inserting a phantom option.
- Continues to the next independent queued entry after a failure.
- Rejects new work explicitly after destruction rather than silently dropping it.

### Rendering adapter responsibilities

The Alpine rendering adapter owns:

- Mapping browser input and keyboard events to semantic commands.
- Mirroring normalized state into compatibility property names during migration.
- Input focus and synchronous input clearing.
- Popover show/hide and positioning.
- Scrolling the active option into view.
- ARIA attributes and announcement wording.
- Empty-Enter form submission rules.
- Escape behavior in ordinary forms, inline editors, and the lightbox.
- Display decoration such as appending category names.
- Form validation listener registration and cleanup.

Popover extraction is deliberately deferred. It stays within the Alpine adapter until the selector core and profiles are stable.

### Form adapter responsibilities

The form adapter owns:

- Repeated hidden controls for selected keys.
- The enabled empty hidden control required to clear nullable relationships.
- Minimum-selection validation.
- Form-reset behavior.
- Dispatch of the compatibility aggregate selection event until its consumers migrate.

The empty hidden control is a required invariant, not incidental markup.

### Scoped selector registry

MRQL must stop discovering selectors through the Alpine declaration string and reading internal state. Introduce a scoped registry keyed by the owning form and field name. Scoping by form prevents collisions between repeated field names on the same page and between hidden global markup and visible forms.

The registered integration handle supports:

- Reading raw selected values.
- Replacing raw selected values silently or non-silently.
- Resolving exact labels through the selector's source and replacing the result.
- Preserving already hydrated values when replacing by key.

Registration occurs during Alpine initialization and is removed during destruction. The first adapter registered in this registry is the existing autocompleter, allowing MRQL to migrate before the headless core becomes production-active.

## Decision Document

### Architectural decisions

- The headless selector is implemented in TypeScript and tested in the existing Vitest environment.
- Alpine and Pongo remain the production rendering stack throughout this refactor.
- The current autocompleter remains a compatibility adapter until all callers have migrated.
- The HTTP source and in-memory source are real adapters at an owned remote seam.
- Debounce is a source decorator, not DOM state and not a timer embedded in the core.
- Popover positioning remains in the rendering adapter for this project.
- The selector registry is introduced before core delegation because MRQL is the riskiest implicit caller.
- Selection replacement is atomic and reports both removed and added values.
- Direct mutation of selected arrays is prohibited once core delegation begins.
- Entity creation and entity association are separate operations.
- The tag-editor profile, not the generic core, owns association persistence and its pending/failure presentation.
- The lean tag suggestion endpoint becomes the default search source for tag profiles.
- Dynamic relation filters are represented by source parameter callbacks wired by the adapter.
- Display decoration remains a profile/rendering concern.

### Compatibility decisions

- Existing templates retain their current DOM structure during core introduction.
- Existing hidden input names and empty-value behavior remain unchanged.
- Existing legacy callbacks and custom events are derived from atomic changes until their callers migrate.
- Existing raw selected values remain exposed under the legacy selected-results property during migration.
- Existing reset behavior remains callable through the legacy reset method, implemented as a silent replace where appropriate.
- Comma remains a token delimiter and space remains non-committing by default.
- Empty Enter continues to submit ordinary forms, while inline editors and standalone selectors keep selection behavior.
- Escape continues to stop propagation and blur in the lightbox behavior profile.
- The legacy “Add X?” confirmation interface remains available until product work explicitly removes it.

### Intentional corrections

- Single-selection replacement emits one removal and one addition rather than silently evicting the old option.
- Aborted searches do not populate the visible error state.
- Search responses must be successful and valid before options are applied.
- Form and window listeners are removed during destruction.
- Destroy aborts active search and creation work.
- Every create path uses the same queue and cannot silently drop a token.

## Commits

Each numbered item is intended to be one commit unless implementation reveals that the commit would mix independently reviewable behavior. In that case, split it further; do not combine adjacent commits merely to reduce the commit count.

### Phase 1 — Strengthen the behavior contract

#### Commit 1: Characterize atomic single-selection replacement

- Add browser coverage using an existing maximum-one selector.
- Select one value, then select another without manually removing the first.
- Assert that one hidden selected value remains and that it is the replacement.
- Capture the current callback behavior separately so the later intentional atomic-notification correction is explicit.
- Run the focused test and the existing nullable-field clearing tests.

#### Commit 2: Characterize rapid-query stale-result handling

- Add focused coverage where the first search response is delayed and the second response returns first.
- Assert that only results for the latest visible query are rendered.
- Assert that cancellation does not render an error message.
- Avoid timing-only assertions; control response order through request interception.

#### Commit 3: Characterize create confirmation and current-query gating

- Cover the legacy “Add X?” confirmation entry, confirm, and cancel paths.
- Assert that a create option is not exposed before the search for that exact query has completed.
- Assert that cancelling leaves no selection and returns focus behavior to the existing input path.

#### Commit 4: Characterize dynamic relation-selector parameters

- Add coverage for the relation creation form where one selector's request parameters depend on other form controls.
- Assert that changing the relation type or relation side affects subsequent group searches.
- This test becomes the guard for replacing document-driven filter configuration with a source parameter callback.

### Phase 2 — Remove implicit integration dependencies

#### Commit 5: Add the scoped selector registry and its tests

- Define the minimal integration handle and form-scoped registry.
- Test registration, lookup, duplicate field names in separate forms, replacement registration, and cleanup.
- Test exact-label resolution and silent replacement through an in-memory handle.
- Keep the registry unused in production in this commit.

#### Commit 6: Register the legacy autocompleter in the scoped registry

- Register selectors that have both a field name and owning form.
- Adapt raw selected-value reads, reset behavior, exact-label lookup, and hydrated-value preservation to the registry handle.
- Unregister on destruction.
- Preserve all existing Alpine properties and events.

#### Commit 7: Move MRQL's selector read path to the registry

- Replace direct Alpine state inspection when converting form relations into MRQL names.
- Resolve the selector by the known filter form and field name.
- Keep a fail-closed fallback that marks the form representation incompatible when a required selector cannot be found.
- Extend MRQL tests to cover multiple forms containing fields with the same name.

#### Commit 8: Move MRQL's selector write path to the registry

- Replace URL borrowing, manual fetch construction, raw selected-array inspection, and direct reset calls.
- Use registry exact-label resolution for name-based MRQL values.
- Use registry replacement for identifier-based values, preserving hydrated existing objects when possible.
- Retain the existing incompatible-form behavior when a referenced entity cannot be resolved.
- Run the MRQL list-bar round-trip browser coverage after this commit.

#### Commit 9: Route every selected-chip removal through the legacy method

- Replace direct selected-array splices in shared chip markup with the existing removal method.
- Preserve keyboard focus restoration after Enter and Space removal.
- Confirm that the compatibility remove callback and aggregate event fire exactly once.

#### Commit 10: Replace external selected-array assignment with reset commands

- Update direct callers that clear or replace selected results to use the legacy reset method.
- Distinguish user-driven replacement from silent external synchronization.
- Preserve the lightbox no-announcement behavior when navigating between resources.
- After this commit, selected arrays may be read for compatibility but must not be assigned or spliced outside the adapter.

### Phase 3 — Build the headless module without production delegation

#### Commit 11: Introduce selector types, source port, and in-memory source

- Define normalized options, keys, state, commands, changes, errors, source behavior, and the selector handle.
- Add an in-memory source capable of deterministic success, failure, cancellation, and deferred completion.
- Export only the intended external module interface from the module entry point.
- Do not connect the module to Alpine yet.

#### Commit 12: Implement lifecycle, snapshots, and subscriptions

- Construct initial state from normalized options.
- Return read-only snapshots that callers cannot mutate to bypass commands.
- Implement subscription and unsubscription.
- Implement destruction and reject commands after destruction with a typed result.
- Test subscriber ordering, unsubscribe behavior, and destroy idempotence.

#### Commit 13: Implement selection, removal, and replacement

- Implement canonical key comparison and duplicate prevention.
- Implement multi-selection with optional maximum limits.
- Implement single-selection as atomic replacement.
- Implement silent and non-silent complete replacement.
- Emit one atomic change for each non-silent transition.
- Test no-op selections, missing removals, ordering, maximum eviction, and silent reset.

#### Commit 14: Implement open state and active-option navigation

- Implement explicit open and close commands.
- Implement next/previous wrapping with no options, one option, and multiple options.
- Keep option-list navigation independent of DOM scrolling and announcement wording.
- Test how the virtual create option participates once creation is added later.

#### Commit 15: Implement immediate search orchestration

- Update the query in state synchronously.
- Abort the previous search and advance a request generation.
- Apply only current-generation results for the current query.
- Filter already selected keys from available options.
- Treat abort as normal and expose non-abort failures as typed errors.
- Test a source that ignores abort to prove the generation check independently prevents stale results.

#### Commit 16: Add the debounced-source decorator

- Delay calls to an underlying source without delaying query state in the core.
- Cancel scheduled work when a newer search or abort arrives.
- Propagate the original abort signal and errors.
- Use fake timers only in decorator tests; core tests remain timer-free.

#### Commit 17: Add the production HTTP source adapter

- Encode the query and dynamic parameter callback output.
- Validate non-success status codes before parsing.
- Validate that search returns a list and creation returns one value suitable for mapping.
- Map raw values through the profile-supplied option mapper.
- Test query parameters, dynamic parameter reevaluation, status errors, malformed JSON shapes, and abort propagation with mocked fetch.

#### Commit 18: Implement current-query create candidates and typed-token precedence

- Track the most recently completed query independently from the visible query.
- Expose a create candidate only after current-query search completion.
- Implement typed-token precedence: selected exact match is a consumed no-op; available exact match selects; otherwise a creatable source queues creation; otherwise the token is not consumed.
- Preserve exact-match compatibility semantics.
- Test every precedence branch.

#### Commit 19: Implement create-confirmation state

- Implement request, confirm, and cancel commands.
- Ensure the confirmation candidate is derived from a valid current query or an explicit compatibility request.
- Clear confirmation on successful creation, cancellation, external replacement, and destruction.
- Keep this state entirely inside the headless module.

#### Commit 20: Implement the unified creation queue

- Route token creation, virtual create-option selection, and confirmed creation through one queue.
- Consume the visible query before awaiting creation.
- Deduplicate queued labels against selected and earlier completed creations.
- Continue processing after an independent failure.
- Select successful creations through the normal atomic transition.
- Abort active creation and discard queued work on destruction with explicit cancelled outcomes.
- Cover back-to-back creation, parallel entry paths, failure followed by success, duplicate queue entries, and destruction.

#### Commit 21: Complete headless-module contract tests

- Add end-to-end module tests that drive only commands, subscriptions, and source adapters.
- Cover the complete flows for a single selector, multi-selector, creatable selector, silent external reset, and error recovery.
- Review tests to ensure they assert state and emitted changes rather than private counters, queue arrays, or implementation methods.

### Phase 4 — Delegate the compatibility adapter to the core

#### Commit 22: Extract the legacy Alpine adapter mechanically

- Move the existing Alpine implementation behind a clearly named compatibility adapter without changing behavior.
- Keep the current module export and Alpine registration stable.
- Run the frontend build and the focused selector browser suite to prove the move is mechanical.

#### Commit 23: Add legacy configuration normalization

- Convert legacy arguments into a normalized source, selection policy, and rendering behavior.
- Parse legacy serialized selected values and dynamic filter declarations at this edge.
- Keep endpoint selection and raw option mapping behavior-compatible.
- Do not delegate behavior yet; this commit only centralizes translation.

#### Commit 24: Delegate selection and replacement to the core

- Instantiate the core during Alpine initialization.
- Mirror selected options into the legacy raw selected-results property.
- Route push, remove, maximum enforcement, reset, and form reset through semantic commands.
- Derive legacy callbacks and aggregate events from atomic changes.
- Preserve removal-before-addition callback ordering for replacements.
- Keep pending callback presentation in the compatibility adapter for now.

#### Commit 25: Delegate open state and navigation to the core

- Route open, close, active index, next, previous, and active commit through commands.
- Keep keyboard event interpretation, focus, scrolling, and announcements in Alpine.
- Include the virtual create row in option counts only when the core exposes a valid candidate.

#### Commit 26: Delegate search to the core and HTTP source

- Replace the legacy debounce timer, abortable-fetch wrapper, stale-response checks, and search error assignment.
- Mirror normalized available options back to raw results for unchanged templates.
- Reevaluate dynamic parameters on every request.
- Confirm cancellation never displays an error.

#### Commit 27: Delegate creation and confirmation to the core

- Route add confirmation, cancellation, token commit, and virtual create-row selection through commands.
- Remove the legacy loading mutex and private creation queue.
- Preserve synchronous input clearing before asynchronous creation can rerender templates.
- Mirror creation status and errors into compatibility presentation properties.

#### Commit 28: Complete compatibility lifecycle cleanup

- Ensure the adapter unsubscribes from the core.
- Remove all form, popover, scroll, and resize listeners added during initialization.
- Destroy the registry registration, source decorator, and core.
- Add regression coverage for remounting a selector without duplicate listeners or events.

### Phase 5 — Introduce explicit profiles

#### Commit 29: Add single- and multi-entity field profiles

- Create profile constructors that choose selection policy, source mapping, form behavior, and default interaction behavior.
- Keep domain endpoint configuration in a private catalog rather than at call sites.
- Support category decoration as presentation metadata, not core label logic.
- Test profile configuration through observable selector behavior.

#### Commit 30: Add creatable-entity and tag-field profiles

- Add the general creatable form profile needed by series.
- Add the tag field preset using the lean suggestion source and tag creation source.
- Capture usage-aware sorting and delimiter rules in the tag profile.
- Confirm non-creatable tag pickers can use the ordinary entity-field profiles without exposing creation.

#### Commit 31: Add the immediate tag-editor profile

- Compose the tag field with an association persistence adapter supplied by the owning domain module.
- Track pending and briefly failed keys at this profile layer.
- Apply selection optimistically and reconcile success or rollback through selector commands.
- Make concurrent operations key-scoped and protect rollback from overwriting a later change.
- Test success, failure, same-key deduplication, different-key concurrency, and external reset during an in-flight write.

### Phase 6 — Migrate direct callers from simplest to hardest

#### Commit 32: Migrate compare selectors

- Replace standalone and custom-window-event configuration with single-entity profiles and atomic change listeners.
- Preserve navigation and selected resource/group behavior.
- Remove the custom dispatch option once no compare caller uses it.

#### Commit 33: Migrate the block-editor query selector

- Use a single-entity profile.
- Translate query selected/cleared behavior from separate callbacks to one atomic change.
- Preserve the current selected-query chip and table-data-source events.

#### Commit 34: Migrate paste-upload selectors

- Use the tag field, single resource-category field, and creatable series field profiles.
- Replace reactive reads of raw selected arrays with atomic selection changes.
- Preserve upload-store values and current form appearance.

#### Commit 35: Migrate entity-picker filter selectors

- Use the dynamic entity-selector profile.
- Preserve runtime endpoints, single/multi policy, filter-store updates, and close-time reset.
- Confirm selectors unregister when the picker closes or rerenders.

#### Commit 36: Migrate lightbox quick-slot selectors

- Use a non-creatable multi-tag profile.
- Preserve usage sorting, exclusion of already configured slot tags, autofocus, Escape behavior, and slot persistence.

#### Commit 37: Migrate the lightbox tag editor

- Replace standalone callbacks with the tag-editor profile and resource association adapter.
- Preserve optimistic detail-cache updates, suggested-tag invalidation, recent-tag tracking, failure announcements, and navigation-safe external reset.
- Remove compatibility pending/failure tracking after this is the last immediate-persistence caller.

### Phase 7 — Migrate the shared form surface

#### Commit 38: Teach the shared form partial to accept explicit profiles

- Add profile identity and domain inputs while retaining legacy arguments temporarily.
- Add stable selector metadata used by the registry and tests.
- Keep generated markup, hidden controls, IDs, and ARIA relationships unchanged.

#### Commit 39: Migrate single-value shared fields

- Convert owner, category, resource-category, note-type, relation-type, merge-winner, and similar maximum-one fields.
- Preserve minimum validation and nullable clearing semantics per field.
- Run entity creation/edit tests that previously caught clearing and required-selection bugs.

#### Commit 40: Migrate non-tag multi-value shared fields

- Convert group, note, resource, category, relation-type, and other multi-select filters and form fields.
- Preserve raw domain objects needed by schema-driven listeners.
- Confirm aggregate compatibility events still reach remaining listeners.

#### Commit 41: Migrate shared tag pickers and tag fields

- Use non-creatable multi-entity profiles for filtering, removal, and merge flows.
- Use tag-field profiles for create/edit/add flows.
- Standardize tag searching on the lean suggestion source.
- Run tag creation, duplicate, merge, bulk edit, search, and accessibility coverage.

#### Commit 42: Migrate dynamic relation fields

- Replace legacy filter declarations with profile parameter callbacks.
- Preserve relation-side and relation-type dependent parameters.
- Remove legacy filter parsing after confirming no caller remains.

### Phase 8 — Remove compatibility surface and finish

#### Commit 43: Migrate aggregate selection-event consumers

- Replace schema-driven form listeners and inline bulk editing with atomic selector changes where practical.
- Retain a narrowly scoped form event only if server-rendered cross-module communication still benefits from it.
- Document any retained event as part of the form adapter interface rather than a legacy accident.

#### Commit 44: Remove legacy configuration and state aliases

- Remove standalone, dispatch-on-select, raw URL, creation URL, extra-info, dynamic filter declarations, and separate select/remove callback options when searches prove no caller remains.
- Remove writable raw selected-results exposure.
- Remove the legacy reset method after all callers use the integration handle or profile interface.
- Keep the old Alpine registration name only if renaming it provides no meaningful interface improvement.

#### Commit 45: Delete superseded implementation and obsolete tests

- Remove legacy search, creation, queue, selection, and watcher logic now owned by the headless module.
- Remove tests that assert obsolete internals or duplicate deeper module-interface coverage.
- Keep browser tests that protect form, keyboard, accessibility, and domain integration behavior.

#### Commit 46: Final documentation and verification

- Document profile selection guidance, the selector-source contract, the registry contract, and the atomic change semantics.
- Record the intentional replacement-notification correction and cancellation behavior.
- Run formatting, type checking, unit tests, frontend build, focused browser regression tests, and the full browser suite.
- Confirm static searches find no direct selected-array mutation, Alpine declaration-string inspection, borrowed selector URLs, or removed legacy options.

## Testing Decisions

### What makes a good test

- Test behavior observable through the module interface: state snapshots, atomic changes, source calls, errors, and lifecycle outcomes.
- Do not assert private timers, queue arrays, generation counters, AbortController instances, or helper method calls.
- Use the in-memory source to control completion order rather than sleeping.
- Use browser tests for DOM, focus, keyboard modifiers, hidden controls, popovers, Alpine integration, and accessibility.
- Use Go endpoint tests only for server contracts; this refactor does not duplicate them in frontend tests.
- Preserve existing browser tests that have historically caught regressions even when equivalent core logic gains unit coverage, because those tests protect the rendering and integration adapters.

### Module tests

The headless module test suite covers:

- Initial normalization and duplicate keys.
- Single atomic replacement.
- Multi-selection and maximum behavior.
- Removal and silent replacement.
- Subscription and destruction.
- Open/close and wrapped navigation.
- Latest-query wins even when a source ignores cancellation.
- Cancellation versus visible failure.
- Filtering already selected options.
- Current-query create-candidate gating.
- Typed-token precedence.
- Create confirmation and cancellation.
- Ordered queued creation and failure recovery.
- HTTP source status and payload validation.
- Dynamic parameter callback reevaluation.
- Debounced source behavior.
- Registry scoping, resolution, replacement, and cleanup.
- Tag-editor optimistic persistence and rollback concurrency.

### Browser regression suites

Focused milestones run the existing coverage for:

- Shared chip input behavior.
- Lightbox chip input, pending state, queued creation, and rollback.
- Duplicate tag prevention.
- Empty Enter form submission.
- Inline tag-editor keyboard behavior.
- Selected-chip removal accessibility labels and focus.
- Lightbox tag prefetch and silent external replacement.
- Compare selectors.
- MRQL list-form synchronization.
- Nullable owner, category, resource-category, and note-type clearing.
- Schema-driven category and note-type listeners.
- Relation creation and dynamic filtering.
- Autocompleter accessibility roles, labels, and lightbox interaction.

### Verification cadence

- Every headless-module commit: type check and focused unit tests.
- Every compatibility-adapter commit: type check, frontend build, focused unit tests, and relevant selector browser tests.
- Every caller migration: tests for that caller plus the shared selector smoke tests.
- Phase completion: full frontend unit suite and frontend build.
- Final completion: full Go tests and full managed-server browser suite in addition to frontend checks.

## Risk Management

### Highest risks

- **MRQL hydration:** mitigated by moving to a tested scoped registry before core delegation.
- **Two sources of selection truth:** mitigated by prohibiting direct selected-array mutation before delegation and making the core authoritative afterward.
- **Creation races:** mitigated by one queue and deterministic deferred-source tests.
- **Alpine stale references:** mitigated by keeping synchronous input clearing and focus operations in the rendering adapter before awaiting commands.
- **Single-selection callback change:** mitigated by explicit tests and documenting removal-before-addition ordering.
- **Lightbox navigation during writes:** mitigated by retaining resource-keyed persistence guards and testing external reset during in-flight association.
- **Repeated field names:** mitigated by form-scoped rather than global-name registry lookup.
- **Dynamic relation parameters:** mitigated by characterization coverage and per-request parameter callback evaluation.

### Rollback strategy

- The legacy autocompleter remains the application entry point until explicit profiles are established.
- Core introduction is additive and unused initially, so it can be reverted independently.
- Registry migration is behavior-preserving and independent of core delegation.
- Each caller migration can be reverted to the compatibility adapter without reverting the core.
- Legacy options are removed only after static search and full test verification show no remaining callers.

## Out of Scope

- Visual redesign of inputs, chips, dropdowns, or confirmation controls.
- Replacing Alpine or Pongo templates.
- Introducing a Lit or form-associated custom element.
- Changing Go entity payloads or route contracts.
- Generalizing every application search module onto the selector core.
- Server-side tag canonicalization or case-insensitive uniqueness changes.
- Changing comma or space token semantics.
- Removing the legacy create-confirmation user experience.
- Popover abstraction beyond what the selector adapter needs.
- Refactoring lightbox cache architecture, recent-tag behavior, or suggested-tag ranking except where required to consume atomic selector changes.
- Adding new selector features unrelated to behavioral parity and the intentional corrections above.

## Completion Criteria

The refactor is complete when:

- All selector behavior is driven through the headless module and explicit profiles.
- No caller mutates selected arrays directly.
- MRQL uses the scoped registry and does not inspect Alpine state or borrow endpoint URLs.
- Search cancellation, stale results, creation, confirmation, and queue behavior have module-interface tests.
- Tags use the lean suggestion source through their profiles.
- Immediate tag association is isolated to the tag-editor profile and domain persistence adapter.
- The legacy configuration options and superseded implementation have been removed.
- Hidden form clearing, keyboard behavior, accessibility announcements, and lightbox synchronization remain covered and passing.
- Frontend type checking, unit tests, build, Go tests, and the full browser suite pass.
