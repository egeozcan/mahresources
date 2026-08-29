# Expose media processing to plugins

## Context

The request started as "expose ffmpeg to plugins". The driving case is a video-downloader
plugin that today gets as far as decoding an obfuscated payload into an `.m3u8` URL and then
stops, because the sandbox structurally cannot do the next three steps: fetch hundreds of
binary `.ts` segments (`mah.http` caps bodies at 5 MB and every call serialises on the one VM
lock), write them somewhere (no `io`/`os`, by design), and run ffmpeg (no process execution,
by design). The 5-minute async-job window makes the serialised version impossible anyway.

Handing Lua raw ffmpeg arguments would answer the literal request and punch three holes in
this tree's own threat model at once: `-i http://…` bypasses the per-plugin egress policy and
the `-allow-private-fetch` deny, `-i /path` gives the VM its first filesystem read, and an
output path is an arbitrary file write. Argument allowlisting for ffmpeg is not winnable
(concat demuxer, `lavfi`, playlist formats). Confirmed with the user: curated operations only.

So the answer is not to expose the tool, but the three powers the plugin actually lacks, each
at a seam that already exists:

1. **HLS ingest inside the host's own downloader** — the missing 90% of the use case, and it
   arrives with no new capability at all, because the host already fetches URLs into the
   library on a plugin's behalf under a consent label that says so.
2. **`mah.download.submit`** — the same power as `mah.db.create_resource_from_url`, taken off
   the VM lock and off the 5-minute clock.
3. **`mah.media`** — probe / extract frame / trim, on resources already in the library.

## Part 1 — Host-side HLS ingest

### Where it goes

There are **two** independent fetch paths and both need it:

- `application_context/resource_upload_context.go:216` `AddRemoteResource` — the sync path
  behind `/v1/resource/remote` and `mah.db.create_resource_from_url`.
- `download_queue/manager.go:600` `downloadWithProgress` — the queue's own HTTP, which does
  **not** go through `AddRemoteResource`.

`download_queue` sits below `application_context`, so the shared code cannot live in either.
New leaf package **`hls/`**, in the shape `groupio/` and `search/` already establish: no
dependency on `application_context`, `server` or `contracts`, and it takes its dependencies
**per call**, never at construction:

```go
package hls

type Deps struct {
    Client     *http.Client // already policed by the caller
    FfmpegPath string
    TempDir    string
}

// IsPlaylist sniffs #EXTM3U from bytes already read off the response.
func IsPlaylist(head []byte, contentType, url string) bool

// Fetch resolves the playlist, downloads every segment through Deps.Client,
// muxes to MP4 and returns a reader over the result plus a cleanup func.
func Fetch(ctx context.Context, d Deps, playlistURL string, head []byte,
    body io.Reader, opt Options, p Progress) (io.ReadCloser, func(), error)
```

`internal/arch/layering_test.go` gets an entry pinning `hls/` below the two consumers, matching
`TestGroupioStaysBelowApplicationContext` (layering_test.go:184).

### What it does

1. **Sniff, don't guess.** Read the first bytes of the response the caller already opened and
   look for `#EXTM3U`. Extension and content-type are hints only — these URLs routinely end in
   neither. Not a playlist ⇒ hand the untouched bytes back and the caller streams as today
   (the `head` parameter exists so nothing is lost to the sniff).
2. **Master playlist** ⇒ pick a variant by `Options.MaxHeight`/highest-bandwidth default,
   resolve relative URIs against the playlist URL, fetch the media playlist.
3. **Segments** fetched through `Deps.Client` — the same already-policed client, so every
   segment gets layers (a), (b) and (c) of `plugin_system/egress.go`: allowlist, per-redirect
   re-check, and the dial-time deny on the resolved address. This is the whole reason we fetch
   the segments ourselves instead of handing ffmpeg the playlist URL.
   Bounded concurrency (4), written to `os.MkdirTemp` under the configured temp dir, retried a
   small bounded number of times per segment.
4. **AES-128** (`EXT-X-KEY`): fetch the key through the same client, write it beside the
   segments, and rewrite the local playlist's `URI=` to the local file. ffmpeg then needs
   `crypto` and `file` and no network. `SAMPLE-AES` and anything naming a DRM system are
   refused with a message that says it is DRM, not a parsing gap. A media playlist with no
   `EXT-X-ENDLIST` is a **live stream** and is refused by name: the byte and segment caps would
   bound it, but only into an arbitrary clip of whichever rolling window happened to be current,
   which is a confusing partial result rather than an answer.
5. **Mux** — `ffmpeg -nostdin -protocol_whitelist file,crypto -i local.m3u8 -c copy
   -bsf:a aac_adtstoasc -movflags +faststart -f mp4 out.mp4`, `exec.CommandContext` on the
   **caller's** context. `-c copy` means no re-encode: minutes of video mux in seconds.
   `requireFfmpeg` semantics before any fetching (mirroring the comment at
   resource_media_context.go:1937), and `truncateStderr` on failure.
6. Return a reader over the temp mp4 + a cleanup; both callers already end at
   `AddResource(reader, …)`, and they set name/extension `.mp4`, content type `video/mp4`,
   exactly as `TrimVideo` does at resource_media_context.go:1988.

Caps, all configurable, all refusing rather than truncating: max segments, max total bytes,
max playlist depth (a master pointing at a master).

### Progress

The queue's `DownloadJob` already carries `Phase`/`PhaseCount`/`PhaseTotal` (job.go:51). The
`hls.Progress` callback drives them: `segment 42/380`, then `muxing`. The sync path passes a
nil progress. `TotalSize` is unknown until the playlist is parsed, which is what
`TotalSize: -1` already means.

### Parsing

Add `github.com/grafov/m3u8` (MIT, the de-facto Go HLS parser) rather than hand-rolling
master/media/`EXT-X-MAP`/`EXT-X-BYTERANGE`/`EXT-X-KEY`. A hand parser is ~250 lines of the
kind of code that is wrong on exactly the playlists we care about.

### Falls out for free

`/v1/resource/remote`, the download queue's own URL box, the `mr` CLI and
`mah.db.create_resource_from_url` all accept an m3u8 afterwards, with **no new capability**:
`CapDBWrite`'s label already reads "…and fetch a URL of its choosing into it", and the power
granted is unchanged — one URL in, one resource out.

## Part 2 — `mah.download.submit(url, opts)`

Enqueues a real download-queue job instead of fetching inline. Host-run, so no VM lock and no
`MaxAsyncJobDuration`; it inherits progress, SSE, the history row, retry and
`after_job_completed` (which already carries the created resource id), so a plugin holding
`job_events` can tag the result when it lands.

**Capability: `CapDBWrite`.** Deliberately not a new name and not `CapJobs`. The power is
identical to `create_resource_from_url` — fetch a URL into the library — differing only in
whether the caller waits. `CapJobs` means *plugin code* runs in the background; here no plugin
code runs at all, the host downloads. Refused inside `mah.db.transaction` via the existing
`refusedInTransaction(…, whyItWaits)` shape (db_api.go:758) — the reason it exists is a
transaction held across a fetch.

**Two things must be got right, and both are the same confused-deputy hazard:**

- **The job carries the plugin's egress policy, not the host's.** `dm.clientPolicy` is
  manager-wide and is the *host* policy, which allows every public host — wider than the
  plugin's declared `network` list. Add an optional per-job policy consulted by
  `createHTTPClient`, and make a plugin-origin job with no policy **refuse**, mirroring the
  `pluginFetch` fallback refusal at resource_upload_context.go:245.
- **The policy is re-derived on every attempt, from the plugin name.** A retry replays a stored
  payload on an unscoped worker, possibly after a restart, so a captured policy value would be
  stale or gone. Store the plugin name on the job and look the live manifest up at fetch time:
  a plugin disabled meanwhile refuses, which is the correct answer and self-healing. This is
  the same rule `validateDownloadScope` applies to scope — a payload records what was once
  asked for, not a standing permission. `download_queue` cannot import `plugin_system`, so the
  lookup arrives as an injected `PolicyForPlugin(name) (ClientPolicy, bool)` on `ManagerConfig`,
  wired where `ClientPolicy` already is; `false` means refuse.

Layer (a) is also checked **at submit**, as `create_resource_from_url` checks before creating
anything (db_api.go:770), so a URL the plugin may not reach is refused at the call site instead
of succeeding into a history row that fails minutes later.

The submitting principal is the job owner (`ownerUserID`), as `/v1/download/submit` does, so
`jobVisibleToPrincipal` shows it to the user who triggered it.

## Part 3 — `mah.media`, capability `media`

New `CapMedia = "media"` in `plugin_system/manifest.go`, plus its entries in
`AllCapabilities`, `CapabilityLabels` ("Read and cut audio and video in your library") and
`CapabilitySurfaces` — `manifest_test.go:50` iterates the list, so a missing entry fails.

New seam in `plugin_system` next to `KVStore`/`PluginLogger`, `SetMediaProcessor` +
`atomic.Value`, implemented in `application_context` — plugin_system stays exec-free:

```go
type MediaProcessor interface {
    Probe(ctx context.Context, resourceID uint) (map[string]any, error)
    ExtractFrame(ctx context.Context, resourceID uint, atSeconds float64) (string, error) // data URI
    TrimToResource(ctx context.Context, resourceID uint, start, end float64, opts map[string]any) (map[string]any, error)
}
```

- `probe` → ffprobe JSON (duration, streams, codecs, resolution, fps, bitrate), via the
  existing `ffprobePath` (resource_media_context.go:2227).
- `extract_frame` → a data URI, composable with `mah.image`; reuses the existing frame
  extraction (`extractVideoFrame`, resource_media_context.go:1058).
- `trim` → wraps `TrimVideo` (resource_media_context.go:1941); `{into = "version"|"resource"}`.

Non-negotiables, each with a test:

- **Principal-bound input.** The resource read must go through the bound adapter the way
  `GetResourceFileData` does (`plugin_db_adapter.go:586`), or a scoped plugin caller probes and
  trims resources outside its subtree.
- **The invocation's context, never `ctx.db.Statement.Context`** (which is always Background).
  A schedule handler blocked in ffmpeg would otherwise outlive `MaxAsyncJobDuration` and break
  the claim-TTL inequality `TestScheduleClaimTTLExceedsTheLongestPossibleRun` pins.
- **Refused inside `mah.db.transaction`**, same shape as above.
- **Bounded concurrency and timeout**, sharing the existing video-thumbnail semaphore and
  `VideoThumbTimeout`, so plugins cannot fork-bomb ffmpeg.
- `requireFfmpeg` first, so a server without ffmpeg says so instead of failing obscurely.

## Files

| Area | Files |
|---|---|
| New package | `hls/` (playlist, fetch, mux, tests) + `internal/arch/layering_test.go` entry |
| Sync fetch | `application_context/resource_upload_context.go` (HLS branch before `AddResource`) |
| Queue fetch | `download_queue/manager.go` (HLS branch, per-job policy), `download_queue/job.go` |
| Plugin surface | `plugin_system/manifest.go`, new `plugin_system/media_api.go`, `plugin_system/download_api.go`, `manager.go` wiring |
| Host impl | `application_context/plugin_media_adapter.go`, `plugin_db_adapter.go` |
| Config | `-hls-max-segments`, `-hls-max-bytes`, `-hls-concurrency` in `main.go` + `application_context/context.go` + the CLAUDE.md config table |
| Docs | `docs-site/docs/features/plugin-lua-api.md`, `plugin-permissions.md`, `plugin-system.md`, CLAUDE.md |
| Tests | `capability_grants_test.go`, `manifest_test.go`, `bundled_plugins_egress_test.go`, new suites per part |

## Verification

1. `go test --tags 'json1 fts5' ./...` — including the arch guards (`internal/arch`), which is
   where a layering or capability-list mistake surfaces.
2. **HLS unit tests against an `httptest` server**: master → variant → 3 segments → mp4;
   AES-128 with a served key; a segment that 404s; a live playlist with no `EXT-X-ENDLIST`
   refused; a playlist pointing at `127.0.0.1` proving
   the egress deny fires **per segment**, not just on the playlist URL (the test that matters
   most — it is the one hole this design exists to avoid).
3. **Policy test**: a plugin-submitted queue job whose plugin declares `network = {"a.example"}`
   is refused a segment on `b.example`; the same job after the plugin is disabled refuses
   outright.
4. Lua integration tests in `plugin_system` for `mah.media.*` and `mah.download.submit`:
   ungranted capability ⇒ key absent; inside a transaction ⇒ refused; scoped principal ⇒ cannot
   probe a resource outside its subtree.
5. Manual end-to-end: `./mahresources -ephemeral`, paste a public HLS URL into the download
   box, watch `segment n/N` then `muxing` in the jobs panel, confirm a playable mp4 resource.
6. `cd e2e && npm run test:with-server:all`, then the Postgres suites.

## Sequencing

On approval the plan is copied to `docs/plans/2026-08-29-plugin-media-hls.md`, which is where
this repo keeps its record (`docs/todo.md` is a parsed ledger, not a plan file).

Part 1 first and merged on its own — it is the whole of the use case that does not need a new
plugin surface, and it is independently useful to every user. Part 2 next (it depends on Part 1
being in the queue path to be worth much). Part 3 last; it touches nothing the first two need.
