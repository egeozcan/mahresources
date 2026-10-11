-- Location History: import a Google location export and ask it where you were.
--
-- The import is an async action on the uploaded JSON file. It parses the file,
-- and keeps the positions and visit/activity segments in mah.kv, bucketed by
-- UTC month, under keys that name the resource:
--
--   imp:<id>                   summary: format, counts, range, months
--   d:<id>:p:YYYY-MM:<part>    positions {t, lat, lng, accuracy_m}, sorted
--   d:<id>:s:YYYY-MM           segments overlapping that month
--
-- mah.kv is per plugin, not per user, so every read goes through
-- mah.db.get_resource first: that call is bound to the acting principal, and an
-- import is only ever shown to someone who can read the resource it came from.
--
-- Formats, detected by their top-level key:
--   timelineEdits      Takeout "Timeline/Timeline Edits.json"
--   semanticSegments   Timeline.json exported from the phone (Android); a
--                      top-level array of segments is the iOS variant
--   locations          legacy Takeout "Records.json"
--   timelineObjects    legacy Takeout "Semantic Location History" month files

plugin = {
    api_version = 1,
    name = "location-history",
    version = "1.0",
    description = "Import a Google location export (Takeout Timeline JSON) and ask where you were at a given time, or how often you were near a place.",
    capabilities = { "db:read", "kv", "actions", "pages", "api", "hooks" },
}

local PAGE = "/plugins/location-history/query"
local CHUNK = 20000            -- positions per KV value, ~1 MB serialized
-- Times outside 1990-2100 are corrupt, and dropped: a timestamp in the year
-- 33658 would otherwise make an import walk thirty thousand years of months.
local MIN_TIME, MAX_TIME = 631152000, 4102444800
local WINDOW = 2 * 3600        -- "at": how far a fix may be from the asked time
local PRESENCE_GAP = 3600      -- "near": fixes further apart start a new presence
local MAX_ROWS = 500           -- "near": rows listed
local MAX_RADIUS = 100000
local EARTH_RADIUS = 6371008.8

-- Dates ----------------------------------------------------------------------
-- The VM has no os.time, so civil dates convert to days by hand (Howard
-- Hinnant's days_from_civil / civil_from_days).

local floor = math.floor

local function days_from_civil(y, m, d)
    if m <= 2 then y = y - 1 end
    local era = floor(y / 400)
    local yoe = y - era * 400
    local mp = (m + 9) % 12
    local doy = floor((153 * mp + 2) / 5) + d - 1
    local doe = yoe * 365 + floor(yoe / 4) - floor(yoe / 100) + doy
    return era * 146097 + doe - 719468
end

local function civil_from_days(z)
    z = z + 719468
    local era = floor(z / 146097)
    local doe = z - era * 146097
    local yoe = floor((doe - floor(doe / 1460) + floor(doe / 36524) - floor(doe / 146096)) / 365)
    local doy = doe - (365 * yoe + floor(yoe / 4) - floor(yoe / 100))
    local mp = floor((5 * doy + 2) / 153)
    local d = doy - floor((153 * mp + 2) / 5) + 1
    local m = mp < 10 and mp + 3 or mp - 9
    local y = yoe + era * 400
    if m <= 2 then y = y + 1 end
    return y, m, d
end

local byte, find, sub = string.byte, string.find, string.sub

-- The number in the n digits of s starting at i, or nil.
local function digits(s, i, n)
    local v = 0
    for k = i, i + n - 1 do
        local b = byte(s, k)
        if not b or b < 48 or b > 57 then return nil end
        v = v * 10 + b - 48
    end
    return v
end

-- parse_time for the shape every export timestamp has,
-- "YYYY-MM-DDTHH:MM:SS[.fff](Z|±HH:MM)", read byte by byte. gopher-lua
-- compiles a pattern on every match, which made the pattern parser below the
-- largest single cost of an import. nil for anything else.
local function parse_time_fast(s)
    local n = #s
    if n < 20 or byte(s, 5) ~= 45 or byte(s, 8) ~= 45 or byte(s, 11) ~= 84
        or byte(s, 14) ~= 58 or byte(s, 17) ~= 58 then
        return nil
    end
    local y, mo, d = digits(s, 1, 4), digits(s, 6, 2), digits(s, 9, 2)
    local h, mi, sec = digits(s, 12, 2), digits(s, 15, 2), digits(s, 18, 2)
    if not (y and mo and d and h and mi and sec) or mo < 1 or mo > 12 or d < 1 or d > 31
        or h > 23 or mi > 59 or sec >= 61 then
        return nil
    end
    local i = 20
    if byte(s, i) == 46 then
        local frac, scale = 0, 1
        i = i + 1
        local b = byte(s, i)
        while b and b >= 48 and b <= 57 do
            frac, scale = frac * 10 + b - 48, scale * 10
            i = i + 1
            b = byte(s, i)
        end
        sec = sec + frac / scale
    end
    local b, offset = byte(s, i), nil
    if (b == 90 or b == 122) and i == n then
        offset = 0
    elseif (b == 43 or b == 45) and i + 5 == n and byte(s, i + 3) == 58 then
        local oh, om = digits(s, i + 1, 2), digits(s, i + 4, 2)
        if not (oh and om) then return nil end
        offset = oh * 60 + om
        if b == 45 then offset = -offset end
    else
        return nil
    end
    return days_from_civil(y, mo, d) * 86400 + h * 3600 + mi * 60 + sec - offset * 60, offset
end

-- Parses "YYYY-MM-DD[THH:MM[:SS[.fff]]][Z|±HH:MM]". Returns epoch seconds and
-- the zone offset in minutes, or nil for the offset when the string has no
-- zone; the seconds are then the wall-clock time read as UTC.
local function parse_time(s)
    if type(s) ~= "string" then return nil end
    local fast, fast_offset = parse_time_fast(s)
    if fast then return fast, fast_offset end
    local y, mo, d, rest = s:match("^(%d%d%d%d)%-(%d%d)%-(%d%d)(.*)$")
    if not y then return nil end
    y, mo, d = tonumber(y), tonumber(mo), tonumber(d)
    if mo < 1 or mo > 12 or d < 1 or d > 31 then return nil end
    local h, mi, sec = 0, 0, 0
    local hh, mm, tail = rest:match("^[T ](%d%d):(%d%d)(.*)$")
    if hh then
        h, mi, rest = tonumber(hh), tonumber(mm), tail
        local ss, tail2 = rest:match("^:(%d%d%.?%d*)(.*)$")
        if ss then sec, rest = tonumber(ss), tail2 end
        if h > 23 or mi > 59 or sec >= 61 then return nil end
    end
    local offset = nil
    if rest == "Z" or rest == "z" then
        offset = 0
    elseif rest ~= "" then
        local sign, oh, om = rest:match("^([%+%-])(%d%d):?(%d%d)$")
        if not sign then return nil end
        offset = tonumber(oh) * 60 + tonumber(om)
        if sign == "-" then offset = -offset end
    end
    local t = days_from_civil(y, mo, d) * 86400 + h * 3600 + mi * 60 + sec
    return t - (offset or 0) * 60, offset
end

-- A legacy "timestampMs" is milliseconds as a string.
local function parse_ms(v)
    local n = tonumber(v)
    if n then return n / 1000 end
    return nil
end

local function month_of(t)
    local y, m = civil_from_days(floor(t / 86400))
    return string.format("%04d-%02d", y, m)
end

local function next_month(key)
    local y, m = tonumber(key:sub(1, 4)), tonumber(key:sub(6, 7))
    m = m + 1
    if m > 12 then y, m = y + 1, 1 end
    return string.format("%04d-%02d", y, m)
end

local function format_offset(off)
    local sign = off < 0 and "-" or "+"
    local a = math.abs(off)
    return string.format("UTC%s%02d:%02d", sign, floor(a / 60), a % 60)
end

-- Epoch seconds as local wall-clock time at the given offset.
local function format_time(t, off)
    local lt = floor(t + 0.5) + (off or 0) * 60
    local days = floor(lt / 86400)
    local y, m, d = civil_from_days(days)
    local s = lt - days * 86400
    return string.format("%04d-%02d-%02d %02d:%02d:%02d", y, m, d, floor(s / 3600), floor(s % 3600 / 60), s % 60)
end

local function format_duration(seconds)
    seconds = floor(seconds + 0.5)
    if seconds < 60 then return seconds .. " s" end
    local d, h, m = floor(seconds / 86400), floor(seconds % 86400 / 3600), floor(seconds % 3600 / 60)
    if d > 0 then return string.format("%d d %d h", d, h) end
    if h > 0 then return string.format("%d h %d min", h, m) end
    return m .. " min"
end

local function local_day(t, off)
    return floor((t + (off or 0) * 60) / 86400)
end

-- Coordinates ----------------------------------------------------------------

local function valid(lat, lng)
    return lat and lng and lat == lat and lng == lng and lat >= -90 and lat <= 90 and lng >= -180 and lng <= 180
end

-- E7 values past 2^31 in old Records.json files are 32-bit overflows.
local function from_e7(v)
    v = tonumber(v)
    if not v then return nil end
    if v > 1800000000 then v = v - 4294967296 end
    return v / 1e7
end

local DEGREE = "\194\176"

-- tonumber of one coordinate, without its degree sign. Plain finds only: see
-- parse_time_fast.
local function coordinate_number(str)
    local d = find(str, DEGREE, 1, true)
    if d then str = sub(str, 1, d - 1) .. sub(str, d + 2) end
    return tonumber(str)
end

-- "50.5837°, 8.6744°", "geo:50.5837,8.6744" or "50.5837, 8.6744".
local function parse_latlng(s)
    local start = 1
    if byte(s, 1) == 103 and byte(s, 2) == 101 and byte(s, 3) == 111 and byte(s, 4) == 58 then start = 5 end
    local comma = find(s, ",", start, true)
    if not comma then return nil end
    local lat, lng = coordinate_number(sub(s, start, comma - 1)), coordinate_number(sub(s, comma + 1))
    if valid(lat, lng) then return lat, lng end
    return nil
end

local function coord(v)
    if type(v) == "string" then return parse_latlng(v) end
    if type(v) ~= "table" then return nil end
    local lat = from_e7(v.latE7 or v.latitudeE7)
    local lng = from_e7(v.lngE7 or v.longitudeE7)
    if valid(lat, lng) then return lat, lng end
    local s = v.latLng or v.LatLng or v.point
    if type(s) == "string" then return parse_latlng(s) end
    return nil
end

local function round7(x)
    return floor(x * 1e7 + 0.5) / 1e7
end

local function rad(x) return x * math.pi / 180 end

local function distance(lat1, lng1, lat2, lng2)
    local dlat, dlng = rad(lat2 - lat1), rad(lng2 - lng1)
    local a = math.sin(dlat / 2) ^ 2 + math.cos(rad(lat1)) * math.cos(rad(lat2)) * math.sin(dlng / 2) ^ 2
    return 2 * EARTH_RADIUS * math.asin(math.min(1, math.sqrt(a)))
end

-- Distance from a point to the nearest point of a {min_lat, min_lng, max_lat,
-- max_lng} box.
local function distance_to_box(lat, lng, box)
    local clat = math.max(box[1], math.min(box[3], lat))
    local clng = math.max(box[2], math.min(box[4], lng))
    return distance(lat, lng, clat, clng)
end

-- Parsing --------------------------------------------------------------------

local function new_collector()
    return { points = {}, segments = {} }
end

-- A point is held as one fixed-width string, "time|lat|lng|accuracy", until it
-- is stored: a Lua table per point costs about ten times the memory, and the
-- zero-padded time makes the strings sort by time with the built-in
-- comparison instead of a Lua comparator.
local POINT_FORMAT = "%013d|%.7f|%.7f|%d"

local function add_point(c, t, lat, lng, acc)
    if not t or t < MIN_TIME or t >= MAX_TIME or not valid(lat, lng) then return end
    c.points[#c.points + 1] = string.format(POINT_FORMAT, floor(t + 0.5), lat, lng, floor((tonumber(acc) or 0) + 0.5))
end

local function decode_point(s)
    local p2 = find(s, "|", 15, true)
    local p3 = find(s, "|", p2 + 1, true)
    return tonumber(sub(s, 1, 13)), tonumber(sub(s, 15, p2 - 1)), tonumber(sub(s, p2 + 1, p3 - 1)), tonumber(sub(s, p3 + 1))
end

-- A text field from the file, or nil. Exports are not trusted to hold strings
-- where strings belong, and a table stored here would break every query that
-- prints it.
local function text(v)
    if type(v) == "string" or type(v) == "number" then return tostring(v) end
    return nil
end

local function add_segment(c, seg)
    if not seg.s or not seg.e or seg.e < seg.s or seg.s < MIN_TIME or seg.e >= MAX_TIME then return end
    if seg.lat and not valid(seg.lat, seg.lng) then return end
    seg.s, seg.e = floor(seg.s + 0.5), floor(seg.e + 0.5)
    if seg.lat then seg.lat, seg.lng = round7(seg.lat), round7(seg.lng) end
    if seg.lat2 then seg.lat2, seg.lng2 = round7(seg.lat2), round7(seg.lng2) end
    c.segments[#c.segments + 1] = seg
end

-- A segment of the current timeline format, as found in Timeline Edits.json
-- (wrapped in .segment) and in the phone export (not wrapped).
local function semantic_segment(c, seg, edited)
    local s, s_off = parse_time(seg.startTime)
    local e = parse_time(seg.endTime)
    local off = tonumber(seg.startTimeTimezoneUtcOffsetMinutes) or s_off
    local inner = seg.segment or seg
    if type(inner.visit) == "table" then
        local top = inner.visit.topCandidate or {}
        local lat, lng = coord(top.placeLocation)
        add_segment(c, { k = "visit", s = s, e = e, lat = lat, lng = lng, off = off, edited = edited,
            place = text(top.placeId or top.placeID), type = text(top.semanticType) })
    elseif type(inner.activity) == "table" then
        local a = inner.activity
        local lat, lng = coord(a.start)
        local lat2, lng2 = coord(a["end"])
        local top = a.topCandidate or {}
        add_segment(c, { k = "activity", s = s, e = e, lat = lat, lng = lng, lat2 = lat2, lng2 = lng2,
            off = off, edited = edited, type = text(top.type), dist = tonumber(a.distanceMeters) })
    end
    if type(inner.timelinePath) == "table" then
        for _, p in ipairs(inner.timelinePath) do
            local lat, lng = coord(p.point)
            local t = parse_time(p.time)
            if not t and s and tonumber(p.durationMinutesOffsetFromStartTime) then
                t = s + tonumber(p.durationMinutesOffsetFromStartTime) * 60
            end
            add_point(c, t, lat, lng, nil)
        end
    end
end

local function timeline_edit(c, entry)
    local signal = type(entry.rawSignal) == "table" and entry.rawSignal.signal
    local p = type(signal) == "table" and signal.position
    if type(p) == "table" then
        local lat, lng = coord(p.point)
        add_point(c, parse_time(p.timestamp), lat, lng, tonumber(p.accuracyMm) and p.accuracyMm / 1000)
    end
    if type(entry.userEditedSemanticSegment) == "table" then
        semantic_segment(c, entry.userEditedSemanticSegment, true)
    elseif type(entry.inferredSemanticSegment) == "table" then
        semantic_segment(c, entry.inferredSemanticSegment, false)
    end
end

local function raw_signal(c, r)
    local p = r.position
    if type(p) == "table" then
        local lat, lng = coord(p.LatLng or p.latLng or p.point)
        add_point(c, parse_time(p.timestamp), lat, lng, p.accuracyMeters)
    end
end

local function record(c, loc)
    local lat, lng = coord(loc)
    add_point(c, parse_time(loc.timestamp) or parse_ms(loc.timestampMs), lat, lng, loc.accuracy)
end

local function legacy_time(duration, which)
    if type(duration) ~= "table" then return nil end
    return parse_time(duration[which .. "Timestamp"]) or parse_ms(duration[which .. "TimestampMs"])
end

local function legacy_path(c, path)
    if type(path) ~= "table" or type(path.points) ~= "table" then return end
    for _, p in ipairs(path.points) do
        local lat, lng = coord(p)
        add_point(c, parse_time(p.timestamp) or parse_ms(p.timestampMs), lat, lng, p.accuracyMeters)
    end
end

local function timeline_object(c, obj)
    local v, a = obj.placeVisit, obj.activitySegment
    if type(v) == "table" then
        local loc = v.location or {}
        local lat, lng = coord(loc)
        add_segment(c, { k = "visit", s = legacy_time(v.duration, "start"), e = legacy_time(v.duration, "end"),
            lat = lat, lng = lng, place = text(loc.placeId), name = text(loc.name), type = text(loc.semanticType) })
        legacy_path(c, v.simplifiedRawPath)
    elseif type(a) == "table" then
        local lat, lng = coord(a.startLocation)
        local lat2, lng2 = coord(a.endLocation)
        add_segment(c, { k = "activity", s = legacy_time(a.duration, "start"), e = legacy_time(a.duration, "end"),
            lat = lat, lng = lng, lat2 = lat2, lng2 = lng2, type = text(a.activityType), dist = tonumber(a.distance) })
        legacy_path(c, a.simplifiedRawPath)
    end
end

-- The handler and format for each top-level array, by its key. The key is
-- false for a root array, which is the iOS phone export.
local ELEMENT_HANDLERS = {
    timelineEdits = { "Takeout Timeline Edits", timeline_edit },
    semanticSegments = { "Phone Timeline export", function(c, seg) semantic_segment(c, seg, false) end },
    rawSignals = { "Phone Timeline export", raw_signal },
    [false] = { "Phone Timeline export (iOS)", function(c, seg)
        if seg.startTime then semantic_segment(c, seg, false) end
    end },
    locations = { "Takeout Records", record },
    timelineObjects = { "Takeout Semantic Location History", timeline_object },
}

-- Reads a resource's export one element at a time (the file is never held
-- whole). Returns the collector and a format label, or nil and an error.
-- progress(n) is called every so many elements.
local function read_export(id, progress)
    local c, format, n = new_collector(), nil, 0
    local ok, err = mah.db.each_resource_json(id, function(key, index, value)
        local handler = ELEMENT_HANDLERS[key or false]
        if not handler or index == nil or type(value) ~= "table" then return end
        format = format or handler[1]
        handler[2](c, value)
        n = n + 1
        if n % 20000 == 0 then progress(n) end
    end)
    if not ok then return nil, err end
    if not format then
        return nil, "this is not a Google location export this plugin recognises "
            .. "(expected Timeline Edits.json, a phone Timeline.json, Records.json or a Semantic Location History month file)"
    end
    return c, format
end

-- Storage --------------------------------------------------------------------

local function meta_key(id) return "imp:" .. id end
local function data_prefix(id) return "d:" .. id .. ":" end
local function points_key(id, month, part) return "d:" .. id .. ":p:" .. month .. ":" .. part end
local function segments_key(id, month) return "d:" .. id .. ":s:" .. month end

-- Set while an import is writing, to the time it started: see sweep_orphans.
local function writing_key(id) return "w:" .. id end

local function forget(id)
    mah.kv.delete(meta_key(id))
    for _, key in ipairs(mah.kv.list(data_prefix(id))) do
        mah.kv.delete(key)
    end
    mah.kv.delete(writing_key(id))
end

local function grow_box(box, lat, lng)
    if not lat then return box end
    if not box then return { lat, lng, lat, lng } end
    box[1], box[2] = math.min(box[1], lat), math.min(box[2], lng)
    box[3], box[4] = math.max(box[3], lat), math.max(box[4], lng)
    return box
end

local function sorted_keys(t)
    local keys = {}
    for k in pairs(t) do keys[#keys + 1] = k end
    table.sort(keys)
    return keys
end

-- The offset most segments were recorded at, for showing positions (which
-- carry none) in local time.
local function usual_offset(segments)
    local counts, best, best_n = {}, nil, 0
    for _, seg in ipairs(segments) do
        if seg.off then
            counts[seg.off] = (counts[seg.off] or 0) + 1
            if counts[seg.off] > best_n then best, best_n = seg.off, counts[seg.off] end
        end
    end
    return best
end

-- Writes a parsed export, replacing the resource's earlier import. The summary
-- goes last, so a failure part way leaves no summary and therefore nothing a
-- query would read; the next import clears the leftovers.
local function store(id, name, format, c, progress)
    table.sort(c.points)
    table.sort(c.segments, function(a, b) return a.s < b.s end)

    local months = {}
    local function month(key)
        local m = months[key]
        if not m then
            m = { p = 0, parts = 0, s = 0, segments = {} }
            months[key] = m
        end
        return m
    end
    for _, seg in ipairs(c.segments) do
        local key, final = month_of(seg.s), month_of(seg.e)
        while true do
            local m = month(key)
            m.segments[#m.segments + 1] = seg
            m.box = grow_box(grow_box(m.box, seg.lat, seg.lng), seg.lat2, seg.lng2)
            if key == final then break end
            key = next_month(key)
        end
    end

    forget(id)
    mah.kv.set(writing_key(id), mah.util.now())

    -- Points arrive sorted, so each month's are written as soon as the next
    -- month begins, and only one month is ever decoded at a time.
    local current, list, until_t = nil, {}, -1
    local function flush()
        if not current then return end
        local m = month(current)
        for start = 1, #list, CHUNK do
            m.parts = m.parts + 1
            local slice = {}
            for j = start, math.min(start + CHUNK - 1, #list) do slice[#slice + 1] = list[j] end
            mah.kv.set(points_key(id, current, m.parts), slice)
        end
        m.p = #list
        list = {}
    end
    local kept, last_t, last_lat, last_lng = 0, nil, nil, nil
    local first, final = nil, nil
    for i, str in ipairs(c.points) do
        local t, lat, lng, acc = decode_point(str)
        if not (t == last_t and lat == last_lat and lng == last_lng) then
            if t >= until_t then
                flush()
                current = month_of(t)
                until_t = days_from_civil(tonumber(next_month(current):sub(1, 4)), tonumber(next_month(current):sub(6, 7)), 1) * 86400
            end
            list[#list + 1] = { t, lat, lng, acc }
            local m = month(current)
            m.box = grow_box(m.box, lat, lng)
            kept = kept + 1
            last_t, last_lat, last_lng = t, lat, lng
            first = first or t
            final = t
        end
        if i % 50000 == 0 then progress(i, #c.points) end
    end
    flush()
    c.points = nil

    local summary = {}
    for key, m in pairs(months) do
        if #m.segments > 0 then mah.kv.set(segments_key(id, key), m.segments) end
        summary[key] = { p = m.p, parts = m.parts, s = #m.segments, box = m.box }
    end

    for _, seg in ipairs(c.segments) do
        first = math.min(first or seg.s, seg.s)
        final = math.max(final or seg.e, seg.e)
    end
    local meta = {
        id = id, name = name, format = format, points = kept, segments = #c.segments,
        first = first, last = final, offset = usual_offset(c.segments),
        imported_at = mah.util.now_iso(), months = summary,
    }
    mah.kv.set(meta_key(id), meta)
    mah.kv.delete(writing_key(id))
    -- The resource may have been deleted while this ran, and its delete hook
    -- skipped because this import held the plugin. Then nothing must be left.
    local resource, err = mah.db.get_resource(id)
    if not resource and not err then
        forget(id)
        return nil
    end
    return meta
end

-- How long an import may be writing before its leftovers count as abandoned:
-- the 5-minute limit on background work, with room to spare.
local WRITING_GRACE = 15 * 60
local SWEEP_EVERY = 10 * 60

-- Forgets every import whose resource no longer exists, and the data of any
-- import that stopped before writing its summary (a timeout part way through
-- store). The delete hook is a notification the host drops when the plugin is
-- busy (an import holds it for minutes), so this catches what it missed.
--
-- "Not found" only means "deleted" to a caller who can read every resource,
-- so this runs only for an administrator with no scope (see maybe_sweep), and
-- forgets on a definite "not found", never on a failed read. An import still
-- writing -- in another server process, which has its own VM -- has a fresh
-- writing marker and is left alone.
local function sweep_orphans()
    local forgotten, summarised = 0, {}
    for _, key in ipairs(mah.kv.list("imp:")) do
        local id = tonumber(key:match("^imp:(%d+)$"))
        if id then
            summarised[id] = true
            local resource, err = mah.db.get_resource(id)
            if not resource and not err then
                forget(id)
                forgotten = forgotten + 1
            end
        end
    end
    local unsummarised = {}
    for _, key in ipairs(mah.kv.list("d:")) do
        local id = tonumber(key:match("^d:(%d+):"))
        if id and not summarised[id] then unsummarised[id] = true end
    end
    local now = mah.util.now()
    for id in pairs(unsummarised) do
        local started = tonumber(mah.kv.get(writing_key(id)))
        if not started or now - started > WRITING_GRACE then
            forget(id)
            forgotten = forgotten + 1
        end
    end
    return forgotten
end

-- Whether the caller can read every resource: an administrator with no scope.
-- With authentication off every request carries such a principal (the
-- implicit super-user); a request without one is not assumed to.
local function sees_everything(principal)
    if type(principal) ~= "table" or principal.isAdmin ~= true then return false end
    local scope = tonumber(principal.scopeGroupId)
    return scope == nil or scope == 0
end

-- Runs the sweep at most every SWEEP_EVERY seconds, on a request from someone
-- who sees everything.
local function maybe_sweep(principal)
    if not sees_everything(principal) then return end
    local now = mah.util.now()
    local last = tonumber(mah.kv.get("sweep:last"))
    if last and now - last < SWEEP_EVERY then return end
    mah.kv.set("sweep:last", now)
    sweep_orphans()
end

-- Imports the caller can see, as { id, meta, name } sorted by id. With `only`,
-- just that one.
local function visible_imports(only)
    local list = {}
    for _, key in ipairs(mah.kv.list("imp:")) do
        local id = tonumber(key:match("^imp:(%d+)$"))
        if id and (not only or only == id) then
            local resource = mah.db.get_resource(id)
            local meta = resource and mah.kv.get(key)
            if meta then
                list[#list + 1] = { id = id, meta = meta, name = resource.name or meta.name or ("Resource " .. id) }
            end
        end
    end
    table.sort(list, function(a, b) return a.id < b.id end)
    return list
end

local function load_points(imp, month)
    local info = imp.meta.months[month]
    if not info or info.p == 0 then return {} end
    local out = {}
    for part = 1, info.parts do
        for _, p in ipairs(mah.kv.get(points_key(imp.id, month, part)) or {}) do out[#out + 1] = p end
    end
    return out
end

local function load_segments(imp, month)
    local info = imp.meta.months[month]
    if not info or info.s == 0 then return {} end
    return mah.kv.get(segments_key(imp.id, month)) or {}
end

-- Months of an import between two times, oldest first.
local function months_between(imp, from_t, to_t)
    local out, key, final = {}, month_of(from_t), month_of(to_t)
    while true do
        if imp.meta.months[key] then out[#out + 1] = key end
        if key == final then break end
        key = next_month(key)
    end
    return out
end

-- Queries --------------------------------------------------------------------

-- Parses the offset field: "auto" (or empty) is nil, otherwise whole minutes.
local function parse_offset(v)
    if v == nil or v == "" or v == "auto" then return nil, nil end
    local n = tonumber(v)
    if not n or n ~= floor(n) or n < -720 or n > 840 then
        return nil, "the UTC offset must be \"auto\" or whole minutes between -720 and 840"
    end
    return n, nil
end

local function better_segment(a, b)
    if not a then return true end
    if (b.edited and true or false) ~= (a.edited and true or false) then return b.edited end
    return (b.e - b.s) < (a.e - a.s)
end

-- The segment nearest t (covering it, or else closest in time) within a day,
-- used to find which offset a wall-clock time was meant in.
local function offset_near(imports, t)
    local best, best_gap = nil, 86400
    for _, imp in ipairs(imports) do
        for _, key in ipairs(months_between(imp, t - 86400, t + 86400)) do
            for _, seg in ipairs(load_segments(imp, key)) do
                if seg.off then
                    local gap = (t < seg.s and seg.s - t) or (t > seg.e and t - seg.e) or 0
                    if gap < best_gap or (gap == best_gap and best == nil) then best, best_gap = seg.off, gap end
                end
            end
        end
    end
    if best then return best end
    for _, imp in ipairs(imports) do
        if imp.meta.offset then return imp.meta.offset end
    end
    return nil
end

local function point_view(p, t, off)
    if not p then return nil end
    return { time = format_time(p[1], off), epoch = p[1], lat = p[2], lng = p[3],
        accuracy_m = p[4] > 0 and p[4] or nil, seconds_from_query = p[1] - t }
end

local function segment_view(seg, off)
    return {
        kind = seg.k, start = format_time(seg.s, seg.off or off), ["end"] = format_time(seg.e, seg.off or off),
        offset = format_offset(seg.off or off), lat = seg.lat, lng = seg.lng, end_lat = seg.lat2, end_lng = seg.lng2,
        place_id = seg.place, place_name = seg.name, type = seg.type, distance_m = seg.dist,
        edited = seg.edited and true or false,
    }
end

-- Nearest fix strictly before (dir = -1) or at/after (dir = 1) t, searching
-- every month of every import outward from t.
local function nearest_fix(imports, t, dir)
    local best = nil
    for _, imp in ipairs(imports) do
        local keys = sorted_keys(imp.meta.months)
        local here = month_of(t)
        local first, last, step = #keys, 1, -1
        if dir > 0 then first, last, step = 1, #keys, 1 end
        for i = first, last, step do
            local key = keys[i]
            if (dir < 0 and key <= here) or (dir > 0 and key >= here) then
                local found = nil
                for _, p in ipairs(load_points(imp, key)) do
                    if dir < 0 and p[1] < t then found = p
                    elseif dir > 0 and p[1] >= t then found = p; break end
                end
                if found then
                    if not best or (dir < 0 and found[1] > best[1]) or (dir > 0 and found[1] < best[1]) then best = found end
                    break
                end
            end
        end
    end
    return best
end

-- "Where was I at <when>?" Returns a result table, or nil and an error.
local function where_at(imports, when, offset_param)
    local local_t, zone = parse_time(when)
    if not local_t then return nil, "the time must look like 2026-04-27T09:30" end
    local off, err = parse_offset(offset_param)
    if err then return nil, err end
    local how = "explicit"
    local t
    if zone then
        t, off, how = local_t, zone, "from the time given"
    elseif off then
        t = local_t - off * 60
    else
        off = offset_near(imports, local_t) or 0
        t = local_t - off * 60
        how = "auto"
    end

    local seg = nil
    for _, imp in ipairs(imports) do
        for _, s in ipairs(load_segments(imp, month_of(t))) do
            if s.s <= t and t <= s.e and better_segment(seg, s) then seg = s end
        end
    end
    local before = nearest_fix(imports, t + 1, -1)
    local after = nearest_fix(imports, t + 1, 1)

    local estimate = nil
    local near_before = before and t - before[1] <= WINDOW
    local near_after = after and after[1] - t <= WINDOW
    if seg and seg.k == "visit" and seg.lat then
        estimate = { lat = seg.lat, lng = seg.lng, basis = "visit" }
    elseif near_before and near_after then
        local span = after[1] - before[1]
        local f = span > 0 and (t - before[1]) / span or 0
        estimate = { lat = round7(before[2] + (after[2] - before[2]) * f), lng = round7(before[3] + (after[3] - before[3]) * f),
            basis = span > 0 and "interpolated between the fixes before and after" or "fix at that time" }
    elseif near_before or near_after then
        local p = near_before and before or after
        estimate = { lat = p[2], lng = p[3], basis = "nearest fix" }
    end

    return {
        query = { time = format_time(t, off), utc = format_time(t, 0) .. "Z", offset = format_offset(off),
            offset_minutes = off, offset_source = how, epoch = t },
        segment = seg and segment_view(seg, off) or nil,
        before = point_view(before, t, off),
        after = point_view(after, t, off),
        estimate = estimate,
        window_seconds = WINDOW,
    }
end

local function parse_point(s)
    if type(s) ~= "string" then return nil end
    return parse_latlng(s)
end

local function day_bound(s, off, last)
    local t, zone = parse_time(s)
    if not t then return nil end
    if zone == nil then t = t - off * 60 end
    if last then t = t + 86400 - 1 end
    return t
end

-- "How often was I within <radius> of <point>?"
local function how_often_near(imports, args)
    local lat, lng = parse_point(args.point)
    if not lat then return nil, "the place must be \"latitude, longitude\", e.g. 50.5837, 8.6744" end
    local radius = tonumber(args.radius) or 250
    if radius <= 0 or radius > MAX_RADIUS then return nil, "the radius must be between 1 and " .. MAX_RADIUS .. " metres" end
    local off, err = parse_offset(args.offset)
    if err then return nil, err end
    local how = "explicit"
    if not off then
        how = "auto"
        for _, imp in ipairs(imports) do
            if imp.meta.offset then off = imp.meta.offset; break end
        end
        off = off or 0
    end
    local from_t, to_t = -math.huge, math.huge
    if args.from and args.from ~= "" then
        from_t = day_bound(args.from, off, false)
        if not from_t then return nil, "the start date must look like 2026-04-01" end
    end
    if args.to and args.to ~= "" then
        to_t = day_bound(args.to, off, true)
        if not to_t then return nil, "the end date must look like 2026-04-30" end
    end

    local intervals, seen_visits = {}, {}
    for _, imp in ipairs(imports) do
        local in_presence, current, last_t = false, nil, nil
        for _, key in ipairs(sorted_keys(imp.meta.months)) do
            local info = imp.meta.months[key]
            local month_start = days_from_civil(tonumber(key:sub(1, 4)), tonumber(key:sub(6, 7)), 1) * 86400
            local month_end = days_from_civil(tonumber(next_month(key):sub(1, 4)), tonumber(next_month(key):sub(6, 7)), 1) * 86400
            if month_end > from_t and month_start <= to_t and info.box and distance_to_box(lat, lng, info.box) <= radius then
                for _, seg in ipairs(load_segments(imp, key)) do
                    local id = seg.s .. ":" .. seg.e .. ":" .. (seg.place or "")
                    if seg.k == "visit" and seg.lat and not seen_visits[id] and seg.e >= from_t and seg.s <= to_t
                        and distance(lat, lng, seg.lat, seg.lng) <= radius then
                        seen_visits[id] = true
                        intervals[#intervals + 1] = { s = seg.s, e = seg.e, fixes = 0, visits = 1,
                            places = { seg.name or seg.place }, d = distance(lat, lng, seg.lat, seg.lng) }
                    end
                end
                for _, p in ipairs(load_points(imp, key)) do
                    if p[1] >= from_t and p[1] <= to_t then
                        local d = distance(lat, lng, p[2], p[3])
                        if d <= radius then
                            if in_presence and current and p[1] - last_t <= PRESENCE_GAP then
                                current.e, current.fixes = p[1], current.fixes + 1
                                current.d = math.min(current.d, d)
                            else
                                current = { s = p[1], e = p[1], fixes = 1, visits = 0, places = {}, d = d }
                                intervals[#intervals + 1] = current
                            end
                            in_presence, last_t = true, p[1]
                        elseif d - p[4] > radius then
                            -- Only a fix whose whole accuracy circle is outside
                            -- ends a presence; a vague one may still be there.
                            in_presence = false
                        end
                    end
                end
            else
                -- A month that cannot hold a fix nearby ends any presence.
                in_presence = false
            end
        end
    end

    -- Overlapping visits and presences, from any import, are one time there.
    table.sort(intervals, function(a, b) return a.s < b.s end)
    local merged = {}
    for _, iv in ipairs(intervals) do
        local top = merged[#merged]
        if top and iv.s <= top.e + 60 then
            top.e = math.max(top.e, iv.e)
            top.fixes, top.visits = top.fixes + iv.fixes, top.visits + iv.visits
            top.d = math.min(top.d, iv.d)
            for _, p in ipairs(iv.places) do top.places[#top.places + 1] = p end
        else
            merged[#merged + 1] = iv
        end
    end

    local days, total, first, last = {}, 0, nil, nil
    local day_count = 0
    for _, iv in ipairs(merged) do
        total = total + (iv.e - iv.s)
        first = first and math.min(first, iv.s) or iv.s
        last = last and math.max(last, iv.e) or iv.e
        for day = local_day(iv.s, off), local_day(iv.e, off) do
            if not days[day] then days[day] = true; day_count = day_count + 1 end
        end
    end

    local rows = {}
    for i = #merged, math.max(1, #merged - MAX_ROWS + 1), -1 do
        local iv = merged[i]
        local places, seen = {}, {}
        for _, p in ipairs(iv.places) do
            if p and not seen[p] then seen[p] = true; places[#places + 1] = p end
        end
        rows[#rows + 1] = { start = format_time(iv.s, off), ["end"] = format_time(iv.e, off), seconds = iv.e - iv.s,
            fixes = iv.fixes, visits = iv.visits, places = mah.json.array(places), closest_m = floor(iv.d + 0.5) }
    end

    return {
        query = { lat = lat, lng = lng, radius_m = radius, offset = format_offset(off), offset_minutes = off,
            offset_source = how, from = args.from, to = args.to },
        times = #merged, days = day_count, total_seconds = total,
        first = first and format_time(first, off) or nil, last = last and format_time(last, off) or nil,
        rows = mah.json.array(rows), rows_omitted = #merged - #rows,
    }
end

-- HTML -----------------------------------------------------------------------

local function esc(s) return mah.html_escape(s) end

local function map_link(lat, lng, label)
    local href = string.format("https://www.openstreetmap.org/?mlat=%.7f&mlon=%.7f#map=17/%.7f/%.7f", lat, lng, lat, lng)
    return '<a class="underline" target="_blank" rel="noopener noreferrer" href="' .. esc(href) .. '">'
        .. esc(label or string.format("%.5f, %.5f", lat, lng)) .. '</a>'
end

local function near_link(lat, lng)
    local href = PAGE .. "?q=near&point=" .. string.format("%.6f%%2C%.6f", lat, lng) .. "&radius=250#near-results"
    return '<a class="underline" href="' .. esc(href) .. '">How often was I here?</a>'
end

local function alert(message)
    return '<p class="bg-red-50 border border-red-200 text-red-800 rounded p-3" role="alert">' .. esc(message) .. '</p>'
end

local OFFSETS = { -720, -660, -600, -540, -480, -420, -360, -300, -240, -210, -180, -120, -60, 0, 60, 120, 180, 210,
    240, 270, 300, 330, 345, 360, 390, 420, 480, 525, 540, 570, 600, 630, 660, 720, 765, 780, 840 }

local function offset_select(id, selected)
    local html = '<select id="' .. id .. '" name="offset" aria-describedby="' .. id .. '-hint" class="w-full border rounded p-2">'
        .. '<option value="auto"' .. ((selected == nil or selected == "" or selected == "auto") and " selected" or "")
        .. '>Auto (the zone recorded in the timeline)</option>'
    for _, off in ipairs(OFFSETS) do
        html = html .. '<option value="' .. off .. '"' .. (tostring(off) == selected and " selected" or "") .. '>'
            .. format_offset(off) .. '</option>'
    end
    return html .. '</select>'
end

local function import_select(id, imports, selected)
    local html = '<select id="' .. id .. '" name="resource" class="w-full border rounded p-2">'
        .. '<option value="">All imports</option>'
    for _, imp in ipairs(imports) do
        html = html .. '<option value="' .. imp.id .. '"' .. (tostring(imp.id) == selected and " selected" or "") .. '>'
            .. esc(imp.name) .. '</option>'
    end
    return html .. '</select>'
end

local function field(id, label, control, hint)
    return '<div><label class="block font-medium mb-1" for="' .. id .. '">' .. label .. '</label>' .. control
        .. (hint and ('<p class="text-xs text-gray-600 mt-1" id="' .. id .. '-hint">' .. hint .. '</p>') or '') .. '</div>'
end

local function input(id, name, kind, value, extra)
    return '<input id="' .. id .. '" name="' .. name .. '" type="' .. kind .. '" value="' .. esc(value or "") .. '" '
        .. (extra or "") .. ' class="w-full border rounded p-2">'
end

local function fix_html(label, p)
    if not p then return '<li>' .. label .. ': none</li>' end
    local delta = p.seconds_from_query
    local when = delta == 0 and "at that time" or (format_duration(math.abs(delta)) .. (delta < 0 and " before" or " after"))
    return '<li>' .. label .. ': ' .. esc(p.time) .. ' (' .. when .. ') at ' .. map_link(p.lat, p.lng)
        .. (p.accuracy_m and (' ±' .. p.accuracy_m .. ' m') or '') .. '</li>'
end

local function at_html(r)
    local html = '<p>' .. esc(r.query.time) .. ' ' .. esc(r.query.offset)
        .. ' <span class="text-gray-600">(' .. esc(r.query.utc) .. ', offset ' .. esc(r.query.offset_source) .. ')</span></p>'
    if r.estimate then
        html = html .. '<p class="text-lg font-semibold mt-2">You were at ' .. map_link(r.estimate.lat, r.estimate.lng)
            .. '</p><p class="text-sm text-gray-600">Based on: ' .. esc(r.estimate.basis) .. '. '
            .. near_link(r.estimate.lat, r.estimate.lng) .. '</p>'
    else
        html = html .. '<p class="text-lg font-semibold mt-2">No position recorded within '
            .. format_duration(r.window_seconds) .. ' of that time.</p>'
    end
    local s = r.segment
    if s then
        if s.kind == "visit" then
            html = html .. '<p class="mt-2">Visit from ' .. esc(s.start) .. ' to ' .. esc(s["end"]) .. ' ' .. esc(s.offset)
                .. (s.place_name and (' at ' .. esc(s.place_name)) or '')
                .. (s.type and s.type ~= "UNKNOWN" and (' (' .. esc(s.type) .. ')') or '')
                .. (s.edited and ' — edited by you' or '') .. '.'
                .. (s.lat and (' ' .. near_link(s.lat, s.lng)) or '') .. '</p>'
        else
            html = html .. '<p class="mt-2">Moving (' .. esc(s.type or "unknown mode") .. ') from ' .. esc(s.start)
                .. ' to ' .. esc(s["end"]) .. ' ' .. esc(s.offset)
                .. (s.lat and (', from ' .. map_link(s.lat, s.lng)) or '')
                .. (s.end_lat and (' to ' .. map_link(s.end_lat, s.end_lng)) or '')
                .. (s.distance_m and (', ' .. floor(s.distance_m + 0.5) .. ' m') or '')
                .. (s.edited and ' — edited by you' or '') .. '.</p>'
        end
    end
    return html .. '<ul class="list-disc pl-5 mt-2">' .. fix_html("Last fix before", r.before)
        .. fix_html("First fix after", r.after) .. '</ul>'
end

local function near_html(r)
    local q = r.query
    local html = '<p>Within ' .. q.radius_m .. ' m of ' .. map_link(q.lat, q.lng)
        .. ((q.from and q.from ~= "") and (' from ' .. esc(q.from)) or '')
        .. ((q.to and q.to ~= "") and (' to ' .. esc(q.to)) or '')
        .. ' <span class="text-gray-600">(times in ' .. esc(q.offset) .. ', ' .. esc(q.offset_source) .. ')</span></p>'
    if r.times == 0 then
        return html .. '<p class="text-lg font-semibold mt-2">You were never recorded there.</p>'
    end
    html = html .. '<dl class="grid grid-cols-2 gap-2 mt-2">'
    for _, item in ipairs({ { "Times", tostring(r.times) }, { "Days", tostring(r.days) },
        { "Total time", format_duration(r.total_seconds) }, { "First / last", r.first .. " / " .. r.last } }) do
        html = html .. '<div class="border rounded p-2"><dt class="text-sm text-gray-600">' .. item[1]
            .. '</dt><dd class="font-semibold">' .. esc(item[2]) .. '</dd></div>'
    end
    html = html .. '</dl><table class="mt-4 w-full text-sm"><caption class="text-left font-medium mb-1">Each time, newest first</caption>'
        .. '<thead><tr class="text-left border-b"><th scope="col" class="p-1">From</th><th scope="col" class="p-1">To</th>'
        .. '<th scope="col" class="p-1">Duration</th><th scope="col" class="p-1">Closest</th><th scope="col" class="p-1">Evidence</th></tr></thead><tbody>'
    for _, row in ipairs(r.rows) do
        local evidence = {}
        if row.visits > 0 then evidence[#evidence + 1] = row.visits .. (row.visits == 1 and " visit" or " visits") end
        if row.fixes > 0 then evidence[#evidence + 1] = row.fixes .. (row.fixes == 1 and " fix" or " fixes") end
        if #row.places > 0 then evidence[#evidence + 1] = table.concat(row.places, ", ") end
        html = html .. '<tr class="border-b"><td class="p-1">' .. esc(row.start) .. '</td><td class="p-1">' .. esc(row["end"])
            .. '</td><td class="p-1">' .. format_duration(row.seconds) .. '</td><td class="p-1">' .. row.closest_m
            .. ' m</td><td class="p-1">' .. esc(table.concat(evidence, "; ")) .. '</td></tr>'
    end
    html = html .. '</tbody></table>'
    if r.rows_omitted > 0 then
        html = html .. '<p class="text-sm text-gray-600 mt-1">' .. r.rows_omitted .. ' older times not listed.</p>'
    end
    return html
end

local function imports_html(imports)
    if #imports == 0 then
        return '<p>Nothing imported yet. Upload the <code>Timeline Edits.json</code> from your Takeout archive '
            .. '(<code>Takeout/Timeline/</code>), or a <code>Timeline.json</code> exported from your phone, '
            .. 'and run <strong>Import location history</strong> on it.</p>'
    end
    local html = '<table class="w-full text-sm"><thead><tr class="text-left border-b">'
        .. '<th scope="col" class="p-1">Resource</th><th scope="col" class="p-1">Format</th>'
        .. '<th scope="col" class="p-1">Positions</th><th scope="col" class="p-1">Visits &amp; trips</th>'
        .. '<th scope="col" class="p-1">Covers (UTC)</th><th scope="col" class="p-1"><span class="sr-only">Actions</span></th></tr></thead><tbody>'
    for _, imp in ipairs(imports) do
        local m = imp.meta
        html = html .. '<tr class="border-b"><td class="p-1"><a class="underline" href="/resource?id=' .. imp.id .. '">'
            .. esc(imp.name) .. '</a></td><td class="p-1">' .. esc(m.format) .. '</td><td class="p-1">' .. m.points
            .. '</td><td class="p-1">' .. m.segments .. '</td><td class="p-1">'
            .. (m.first and (esc(format_time(m.first, 0):sub(1, 10)) .. ' – ' .. esc(format_time(m.last, 0):sub(1, 10))) or '–')
            .. '</td><td class="p-1"><form method="POST"><input type="hidden" name="forget" value="' .. imp.id .. '">'
            .. '<button type="submit" class="underline" aria-label="Forget the import of ' .. esc(imp.name) .. '">Forget</button>'
            .. '</form></td></tr>'
    end
    return html .. '</tbody></table>'
end

local function section(id, title, body)
    return '<section class="mb-8" aria-labelledby="' .. id .. '"><h2 id="' .. id .. '" class="text-xl font-bold mb-3">'
        .. title .. '</h2>' .. body .. '</section>'
end

local function positive_int(value)
    if type(value) ~= "string" or not value:match("^%d+$") then return nil end
    local n = tonumber(value)
    if not n or n < 1 or n > 2 ^ 53 then return nil end
    return n
end

local function is_guest(principal)
    return type(principal) == "table" and principal.role == "guest"
end

local function page(ctx)
    maybe_sweep(ctx.principal)
    local q = ctx.query or {}
    local params = ctx.params or {}
    local notice = ""

    if ctx.method == "POST" and params.forget then
        local id = positive_int(params.forget)
        if is_guest(ctx.principal) then
            notice = alert("Guests cannot forget imports.")
        elseif id and #visible_imports(id) == 1 then
            forget(id)
            notice = '<p class="bg-green-50 border border-green-200 rounded p-3" role="status">Import forgotten.</p>'
        else
            notice = alert("There is no such import you can see.")
        end
    end

    local imports = visible_imports(nil)
    local only = positive_int(q.resource)
    local selected = only and visible_imports(only) or imports

    local at_result, near_result = "", ""
    if q.q == "at" and q.time and q.time ~= "" then
        local r, err = where_at(selected, q.time, q.offset)
        at_result = '<div id="at-results" class="mt-4 border rounded p-4" role="region" aria-label="Where you were">'
            .. (r and at_html(r) or alert(err)) .. '</div>'
    elseif q.q == "near" and q.point and q.point ~= "" then
        local r, err = how_often_near(selected, q)
        near_result = '<div id="near-results" class="mt-4 border rounded p-4" role="region" aria-label="How often you were there">'
            .. (r and near_html(r) or alert(err)) .. '</div>'
    end

    local at_form = '<form method="GET" class="space-y-3 max-w-lg"><input type="hidden" name="q" value="at">'
        .. field("at-time", "Date and time", input("at-time", "time", "datetime-local", q.q == "at" and q.time or "", 'step="1" required'))
        .. field("at-offset", "Time zone", offset_select("at-offset", q.q == "at" and q.offset or nil),
            "Auto reads the time in the zone your timeline recorded around then.")
        .. field("at-resource", "Import", import_select("at-resource", imports, q.q == "at" and q.resource or nil))
        .. '<button type="submit" class="bg-blue-600 text-white px-6 py-2 rounded hover:bg-blue-700">Find</button></form>'

    local near_form = '<form method="GET" class="space-y-3 max-w-lg"><input type="hidden" name="q" value="near">'
        .. field("near-point", "Place (latitude, longitude)",
            input("near-point", "point", "text", q.q == "near" and q.point or "", 'required placeholder="50.5837, 8.6744" aria-describedby="near-point-hint"'),
            "Right-click a spot in a map app to copy its coordinates.")
        .. field("near-radius", "Radius (metres)", input("near-radius", "radius", "number", q.q == "near" and q.radius or "250", 'min="1" max="' .. MAX_RADIUS .. '" step="1"'))
        .. '<div class="grid grid-cols-2 gap-3">'
        .. field("near-from", "From (optional)", input("near-from", "from", "date", q.q == "near" and q.from or ""))
        .. field("near-to", "To (optional)", input("near-to", "to", "date", q.q == "near" and q.to or ""))
        .. '</div>'
        .. field("near-offset", "Time zone", offset_select("near-offset", q.q == "near" and q.offset or nil),
            "Used for the date range and for counting days. Auto uses the zone most of the import was recorded in.")
        .. field("near-resource", "Import", import_select("near-resource", imports, q.q == "near" and q.resource or nil))
        .. '<button type="submit" class="bg-blue-600 text-white px-6 py-2 rounded hover:bg-blue-700">Count</button></form>'

    return '<div class="p-4 max-w-4xl">' .. notice
        .. section("lh-at", "Where was I?", at_form .. at_result)
        .. section("lh-near", "How often was I near…?", near_form .. near_result)
        .. section("lh-imports", "Imports", imports_html(imports))
        .. '</div>'
end

-- API ------------------------------------------------------------------------

local function api_imports(ctx)
    maybe_sweep(ctx.principal)
    ctx.query = ctx.query or {}
    local only = positive_int(ctx.query.resource)
    if ctx.query.resource and ctx.query.resource ~= "" and not only then
        ctx.status(400)
        ctx.json({ error = "resource must be a positive whole number" })
        return nil
    end
    return only and visible_imports(only) or visible_imports(nil)
end

local function api_answer(ctx, result, err)
    if not result then
        ctx.status(400)
        ctx.json({ error = err })
    else
        ctx.json(result)
    end
end

function init()
    mah.action({
        id = "import",
        label = "Import location history",
        description = "Reads this Google location export (Takeout Timeline Edits.json, a phone Timeline.json, "
            .. "Records.json or a Semantic Location History file) so you can ask where you were. Replaces an earlier import of this file.",
        entity = "resource",
        placement = { "detail", "card" },
        filters = { content_types = { "application/json", "text/plain", "text/plain; charset=utf-8" } },
        async = true,
        handler = function(ctx)
            local id = ctx.entity_id
            mah.job_progress(ctx.job_id, 2, "Reading the file")
            local resource = mah.db.get_resource(id)
            if not resource then
                mah.job_fail(ctx.job_id, "The resource could not be found.")
                return
            end
            local c, format = read_export(id, function(n)
                mah.job_progress(ctx.job_id, 5, "Read " .. n .. " entries")
            end)
            if not c then
                mah.job_fail(ctx.job_id, "Cannot import: " .. format)
                return
            end
            if #c.points == 0 and #c.segments == 0 then
                mah.job_fail(ctx.job_id, "Cannot import: the " .. format .. " file holds no positions or visits.")
                return
            end
            local meta = store(id, resource.name, format, c, function(done, total)
                mah.job_progress(ctx.job_id, 50 + floor(45 * done / total), "Saved " .. done .. " of " .. total .. " positions")
            end)
            if not meta then
                mah.job_fail(ctx.job_id, "The resource was deleted while it was being imported; nothing was kept.")
                return
            end
            mah.job_complete(ctx.job_id, {
                message = string.format("Imported %d positions and %d visits and trips (%s, %s to %s UTC).",
                    meta.points, meta.segments, format,
                    meta.first and format_time(meta.first, 0) or "-", meta.last and format_time(meta.last, 0) or "-"),
                redirect = PAGE,
            })
        end,
    })

    mah.page("query", page)
    mah.menu("Location History", "query")

    mah.api("GET", "imports", function(ctx)
        local imports = api_imports(ctx)
        if not imports then return end
        local out = {}
        for _, imp in ipairs(imports) do
            local m = imp.meta
            out[#out + 1] = { resource_id = imp.id, name = imp.name, format = m.format, points = m.points,
                segments = m.segments, first = m.first and (format_time(m.first, 0) .. "Z") or nil,
                last = m.last and (format_time(m.last, 0) .. "Z") or nil, imported_at = m.imported_at }
        end
        ctx.json({ imports = mah.json.array(out) })
    end)

    mah.api("GET", "at", function(ctx)
        local imports = api_imports(ctx)
        if not imports then return end
        api_answer(ctx, where_at(imports, ctx.query.time, ctx.query.offset))
    end, { timeout = 120 })

    mah.api("GET", "near", function(ctx)
        local imports = api_imports(ctx)
        if not imports then return end
        api_answer(ctx, how_often_near(imports, ctx.query))
    end, { timeout = 120 })

    mah.on("after_resource_delete", function(data)
        local id = tonumber(data and data.id)
        if id then forget(id) end
    end)
end
