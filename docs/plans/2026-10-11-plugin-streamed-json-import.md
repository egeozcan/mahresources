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
