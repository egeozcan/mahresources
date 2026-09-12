# Shared entity picker

Status: written spec approved by the user; implementation plan prepared.
Date: 2026-09-12

## Goal

Advance the existing group picker used by note reference blocks into a shared,
property-filtered entity browser. Expose it through an accessible icon button
beside every entity selector while retaining fast inline autocomplete.

The motivating tasks are finding the correct group within a specific category
(including distinguishing identical names), and identifying a resource from its
thumbnail. This is selecting existing entities, not creating entities, editing
entity membership directly, or running a bulk action.

## Existing foundation

- `src/components/picker/entityPicker.js` and `entityConfigs.js` implement the
  existing resource/group/note dialog. Block and plugin-action consumers already
  use it; the caller owns the destination and persistence.
- `templates/partials/entityPicker.tpl` renders that global dialog.
- `src/selector/` owns entity profiles, selection, transport and integration.
  `docs/architecture/selector-architecture.md` defines its atomic-change,
  cancellation, registry and standalone-mount contracts.
- The current popup exposes a subset of available filters and a bounded first
  page without continuation. Its main search does not inherit the headless
  autocomplete core's stale-response protection.
- Categories, NoteTypes and ResourceCategories already expose paired custom
  HTML/CSS slots and shared `CustomCSS`.

Extend this foundation. Do not create a parallel advanced-picker implementation
or embed full list pages with their navigation and bulk-action controls.

## Coverage and entry points

The Browse icon is available beside every host entity selector, including
single, multi, creatable, tag, dynamic-filter and standalone mounted selectors.
The current entity catalog includes category, group, note, noteType, query,
relationType, resource, resourceCategory, series and tag. Existing popup consumers
(reference/gallery/calendar blocks and plugin entity parameters) use the same
improved dialog.

A picker session has exactly one entity type, inherited from its originating
field. It does not switch the field to another type or offer mixed-type results.

The icon has a meaningful accessible name such as “Browse groups”; a tooltip is
supplemental, not its only label. Disabled fields cannot launch it. Existing
inline creation remains where it is; the popup browses existing entities only.

Originating constraints apply to every request: entity type, dynamic parameters,
locked eligibility filters and selection limits. Editable search filters cannot
replace or widen those constraints. The current field remains the authority if
it is disabled, destroyed or its eligibility changes while the picker is open;
confirmation must not apply stale, no-longer-eligible choices.

## Search and pagination

Use the entity's existing list-search semantics, not a new query language.
Expose name and applicable category/type, owner and tag filters prominently.
Expose the additional filters available on that entity's ordinary list page
under **More filters**. Unsupported properties are absent, not nonfunctional
controls. Owner and related groups remain distinct concepts.

Examples of the relevant existing filter families:

| Entity | Prominent filters | Additional existing list filters |
| --- | --- | --- |
| Group | Name, category, owner, tags | Description, related entities, dates, metadata and other group-list controls |
| Note | Name, note type, owner, tags | Description, related entities, created/updated and note date ranges, metadata and other note-list controls |
| Resource | Name, resource category/content type, owner, tags | Related groups/notes, description, dates, dimensions, metadata and other resource-list controls |
| Other catalog types | Name and applicable list properties | Their existing list controls, without invented ownership or taxonomy fields |

All filter selectors themselves have Browse icons. Requests are cancellable and
protected by a session/step/request generation check: a late response cannot
replace newer results, revive a closed session, or populate another nested step.
Changing filters resets the result page but retains pending selected entities.

Use explicit Previous/Next pagination with a visible page indicator and loading,
empty and retryable-error states. A page holds at most 50 results. Availability of
the next page is determined server-side, not guessed from an unbounded client
fetch. Exact total counts are not required; do not count or hydrate the entire
library just to display navigation. Use deterministic ordering with an ID tie
breaker. Pagination is a live browse, not a frozen database snapshot.

There is no “select every matching entity” operation. Pending choices are
explicit entity identities retained across pages and filters.

## Result presentation

Default results show, where the entity actually has these properties:

- Full, readable name linked to details in a new tab.
- Tags.
- Thumbnail, or a neutral fallback when unavailable.
- Owner.
- Category/type, especially to distinguish same-name groups.

Do not make hover-only text the only way to distinguish results. Long names and
categories must be readable without depending on truncated badges. Entity types
without these properties omit them rather than fabricating values.

Use separate host-owned checkbox/radio controls. Clicking a name or a link inside
custom content must not toggle selection. Default links use the entity's real
detail route and clearly communicate that they open a new tab. The default card
is not an embedded video/audio/document viewer; the detail page provides further
inspection. Custom result authors may provide their own links and presentation.

## Custom picker-result slots

Add the following string fields to group Category, NoteType and ResourceCategory:

- `CustomEntityPickerResult`
- `CustomEntityPickerResultCSS`

The HTML slot replaces the result's content inside a standard host-owned wrapper.
It cannot own the selection control, selection state, accessibility label for that
control, or dialog keyboard/focus machinery. A host-owned label remains available
even if a custom template omits the name.

Use the same entity context, escaping/trusted-HTML policy, shortcode conventions,
principal binding and render budgets as existing custom template surfaces. This
is not a new arbitrary execution mechanism. No new promise of Alpine component
initialization inside result markup is introduced; native links work normally.

Load the carrier's existing `CustomCSS` alongside its
`CustomEntityPickerResultCSS`, once per relevant carrier in the rendered result
set. Document a picker-specific wrapper for targeted selectors. Do not claim
stylesheet isolation: these are trusted global styles, consistent with existing
custom-template slots. Dispose of picker-owned styles with the dialog/step
lifecycle rather than accumulating duplicates on each search.

An empty template uses the default result, never `CustomSummary`. A rendering
failure also uses that default and emits a bounded diagnostic to the application
log. A failed custom result must not make an otherwise selectable entity
unreachable. Intentional neutral output from an unavailable plugin is not an
excuse to bypass plugin access checks.

The fields participate in the existing template-slot lifecycle: storage,
create/edit/read interfaces, category/type editor controls, template tooling,
and relevant CLI/API documentation and round-trip paths. Preserve existing
permissions for editing each carrier. Any archive additions must be additive and
respect the stable archive schema contract.

## Selection and confirmation

### Multiple-selection fields

Opening snapshots the field's existing entities as already added. They are
marked and cannot be removed through this dialog. The user can toggle pending
additions, including after moving between pages and filters. Confirm appends
pending entities without duplicates in one atomic selector transition.

Respect the field's remaining capacity. Do not silently discard earlier field
values or pending choices when a maximum is reached. Pending choices retain a
stable insertion order for presentation, but the domain contract remains a set,
not a new ordered-list feature.

### Single-selection fields

Use a radio selection and explicit Confirm, rather than committing on card
click. Confirm replaces the field value atomically. Reconfirming an unchanged
selection is a no-op.

### Persistence ownership

Confirm changes the originating selector through its normal integration path,
using hydrated entity values so labels and downstream observers are correct.
Do not mutate Alpine rendering mirrors or scrape hidden inputs. Form selectors
still wait for form submission; autosaving selectors still invoke their normal
persistence mechanism and retain their normal failure/rollback behavior.

The picker never writes associations directly. Existing ID-callback consumers
remain supported by an adapter at that boundary. A nested picker confirms only
into its parent step's filter, not into the outer originating field.

## Nested browsing and lifecycle

A single dialog owns a stack of browsing steps. Each step retains its filters,
page, results and pending selections while a child filter picker is active.
Confirming the child updates the parent filter and reruns the parent's search
from its first page, keeping pending entity additions.

Back or Escape in a nested step discards that step's unconfirmed choices and
returns to its parent. Escape at the outermost step cancels. The outer close
button/backdrop dismissal cancels the whole session. Cancel never changes the
originating selector. Reopening creates a fresh session from its current values;
there is no cross-session filter or pending-selection persistence.

The dialog owns focus trapping and scroll locking. Entering a child focuses its
search; returning restores focus to its launching filter control. Closing the
outer dialog returns focus to the Browse button after trap teardown. A picker
launched from another existing modal must not let Escape close both surfaces or
lose focus to the underlying page. Results, selection counts, loading and errors
are accessible without relying on color or pointer interaction.

## Implementation boundaries

Keep three responsibilities distinct:

1. **Selector integration:** the existing profile/core/registry or standalone
   handle owns field selection and atomic publication. Profiles provide entity,
   constraints and result identity/labels.
2. **Browsing session:** one dialog controller owns nested steps, filters,
   pagination, pending additions, request generations and focus restoration.
3. **Search/render service:** bounded scoped list searches return only the
   current page, identity/label data, custom/default result content, applicable
   styles and continuation information. Reuse the host filter and template
   mechanisms without making autocomplete fetch rich cards.

Keep the existing lightweight autocomplete source contract intact. A richer
browse response is separate from that array-of-options contract. Exact route
and helper names belong to implementation planning, not a second search dialect.

Server handlers consume appropriate contracts; shared boundary types belong in
`contracts/`. Application services receive the caller's scoped/transactional DB
handle per call. No service captures a singleton unscoped DB for later browsing
or rendering. Plugin shortcodes retain the caller's access predicate. Mutation
endpoints continue to validate association permissions; client eligibility is
not authorization.

Hydrate/render only the bounded page and share template/CSS preparation across
its rows. Preserve render query budgets and cancellation. Do not introduce
per-result unbounded related-entity loads or a per-card query amplification path.

## Verification and acceptance

Automate at least the following behaviors:

- Browse appears across the full catalog and profile families, including a
  standalone mount, autosaving tag editor and selectors inside filter steps.
- Category-filtered groups with identical names are distinguishable; resources
  show thumbnail, name, tags and owner.
- Custom slots render for all three carriers; empty/error fallback works;
  CSS is deduplicated; fields survive supported edit/read round trips.
- A name/custom-content link opens details without selecting the card.
- Multiple selections survive pagination/filter changes and append once without
  duplicates; a single replacement publishes exactly one change.
- Selection maxima and locked/dynamic restrictions cannot be widened; destroyed
  or disabled origins do not receive a stale confirmation.
- Nested confirm updates only the parent filter; Back/Escape/Cancel preserve or
  discard the correct step; outer reopen starts fresh.
- A late search response cannot replace current results after typing, paging,
  step navigation or closure. Network failure can be retried without losing
  pending choices.
- Keyboard-only and screen-reader flows cover focus restoration, nested steps,
  links, selection controls and launch from an existing modal.
- Scoped users see no inaccessible entities or custom-rendered data; plugin
  access and template budgets remain enforced.
- Existing reference/gallery/calendar and plugin picker consumers retain their
  selection and persistence semantics. Validate note/resource association
  filtering against the actual DTO rather than copying an owner-id shortcut.

Use frontend unit tests, Go/API/template tests, browser integration and
accessibility tests. Run browser and CLI E2E against separate ephemeral servers,
plus the PostgreSQL coverage required by repository guidance. Rebuild committed
JS assets after source changes, run CSS scan separately from template-walk tests,
and update selector architecture, picker documentation and affected CLI help.

## Non-goals

- A second picker implementation or embedded full list pages.
- Mixed-entity selection, query-wide selection or new bulk-action semantics.
- Removing existing multi-select values from inside the popup.
- Creating entities inside the popup.
- A default embedded media viewer.
- CSS sandboxing or new custom-script execution guarantees.
- Changing field persistence ownership, authorization or archive compatibility.

## Spec review

Checked against the interview decisions: existing-picker evolution, icon entry,
all-selector coverage, basic/advanced search, explicit pagination, custom result
slots and CSS on all three carriers, default identification fields, new-tab links,
add-only multi-select, confirmed single replacement, nested steps, persistence
ownership, fresh reopening and shared CSS behavior are all represented.

No new glossary entry is necessary: this adds a UI interaction, not a different
meaning of Entity Selection, Bulk Action, Group or Mass Edit. No ADR is proposed;
the reversible UI and integration choices do not meet the repository's ADR bar.
The user approved the written spec. Implementation has not started; the plan is
`docs/superpowers/plans/2026-09-12-shared-entity-picker.md`.
