# Resource compare page: 33 findings from the 2026-08-25 review

A code-and-browser review of `/resource/compare` turned up seventeen defects and
sixteen usability gaps. This is the record of what was changed and what was
deliberately left alone.

## Defects

- [x] **PDF comparison downloaded both files on load and could never render.**
  Both `<iframe>`s were server-rendered and hidden with `x-show`, so the browser
  fetched them immediately — and `/v1/resource/version/file` answers with
  `Content-Disposition: attachment`, so the fetches became downloads. The frames
  were blocked anyway by the primary server's blanket `X-Frame-Options: DENY`.
  Fixed with `x-if` plus an opt-in `?disposition=inline` on the version-file
  route, **safelisted to `application/pdf`**: version files are arbitrary
  uploads, and serving one inline and same-origin-framable is stored XSS for
  anything the browser executes in a document context. The button also toggles
  now, instead of removing itself and stranding the reader in the viewer.
- [x] **The side-by-side text diff did not align its columns.** The padding rows
  were emitted correctly but rendered in empty table cells, which collapse to
  zero height. Rows are CSS grid lines now, so a padding row still occupies a
  line.
- [x] **The panel Swap inverted the old/new colour coding.** It exchanged the two
  URLs and labels in place, leaving the pink "older" panel holding the newer file
  and the server-rendered `alt` describing whichever side had originally been
  there. Everything that varies is derived from one `swapped` flag now. Renamed
  **Flip**, because the toolbar's own Swap does something different.
- [x] **`?r1=<id>` alone dead-ended.** The redirect that fills in version numbers
  ran for cross-resource comparisons only, so a same-resource URL rendered the
  empty state while both selects displayed a version, and picking one wrote
  `v1=0`. Same-resource now resolves to previous-versus-current.
- [x] **Mismatched content types were diffed as text.** The comparator was chosen
  from the left-hand version alone, so a JSON-versus-PNG comparison fetched the
  PNG in full and printed its bytes as added lines. Both sides have to agree, and
  the binary panel says the types differ.
- [x] **Slider and onion skin ignored aspect ratio.** Both images filled the
  container width and took their own height, so a pair with different proportions
  overlaid nothing. One shared box built from the larger of the two in each axis,
  with both `object-fit: contain` inside it.
- [x] **The page never named what it was comparing.** The pickers were a copy of
  the shared autocompleter with the selected-item chips left out, and the heading
  and `<title>` were the constant "Compare Versions". Now the shared partial, a
  breadcrumb, and a title that names the resources and versions.
- [x] **The pickers had no combobox semantics.** Same copy, same fix — the shared
  partial brings `role="combobox"`, `aria-expanded`, `aria-controls`,
  `aria-activedescendant` and the listbox/option roles.
- [x] **Toggle mode ignored Enter.** It was a `<div role="button">` binding only
  `@keydown.space`. It is a real `<button>` now.
- [x] **Arrow keys fired twice.** The image shortcut listened on `document` and
  the radiogroup handler did not stop propagation, so one ArrowRight changed the
  mode *and* moved the new mode's control. Scoped to the container, and events
  from inside the radiogroup are skipped.
- [x] **The image slider had no keyboard-operable control.** The handle was an
  unlabelled `<div>` reachable only through that undiscoverable shortcut. It is
  `role="slider"` with `aria-valuenow`, `aria-valuetext` and its own arrow keys.
- [x] **Every compare page reserved an empty 400px sidebar.** Neither compare
  provider set `hideSidebar`, so a third of the viewport went to an empty
  `<aside>` on the page that most wants the width — and on mobile that aside
  rendered as a "Filters and details" disclosure revealing nothing. Both
  providers set it now.
- [x] **Three contrast failures.** The diff line-number gutter measured 2.47:1,
  `.compare-swap-btn-sm` 4.39:1, and the PDF panel used `<h3>` under the page
  `<h1>`. A full-page axe scan across all six page states now returns nothing.
- [x] **Merge disappeared when either side was not the current version.** The
  section is always rendered, disabled, and carries the reason — following
  `bulkCompareAction.tpl`, which already argues that case for its own rule.
- [x] **"Dimensions 0×0" for every file without dimensions.** The card renders
  only when either side has a non-zero width.
- [x] **The binary panel's thumbnail came from the resource, not the version**, so
  a same-resource comparison would have rendered the same picture twice. Replaced
  with the content-type placeholder, which is at least true of both.
- [x] **A matching hash rendered six trailing dots** — a literal `...` after
  `truncatechars`' own ellipsis. The hash is shown whole with a copy button; a
  truncated hash cannot be checked against anything.

## UX gaps

- [x] Prev/next change navigation with a count.
- [x] Unchanged context collapses to an expandable "Show N unchanged lines" row
  (three lines of context either side, runs of six or more folded).
- [x] Only the visible diff view is built. Both were in the DOM at once, so every
  line of a large file was materialised three times over.
- [x] A size guard: above 2 MB combined the diff asks before loading, and the
  fetches carry an `AbortSignal` so leaving the page cancels them.
- [x] Word-level highlighting inside a changed line, when the two lines still
  share enough to make it meaningful.
- [x] Merge names both resources, in the buttons and in the confirmation, and the
  checkbox says "Keep the other file as an earlier version".
- [x] Cross-resource panes are labelled with the resource names.
- [x] "Created" no longer prints the same string twice; sub-minute gaps read
  "less than a minute apart".
- [x] Version dropdowns carry the current marker, the size and the comment, and
  fall back to a time when two versions share a date.
- [x] Comparing a version with itself says so instead of reporting a file
  identical to itself.
- [x] The version panel sorts the two ticked versions, so the diff always reads
  older-to-newer whatever order they were clicked in.
- [x] The picker leaves the changed side's version to the server, which resolves
  `CurrentVersionID` rather than the highest number.
- [x] "Copy diff" puts the comparison on the clipboard as a patch.
- [x] The toolbar stacks as two blocks on narrow screens instead of wrapping
  field by field and losing its symmetry.
- [x] A breadcrumb links back to both resources.

## Deliberately not changed

- **Picking a resource on one side still turns a same-resource comparison into a
  cross-resource one.** That is how you start one from a version panel, and
  making both sides follow would discard the selection the reader already made.
  What was actually wrong was that nothing said it had happened; the heading, the
  breadcrumb, both pane labels and the cross-resource badge now do.
- **No per-version thumbnail for binary files.** `/v1/resource/preview` is keyed
  on the resource, and adding a version-scoped preview route is a larger change
  than this pass. The placeholder is honest in the meantime.

## Verification

- `go test --tags 'json1 fts5' ./...`
- `npx vitest run src/components/textDiff.test.ts`
- `./scripts/css-scan-test.sh`
- `cd e2e && npm run test:with-server:all`
- Postgres: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/...`
  and `cd e2e && npm run test:with-server:postgres`
- axe-core over all six compare states (image, text, PDF, binary, cross-resource,
  folded log) at 1440×1000.

New coverage: `server/api_tests/compare_context_versions_test.go`,
`src/components/textDiff.test.ts`,
`e2e/tests/regressions/compare-page-teardown.spec.ts`. The two provider tests
that matter were verified failing against the old behaviour before the fix went
in.

## Review round 1

An independent review of the commit turned up four things. All four are fixed.

- **A resource whose versions were never migrated redirected into an error
  page.** `GetVersions` synthesises a v1 for one, so a version panel has
  something to show, but that row has no id: no file route can serve it and
  `GetVersionByNumber` reads the table, so the new previous-versus-current
  default resolved to it and landed on "version 1 not found". The provider now
  drops the synthesised row (`persistedVersions`), which leaves the version
  select with nothing to offer and the comparison unbuilt, so the empty state
  carries the reason and the select renders disabled instead of empty. Reachable
  with `-skip-version-migration`, where it covers every legacy resource; the
  cross-resource half of it was already broken before this branch.

  Making the comparison *work* for such a resource was considered and is
  separate: it needs a file route that can serve a version with no id, which
  means a second `?disposition=inline` surface and the safelist argument made
  again for it. An explicit `?v1=1` on a legacy resource still errors, the same
  way `?v1=99` errors anywhere.

- **Onion skin stopped overlaying when neither version had stored dimensions.**
  With no ratio the overlay box has no height of its own, so the fallback puts
  the images back in the flow — and it put *both* of them there, which is two
  images stacked with the lower one faded. The image that is meant to sit over
  the other is marked (`compare-overlay-img--over`) and stays out of the flow.
  A class rather than a sibling selector, because toggle mode swaps its two
  images with `x-show` and whichever is visible has to give the box its height.

- **Clearing a picker left it empty over the comparison it still described.**
  A removal names nothing to navigate to, so the handler returned; the page
  always compares two resources, so the side is reloaded back to what it holds.

- **"Copy diff" produced no patch.** Prefixed lines with no file or hunk headers
  read as a patch and apply as nothing. It is `createTwoFilesPatch` now, named
  with the pane titles — folded to ASCII first, because the library escapes a
  non-ASCII name to octal the way git does.

New coverage: `TestCompareContextProvider_UnmigratedResourceHasNothingToCompare`,
the two patch-export vitests, and an e2e that compares two icons — the one image
format in reach that Chromium renders and no decoder in the binary can measure,
so the version rows carry no dimensions. Both were verified failing first.

## Review round 2

One major, and two of the five minors worth acting on.

- **The hash copy button was stored XSS.** The value was written into the
  `@click` expression, and an attribute is HTML-decoded before Alpine parses
  what is left — so `&#39;` becomes `'` and closes the string. A version's hash
  is not a hash by construction: group import, which a plain user may run,
  writes it straight from the manifest with no validation
  (`groupio/apply_import.go`), so a crafted archive runs script in the session
  of whoever compares those versions and clicks Copy. The control reads the
  element that renders the hash instead, so the value never reaches script.
  Introduced by this branch — before it, the hash was truncated with no copy
  control.

- **A failed version read read as "no version history".** Both `GetVersions`
  errors were discarded, which was survivable while an empty list only meant
  the empty state; once `persistedVersions` made an empty list mean "nothing to
  compare, and here is why", an outage started explaining itself as a fact
  about the resource. The error is surfaced.

- **The redirect dropped the requested suffix.** `/resource/compare.json` and
  `.body` are real routes, and a redirect built from a literal path answered a
  client that asked for one of those with a page. It is built from the
  request's own URL now, following `maybeRedirectToLastPage`'s shape, so the
  path and any other query parameters survive and the target stays relative.

Declined, with reasons:

- **`?disposition=inline` accepts `application/pdf;garbage` and does not verify
  the bytes.** The safelist is about the declared type, and the security
  argument rests on the browser honouring that type — which `nosniff`, set on
  every primary-server response, is what guarantees. Bytes are not verified for
  any content type anywhere in this tree; a PDF that will not parse is a broken
  viewer, not an escalation.
- **A single-version resource renders both the "nothing to compare" notice and
  the identical-file report.** That is the designed answer for that case: the
  notice is what explains the report.
- **No "\ No newline at end of file" marker**, and **the slider drag does not
  clean up on `touchcancel` or from `destroy()`.** Both are real and both
  predate this branch; neither is what it changed.

## Review round 3

Two findings arrived as majors and neither survived checking. Three minors were
real.

**Declined: `?disposition=inline` trusts the stored content type.** The claim is
that a crafted import declaring `application/pdf` over HTML bytes gets served
inline and same-origin-framable. It does — and nothing happens, because the
whole safelist rests on the browser honouring the type we declare, which
`X-Content-Type-Options: nosniff` guarantees on every primary-server response.
Tested rather than argued: a file beginning `%PDF-1.4` and continuing
`<html><script>parent.document.title="PWNED"</script>` sniffs as
`application/pdf` on upload, so it reaches the branch without an import at all.
Requested inline, the frame's `document.contentType` is `application/pdf`, it
holds zero scripts and an empty body, and the parent's title is untouched — in
bundled Chromium and in Chrome with a real PDF viewer. What an attacker can
produce is a PDF viewer that fails to parse. Bytes are not verified for any
content type anywhere in this tree, and verifying them would not be what makes
this safe.

**Declined: sorting the version panel's two picks by number is unreliable.** The
scenario given is a merge with "keep the other file as an earlier version",
where the merged-in file takes the highest number while the current version
keeps a lower one. True — but `MergeResources` creates that row without setting
`CreatedAt`, so GORM stamps it *now*: sorting by date orders it exactly the same
way. There is no signal in the schema that says the file is older, so there is
no ordering that is right in every case, and the previous behaviour it replaced
was click order. What was wrong is the comment, which claimed the diff "always
reads older-to-newer" — a promise the code cannot keep. It now says what is
true, and points at the compare page labelling the *current* version rather than
calling either side newer.

Fixed:

- **`contentCategoryFor` did not normalise the type**, so a legitimate PDF
  stored as `application/pdf; charset=binary` was served inline by the file
  route and classified as binary by the page. It goes through
  `models.BaseContentType` now, which is what the file route already does.
- **pongo2's `escapejs` is broken for astral characters.** It emits a bare
  `ὠ0`, which JavaScript reads as `ὠ` followed by a stray "0", so a
  resource named with an emoji reached the pane labels, the patch headers and
  the merge confirmations mangled. Every server value that reaches script in
  these templates goes through `json` instead, which emits a complete quoted
  literal; the merge sentences are built in the provider so they can. Not a
  hole — the fifth character is always a hex digit, never a quote — and the
  other 20 `escapejs` uses in `templates/` have the same weakness and are left
  for their own change.
- **The version panel's checkboxes were not bound to the selection.** Opening
  Compare on a resource with exactly two versions preselects both, and the boxes
  rendered empty over it, so the first click cleared one instead of adding one.
  Pre-existing; one attribute.

## Review round 4

No majors. Six of the seven minors are fixed, and chasing one of them turned up
a control that did nothing.

**Change navigation never scrolled.** `goToChange` looked for the target row
inside `this.$el`, and `$el` read from a method is whichever element's
expression made the call — the toolbar button, which contains no diff. The
counter advanced, `activeChange` moved, the row highlighted, and the page stayed
where it was. The component captures its root in `init`, the one place `$el`
names it, and both lookups go through that. Measured before and after: the first
change sat 892px below the fold and the window never moved; it now scrolls to
centre it.

The same call found the same defect in the fold restore that round 4 asked for,
which is how it surfaced.

- **Opening a fold dropped focus to the document.** Activating the control
  removes it from the page, and in a four-thousand-line diff that costs the
  reader their place. Focus lands on the first line the fold revealed. The rows
  are not in the DOM by the next tick — `x-for` rebuilds a list that size across
  several frames — so it waits for them rather than looking once.
- **The error pages regained the sidebar.** `hideSidebar` was set on the success
  path only, so every early return got the empty 400px column back. Both
  providers set it on the base context, which is every state they can return.
- **The radiogroups ignored Down and Up.** The pattern specifies both pairs.
- **A cancelled drag was never cleaned up.** `touchcancel` now ends it, and
  `destroy` ends one the page is leaving in the middle of, which otherwise left
  the whole page with no text selection and a resize cursor.
- **Two copy timers raced.** A second copy inside the 1.6s window was reported
  as finished by the first one's timer. Both the patch copy and the hash copy.
- **A version select could show what the page did not hold.** When one side has
  no history, the redirect does not run and `v1` stays 0 while the select
  displayed its first option. It carries an explicit "Select a version" row in
  that state, which is the only state it can occur in.

Declined again: **the end-of-file newline is not marked**. `"a"` against `"a\n"`
reads as one line added rather than as a missing final newline. Real, and it
predates this branch — the line splitter is unchanged.

## Review round 5

- **The previous-versus-current default could pick one version twice.** A merge
  renumbers the loser's history above the winner's highest and leaves the
  winner's own current version where it was, so a winner whose current version
  is v1 can hold v2 and v3 as history. `previousVersionNumber` answered "nothing
  is earlier" and the page defaulted to comparing v1 with itself while two other
  versions sat in the dropdown. It falls back to the nearest version *above*
  when there is none below, and returns its argument only when the resource
  genuinely has one version.
- **A self-comparison still ran the comparator**, which fetched one file twice
  to diff it against itself, and offered two identical PDF viewers directly
  under a notice saying there was nothing to compare. One flag now decides both.
- **`SameType` was raw string equality**, so two versions the page routes to the
  same comparator could be reported as a type change. It normalises the way
  `contentCategoryFor` and the file route already do.
- **An end-of-file newline change was invisible.** Splitting on `\n` turns both
  `"a"` and `"a\n"` into `["a"]`, so the pair rendered as an identical removed
  and added row with nothing to say why. The last row of the affected side
  carries the marker, the way a patch does. Raised in three consecutive rounds;
  it predates the branch, and the row model absorbed it without touching the
  fold ranges.

Declined: **the merge buttons are drawn for a principal who cannot write.** True
— and no template in this tree gates a write control on capability; every one of
them is drawn and refused at submit. Gating merge alone would make it the only
such check in the codebase and would not be the doctrine it half-implements.
That belongs to a change that does it everywhere.

## Review round 6

- **Converging both pickers on one resource discarded the other side's
  version.** Round 1 cleared both numbers when a pick made `r1 == r2`, reasoning
  that a version number counted against a different resource means nothing here.
  That is true of the side that changed, which is already cleared, and false of
  the side that did not: its resource has not moved, so its number is as valid
  as it was. Comparing A v1 against B v5 and then picking A on the right threw
  away the v1 the reader had chosen and answered with the page's default. Only
  the picked side moves now.
- **Naming the current version and nothing else compared it with itself.**
  `?r1=42&v1=3` with v3 current filled the empty side with the current version —
  the one already named — and reported "nothing to compare" for a resource with
  two partners available. The empty side takes the nearest other version when
  the fill would collide.
- **An HTTP error left the sibling transfer running.** One side missing means
  there is no diff to build, so the other multi-megabyte body was streaming for
  nobody. The controller is aborted before the error is raised.

Declined:

- **`?disposition=inline` trusts the declared type** — third raising, answered
  with the experiment recorded under round 3. `nosniff` is not a "remaining
  browser-dependent defense", it is the mechanism the safelist is built on, and
  the payload was measured not executing.
- **`escapejs` mangles an astral character in the version panel's delete
  confirmation.** True, and it is one of the twenty remaining `escapejs` uses
  under `templates/`. Fixing it the way the compare page's were fixed means
  building the sentence in a provider, which for a partial shared by another
  page is that change's work, not this one's.
- **Dimensionless images are not registered against each other.** They overlay
  at one origin, which is what round 1 restored; registering two images of
  different shapes needs both intrinsic sizes, and the point of that branch is
  that neither is recorded. Reading them from the images once they load, and
  filling `_sizes` from that, would remove the branch entirely — worth doing,
  and it is new capability rather than a defect in what is here.
