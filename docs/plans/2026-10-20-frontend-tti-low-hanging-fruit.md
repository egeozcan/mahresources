# Front-end load time: low-hanging fruit

Goal: pages become interactive (Alpine initialized) sooner, with no behavior change a user would notice.

Success criterion: `alpine:initialized` fires earlier on list, detail, and form pages, measured with identical data on the current build and on `master`. The full E2E suite (browser and CLI) and the Go tests stay green.

## How it was measured

- Two ephemeral servers ran the same binary with identical seeded data: a category with a MetaSchema, two groups, a tag, a note, and two PNG resources.
  - One server ran from a `master` worktree.
  - The other ran from the working tree.
- Playwright drove headless Chromium with 4x CPU throttling. Each page got 9 fresh contexts.
- Two numbers are reported per page, both as medians:
  - cold: the `alpine:initialized` time on first load;
  - warm: the same after a reload.
- A CPU profile and a per-root binding count located the cost. An HTML-stripping experiment, which served each page with one overlay removed, priced each overlay.

## What the time was spent on

Profiled on `master` at /resources, warm cache, 4x throttle:

| Phase | Time |
| --- | --- |
| HTML parse finished (domInteractive) | 60 ms |
| main.js evaluated, up to `alpine:init` | +9 ms |
| `Alpine.start()` | +72 ms |

Bundle size was not the bottleneck. The page was 408 KB of HTML for two resources. Over half of it was overlays rendered into every page and hidden with `x-show`. Alpine still walks and binds hidden markup at startup:

| Root | Live bindings | Alpine.start cost (strip experiment) |
| --- | --- | --- |
| lightbox | 291 | 14–20 ms |
| list bulk editors | 238 | ~18 ms |
| paste-upload modal | 80 | 6–9 ms |
| entity picker | 38 | ≤ 5 ms |

## Changes

- [x] **The lightbox, the paste-upload modal and the list bulk editors render on first use, not with the page.**
  - Their contents are wrapped in `<template x-if>` on a flag that their own `x-effect` sets the first time `$store.lightbox.isOpen`, `$store.pasteUpload.isOpen` or a non-empty selection is seen.
  - Once rendered, they stay rendered, hidden by `x-show` or `:hidden` exactly as before. So everything after the first use behaves as it did on `master`.
  - Tearing them down on close was tried first and failed review:
    - The /jobs bulk commands keep focus through a keeper that has to outlive the selection it empties (`e2e/tests/jobs/job-commands-ux.spec.ts`).
    - `freeFields` in the bulk Meta editor adds a window listener it never removes.
    - The paste modal's selectors left two `scroll` and two `resize` window listeners behind per open/close cycle.
  - Deselect All, Select All and the selection count stay outside the bulk template, so the bar's DOM order is unchanged (`e2e/tests/regressions/ws2-select-all-rejection.spec.ts`).
  - A `display: contents` wrapper keeps the children laid out as before. That covers the flex items of the lightbox root and `min-h-full` in the paste modal.
  - In the bulk toolbar the wrapper is `.bulk-editors-items`, and `public/index.css` gives its children the same `order: 1` that `.bulk-editors > *` gave them.
- [x] **Template pages are gzipped.**
  - `compressPage` (`server/static_delivery.go`) wraps every `RenderTemplate` route: the page, its `.json` and its `.body`. It reuses the static file gzip writer, so only a 200 of a text type is compressed.
  - Pages are rendered whole by pongo2 before writing, so nothing streams through it. The API, SSE and file routes are untouched.
  - /resources went from 420 KB to 52 KB on the wire.
- [x] **The Google Fonts stylesheet no longer blocks rendering** (`media="print" onload`).
  - A pending stylesheet also holds back module scripts. On a private network with no internet, every page's scripts waited for that request to fail.
  - The primary server sends no CSP, so the inline `onload` is allowed.
- [x] **Image hints.**
  - The resource page's main preview was `loading="lazy"` although it is the largest above-the-fold image. It is now `fetchpriority="high"`.
  - Every lazy-loaded `<img>` in the templates gained `decoding="async"`.

## Rejected

- **Splitting page-specific Alpine components into lazy chunks.**
  - Built and measured. main.js went from 820 KB to 469 KB (231 KB to 141 KB gzipped), but every page got slower: 15–30 ms warm.
  - Module scripts run only after the HTML is parsed. A chunk that a page's markup needs can only be requested after that, which adds a round trip before Alpine can start.
  - Evaluating main.js took about 9 ms, so there was little to win.
  - It also needed a `main.js` stub, because the lazy chunks import the entry by its plain URL while templates load `main.js?v=…`. Without the stub, a page would run two copies of the app.
- **`--minify` for Tailwind.** `tailwind.css` is already gzipped (about 20 KB on the wire) and is committed, so minifying would mostly cost readable diffs.
- **Entity picker `x-if`.** Its dialog controller binds to the DOM in `init()`, and the measured gain was at most 5 ms.
- **Immutable caching for `main.js?v=`.** It is rejected in `cacheHashedBuildAssets`'s comment for a documented reason: the version is fixed at startup, so `npm run dev` rebuilds would go stale.

## Review

Fair comparison, `master` vs this change: `alpine:initialized` medians in ms, 4x CPU throttle, same data.

| Page | master cold | new cold | master warm | new warm |
| --- | --- | --- | --- | --- |
| /resources | 293 | 247 | 130 | 102 |
| /note?id=1 | 258 | 222 | 102 | 87 |
| /group?id=2 | 201 | 169 | 98 | 82 |
| /category/edit?id=4 | 309 | 283 | 177 | 159 |
| /jobs | 212 | 195 | 97 | 77 |
| /mrql | 230 | 185 | 102 | 83 |

A browser smoke run over 56 page routes found no console errors and no `x-data` element left uninitialized.

Visible behavior change: none intended after first use. The first open of the lightbox or the paste modal, and the first selection on a list page, now builds that markup at that moment. That cost moved out of every page load.

Window listeners were counted over five open/close cycles of the lightbox (with the quick-tag panel) and of the paste modal. Neither count grows.
