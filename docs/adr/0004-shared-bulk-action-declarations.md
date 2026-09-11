# Share bulk-action declarations across ordinary lists and MRQL

Ordinary entity lists and MRQL entity results need the same actions, so built-in bulk actions and adapted plugin registrations will use one declaration contract for entity applicability, eligibility, standard inputs, confirmation, and execution. Specialized actions may provide a custom component through that contract, preserving interactions such as Resource Reduction without forcing every workflow into a form schema or duplicating controls by page. Mass Edit remains the existing interaction for edits across query results, with entity-specific fields and query bounds; general bulk-action declarations do not acquire a separate all-query-results targeting mechanism.

## Considered options

- Separate toolbars per surface would keep template authoring simple, but each action's eligibility and inputs would have to be maintained twice. This was already causing ordinary lists and MRQL to diverge.
- One universal form schema would centralize the controls, but Resource Reduction, Compare, downloads and plugin actions have distinct navigation or execution flows that cannot use a normal bulk-edit form.
- Shared declarations with optional trusted components retain those specialized interactions while making entity applicability and eligibility common to both surfaces.

## Consequences

A new action is declared once and appears on every supported entity list. Component names must resolve to checked-in templates, guarded by the Go architecture suite. Server handlers still authorize and validate every operation; client eligibility is presentation only.

Alpine scopes and disabled fieldsets wrap actions, so form wrappers must preserve the toolbar's flex layout: action buttons share a row and expanded forms occupy a full row. Selection is local to each entity type and must be passed to specialized actions explicitly.

MRQL cards use the same rendering helper and request state throughout a response, including CSS deduplication, shortcode budgets, request cancellation and ORM scope. CustomMRQLResult remains an override inside the selectable shell. Display pages bound cards even within a bucket, and continuation offsets preserve the authored per-bucket limits. Mass Edit resolves the executed query independently of its display page, with a signed snapshot for a random sample.
