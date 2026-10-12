# Streamed JSON reads for plugins (location-history imports over 50 MB)

The location-history import reads its file with `mah.db.get_resource_data`,
which refuses anything over 50 MB, returns the whole file base64-encoded, and
leaves the plugin to `mah.json.decode` all of it into Lua tables at once. A
58 MB phone `Timeline.json` is refused outright, and a 40 MB synthetic
`Records.json` already pushed the server to ~3.3 GB while importing.

## Change

A new read-only host function:

```lua
local ok, err = mah.db.each_resource_json(resource_id, function(key, index, value)
    -- return false to stop early
end)
```

It streams the resource's file through Go's `json.Decoder` and calls the
function once per piece:

- **Root object:** for each top-level member, if its value is an array, once
  per element with `(key, i, element)`; otherwise once with `(key, nil, value)`.
- **Root array:** once per element with `(nil, i, element)`.

Only one element is decoded at a time, so memory follows the largest element,
not the file. The bytes never pass through base64 or a Lua string.

Rules, matching `get_resource_data` where they overlap:

- Needs `db:read`; reads through the caller's scoped querier, so a resource the
  principal cannot read is an error, exactly as now.
- Refused inside `mah.db.transaction` (file I/O under the writer lock).
- File cap 1 GiB (the decoder holds one element, so the cap is about time, not
  memory). Invalid JSON or a root that is neither object nor array returns
  `nil, err` after the elements already delivered. An error raised in the
  callback propagates as if the callback had been called directly.
- Stops between elements when the VM's context is cancelled (job deadline,
  plugin disabled, shutdown).
- Returns `true` when the whole file was read, or `false` when the callback
  stopped it.

Go side: `EntityQuerier` gains `OpenResourceFile(id) (io.ReadCloser, int64, error)`
in `plugin_system/db_api.go`, implemented in `application_context/plugin_db_adapter.go`
with the same lookup and filesystem path as `GetResourceFileData`. Streaming
lives in `plugin_system/` (new file), not in the adapter.

## Plugin

`location-history` imports through `each_resource_json`, dispatching each
element by its top-level key (`timelineEdits`, `semanticSegments`,
`rawSignals`, `locations`, `timelineObjects`, root array for iOS). Points go
straight into per-month buckets instead of one global list, so the import
holds the parsed points once. The format is decided by the first recognised
key; a file with none is refused as now.

## Verification

- `plugin_system` tests for `each_resource_json`: root object with arrays and
  scalars, root array, early stop, invalid JSON mid-stream (elements before it
  delivered, then the error), scalar root refused, unreadable resource, inside
  a transaction, cancellation stops it, and peak memory: a ~60 MB generated file
  streamed with heap growth far below the file size.
- Existing location-history tests unchanged and green (they exercise every
  format through the new path); the mock querier serves files via
  `OpenResourceFile`.
- Real server: a generated ~60 MB phone-format `Timeline.json` and a ~150 MB
  `Records.json` import within the 5-minute job limit; RSS and time recorded
  before/after against the current code with the 40 MB file.
- `go test --tags 'json1 fts5' ./plugin_system/... ./application_context/... ./internal/...`.

## Docs

- `plugin-lua-api.md`: `mah.db.each_resource_json` under Resource File Access.
- `built-in-plugins.md`: location-history size limit (1 GiB) and memory note.

## Checklist

- [x] `OpenResourceFile` on the querier and adapter
- [x] `mah.db.each_resource_json` + tests
- [x] location-history on the streamed path
- [x] Docs
- [x] Measured run with large files

## Review

Done as planned, plus a second round in the plugin once the profile showed
where the remaining time and memory went:

- **Patterns.** gopher-lua compiles a pattern on every `string.match`, and the
  timestamp and coordinate parsers ran several per point: about a third of the
  import's CPU. The common shapes are now read with `string.byte` and plain
  `find`; anything else falls back to the pattern parser.
  `TestLocationHistoryReadsTimeAndPlaceSpellings` pins both paths.
- **Points as strings.** A Lua table per point cost ~700 bytes. Points are now
  held as fixed-width `"time|lat|lng|acc"` strings (sortable with the built-in
  comparison) and decoded one month at a time while storing. The KV format is
  unchanged, so existing imports still read.

Measured on a real ephemeral server with synthetic Android `Timeline.json`
files (same counts and same answers before and after):

| File | Before | Streamed | Streamed + parser/points |
|------|--------|----------|--------------------------|
| 39 MB, 408k positions | 44.9 s, peak RSS 3833 MB | 28.5 s, 667 MB | 15.9 s, 293 MB |
| 52.5 MB, 543k positions | refused (over 50 MB) | 36.2 s, 849 MB | 20.8 s, 366 MB |
| 254 MB, 2.6M positions | refused | — | 98 s, +576 MB over idle |

At 2.6M positions an "at" query takes 0.1 s, a one-year "near" 1.7 s and a
ten-year "near" 14 s (the page's limit is 30 s). Query speed was not in scope.

`TestEachResourceJSONStreamsInBoundedMemory` walks a 76 MB file with 3 MB of
heap growth.

Not done: a test that cancellation stops the walk between elements. The check
is there, but gopher-lua also stops the callback itself at the next
instruction when the context is cancelled, so a test could not tell the two
apart. No Postgres run: nothing here touches SQL.

## Review round 1 (subagent)

Three P0/P1 findings, all fixed:

- **P0: unbounded nesting on the token path.** A non-array top-level member
  was assembled from `Decoder.Token` recursively with no depth limit; ~7M
  levels of `{` overflowed the goroutine stack, a fatal error that kills the
  process. Now capped at 10,000 levels, as `Decode` is.
- **P1: one huge piece could exhaust memory.** Pieces are decoded whole into Go
  and then Lua values at many times their size. Each piece is now capped at
  16 MiB: a reader window anchored at the decoder's parse offset stops the
  decoder buffering past it, and the parsed size is checked exactly.
  `TestEachResourceJSONRefusesOversizedPieces`.
- **P1: delete hook dropped while the plugin is busy.** The host skips an after
  hook that cannot get the VM within 5 s, and an import holds it for minutes,
  so a resource deleted meanwhile kept its location data. An import now checks
  its resource still exists after storing, and an hourly `mah.schedule` sweep
  (running as the enabling administrator) forgets imports whose resource is
  definitely gone, plus data an interrupted import left without a summary.
  `TestLocationHistorySweepForgetsImportsOfDeletedResources`,
  `TestLocationHistoryImportOfAResourceDeletedMidwayKeepsNothing`. The plugin
  now also declares `schedule`, so an existing install must be re-enabled to
  consent to it.

P2s fixed alongside: text fields coerced at import
(`TestLocationHistoryToleratesMalformedFields`), visits link to the "near"
query as documented, the 1 GiB claim restated as the practical limit, the
first plan's stale design section.

P2s left: a re-import deletes the previous import before writing the new one,
so a re-import that fails part way loses the old one (the sweep then clears the
partial data); Forget while an import of the same resource is queued is undone
when it finishes.

## Review round 2 (fresh subagent)

Three P0/P1 findings, all fixed:

- **P0: bytes do not bound Lua memory.** `goToLuaValue` preallocates ~3 KB per
  table, so 1 MiB of `[[],[],...]` became ~1 GB and a 16 MiB piece got the
  process OOM-killed. Pieces now convert through `streamedValueToLua`, which
  sizes tables exactly and caps a piece at 200,000 values; the byte cap drops
  to 4 MiB (Go's own `[]any` decoding amplifies too). The window gets 4 KiB of
  slack so a piece at the cap followed by whitespace is not refused; the exact
  cap is still checked on the parsed size. `TestEachResourceJSONRefusesPiecesOfTooManyValues`,
  `TestEachResourceJSONByteCapIsExact`.
- **P1: the sweep trusted a principal it could not see.** A schedule runs as
  whoever enabled the plugin; demoted to a scoped user, its sweep would read
  every out-of-scope resource as deleted and forget other users' imports.
  Plugin code cannot see a schedule's principal, so the schedule is gone (and
  with it the `schedule` capability): the sweep runs on page/API requests from
  an administrator with no scope, at most every ten minutes.
- **P1: the sweep could delete an import still writing in another process.**
  Each process has its own VM, so "both hold the VM" did not hold. An import
  now writes `w:<id>` (its start time) before its data and deletes it with the
  summary; the sweep leaves unsummarised data alone while that marker is
  younger than 15 minutes.

P2 fixed alongside: times outside 1990–2100 are dropped (a year-33658
timestamp made the month walk spin until the job timed out), and the user docs
no longer claim a timed-out import "keeps nothing".

## Review round 3 (fresh subagent)

One P0, fixed:

- **P0: queries rebuilt a table per stored point.** `mah.kv.get` converts
  values through `goToLuaValue`, whose ~3 KB-per-table preallocation turned
  each stored `[t, lat, lng, acc]` into a 3 KB table, and `load_points`
  concatenated a whole month: a dense month cost hundreds of megabytes to
  gigabytes per query. Parts are now stored as four number columns
  (`{t, lat, lng, acc}`), queries walk one part at a time (`each_part`), "at"
  binary-searches each part, and "near" keeps its visits and presences as
  number columns joined as they grow, merged into a running summary that keeps
  only the newest rows. `TestLocationHistoryDenseMonthQueriesInBoundedMemory`
  (100,000 fixes in one month, five parts): peak heap growth 17 MB, and "at"
  answers correctly across part boundaries. The stored format changed; this
  branch has not shipped, so there is no migration.

P2s fixed alongside: the sweep re-reads the summary and marker just before
forgetting unsummarised data (an import finishing in another process between
the sweep's two listings), visits are clipped to a "near" date range
(`TestLocationHistoryNearClipsVisitsToTheRange`), and two comments that
understated per-piece amplification.

`goToLuaValue`'s preallocation is unchanged: it is shared host code that every
plugin's `mah.kv.get` and `mah.json.decode` use, and sizing its tables exactly
would be a separate, host-wide change.

Re-measured on the 254 MB export: import 118 s and ~700 MB over idle (98 s /
~580 MB last time, partly run-to-run noise); a ten-year "near" 11 s (was 14 s).

## Review round 4 (fresh subagent)

No regressions from round 3's rewrite (the reviewer checked visit ordering,
presence boundaries across months and parts, the merge, the ring buffer and
the binary search with experiments). Two P1s, both abuse cases a user-role
account could trigger with small crafted files, both fixed:

- **A segment was stored once per month it spanned.** A 1990–2099 visit was
  written into 1,320 months; a 3 MB file could write ~1.5 GB. Segments are
  now stored once, under their start month, as columns in parts of 5,000, and
  each import records its longest segment so queries look back that far.
  `TestLocationHistoryStoresALongSegmentOnce`.
- **Every request loaded every import's per-month summary as tables.** The
  summary is now small (`imp:<id>`), and the month index is a separate
  columnar value (`idx:<id>`) loaded only for imports a query reads. The sweep
  finds interrupted writes through their `w:<id>` markers (a forget now sets
  one too) instead of listing every data key.
  `TestLocationHistoryManyLongImportsStayCheapPerRequest`.

P2/P3s fixed alongside: wrong-shaped objects are skipped rather than failing
the import (`TestLocationHistorySkipsWrongShapedObjects`), tests for the
cross-import merge and ring buffer (`TestLocationHistoryNearMergesImportsAndKeepsTheNewestRows`)
and for "at" across a month boundary, the accuracy is zero-padded so dedupe
keeps the most precise duplicate, "before" is never after a fractional query
time, and the stale storage descriptions.

## Review round 5 (fresh subagent)

A randomized check against a brute-force answer found no mismatches in the new
layout. One P0 and one P1, both memory a crafted file could drive:

- **P0: an import held a Lua table per segment** (~2.7 KB each) until it was
  stored, so a few hundred MB of tiny segments could reach ~10 GB. Segments
  are now packed strings while importing, like points (unpacked one at a time
  when stored), and an import stops and fails past 4,000,000 positions or
  400,000 visits and trips.
- **P1: "near" kept a table of place names per visit.** It now keeps one place
  string per entry and at most five per listed row, tracks the stay being
  built in locals, and reuses the ring's 500 slots, so merging allocates
  nothing per stay. A question stops past 1,000,000 separate visits and stays
  and asks to be narrowed.

Every round so far found the next structure that grew with what a file holds,
so this round adds hard limits rather than only compacting: the three limits
above are plugin settings (`max_positions`, `max_segments`,
`max_near_entries`) with those defaults, which also lets
`TestLocationHistoryLimits` exercise them. `TestLocationHistoryManyVisitsStayCompact`:
100,000 visits import at ~560 bytes each and a "near" over them at ~570.

P2/P3 fixed alongside: a forget no longer takes over or removes an import's
marker in another process, and the sweep removes the stale marker of an
interrupted forget (`TestLocationHistoryForgetMarkers`); a test crosses the
5,000-segment part boundary; query times are clamped to 1990–2100 before the
month search. Memory-measuring tests skip under `-race`.

Left: one very long segment makes "at" scan the import's earlier segments
(time only: 0.75 s with 200,000 visits).

Re-measured on the 254 MB export: import 104 s, ~300 MB over idle (was ~830).

## Review round 6 (fresh subagent): no P0/P1

The reviewer confirmed every earlier fix, checked pack/unpack round-trips with
edge values through the real VM, and compared "near" against an independent
brute force over 12 random seeds (times, totals, days, rows, ring order past 500
rows): all matched.

Open P2/P3s, not yet fixed:

- When visits in one import overlap or touch, "near" names only the first
  visit's place for that row (counts are right). A regression from round 5's
  one-place-per-entry change.
- `text()` strips the separator quadratically and text fields have no length
  cap: a ~6 MB crafted file can hold the plugin's VM until the 5-minute limit,
  and very long names could push a 5,000-segment part past the 8 MB KV value
  limit mid-store.
- Segment UTC offsets are not range-checked at import (the query side checks
  -720..840), so a crafted offset can skew "auto" offset answers for anyone
  who can see that import.
- With more than one server process, a forget or a second import of the same
  resource during an import's write phase can leave an index naming parts that
  were deleted. Worse than the "Forget while queued" case recorded earlier;
  needs `store` to take the marker with compare-and-set and forget to defer
  while an import marker is fresh.
- P3: a forget's marker (0) looks abandoned to a sweep in another process at
  once; more than 4 MiB of trailing whitespace is reported as "data after the
  end of the document"; the docs undersell that a re-import over a limit keeps
  the earlier import; a duplicated heap-sampling helper in the tests; no test
  for invalid limit settings.

## Round-6 follow-ups (fixed before round 7)

- "near" entries keep up to five places as one SEP-joined string, and the
  merge splits them into the row, so merged visits name every place again.
- `text()` strips SEP with one `gsub`, caps fields at 200 bytes on a UTF-8
  boundary (pre-truncating huge inputs first), and drops empty results.
- Segment offsets outside -720..840 or not whole minutes, and distances that
  are negative, NaN or huge, are dropped at import.
- Writes take the `w:<id>` marker with compare-and-set: imports store the
  start time, forgets minus it. A fresh marker held by someone else makes an
  import fail ("try again") and a forget return false (the page says so; the
  delete hook relies on the import's end-of-store recheck). Markers older
  than WRITING_GRACE are taken over with CAS; release deletes only a marker
  still equal to the one taken. The store's recheck removes data while still
  holding its marker. The sweep forgets under the abandoned marker, or clears
  a stale marker whose write did finish.
- Trailing data after the document is checked by reading the rest of the
  file in buffers, outside the piece window, so any amount of whitespace is
  accepted and other data refused without being parsed.
- Docs: a re-import over a limit keeps the earlier import; text and offset
  caps; cross-process waiting. Tests: one heap-sampling helper; invalid
  limit settings; place merging; text/offset caps; overlapping writes;
  trailing whitespace.

## Review round 7 (fresh subagent): no P0/P1

Confirmed the round-6 follow-ups (marker CAS and takeover, recheck under the
marker, UTF-8 cut, offset/distance checks, place join/split, trailing reader),
authorization through the caller's querier, non-reused resource ids, and the
page's accessibility.

Open, not yet fixed:

- P2: JSON escaping (`<` becomes `<`) can grow a 200-byte text field
  six-fold, so a crafted 5,000-segment part can pass the 8 MB KV cap (18 MB
  reproduced); the store then raises after `remove_data`. Fix: smaller
  SEGMENT_CHUNK (~1,500) or flush on a running size estimate, plus a test KV
  mock that enforces the cap.
- P3: a store that raises (timeout, KV error) leaves its marker for 15
  minutes, blocking re-import and Forget in the meantime; the stream window
  is not re-opened after a piece, so a near-4 MiB piece followed by >4 KiB of
  whitespace is refused; the sweep's get-then-delete release can drop a
  marker another process just took over (microsecond window); marker ages
  trust each process's clock; "at" accepts an out-of-range offset typed into
  the time (display only); the 800-byte pre-cut in `text()` can leave a
  partial UTF-8 character.
