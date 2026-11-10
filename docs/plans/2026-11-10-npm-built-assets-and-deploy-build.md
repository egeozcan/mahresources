# Plan: build frontend assets from npm instead of committing them, and build on deploy

## Context

The main app already takes every JS library from npm through Vite. Three things
are still committed as copied or generated files:

1. **Leaflet 1.9.4**, hand-copied into `plugins/location-history/public/vendor/`
   and mirrored in `e2e/test-plugins/location-history/public/vendor/` (10 files
   each, 416 KB).
2. **`public/dist/`** (Vite output, 21 files, 1.6 MB), restaged by
   `.husky/pre-commit` and by a `PreToolUse` hook in `.claude/settings.json`.
3. **`public/tailwind.css`** (Tailwind output), restaged by the same pre-commit
   hook.

Decided with the user: all three come from npm at build time and leave git, and
the production host's deploy hook builds them.

Out of scope: `plugins/ketchup/public/ketchup.js`. It is built from a separate
repository by `scripts/update-ketchup.sh`, is not on npm, and stays committed.

## What the survey found

- Nothing embeds the assets (`go:embed` is used only for CLI help). The server
  reads `public/` and `plugins/` from disk, so a checkout without a build serves
  pages with no CSS and no JS, and logs nothing about it.
- No Go test reads the built files. `server/static_delivery_test.go` uses an
  in-memory fs, and `api_tests/public_assets_cors_test.go` states that its
  working directory has no `./public`. The CI `test` job runs without npm and
  stays that way.
- Every E2E path already builds: `e2e/scripts/run-tests.js` runs the root
  `npm run build` unconditionally, and the CI E2E jobs run `npm ci` first.
- `Dockerfile` already builds CSS and JS in its own stage. It does not ship
  `plugins/`, so Leaflet does not concern it.
- `TestLocationHistoryE2EFixtureMatchesBundledPlugin` walks both plugin
  directories on disk and compares every file. With both vendor directories
  absent (CI `test` job) or both populated by the same copy step, it passes.
- **The production host** is a non-bare checkout that receives pushes with
  `receive.denyCurrentBranch=updateInstead`. Its `post-receive` hook builds the
  Go binary, swaps it and restarts the service. The service runs from that
  directory, so it serves `public/` and `plugins/` from the checkout. The hook
  exists only on the host.
- **The host's Node is too old.** Vite 8 requires `^20.19.0 || >=22.12.0`, and
  Tailwind v4's CLI also needs a current Node. The host cannot build the
  frontend today.

## Changes

### 1. Leaflet from npm

- [ ] Add `leaflet` pinned to exactly `1.9.4` in `dependencies`. The commit
      message says why: it replaces the same version already vendored, so no
      new code enters the project.
- [ ] Add `scripts/copy-plugin-assets.mjs` (Node built-ins only). It copies
      `leaflet.js`, `leaflet.js.map`, `leaflet.css` and `images/` from
      `node_modules/leaflet/dist/`, plus `LICENSE`, into both vendor
      directories. It exits non-zero when a source file is missing.
- [ ] Add `"build-plugins": "node scripts/copy-plugin-assets.mjs"` and put it
      into `build` (and the `watch` command) so every existing build path picks
      it up.
- [ ] `git rm` both vendor directories and gitignore them. `plugin.lua` keeps
      its `/plugins/location-history/public/vendor/...` URLs, so no plugin code
      changes.
- [ ] The vendored `README.md` goes away with the directory. Its still-true
      content (why Leaflet, OSM tile policy link) moves to the header comment
      of the copy script.

### 2. Stop committing `public/dist/` and `public/tailwind.css`

- [ ] `git rm --cached` both, add them to `.gitignore`.
- [ ] `.husky/pre-commit`: remove the frontend rebuild-and-stage block. The
      OpenAPI block stays.
- [ ] `.claude/settings.json`: remove the `git add -A public/dist/` hook.
- [ ] Startup warning: when `public/dist/main.js` or `public/tailwind.css` is
      missing at startup, log one line naming the file and `npm run build`.
      This is the only new Go code. It exists because un-committing the files
      creates a new state (fresh clone, `go build`, run) that today cannot
      happen and would otherwise fail silently. Test first: a unit test on the
      check function with a temp dir.
- [ ] Docs that currently say the output is committed: `AGENTS.md` (the
      `public/` section and the merge-conflict advice), `README.md` line 162,
      the comment at `.github/workflows/ci.yml:213`. Each is rewritten to match
      the new behavior, nothing else on those pages changes.

### 3. The production host builds on deploy

- [ ] Install a current Node LTS on the host from the official tarball, in a
      user-owned directory (no sudo). The system Node is left alone; only the
      hook's `PATH` sees the new one.
- [ ] Version the deploy script in the repo as `scripts/deploy/post-receive`.
      It is generic: it names no host, path or service. The host's hook becomes
      a shim that sets those through environment variables and `exec`s the
      script from the checkout. The hook already builds and runs whatever is
      pushed, so this adds no trust, and hook changes become reviewable diffs.
- [ ] New hook sequence, for a push to `master`:
      1. `npm ci` with `HUSKY=0`, only when `package-lock.json` differs from a
         stamp copy kept in `node_modules/` (skips about a minute on most
         deploys).
      2. Build Tailwind, Vite and the Go binary into a gitignored scratch
         directory inside the checkout (`.deploy-build/`), so the swap is a
         same-filesystem rename and Tailwind's scan does not read it.
      3. If all three succeed: copy plugin assets, swap the three outputs into
         place with `mv`, restart the service.
      4. If any step fails: delete the temporary outputs, print
         `!! BUILD FAILED`, exit 1. The running binary and the assets it serves
         are untouched.
- [ ] The Go build tags come from the shim, so the host keeps the tags it
      builds with today.

## Assumptions and tradeoffs

- **One-time degraded window on the transition push.** `updateInstead` updates
  the checkout before `post-receive` runs, and the commit that untracks
  `public/dist/` makes git delete those files on the server. The site has no
  CSS or JS from that moment until the hook's build finishes. I will run
  `npm ci` on the server before that push so the window is the build only
  (estimated under a minute; I have not measured it on that machine). After the
  transition the files are ignored, git never touches them, and each deploy
  swaps them atomically.
- **Templates still change before the binary does.** `updateInstead` puts new
  templates on disk before the build finishes. That is how deploys work today
  and this plan does not change it.
- **Deploys get slower**, by the frontend build on every push and by `npm ci`
  when the lockfile changes.
- **A failed frontend build now blocks a deploy.** Today only a Go build
  failure does.
- **A fresh clone or worktree needs `npm ci && npm run build`** before the
  server is usable. The startup warning covers the case where someone forgets.
- **Leaflet copies are build output in two places.** The parity test still
  compares them when present; when both are absent it has nothing to compare.
  The E2E suite (`plugin-location-history.spec.ts`) is what fails when the copy
  step did not run.
- **Merge conflicts on minified output disappear**, as does the shared-index
  race where one session's commit swept in another's rebuilt bundle.

## Verification

Done means:

- [ ] Baseline before any change: Go unit tests, `npm run test:unit`, and
      `cd e2e && npm run test:with-server:all` on current `master`.
- [ ] A fresh `git clone` of the branch into a scratch directory, then
      `npm ci && npm run build`, produces `public/dist/main.js`,
      `public/tailwind.css` and both Leaflet directories, and `git status` in
      that clone is clean (the ignore rules cover everything generated).
- [ ] In that clone, before `npm run build`, the server logs the missing-asset
      warning; after it, it does not.
- [ ] Same three suites pass after the change, plus the Postgres Go and E2E
      runs.
- [ ] `docker build .` succeeds.
- [ ] `./scripts/css-scan-test.sh` passes.
- [ ] The deploy script, run locally against a scratch clone with the restart
      command stubbed: a good push swaps all three outputs; a forced Vite
      failure and a forced Go failure each leave the previous outputs
      byte-identical.
- [ ] On the host after the real push: hook prints the deployed sha, the site
      loads with styles and JS, `/plugins/location-history/public/vendor/leaflet.js`
      returns 200, and free disk space is unchanged.

## Order of work

1. Branch `chore/npm-built-assets`. Sections 1 and 2 plus the deploy script,
   with tests, as separate commits.
2. Local verification as listed.
3. With your go-ahead for each server step: install Node on the host, install
   the shim hook, pre-run `npm ci`, then merge and push.

## Review

(Filled in when the work is done.)
