# Nearby search map picker

## Success criterion

The "How often was I near…?" section has its own map picker alongside the
existing latitude/longitude and radius inputs. Clicking either map or selecting
a recorded location updates both map selections and the coordinate input.
Typing coordinates or changing the radius immediately updates the picker.
Counting uses the existing query and updates without reloading.

## Behavior and tradeoffs

- Add a compact Leaflet map in the nearby form, with a distinct accessible
  region label and a "Use picker center" button. The keyboard can pan/zoom the
  picker and select its center. Map controls are hidden without JavaScript;
  ordinary coordinate-based GET searches remain available.
- Share the coordinate/radius state between the two maps, with separate
  selection layers. Center the picker on typed or selected coordinates and
  fit the search circle. Clearing/invalidating coordinates clears both maps'
  selection. Selection does not submit a search until Count is pressed.
- When no coordinates have been chosen, initialize the picker from the first
  successfully loaded day's bounds. Keep a viewport the user has already
  interacted with while that request was pending. No additional history API
  request is needed for the picker.
- The two real tile-layer consumers share setup, including origin-only
  identification, caching defaults, attribution and independent tile warnings.
  Preserve the existing request cancellation and back-cache lifecycle behavior
  for both maps.

## Work and verification

- [x] Baseline existing Location History browser tests.
- [x] Add regressions for the embedded picker, click/input synchronization,
      radius updates, keyboard selection and ordinary Count submission.
- [x] Add markup, shared tile setup and selection rendering, styling and fixture.
- [x] Verify both maps' identification, error recovery, accessibility, mobile
      widths and operation without JavaScript.
- [x] Update plugin documentation, review, and run appropriate checks.
- [x] Prepare deployment to both master remotes under existing authorization.

## Review

Two independent review rounds: the first found one P3 (picker instructions
visible without JavaScript), which was fixed and covered by the no-JavaScript
regression. The second found zero P0, P1, P2 or P3 issues.

Verification uses ephemeral servers and synthetic imports, with tile requests
mocked. The 19-test baseline passed; the final expanded browser run passed all
22 tests with retries disabled. Focused Location History Go tests, the full Go
suite (`json1 fts5`, Go 1.26.4), and the CSS source-scan checks passed.

The full browser/auth/CLI/CLI-doctest suite completed with 2,474 passed,
5 skipped and 3 modal-focus checks passing on retry. Separate repetition
reproduced the Image Actions failure: Tab raced Alpine's delayed trap
activation. Narrow test-readiness fixes now wait for the sidebar traps'
autofocus and for settled initial Search focus, preserving immediate assertions
after every Tab. All 21 tests in the three affected files passed without
retries; both sidebar checks also passed 10 repeats each. These verification
fixes are committed separately from the picker.
Search also passed 10 repeats without retries after its readiness fix. A final
review of the picker and verification fixes found zero issues at every priority.

No database behavior changed. Postgres checks were not run because Docker is
unavailable in this environment.
