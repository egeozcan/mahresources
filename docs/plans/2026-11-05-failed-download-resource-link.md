# Failed download links to the existing Resource

## Success criterion

A background download refused because its content hash already exists remains
failed and shows a working “View existing resource” link beside its failure on
the Jobs drawer, Jobs list and Job detail page. Other failures show no such link.
The link uses the existing authorized Job output route and disappears when the
Resource is unavailable or the viewer cannot read it.

## Assumptions and tradeoffs

The Job already publishes an optional `existing-resource` output for a
`resource-exists` failure. Reuse it; do not parse a Resource ID out of error text
or change deduplication, failure classification or retry behavior. Keep failure
links distinct from successful result links, because the Job did not create the
existing Resource. Previously recorded failures without that typed output stay
plain text.

## Plan

- [x] Reproduce the missing drawer/list link with the existing real duplicate
  download E2E fixture, and establish baseline focused Go and JavaScript tests.
- [x] Expose the authorized failure output in the list projection and drawer,
  using the existing link label and URL conventions. Add regression coverage
  for unrelated failures, stale outputs and unavailable Resources.
- [x] Rebuild the committed frontend bundle, run focused Go and JavaScript
  checks and the real duplicate-download browser regression, review the diff
  and update the architecture note.
- [ ] Complete full-suite and PostgreSQL verification (limitations below).

## Review

The rendered-card and drawer regressions failed before the fix and passed
afterwards. The real duplicate-download browser test now follows the link from
the detail page, Jobs list and drawer to the original Resource. Focused Go and
JavaScript checks pass; a separate read-only review found no actionable issues.

The full JavaScript suite passed 2,452 tests and failed one unrelated schema
editor timing assertion (2.53 seconds against a two-second limit; 2.25 seconds
with four workers). Full Go and browser/CLI runs were attempted, but their final
results were unavailable after the execution environment reset. PostgreSQL
verification was unavailable because Docker was not installed.
