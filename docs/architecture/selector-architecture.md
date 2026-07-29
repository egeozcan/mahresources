# Selector architecture

Every entity picker in the application — the shared form field, the lightbox tag editor, the
entity picker's filters, the compare pages, the paste-upload panel — is built from one headless
selector core plus a thin Alpine rendering adapter. This document is the reference for adding a
selector, changing one, or integrating another module with one.

Source layout:

| Path | Responsibility |
|------|----------------|
| `src/selector/selectorCore.ts` | Selection, search concurrency, and creation queue. No DOM. |
| `src/selector/types.ts` | The core's state, command, change, and source contracts. |
| `src/selector/httpSelectorSource.ts` | Endpoint construction, status and payload validation. |
| `src/selector/debouncedSelectorSource.ts` | Debounce, as a source decorator. |
| `src/selector/entityFieldProfiles.ts` | The named profiles and the entity endpoint catalog. |
| `src/selector/tagEditorProfile.ts` | Tag field whose selections optimistically persist. |
| `src/selector/selectorRegistry.ts` | Form-scoped integration surface for other modules. |
| `src/components/profiledAutocompleter.js` | Alpine `x-data` factories, one per profile. |
| `src/components/selectorFieldAdapter.js` | The rendering adapter: popover, focus, live region. |
| `templates/partials/form/autocompleter.tpl` | The shared form field markup. |

## Choosing a profile

Pick by what the field *means*, not by which endpoint it happens to hit. A profile owns its
endpoints; call sites never name a URL.

| Profile | Alpine factory | `profile=` in the shared partial | Use for |
|---------|----------------|----------------------------------|---------|
| Single entity | `singleEntitySelector` | `single` | A field that holds at most one existing entity: owner, resource category, note type, merge winner. |
| Multi entity | `multiEntitySelector` | `multi` | A field that holds several existing entities: groups, notes, resources, category filters. |
| Creatable entity | `creatableEntitySelector` | `creatable` | At most one entity, which the user may also create inline. Only `category` and `series` have creation endpoints. |
| Tag field | `tagFieldSelector` | `tag` | Creatable multi-tag input ranked by how often tags are used on the owning entity type (`usage`). |
| Tag editor | `tagEditorSelector` | — | A tag field whose every change immediately persists an association (the lightbox editor). |
| Dynamic | `dynamicEntitySelector` | — | The escape hatch when the endpoint is only known at runtime, such as the entity picker's configured filters. It never offers creation. |

Guidance:

- **Prefer the named entity profiles.** `dynamicEntitySelector` exists for configuration-driven
  filters; reaching for it because an entity is missing from the catalog is a signal to add the
  entity to `entityEndpointCatalog` instead.
- **`usage` versus `tagSuggestions`.** `tagFieldSelector` takes `usage` directly. A *non-creatable*
  tag picker takes `tagSuggestions: { usage }`, which selects the same lean, usage-ranked
  suggestion source without enabling tag creation. Lean suggestions are rejected for any entity
  other than `tag`.
- **`maximum`.** The shared form templates spell "no limit" as `max=0`; the core reads
  `maxSelected: 0` literally as "nothing may be selected". The Alpine factories reconcile the two,
  so templates keep saying `max=0`.
- **`form`.** Supplying `form: { name, minimum }` is what makes a selector part of a submitted
  form: it names the hidden controls, enables the minimum-selection check on submit, clears on
  form reset, and registers the field for other modules to observe. A profile with no `form` is
  not on a form — Escape blurs back to the surrounding surface instead.
- **`parameters`.** Request parameters that depend on other controls are supplied as a callback,
  re-evaluated per request. Neither the core nor the HTTP source reads the document;
  `selectorFormParameters` is the helper that lets a template declare a callback over sibling
  controls.

## The selector-source contract

A source is the only thing that talks to a server:

```ts
interface SelectorSource<TRaw> {
    search(query: string, signal: AbortSignal): Promise<readonly SelectorOption<TRaw>[]>;
    create?(label: string, signal: AbortSignal): Promise<SelectorOption<TRaw>>;
    destroy?(): void;
}
```

- **`search` is cancellable and must honour the signal.** The core aborts the previous search on
  every query transition.
- **A source without `create` is a non-creatable field.** The core offers no create candidate and
  no confirmation flow.
- **`destroy` releases decorator-owned timers and listeners.** The debounce decorator implements
  it; a plain HTTP source does not need to.
- **Options are normalized at the source boundary.** `{ key, label, raw }` — `raw` is the untouched
  server value, which is what form-integration consumers and schema-driven listeners read.

### Search concurrency

On every query transition the core cancels the previous search, advances an internal request
generation, and starts a new one. A response is accepted only when both the generation and the
query still match, so **the latest query always wins even when a source ignores cancellation**.

Debouncing is a decorator, not a core concern: the core receives every keystroke immediately and
stays the single source of truth for the query and the create candidate. Production profiles wrap
their HTTP source in `createDebouncedSelectorSource(..., 200)`; deterministic tests use
`InMemorySelectorSource` with no fake rendering state.

### Creation queue

Every creation entry point — typed-token commit, the virtual create row, and the "Add X?"
confirmation — goes through one queue. No path bypasses it, and no path uses a loading boolean as
a silent mutex. The create candidate only becomes available once the search for the *current*
trimmed query has completed and no available or selected option matches the label exactly; a
create row that appeared before its search completed would race real results.

## The registry contract

`selectorRegistry` is how one module integrates with a selector it does not own, without reading
Alpine state or borrowing the selector's endpoint. It is keyed by **owning form plus field name**,
so two forms on one page can both have a `categories` field without cross-wiring. Form keys are
weak, so a detached form is collectable even if Alpine cleanup did not run.

**Reading and writing a field** — `selectorRegistry.get(form, fieldName)` returns a
`SelectorIntegrationHandle`:

| Method | Meaning |
|--------|---------|
| `getRawValues()` | The current selection as raw server values. |
| `replaceRawValues(values, { silent })` | Replace the whole selection with values the caller already has. |
| `replaceByKeys(keys, { silent })` | Replace by id, preserving any already-hydrated value for a key it still holds and standing in `#<key>` for the rest. |
| `resolveExactLabels(labels, { silent })` | Look each label up through the profile's own endpoint and replace only if *every* label resolves exactly; returns `false` and changes nothing otherwise. |

The MRQL filter bar is the reason `resolveExactLabels` exists: it hydrates a field from names
alone. It reads the endpoint from the profile (`profile.lookup.searchUrl`) rather than being told
one, which is why a profile publishes its search URL even though the adapter never searches.

**Observing a field** — `selectorRegistry.observe(form, fieldName, listener)`, or
`observeSelectorField(element, fieldName, listener)` when the consumer is a DOM node that shares
the form. Both return a cleanup function.

- **Observation does not depend on registration order.** A consumer rendered before its selector
  gets the same result as one rendered after, so no module reasons about Alpine init sequence.
- **Delivery is synchronous and isolated.** A throwing observer is logged and skipped; the rest
  still see the change. Synchronous means the notification runs inside the selector's own state
  publication, *before* Alpine re-renders the field's hidden controls — so an observer that reads
  the DOM (serializing the form, for instance) must wait a tick (`Alpine.nextTick`) or it will see
  the previous selection. Observers that use the values in the payload need no such care.
- **Nothing is delivered on subscribe.** Server-rendered initial state is the template's job;
  observers only see subsequent changes.

This replaced a `multiple-input` CustomEvent that bubbled to the document, where every consumer
re-derived its own scoping by comparing `$event.detail.name`. Current consumers:
`schemaMetaFields` and `schemaSearchFields` (which swap the meta editor or the schema filter to
follow a category-like field), and the resource list's inline tag editor.

## Atomic change semantics

The core publishes **one change per selection transition**, never a sequence of smaller ones. A
change carries `previous`, `current`, `added`, `removed`, and a `reason`
(`select | remove | create | replace | reset`).

- **A maximum-one replacement is one change, not two.** Selecting a second value into a field that
  holds one publishes a single change whose `removed` names the outgoing value and whose `added`
  names the incoming one. This is an intentional correction: the previous implementation notified
  per item, so a consumer could observe an empty intermediate selection that the user never
  chose. Consumers that care about ordering apply `removed` before `added`.
- **Selection is a set.** Duplicate keys collapse, and a selection over the limit keeps the *last*
  values (`slice(-limit)`), which is what makes replacement rather than rejection the behaviour of
  a full maximum-one field.
- **A no-op publishes nothing.** Selecting an already-selected option, removing an absent one, or
  replacing with an equal selection produces no change.
- **`silent` means "the selector did not choose this".** A silent replacement updates the snapshot
  and re-renders, but publishes no change: no `onChange`, no registry notification, no
  announcement. It is the right mode for state the surrounding domain owns — navigating the
  lightbox to another resource, the entity picker clearing its filters on close, an MRQL
  hydration. A user-triggered form reset is deliberately *not* silent, because the user did
  choose it.

### Cancellation is a transition, not a failure

An aborted search is a normal state transition and never renders an error. Only a search that
actually failed sets an error message. The same holds for creation: cancelling the "Add X?"
confirmation is a `cancelled` outcome, not an error, and returns focus to the input.

## Invariants worth knowing before you change this code

- **`addModeForTag: false` is a sentinel, not merely falsy.** The shared field autofocuses on
  `addModeForTag !== false`. It stays `false` until an "Add X?" confirmation has actually been
  shown, and becomes `''` once that flow closes — which is what returns focus to the input after
  Cancel. Downgrading the initial value to `''` makes every selector on the page grab focus and
  leave an open popover.
- **Mouse selection is identity-based; Enter is index-based.** A click names one rendered row and
  must commit *that row*, or the click is silently dropped whenever the user out-types the
  debounce. The stale-search guard belongs on the keyboard path only.
- **Clear the DOM buffer synchronously before an `await`.** Creation re-renders the field, so an
  `$refs` entry captured before an await may be stale afterwards.
- **No caller mutates the selected array.** The core is the single source of truth; the adapter's
  `selectedResults` is a rendering mirror. Anything that needs to change a selection dispatches a
  command or goes through the registry handle.
