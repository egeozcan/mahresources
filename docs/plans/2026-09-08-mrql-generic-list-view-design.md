# MRQL generic list view

Status: Confirmed and implemented on 2026-09-08.

## Confirmed design

MRQL Entity Results should use the standard list presentation and full list interactions, including selection, bulk actions, display controls, and pagination. This applies both to ordinary entity results and entities within result buckets.

Aggregate rows remain tabular summaries rather than selectable entities.

The initial implementation covers the MRQL editor and ordinary lists. Embedded `[mrql]` formats and custom templates remain unchanged; embedded interactive controls are deferred.

Mixed results have independent selection and bulk controls per entity type. Selection may span buckets on the current page. Repeated appearances of the same entity share selection state, and an action processes that entity only once.

Changing pages clears selection. Select All targets only entities of the relevant type on the current page.

For flat entity results, an explicit query LIMIT caps the result set rather than setting the display page size: LIMIT 100 with 25 items per page allows at most four pages. Pagination leaves the authored query text unchanged; sorting controls update ORDER BY.

For `ORDER BY RANDOM()` results, pagination and query-wide Mass Edit preserve the bounded sample from that execution. The server seals its ordered typed identities in a token bound to the exact query, parameters, principal, and scope. The token expires after one hour; an expired or invalid token requires running the query again. Reusing a token checks live query membership and authorization, removes unavailable entities, and never samples replacements. Mass Edit requires that token and retains its count handshake. A new execution, sort change, or successful mutation refresh produces a fresh sample. The language already prohibits random ordering with GROUP BY.

For bucketed results, preserve MRQL's existing semantics: LIMIT caps entities per bucket, OFFSET advances through buckets, and the bucket page size controls how many buckets are displayed. Selection can span the buckets on the current page.

Bulk actions come from central declarations identifying the entity types they support. Registering an action for an entity type makes it available in that entity's ordinary list and in MRQL when entities of that type are selected; neither surface should require its own action registration.

Action declarations describe eligibility, standard inputs, confirmation, and execution. Specialized interactions may supply a custom component through the same registration mechanism; ordinary lists and MRQL render the same registered action.

Migrate all existing built-in bulk actions, including tag and download list actions, to the shared declarations. Adapt existing plugin registrations into the same mechanism.

An action requires the entire selection to satisfy its eligibility rules. An unavailable action remains visible but disabled with an explanation; do not silently act on only an eligible subset. Compare, for example, requires exactly two selected entities.

Reuse the existing Mass Edit interaction for selected entities and for targets beyond the current page. In MRQL it operates independently per entity type and shows only edits relevant to that type. Do not introduce a separate general-purpose all-query-results bulk-action mechanism or add query-wide targeting to every action declaration.

Mass Edit's all-results target respects the executed query's limits and offsets. It must not expand to every entity matching the underlying filter. Show the target count through the existing Mass Edit confirmation flow; for bucketed results, preserve the existing per-bucket limits and bucket offset semantics.

After an action successfully changes entities, rerun the query that produced the displayed results, preserving its parameters and current page, and clear selection. Entities that no longer match disappear. Navigation actions such as Compare and Resource Reduction retain their intended navigation.

## Implementation outline

1. Introduce the shared bulk-action declaration contract and renderer, with standard forms, custom components, and an adapter for plugin registrations. Migrate the existing built-in list actions while retaining their behavior.
2. Make list selection independent per entity type and local to the displayed result page. Keep repeated appearances synchronized and deduplicate execution targets.
3. Integrate the shared list presentation, display controls, and action controls into MRQL entity results, including bucketed results. Keep aggregate tables and embedded shortcode output intact.
4. Add display pagination over flat query results and bucket pagination using existing grouped-query semantics. Preserve the executed query and parameters as the source for refresh and action targeting.
5. Connect Mass Edit to explicit entity selections and MRQL query targets, with the relevant entity fields and its existing dry-run and confirmation behavior.

## Acceptance checks

- Registering a bulk action for an entity type exposes the same action in its ordinary list and in MRQL without page-specific registration.
- Existing built-in actions, including tag and download actions, and plugin actions retain their input, confirmation, execution, and navigation behavior.
- A resource and a note with the same numeric ID have independent selection and action targets.
- An entity appearing in several buckets is selected consistently and processed only once. Changing pages clears selection; Select All stays within the current page and entity type.
- Flat LIMIT 100 with a page size of 25 never exposes a fifth page. Bucketed LIMIT remains a per-bucket item cap.
- Randomly ordered result pages and Mass Edit use the same sampled identities. Tokens reject tampering, expiry, changed query/parameters/principal/scope; live removals cannot silently substitute different targets.
- Mass Edit follows recoverable bucket continuations, including the per-page item ceiling, before applying its ceiling to the deduplicated entity target set.
- Eligibility is checked against the whole selection. Invalid selections disable the action with a reason, including Compare with a count other than two.
- Mass Edit offers the correct fields for resources, notes, and groups and targets only the appropriate entities within the executed query's bounds. Existing dry-run, confirmation-count, authorization, and execution bounds remain effective.
- Successful mutations refresh the executed query and its parameters rather than unexecuted editor text, and clear selection. Navigation actions still navigate.
- Aggregate rows remain tables, and embedded shortcode formats and custom templates remain unchanged.

## Architecture decision

See [Shared declarations for bulk actions](../adr/0004-shared-bulk-action-declarations.md).

## Evidence gathered before implementation

- `templates/mrql.tpl` renders separate MRQL result cards and aggregate tables.
- Standard entity lists use shared card partials, display controls, and entity-specific bulk editors.
- `src/components/bulkSelection.js` uses a shared selection store keyed by item ID; mixed entity types need explicit selection boundaries or typed identities.
- Built-in bulk actions are currently authored in entity-specific templates. They include forms, navigation actions such as Compare, and specialized interactions such as Resource Reduction.
- Plugin actions already declare entity type, placement, input parameters, filters, confirmation text, and a maximum selection size in `plugin_system/actions.go`; list pages expose these through separate plugin action controls.
- Mass Edit already has entity-specific operations and selected-ID/filter targeting, with a dry-run count and ExpectedCount check. Its frontend currently reads a global selection store and the page URL's list filter; its template assumes one entity type per page. MRQL requires explicit entity context and a target derived from the query that produced the results.
- Bucketed MRQL currently interprets LIMIT as items per bucket and OFFSET as a bucket offset; BucketLimit controls the number of buckets per page. A flat result's total-item cap cannot be applied to buckets without changing existing query semantics.

## Implemented structure

- `listviews/bulk_actions.go` is the built-in action catalog. Add an entry naming its supported `Entities`, eligibility filters/counts, endpoint, inputs, and confirmation. Both list surfaces discover it automatically. A `Component` names a trusted partial under `templates/partials/bulkActions/` for specialized interactions.
- `server/template_handlers/template_context_providers/bulk_actions.go` adapts access-filtered plugin registrations to the same declaration contract. Plugin declarations remain the source of their own entity, inputs, limits, and filters.
- `templates/partials/bulkActions.tpl` renders the shared toolbar. Scoped `$selection` stores own selection, duplicate appearances, and refresh callbacks; ordinary lists retain their default selection store.
- `server/api_handlers/mrql_list_render.go` hydrates bounded result pages and renders the existing resource, note, and group cards. `render=list` adds display-page metadata while preserving the authored query bounds and mixed-query ordering. Bucket continuations use the returned offset, including pages shortened by the query budget.
- `application_context/mass_edit_mrql.go` resolves MRQL targets within the existing Mass Edit ceiling and one query timeout, deduplicates bucket appearances, and retains the count handshake. The modal selects operations, tag suggestions, and metadata keys by entity type.
- `src/components/mrqlEditor.js` retains the executed query and parameters for paging and mutation refresh. Display controls offer cards or a single-column list. Completed background plugin actions also refresh relevant entity results.

## Validation

Coverage includes shared template rendering, flat LIMIT/OFFSET pagination, mixed-query Mass Edit bounds, bucket continuation and deduplication, independent entity selections, correct Mass Edit fields, ordinary bulk forms and downloads, plugin actions, and refresh with an unexecuted editor draft. The frontend production build and full JavaScript unit suite pass; backend API, handler, application-context, and list-view suites were checked, with focused reruns after the final changes. Browser regression tests cover the shared list flows, Mass Edit, plugin refusals, group deletion warnings, and merge guards.
