-- Location History: import a Google location export and ask it where you were.
--
-- The import is an async action on the uploaded JSON file. It parses the file,
-- and keeps the positions and visit/activity segments in mah.kv, bucketed by
-- UTC month, under keys that name the resource:
--
--   imp:<id>                   summary: format, counts, range, longest segment
--   idx:<id>                   month index, as columns: months, counts, boxes
--   d:<id>:p:YYYY-MM:<part>    positions, sorted, as columns {t, lat, lng, acc}
--   d:<id>:s:YYYY-MM:<part>    segments starting that month, sorted, as columns
--   w:<id>                     set while an import or a forget is writing
--   sweep:last                 when the orphan sweep last ran
--
-- Nothing is stored more than once and nothing is stored as a table per item:
-- mah.kv.get turns every table it returns into a Lua table of a few kilobytes,
-- so a layout with a table per point, per segment or per month costs memory in
-- proportion to the data on every query.
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
    settings = {
        { name = "max_positions", type = "number", label = "Most positions one import may hold", default = 4000000 },
        { name = "max_segments", type = "number", label = "Most visits and trips one import may hold", default = 400000 },
        { name = "max_near_entries", type = "number", label = "Most separate visits and stays one \"How often\" question may count", default = 1000000 },
    },
}

local PAGE = "/plugins/location-history/query"
local CHUNK = 20000            -- positions per KV value, ~1 MB serialized
-- Times outside 1990-2100 are corrupt, and dropped: a timestamp in the year
-- 33658 would otherwise make an import walk thirty thousand years of months.
local MIN_TIME, MAX_TIME = 631152000, 4102444800
-- An import holds what it has read until it is stored, and "near" holds an
-- entry per separate visit or run of fixes until it has merged them, so each
-- stops past a limit rather than grow with whatever a file holds. Ten years of
-- a dense phone history is under three million positions and a hundred
-- thousand visits and trips. The limits are settings, for a deployment with
-- more or less memory to spare.
local function limit(name, default)
    local n = tonumber(mah.get_setting(name))
    if not n or n < 1 then return default end
    return math.floor(n)
end
local WINDOW = 2 * 3600        -- "at": how far a fix may be from the asked time
local PRESENCE_GAP = 3600      -- "near": fixes further apart start a new presence
local MERGE_GAP = 60           -- "near": times there closer than this are one
local MAX_PLACES = 5           -- "near": places named per row
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
    return { points = {}, segments = {},
        max_points = limit("max_positions", 4000000), max_segments = limit("max_segments", 400000) }
end

-- Marks the collector full, which stops the read (see read_export).
local function full(c, what, limit)
    c.over = c.over or string.format("the export holds more than %d %s, more than one import can take", limit, what)
end

-- A point is held as one fixed-width string, "time|lat|lng|accuracy", until it
-- is stored: a Lua table per point costs about ten times the memory, and the
-- zero-padded time makes the strings sort by time with the built-in
-- comparison instead of a Lua comparator. The accuracy is zero-padded too, so
-- of two fixes at the same time and place the more precise sorts first and is
-- the one kept.
local POINT_FORMAT = "%013d|%.7f|%.7f|%09d"

local function add_point(c, t, lat, lng, acc)
    if not t or t < MIN_TIME or t >= MAX_TIME or not valid(lat, lng) then return end
    if #c.points >= c.max_points then return full(c, "positions", c.max_points) end
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
local SEP = "\31" -- separates the fields of a packed segment

local function text(v)
    if type(v) ~= "string" and type(v) ~= "number" then return nil end
    v = tostring(v)
    -- The separator cannot appear in a field, or the segment would not unpack.
    local at = find(v, SEP, 1, true)
    while at do
        v = sub(v, 1, at - 1) .. sub(v, at + 1)
        at = find(v, SEP, at, true)
    end
    return v
end

-- A segment is held as one string until it is stored, like a point: a table
-- with a dozen fields costs kilobytes. The zero-padded start sorts first.
-- SEGMENT_FIELDS, below, lists the fields; these are packed after the start.
local PACKED = { "k", "e", "lat", "lng", "lat2", "lng2", "place", "name", "type", "off", "edited", "dist" }
local PACKED_NUMBER = { e = true, lat = true, lng = true, lat2 = true, lng2 = true, off = true, dist = true }

local function pack_segment(seg)
    local parts = { string.format("%013d", seg.s) }
    for i, f in ipairs(PACKED) do
        local v = seg[f]
        if v == nil or v == false then
            v = ""
        elseif v == true then
            v = "1"
        elseif PACKED_NUMBER[f] then
            v = string.format("%.17g", v)
        end
        parts[i + 1] = v
    end
    return table.concat(parts, SEP)
end

local function unpack_segment(str)
    local seg, at, i = {}, 1, 0
    while true do
        local stop = find(str, SEP, at, true)
        local v = sub(str, at, (stop or 0) - 1)
        if i == 0 then
            seg.s = tonumber(v)
        else
            local f = PACKED[i]
            if v ~= "" then
                if f == "edited" then
                    seg.edited = true
                elseif PACKED_NUMBER[f] then
                    seg[f] = tonumber(v)
                else
                    seg[f] = v
                end
            end
        end
        if not stop then break end
        at, i = stop + 1, i + 1
    end
    return seg
end

local function add_segment(c, seg)
    if not seg.s or not seg.e or seg.e < seg.s or seg.s < MIN_TIME or seg.e >= MAX_TIME then return end
    if seg.lat and not valid(seg.lat, seg.lng) then return end
    seg.s, seg.e = floor(seg.s + 0.5), floor(seg.e + 0.5)
    if seg.lat then seg.lat, seg.lng = round7(seg.lat), round7(seg.lng) end
    if seg.lat2 then seg.lat2, seg.lng2 = round7(seg.lat2), round7(seg.lng2) end
    if #c.segments >= c.max_segments then return full(c, "visits and trips", c.max_segments) end
    c.segments[#c.segments + 1] = pack_segment(seg)
end

-- v if it is a table, otherwise an empty one: exports are not trusted to hold
-- objects where objects belong, and indexing anything else would fail the
-- whole import.
local function obj(v)
    if type(v) == "table" then return v end
    return {}
end

-- A segment of the current timeline format, as found in Timeline Edits.json
-- (wrapped in .segment) and in the phone export (not wrapped).
local function semantic_segment(c, seg, edited)
    local s, s_off = parse_time(seg.startTime)
    local e = parse_time(seg.endTime)
    local off = tonumber(seg.startTimeTimezoneUtcOffsetMinutes) or s_off
    local inner = type(seg.segment) == "table" and seg.segment or seg
    if type(inner.visit) == "table" then
        local top = obj(inner.visit.topCandidate)
        local lat, lng = coord(top.placeLocation)
        add_segment(c, { k = "visit", s = s, e = e, lat = lat, lng = lng, off = off, edited = edited,
            place = text(top.placeId or top.placeID), type = text(top.semanticType) })
    elseif type(inner.activity) == "table" then
        local a = inner.activity
        local lat, lng = coord(a.start)
        local lat2, lng2 = coord(a["end"])
        local top = obj(a.topCandidate)
        add_segment(c, { k = "activity", s = s, e = e, lat = lat, lng = lng, lat2 = lat2, lng2 = lng2,
            off = off, edited = edited, type = text(top.type), dist = tonumber(a.distanceMeters) })
    end
    if type(inner.timelinePath) == "table" then
        for _, p in ipairs(inner.timelinePath) do
            p = obj(p)
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
        p = obj(p)
        local lat, lng = coord(p)
        add_point(c, parse_time(p.timestamp) or parse_ms(p.timestampMs), lat, lng, p.accuracyMeters)
    end
end

local function timeline_object(c, entry)
    local v, a = entry.placeVisit, entry.activitySegment
    if type(v) == "table" then
        local loc = obj(v.location)
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
        if c.over then return false end
        n = n + 1
        if n % 20000 == 0 then progress(n) end
    end)
    if c.over then return nil, c.over end
    if not ok then return nil, err end
    if not format then
        return nil, "this is not a Google location export this plugin recognises "
            .. "(expected Timeline Edits.json, a phone Timeline.json, Records.json or a Semantic Location History month file)"
    end
    return c, format
end

-- Storage --------------------------------------------------------------------

local function meta_key(id) return "imp:" .. id end
local function index_key(id) return "idx:" .. id end
local function data_prefix(id) return "d:" .. id .. ":" end
local function points_key(id, month, part) return "d:" .. id .. ":p:" .. month .. ":" .. part end
local function segments_key(id, month, part) return "d:" .. id .. ":s:" .. month .. ":" .. part end

-- Set while an import or a forget is writing: to the time an import started,
-- or 0 for a forget. One left behind marks data a stopped write left without a
-- summary; see sweep_orphans.
local function writing_key(id) return "w:" .. id end

-- The fields of a segment, stored as one column each. A missing value is
-- stored as false, since a JSON null would leave a hole in the column.
local SEGMENT_FIELDS = { "k", "s", "e", "lat", "lng", "lat2", "lng2", "place", "name", "type", "off", "edited", "dist" }
local SEGMENT_CHUNK = 5000

local function remove_data(id)
    mah.kv.delete(meta_key(id))
    mah.kv.delete(index_key(id))
    for _, key in ipairs(mah.kv.list(data_prefix(id))) do
        mah.kv.delete(key)
    end
end

local function forget(id)
    -- Mark the forget only where no import (in another server process) holds
    -- the marker, and remove only that mark: overwriting or removing an
    -- import's would leave what it goes on to write with no marker at all.
    local mine = mah.kv.compare_and_set(writing_key(id), mah.kv.ABSENT, 0)
    remove_data(id)
    if mine and mah.kv.get(writing_key(id)) == 0 then mah.kv.delete(writing_key(id)) end
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


-- Writes a parsed export, replacing the resource's earlier import. The summary
-- goes last, so a failure part way leaves no summary and therefore nothing a
-- query would read; the next import clears the leftovers.
local function store(id, name, format, c, progress)
    table.sort(c.points)
    table.sort(c.segments)

    local months = {}
    local function month(key)
        local m = months[key]
        if not m then
            m = { p = 0, parts = 0, s = 0, sparts = 0 }
            months[key] = m
        end
        return m
    end

    mah.kv.set(writing_key(id), mah.util.now())
    remove_data(id)

    -- Segments, each once, under the month it starts in. A query looking for
    -- segments overlapping a time looks back as far as the longest one.
    local span = 0
    local seg_month, seg_cols, seg_n = nil, nil, 0
    local function write_segment_part()
        if seg_n == 0 then return end
        local m = month(seg_month)
        m.sparts = m.sparts + 1
        m.s = m.s + seg_n
        mah.kv.set(segments_key(id, seg_month, m.sparts), seg_cols)
        seg_cols, seg_n = nil, 0
    end
    local offsets, usual, usual_n = {}, nil, 0
    local seg_first, seg_last = nil, nil
    for _, packed in ipairs(c.segments) do
        local seg = unpack_segment(packed)
        if seg.off then
            offsets[seg.off] = (offsets[seg.off] or 0) + 1
            if offsets[seg.off] > usual_n then usual, usual_n = seg.off, offsets[seg.off] end
        end
        seg_first = seg_first or seg.s
        if not seg_last or seg.e > seg_last then seg_last = seg.e end
        local key = month_of(seg.s)
        if key ~= seg_month then
            write_segment_part()
            seg_month = key
        end
        if not seg_cols then
            seg_cols = {}
            for _, f in ipairs(SEGMENT_FIELDS) do seg_cols[f] = {} end
        end
        seg_n = seg_n + 1
        for _, f in ipairs(SEGMENT_FIELDS) do
            local v = seg[f]
            if v == nil then v = false end
            seg_cols[f][seg_n] = v
        end
        local m = month(key)
        m.box = grow_box(grow_box(m.box, seg.lat, seg.lng), seg.lat2, seg.lng2)
        if seg.e - seg.s > span then span = seg.e - seg.s end
        if seg_n == SEGMENT_CHUNK then write_segment_part() end
    end
    write_segment_part()

    -- Points arrive sorted, so they are written a part at a time as they
    -- come, each part as four columns of numbers. A table per point costs
    -- kilobytes once mah.kv.get hands it back, which made a dense month cost
    -- gigabytes to query; columns cost sixteen bytes a number.
    local current, until_t = nil, -1
    local cols, n = nil, 0
    local function write_part()
        if n == 0 then return end
        local m = month(current)
        m.parts = m.parts + 1
        m.p = m.p + n
        mah.kv.set(points_key(id, current, m.parts), cols)
        cols, n = nil, 0
    end
    local kept, last_t, last_lat, last_lng = 0, nil, nil, nil
    local first, final = nil, nil
    for i, str in ipairs(c.points) do
        local t, lat, lng, acc = decode_point(str)
        if not (t == last_t and lat == last_lat and lng == last_lng) then
            if t >= until_t then
                write_part()
                current = month_of(t)
                until_t = days_from_civil(tonumber(next_month(current):sub(1, 4)), tonumber(next_month(current):sub(6, 7)), 1) * 86400
            end
            if not cols then cols = { t = {}, lat = {}, lng = {}, acc = {} } end
            n = n + 1
            cols.t[n], cols.lat[n], cols.lng[n], cols.acc[n] = t, lat, lng, acc
            local m = month(current)
            m.box = grow_box(m.box, lat, lng)
            kept = kept + 1
            last_t, last_lat, last_lng = t, lat, lng
            first = first or t
            final = t
            if n == CHUNK then write_part() end
        end
        if i % 50000 == 0 then progress(i, #c.points) end
    end
    write_part()
    c.points = nil

    -- The month index, as columns in month order.
    local index = { m = {}, p = {}, parts = {}, s = {}, sparts = {}, b1 = {}, b2 = {}, b3 = {}, b4 = {} }
    for i, key in ipairs(sorted_keys(months)) do
        local m = months[key]
        local box = m.box or { false, false, false, false }
        index.m[i], index.p[i], index.parts[i], index.s[i], index.sparts[i] = key, m.p, m.parts, m.s, m.sparts
        index.b1[i], index.b2[i], index.b3[i], index.b4[i] = box[1], box[2], box[3], box[4]
    end
    mah.kv.set(index_key(id), index)

    if seg_first then
        first = math.min(first or seg_first, seg_first)
        final = math.max(final or seg_last, seg_last)
    end
    local meta = {
        id = id, name = name, format = format, points = kept, segments = #c.segments,
        first = first, last = final, offset = usual, span = span,
        imported_at = mah.util.now_iso(),
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
-- import or forget that stopped before finishing (a timeout part way through
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
    -- Every write that can stop part way -- an import, a forget -- leaves its
    -- marker until it finishes, so data without a summary always has one.
    local now = mah.util.now()
    for _, key in ipairs(mah.kv.list("w:")) do
        local id = tonumber(key:match("^w:(%d+)$"))
        -- Read again just before forgetting: in another process, an import may
        -- have finished (summary written, marker removed) since the list above.
        local started = id and tonumber(mah.kv.get(key))
        if started and not summarised[id] and mah.kv.get(meta_key(id)) == nil and now - started > WRITING_GRACE then
            forget(id)
            -- forget leaves a marker it did not set; this one is abandoned.
            if mah.kv.get(key) == started then mah.kv.delete(key) end
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

-- An import's month index, loaded once per request and only for the imports a
-- query reads. idx.at maps a month to its row.
local function index_of(imp)
    if imp.index then return imp.index end
    local idx = mah.kv.get(index_key(imp.id))
    if type(idx) ~= "table" or type(idx.m) ~= "table" then idx = { m = {} } end
    idx.at = {}
    for i, key in ipairs(idx.m) do idx.at[key] = i end
    imp.index = idx
    return idx
end

-- The box of a month's row, or nil.
local function box_of(idx, i)
    if idx.b1[i] == false then return nil end
    return { idx.b1[i], idx.b2[i], idx.b3[i], idx.b4[i] }
end

-- Calls fn(cols) with each part of a month's positions in time order, one
-- part in memory at a time, until fn returns false. cols holds the columns t,
-- lat, lng and acc.
local function each_part(imp, month, fn)
    local idx = index_of(imp)
    local i = idx.at[month]
    if not i or idx.p[i] == 0 then return end
    for part = 1, idx.parts[i] do
        local cols = mah.kv.get(points_key(imp.id, month, part))
        if type(cols) == "table" and type(cols.t) == "table" and fn(cols) == false then return end
    end
end

-- Calls fn(cols, n) with each part of the segments starting in a month, in
-- start order. cols holds a column per SEGMENT_FIELDS name, false for missing.
local function each_segment_part(imp, month, fn)
    local idx = index_of(imp)
    local i = idx.at[month]
    if not i or idx.s[i] == 0 then return end
    for part = 1, idx.sparts[i] do
        local cols = mah.kv.get(segments_key(imp.id, month, part))
        if type(cols) == "table" and type(cols.s) == "table" then fn(cols, #cols.s) end
    end
end

-- One segment out of its columns, as a table.
local function segment_at(cols, i)
    local seg = {}
    for _, f in ipairs(SEGMENT_FIELDS) do
        local v = cols[f][i]
        if v ~= false then seg[f] = v end
    end
    return seg
end

-- Months of an import with data between two times, oldest first. Only months
-- the import has are visited, however far apart the times.
local function months_between(imp, from_t, to_t)
    local out, from_key, to_key = {}, month_of(math.max(from_t, MIN_TIME)), month_of(math.min(to_t, MAX_TIME - 1))
    for _, key in ipairs(index_of(imp).m) do
        if key >= from_key and key <= to_key then out[#out + 1] = key end
    end
    return out
end

-- The largest index in the sorted times ts whose time is before t, or 0.
local function last_before(ts, t)
    local lo, hi, found = 1, #ts, 0
    while lo <= hi do
        local mid = floor((lo + hi) / 2)
        if ts[mid] < t then found, lo = mid, mid + 1 else hi = mid - 1 end
    end
    return found
end

local function point_at(cols, i)
    return { cols.t[i], cols.lat[i], cols.lng[i], cols.acc[i] }
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
        for _, key in ipairs(months_between(imp, t - 86400 - (imp.meta.span or 0), t + 86400)) do
            each_segment_part(imp, key, function(cols, n)
                for i = 1, n do
                    local seg_off, ss, se = cols.off[i], cols.s[i], cols.e[i]
                    if seg_off ~= false then
                        local gap = (t < ss and ss - t) or (t > se and t - se) or 0
                        if gap < best_gap or (gap == best_gap and best == nil) then best, best_gap = seg_off, gap end
                    end
                end
            end)
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
        local keys = index_of(imp).m
        local here = month_of(t)
        local first, last, step = #keys, 1, -1
        if dir > 0 then first, last, step = 1, #keys, 1 end
        for i = first, last, step do
            local key = keys[i]
            if (dir < 0 and key <= here) or (dir > 0 and key >= here) then
                local found = nil
                each_part(imp, key, function(cols)
                    local i = last_before(cols.t, t)
                    if dir < 0 then
                        if i > 0 then found = point_at(cols, i) end
                        -- A later part holds only later times.
                        return i == #cols.t
                    end
                    if i < #cols.t then
                        found = point_at(cols, i + 1)
                        return false
                    end
                end)
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
        for _, key in ipairs(months_between(imp, t - (imp.meta.span or 0), t)) do
            each_segment_part(imp, key, function(cols, n)
                for i = 1, n do
                    if cols.s[i] <= t and t <= cols.e[i] then
                        local candidate = segment_at(cols, i)
                        if better_segment(seg, candidate) then seg = candidate end
                    end
                end
            end)
        end
    end
    -- Fixes are whole seconds: the last at or before t, the first after it.
    -- No data lies outside MIN_TIME..MAX_TIME, and month keys only sort as
    -- times inside it, so the search starts from t clamped there.
    local search = math.max(MIN_TIME, math.min(floor(t) + 1, MAX_TIME - 1))
    local before = nearest_fix(imports, search, -1)
    local after = nearest_fix(imports, search, 1)

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

    -- Each import gives two lists already in time order: its visits near the
    -- place and its presences (runs of fixes near it). They are kept as columns
    -- of numbers, joined as they grow when they overlap, and then merged across
    -- lists into a running summary that keeps only the newest MAX_ROWS rows, so
    -- memory follows the number of separate times there, not of fixes.
    local lists, entries = {}, 0
    local max_entries = limit("max_near_entries", 1000000)
    local function new_list()
        local list = { n = 0, s = {}, e = {}, fixes = {}, visits = {}, d = {}, places = {} }
        lists[#lists + 1] = list
        return list
    end
    local function add(list, s0, e0, fixes, visits, d, place)
        local n = list.n
        if n > 0 and s0 <= list.e[n] + MERGE_GAP then
            if e0 > list.e[n] then list.e[n] = e0 end
            list.fixes[n], list.visits[n] = list.fixes[n] + fixes, list.visits[n] + visits
            if d < list.d[n] then list.d[n] = d end
        else
            n = n + 1
            entries = entries + 1
            list.n, list.s[n], list.e[n], list.fixes[n], list.visits[n], list.d[n] = n, s0, e0, fixes, visits, d
        end
        -- One place per entry, as a string column: the first visit's.
        if place and not list.places[n] then list.places[n] = place end
    end

    for _, imp in ipairs(imports) do
        if entries > max_entries then break end
        local visits, presences = new_list(), new_list()
        local in_presence, last_t = false, nil
        local idx = index_of(imp)
        -- A segment is stored under the month it starts in, so one overlapping
        -- the range can start up to the import's longest segment before it.
        local seg_from = from_t - (imp.meta.span or 0)
        for i, key in ipairs(idx.m) do
            if entries > max_entries then break end
            local month_start = days_from_civil(tonumber(key:sub(1, 4)), tonumber(key:sub(6, 7)), 1) * 86400
            local month_end = days_from_civil(tonumber(next_month(key):sub(1, 4)), tonumber(next_month(key):sub(6, 7)), 1) * 86400
            local box = box_of(idx, i)
            local near = box and distance_to_box(lat, lng, box) <= radius
            if near and month_end > seg_from and month_start <= to_t then
                -- Months in order and segments in start order within each: the
                -- visits list stays in start order, clipped or not.
                each_segment_part(imp, key, function(cols, n)
                    for j = 1, n do
                        local slat, slng, ss, se = cols.lat[j], cols.lng[j], cols.s[j], cols.e[j]
                        if cols.k[j] == "visit" and slat ~= false and se >= from_t and ss <= to_t then
                            local d = distance(lat, lng, slat, slng)
                            if d <= radius then
                                local place = cols.name[j]
                                if place == false then place = cols.place[j] end
                                -- Only the part of a visit inside the range counts.
                                add(visits, math.max(ss, from_t), math.min(se, to_t), 0, 1, d, place or nil)
                                if entries > max_entries then return end
                            end
                        end
                    end
                end)
            end
            if near and month_end > from_t and month_start <= to_t then
                each_part(imp, key, function(cols)
                    local ts, lats, lngs, accs = cols.t, cols.lat, cols.lng, cols.acc
                    for i = 1, #ts do
                        local pt = ts[i]
                        if pt >= from_t and pt <= to_t then
                            local d = distance(lat, lng, lats[i], lngs[i])
                            if d <= radius then
                                local n = presences.n
                                if in_presence and n > 0 and pt - last_t <= PRESENCE_GAP then
                                    presences.e[n], presences.fixes[n] = pt, presences.fixes[n] + 1
                                    if d < presences.d[n] then presences.d[n] = d end
                                else
                                    add(presences, pt, pt, 1, 0, d, nil)
                                    if entries > max_entries then return false end
                                end
                                in_presence, last_t = true, pt
                            elseif d - accs[i] > radius then
                                -- Only a fix whose whole accuracy circle is outside
                                -- ends a presence; a vague one may still be there.
                                in_presence = false
                            end
                        end
                    end
                end)
            else
                -- A month that cannot hold a fix nearby ends any presence.
                in_presence = false
            end
        end
    end

    if entries > max_entries then
        return nil, "more than " .. max_entries .. " separate visits and stays match; narrow the radius or the dates"
    end

    -- Merge the lists in time order. Overlapping visits and presences, from
    -- any import, are one time there.
    -- The stay being built is held in locals and a finished one is copied into
    -- a reused slot of the ring of the newest MAX_ROWS, so merging allocates
    -- nothing per stay.
    local days, day_count, total, first, last, times = {}, 0, 0, nil, nil, 0
    local ring = {}
    local open, ts, te, tf, tv, td, tplaces = false, 0, 0, 0, 0, 0, {}
    local function finish()
        times = times + 1
        total = total + (te - ts)
        first = first and math.min(first, ts) or ts
        last = last and math.max(last, te) or te
        for day = local_day(ts, off), local_day(te, off) do
            if not days[day] then days[day] = true; day_count = day_count + 1 end
        end
        local slot = (times - 1) % MAX_ROWS + 1
        local row = ring[slot] or { places = {} }
        ring[slot] = row
        row.s, row.e, row.fixes, row.visits, row.d = ts, te, tf, tv, td
        for k = 1, MAX_PLACES do row.places[k] = tplaces[k] end
    end
    local heads = {}
    for i = 1, #lists do heads[i] = 1 end
    while true do
        local pick = nil
        for i, list in ipairs(lists) do
            if heads[i] <= list.n and (not pick or list.s[heads[i]] < lists[pick].s[heads[pick]]) then pick = i end
        end
        if not pick then break end
        local list, j = lists[pick], heads[pick]
        heads[pick] = j + 1
        local s0, e0 = list.s[j], list.e[j]
        if open and s0 <= te + MERGE_GAP then
            if e0 > te then te = e0 end
            tf, tv = tf + list.fixes[j], tv + list.visits[j]
            if list.d[j] < td then td = list.d[j] end
        else
            if open then finish() end
            open, ts, te, tf, tv, td = true, s0, e0, list.fixes[j], list.visits[j], list.d[j]
            for k = 1, MAX_PLACES do tplaces[k] = nil end
        end
        -- A row names at most MAX_PLACES places, so a stay made of many
        -- visits holds a few strings, not one per visit.
        local place = list.places[j]
        if place then
            for k = 1, MAX_PLACES do
                if tplaces[k] == place then break end
                if tplaces[k] == nil then tplaces[k] = place; break end
            end
        end
    end
    if open then finish() end

    local rows = {}
    for k = 0, math.min(times, MAX_ROWS) - 1 do
        local iv = ring[(times - 1 - k) % MAX_ROWS + 1]
        local places = {}
        for k = 1, MAX_PLACES do places[k] = iv.places[k] end
        rows[#rows + 1] = { start = format_time(iv.s, off), ["end"] = format_time(iv.e, off), seconds = iv.e - iv.s,
            fixes = iv.fixes, visits = iv.visits, places = mah.json.array(places), closest_m = floor(iv.d + 0.5) }
    end

    return {
        query = { lat = lat, lng = lng, radius_m = radius, offset = format_offset(off), offset_minutes = off,
            offset_source = how, from = args.from, to = args.to },
        times = times, days = day_count, total_seconds = total,
        first = first and format_time(first, off) or nil, last = last and format_time(last, off) or nil,
        rows = mah.json.array(rows), rows_omitted = times - #rows,
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
