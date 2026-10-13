# Location History tile identification

## Success criterion

OpenStreetMap tile requests send the app's origin as Referer, even with the
primary server's `Referrer-Policy: no-referrer` header. The header must not
include the plugin path, source resource, coordinates, time or other query
parameters. The app's global privacy headers remain in effect for other images.

## Reproduction and diagnosis

The browser regression in `plugin-location-history.spec.ts` intercepts actual
tile requests and implements OpenStreetMap's documented identification check:
an origin Referer yields 200; missing identification yields 403. The test ran
red against the deployed code: every captured tile response was 403.

`withPrimarySecurityHeaders` sets `no-referrer` on every page. The tile layer
did not override that document policy, so the browser sent no Referer. Browser
caching remains at its normal defaults; no network proxy or tile scraping is
involved. The screenshot alone cannot identify a provider-side IP block, so
verification distinguishes the reproduced application defect from such blocks.

## Implementation and verification

- [x] Capture actual browser request headers in a failing regression.
- [x] Set Leaflet's per-image `referrerPolicy` to `origin` for this tile layer.
- [x] Mirror the plugin fixture and explain origin-only identification in docs.
- [x] Confirm the policy regression and all Location History browser tests pass.
- [x] Check fixture parity and existing main/share security-header tests.
- [x] Review the fix and prepare deployment to origin/master and mahlayf/master
      using the user's existing authorization.

## Review

Independent subagent review found zero P0/P1/P2/P3 issues. The browser regression
went red with 403 responses before the fix, then passed with an origin-only
Referer. All 19 map browser tests passed; the Location History Go tests (including
fixture parity) and primary/share security-header tests passed using Go 1.26.4.
The global `no-referrer` header remains in place. This is a focused correction;
the full suites passed during the preceding feature work. Postgres remains
unavailable without Docker.

Policy references: https://operations.osmfoundation.org/policies/tiles/ and
https://wiki.openstreetmap.org/wiki/Blocked_tiles . Tests mock the tile service
and use ephemeral servers with synthetic location data; no automated traffic
was sent to the public tile service. Provider-side IP blocks or browser privacy
extensions cannot be ruled out from the user's screenshot alone.
