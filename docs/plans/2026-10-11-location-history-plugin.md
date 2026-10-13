# Location History plugin

A bundled Lua plugin, `location-history`, that imports a Google location
export (a Takeout "Timeline" JSON file uploaded as a Resource) and answers two
questions about it:

1. **Where was I at a given date and time?**
2. **How often was I near a given place** (latitude/longitude + radius,
   optionally within a date range)?

## Inputs

The example Takeout the request came with holds `Timeline/Timeline Edits.json`,
`Settings.json` and `Encrypted Backups.txt`. Since 2024 Google keeps the full
timeline on the phone, so Takeout only carries "Timeline Edits": raw position
fixes (`rawSignal.signal.position`), inferred and user-edited visit/activity
segments, wifi scans, activity records and place aggregates.

Formats the importer recognises, detected by their top-level key:

| Format | Detected by | Read |
|---|---|---|
| Takeout `Timeline Edits.json` | `timelineEdits` | `rawSignal.signal.position` points; `inferredSemanticSegment` / `userEditedSemanticSegment` visits and activities |
| Phone export `Timeline.json` | `semanticSegments` | `visit`, `activity`, `timelinePath` points; `rawSignals[].position` points |
| Legacy `Records.json` | `locations` | `latitudeE7`/`longitudeE7` with `timestamp` or `timestampMs` |
| Legacy semantic `YYYY_MONTH.json` | `timelineObjects` | `placeVisit` and `activitySegment` |

Wifi scans, activity records, place aggregates and `semanticLocationStates`
carry no position of their own worth answering from and are skipped.

The Takeout `.zip` itself cannot be read from Lua; the user uploads the JSON
file inside it. (As first built, imports went through `mah.db.get_resource_data`
and its 50 MB cap; `2026-10-11-plugin-streamed-json-import.md` replaced that
with streamed reads.)

## Design

**Capabilities:** `db:read`, `kv`, `actions`, `pages`, `api`, `hooks`. No
network.

**Import** is an async resource action, "Import location history"
(`placement = detail, card`). It reads the file, detects the format, parses it
and writes the result to `mah.kv`, replacing whatever an earlier import of the
same resource wrote. Progress is reported per phase. Timestamps are parsed by a
small ISO-8601 parser in Lua (the VM has no `os.time`), accepting `Z`, `±HH:MM`
offsets and fractional seconds.

**Storage** is `mah.kv`, keyed by resource and UTC month, so a query touches
only the months it needs:

- `imp:<id>` — format, counts, first/last time, usual offset, longest
  segment, import time.
- `idx:<id>` — the month index as columns: months, counts, parts, boxes.
- `d:<id>:p:YYYY-MM:<part>` — positions, sorted, as four number columns
  (`t`, `lat`, `lng`, `acc`), 20,000 per part.
- `d:<id>:s:YYYY-MM:<part>` — segments starting that month, sorted, as one
  column per field, 5,000 per part. A query looking for segments overlapping
  a time looks back as far as the import's longest segment.
- `w:<id>` — set while an import or forget is writing.

(Layout as revised through four review rounds; see
`2026-10-11-plugin-streamed-json-import.md`. Nothing is stored once per month
it spans, and nothing as a table per item, because `mah.kv.get` makes every
returned table cost kilobytes.)

**Scoping.** `mah.kv` is per plugin, not per user. Every read of an import goes
through `mah.db.get_resource(id)` first, which is bound to the acting principal,
so a group-limited user only ever sees imports of resources they can read. The
data is deleted when its resource is deleted (`after_resource_delete` hook) and
from a "Forget this import" button.

**Queries** live on one page, `/plugins/location-history/query` (menu entry
"Location History"), and the same functions back two JSON endpoints,
`GET /v1/plugins/location-history/at` and `.../near`, so they can be scripted.

- *Where was I at T.* `T` is entered as local wall-clock time. Its UTC offset
  is either chosen explicitly or "auto": the offset recorded on the segment
  nearest to that time, else UTC. The answer is the segment covering `T`
  (a user-edited one wins over an inferred one): the place for a visit, or the
  mode, start and end for an activity. Alongside it, the last fix at or before
  `T` and the first after it within ±2 h, with a position interpolated between
  them. If nothing is within ±2 h, the nearest fix either side and how far away
  in time. Every position links to OpenStreetMap.
- *How often near (lat, lng, R).* Over all readable imports (or one), within an
  optional date range: visits whose place is within `R` (count, total time,
  distinct days), and fixes within `R` grouped into "presences" (consecutive
  fixes within `R` no more than 1 h apart). The summary gives distinct days,
  number of presences, total time, first and last seen; below it, the
  presences newest first (capped at 500 rows with the count of the rest). Months
  whose bounding box is farther than `R` from the point are skipped unread.

Distances use the haversine formula.

## Verification

- `plugin_system/location_history_plugin_test.go` loads the bundled plugin in
  a real VM with a mock querier and KV store, with small **synthetic** fixtures
  for each format (the user's own data is not committed):
  - import of each format stores the expected points and segments;
  - "at" inside a visit, inside an activity, between fixes (interpolation),
    with no data near, and with an explicit vs auto offset;
  - "near" counts visits, presences and distinct days, respects the radius and
    date range;
  - a resource the principal cannot read (mock returns nil) yields no data on
    the page and the API;
  - re-import replaces, the delete hook and "forget" purge every key;
  - an unrecognised JSON and a non-JSON file fail the job with a clear message.
- The existing `bundled_plugins_test.go` / `bundled_plugins_egress_test.go`
  cover manifest and egress rules.
- Manual run: build, start an ephemeral server, upload the example
  `Timeline Edits.json`, run the import, query a time on 2026-04-27 and a
  point near a visit, and check the answers against the raw file.
- `go test --tags 'json1 fts5' ./plugin_system/... ./internal/...` before and
  after.

## Docs

- `docs-site/docs/features/built-in-plugins.md`: count, capability table and a
  `## location-history` section.

## Checklist

- [x] Plugin manifest, ISO parser, format parsers, import action
- [x] KV layout, delete hook, forget
- [x] "at" and "near" queries, page and API
- [x] Go tests
- [x] Docs
- [x] Manual run with the example export

## Review

Done as planned, with three changes found along the way:

- A fix outside the radius ends a "near" presence only when its whole
  accuracy circle is outside (`d - accuracy > R`). Without this, ±100 m Wi-Fi
  fixes split one stay into several. Covered by
  `TestLocationHistoryVagueFixDoesNotEndAPresence`.
- Lua patterns treat `°?` as "optional last byte of the UTF-8 sign", so the
  coordinate parser strips `°` before matching.
- The page uses only classes already in `public/tailwind.css` (checked one by
  one); `bg-blue-700`, `ml-6` and `sm:grid-cols-4` were not shipped.

Verified against the example export on a real ephemeral server: 494 positions
and 8 segments imported (matching an independent count of the file); visit,
trip and between-fix answers match the raw timestamps; the page renders. A
synthetic 40 MB `Records.json` (400,000 fixes, 4 years) imported in 37 s, a
full "near" scan took 3.2 s and an "at" 40 ms. Server RSS reached about 3.3 GB
during that import (ephemeral server, files held in memory too): decoding a
large file into Lua tables is the cost of a pure-Lua importer, noted in the
docs.

Not done: an E2E spec (the plugin is covered by Go tests running it in a real
VM, plus the manual run), and no Postgres run, since the plugin touches no SQL
of its own.
