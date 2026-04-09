-- Data Views plugin for mahresources
-- Provides 17 data-viewing shortcodes for rich display of meta values,
-- charts, tables, and more.
-- Usage in templates: [plugin:data-views:shortcode-name attr="value"]

plugin = {
    name = "data-views",
    version = "1.0",
    description = "18 data viewing shortcodes for rich display of meta values, charts, tables, and more.",
}

-- ---------------------------------------------------------------------------
-- Shared Helpers
-- ---------------------------------------------------------------------------

--- Escape HTML special characters.
local function html_escape(str)
    if str == nil then return "" end
    str = tostring(str)
    str = str:gsub("&", "&amp;")
    str = str:gsub("<", "&lt;")
    str = str:gsub(">", "&gt;")
    str = str:gsub('"', "&quot;")
    str = str:gsub("'", "&#39;")
    return str
end

--- Navigate a dot-separated path inside a table.
local function get_nested(tbl, path)
    if not tbl or not path then return nil end
    local current = tbl
    for segment in path:gmatch("[^%.]+") do
        if type(current) ~= "table" then return nil end
        current = current[segment]
    end
    return current
end

--- Split comma-separated string into an array.
local function parse_csv(str)
    local result = {}
    if not str or str == "" then return result end
    for item in str:gmatch("[^,]+") do
        result[#result + 1] = item:match("^%s*(.-)%s*$")
    end
    return result
end

--- Lua value to JSON string for HTML embedding.
local function json_value(val)
    if val == nil then return "null" end
    local t = type(val)
    if t == "boolean" then return val and "true" or "false" end
    if t == "number" then return tostring(val) end
    if t == "string" then return mah.json.encode(val) end
    if t == "table" then return mah.json.encode(val) end
    return "null"
end

--- Clamp a number.
local function clamp(n, lo, hi)
    if n < lo then return lo end
    if n > hi then return hi end
    return n
end

-- ---------------------------------------------------------------------------
-- Formatting Helpers
-- ---------------------------------------------------------------------------

--- Add thousands separators: 1234567.89 -> "1,234,567.89"
local function format_number(n, decimals)
    if not n then return "0" end
    n = tonumber(n)
    if not n then return "0" end

    local formatted
    if decimals then
        formatted = string.format("%." .. decimals .. "f", n)
    else
        -- Auto: remove trailing zeros after decimal
        formatted = string.format("%.2f", n)
        formatted = formatted:gsub("%.?0+$", "")
        if formatted == "" then formatted = "0" end
    end

    -- Insert thousands separators in the integer part.
    local int_part, dec_part = formatted:match("^(-?%d+)(%.?.*)$")
    if not int_part then return formatted end
    int_part = int_part:reverse():gsub("(%d%d%d)", "%1,"):reverse():gsub("^,", ""):gsub("^(-),", "-%1")
    return int_part .. dec_part
end

--- Human-readable file size.
local function format_filesize(bytes)
    bytes = tonumber(bytes)
    if not bytes then return "0 B" end
    local units = {"B", "KB", "MB", "GB", "TB"}
    local i = 1
    local val = bytes
    while val >= 1024 and i < #units do
        val = val / 1024
        i = i + 1
    end
    if i == 1 then return string.format("%d B", val) end
    return string.format("%.1f %s", val, units[i])
end

--- Duration: 3661 -> "1h 1m 1s"
local function format_duration(seconds)
    seconds = tonumber(seconds)
    if not seconds then return "0s" end
    seconds = math.floor(seconds)
    if seconds < 0 then seconds = 0 end
    local h = math.floor(seconds / 3600)
    local m = math.floor((seconds % 3600) / 60)
    local s = seconds % 60
    local parts = {}
    if h > 0 then parts[#parts + 1] = h .. "h" end
    if m > 0 then parts[#parts + 1] = m .. "m" end
    if s > 0 or #parts == 0 then parts[#parts + 1] = s .. "s" end
    return table.concat(parts, " ")
end

--- ISO date string -> "Jan 15, 2024"
local function format_date(iso)
    if not iso or type(iso) ~= "string" then return "" end
    local y, mo, d = iso:match("(%d%d%d%d)-(%d%d)-(%d%d)")
    if not y then return iso end
    local months = {"Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"}
    local mi = tonumber(mo)
    if mi and mi >= 1 and mi <= 12 then
        return string.format("%s %d, %s", months[mi], tonumber(d), y)
    end
    return iso
end

--- Unified formatter dispatcher.
local function format_value(val, fmt_type, attrs)
    attrs = attrs or {}
    if val == nil then return "" end
    local result
    if fmt_type == "currency" then
        local sym = attrs["currency"] or "$"
        local dec = tonumber(attrs["decimals"]) or 2
        result = sym .. format_number(val, dec)
    elseif fmt_type == "percent" then
        local dec = tonumber(attrs["decimals"]) or 1
        result = format_number(tonumber(val) or 0, dec) .. "%"
    elseif fmt_type == "date" then
        result = format_date(tostring(val))
    elseif fmt_type == "filesize" then
        result = format_filesize(val)
    elseif fmt_type == "number" then
        local dec = tonumber(attrs["decimals"])
        result = format_number(val, dec)
    elseif fmt_type == "duration" then
        result = format_duration(val)
    else
        result = tostring(val)
    end

    local prefix = attrs["prefix"] or ""
    local suffix = attrs["suffix"] or ""
    return prefix .. result .. suffix
end

-- ---------------------------------------------------------------------------
-- SVG Icon Helpers
-- ---------------------------------------------------------------------------

local ICONS = {
    chart = '<svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 3v18h18"/><rect x="7" y="10" width="3" height="8" rx="0.5"/><rect x="12" y="6" width="3" height="12" rx="0.5"/><rect x="17" y="3" width="3" height="15" rx="0.5"/></svg>',
    users = '<svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="9" cy="7" r="4"/><path d="M3 21v-2a4 4 0 014-4h4a4 4 0 014 4v2"/><circle cx="17" cy="9" r="3"/><path d="M21 21v-2a3 3 0 00-3-3h-1"/></svg>',
    files = '<svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 2h10l6 6v14H4V2z"/><path d="M14 2v6h6"/></svg>',
    clock = '<svg xmlns="http://www.w3.org/2000/svg" class="w-6 h-6" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/></svg>',
    check = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 8l4 4 6-7"/></svg>',
    file = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 1h7l3 3v11H3V1z"/><path d="M10 1v3h3"/></svg>',
    note = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 1h10v14H3V1z"/><path d="M5 5h6M5 8h6M5 11h3"/></svg>',
    folder = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 3h5l2 2h7v9H1V3z"/></svg>',
    star = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M8 1l2.2 4.5 5 .7-3.6 3.5.9 5L8 12.4 3.5 14.7l.9-5L.8 6.2l5-.7z"/></svg>',
    globe = '<svg xmlns="http://www.w3.org/2000/svg" class="w-8 h-8" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><path d="M2 12h20M12 2a15.3 15.3 0 014 10 15.3 15.3 0 01-4 10 15.3 15.3 0 01-4-10A15.3 15.3 0 0112 2z"/></svg>',
    external = '<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M12 9v4H3V4h4"/><path d="M8 8L14 2m0 0h-4m4 0v4"/></svg>',
}

local function get_icon(name, size_class)
    local svg = ICONS[name]
    if not svg then return "" end
    if size_class then
        svg = svg:gsub('class="[^"]*"', 'class="' .. size_class .. '"', 1)
    end
    return svg
end

-- ---------------------------------------------------------------------------
-- Default color palette for charts
-- ---------------------------------------------------------------------------

local DEFAULT_COLORS = {"#d97706", "#3b82f6", "#ef4444", "#22c55e", "#8b5cf6", "#ec4899", "#06b6d4", "#6b7280"}

-- ---------------------------------------------------------------------------
-- Base64 Decoder (for embed shortcode)
-- ---------------------------------------------------------------------------

local b64chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
local b64lookup = {}
for i = 1, #b64chars do
    b64lookup[b64chars:byte(i)] = i - 1
end

local function base64_decode(data)
    if not data then return "" end
    data = data:gsub("[^" .. b64chars .. "=]", "")
    local result = {}
    local i = 1
    while i <= #data do
        local a = b64lookup[data:byte(i)] or 0
        local b = b64lookup[data:byte(i + 1)] or 0
        local c = b64lookup[data:byte(i + 2)] or 0
        local d = b64lookup[data:byte(i + 3)] or 0
        local n = a * 262144 + b * 4096 + c * 64 + d
        result[#result + 1] = string.char(
            math.floor(n / 65536) % 256,
            math.floor(n / 256) % 256,
            n % 256
        )
        i = i + 4
    end
    local str = table.concat(result)
    -- Trim padding bytes
    local pad = data:match("(=+)$")
    if pad then
        str = str:sub(1, #str - #pad)
    end
    return str
end

-- ---------------------------------------------------------------------------
-- Code 128 Barcode (for qr-code shortcode)
-- ---------------------------------------------------------------------------

-- Code 128 patterns: each character is encoded as 6 alternating bar/space widths.
-- Code Set B covers ASCII 32-127 (printable characters).
local CODE128_PATTERNS = {
    [0]  = {2,1,2,2,2,2}, [1]  = {2,2,2,1,2,2}, [2]  = {2,2,2,2,2,1},
    [3]  = {1,2,1,2,2,3}, [4]  = {1,2,1,3,2,2}, [5]  = {1,3,1,2,2,2},
    [6]  = {1,2,2,2,1,3}, [7]  = {1,2,2,3,1,2}, [8]  = {1,3,2,2,1,2},
    [9]  = {2,2,1,2,1,3}, [10] = {2,2,1,3,1,2}, [11] = {2,3,1,2,1,2},
    [12] = {1,1,2,2,3,2}, [13] = {1,2,2,1,3,2}, [14] = {1,2,2,2,3,1},
    [15] = {1,1,3,2,2,2}, [16] = {1,2,3,1,2,2}, [17] = {1,2,3,2,2,1},
    [18] = {2,2,3,2,1,1}, [19] = {2,2,1,1,3,2}, [20] = {2,2,1,2,3,1},
    [21] = {2,1,3,2,1,2}, [22] = {2,2,3,1,1,2}, [23] = {3,1,2,1,3,1},
    [24] = {3,1,1,2,2,2}, [25] = {3,2,1,1,2,2}, [26] = {3,2,1,2,2,1},
    [27] = {3,1,2,2,1,2}, [28] = {3,2,2,1,1,2}, [29] = {3,2,2,2,1,1},
    [30] = {2,1,2,1,2,3}, [31] = {2,1,2,3,2,1}, [32] = {2,3,2,1,2,1},
    [33] = {1,1,1,3,2,3}, [34] = {1,3,1,1,2,3}, [35] = {1,3,1,3,2,1},
    [36] = {1,1,2,3,1,3}, [37] = {1,3,2,1,1,3}, [38] = {1,3,2,3,1,1},
    [39] = {2,1,1,3,1,3}, [40] = {2,3,1,1,1,3}, [41] = {2,3,1,3,1,1},
    [42] = {1,1,2,1,3,3}, [43] = {1,1,2,3,3,1}, [44] = {1,3,2,1,3,1},
    [45] = {1,1,3,1,2,3}, [46] = {1,1,3,3,2,1}, [47] = {1,3,3,1,2,1},
    [48] = {3,1,3,1,2,1}, [49] = {2,1,1,3,3,1}, [50] = {2,3,1,1,3,1},
    [51] = {2,1,3,1,1,3}, [52] = {2,1,3,3,1,1}, [53] = {2,1,3,1,3,1},
    [54] = {3,1,1,1,2,3}, [55] = {3,1,1,3,2,1}, [56] = {3,3,1,1,2,1},
    [57] = {3,1,2,1,1,3}, [58] = {3,1,2,3,1,1}, [59] = {3,3,2,1,1,1},
    [60] = {2,1,1,2,1,3}, [61] = {2,1,1,2,3,1}, [62] = {2,3,1,2,1,1},
    [63] = {1,2,1,1,2,3}, [64] = {1,2,1,3,2,1}, [65] = {1,2,1,1,3,2},  -- Note: 65 is not used directly
    -- Special codes
    [66] = {1,2,3,1,1,2}, [67] = {1,2,1,2,3,1}, [68] = {1,1,1,2,2,3},  -- Note: 65-68 are less common
    [69] = {1,2,2,3,1,1}, [70] = {1,1,3,2,1,2}, [71] = {2,1,2,2,1,3},
    [72] = {2,1,2,2,3,1}, [73] = {2,1,1,1,3,2}, [74] = {3,2,1,2,1,1},
    [75] = {3,1,1,2,1,2}, [76] = {1,1,2,2,1,3}, [77] = {1,1,2,2,3,1},
    [78] = {1,3,2,2,1,1}, [79] = {2,2,1,1,2,3}, [80] = {2,2,1,3,1,1},  -- Note: corrections may be needed
    [81] = {3,2,1,1,1,2}, [82] = {3,1,1,1,3,1}, [83] = {1,1,3,1,1,3},
    [84] = {1,3,1,1,1,3}, [85] = {1,3,1,3,1,1}, [86] = {1,1,1,3,1,3},
    [87] = {1,3,1,1,3,1}, [88] = {1,1,1,3,3,1}, [89] = {3,1,1,1,1,3},
    [90] = {3,1,1,3,1,1}, [91] = {3,3,1,1,1,1}, [92] = {3,1,4,1,1,1},
    [93] = {2,2,1,4,1,1}, [94] = {4,3,1,1,1,1}, [95] = {1,1,1,2,2,4},
    [96] = {1,1,1,4,2,2}, [97] = {1,2,1,1,2,4}, [98] = {1,2,1,4,2,1},
    [99] = {1,4,1,1,2,2}, [100] = {1,4,1,2,2,1}, [101] = {2,4,1,2,1,1},
    [102] = {2,2,1,1,4,1}, [103] = {2,1,1,2,4,1},
    -- Start codes
    [104] = {2,1,1,4,1,2}, -- Start Code B
    [105] = {2,1,4,1,1,2}, -- Start Code A (unused here)
    [106] = {2,3,3,1,1,1,2}, -- Stop (7 elements)
}

--- Encode text as Code 128 Set B and return SVG string.
local function code128_svg(text, height)
    if not text or text == "" then return "" end
    height = height or 50

    -- Start Code B = 104
    local codes = {104}
    local checksum = 104

    for i = 1, #text do
        local byte = text:byte(i)
        local code_val = byte - 32
        if code_val < 0 or code_val > 95 then
            code_val = 0 -- Replace unprintable with space
        end
        codes[#codes + 1] = code_val
        checksum = checksum + code_val * i
    end

    -- Checksum
    codes[#codes + 1] = checksum % 103
    -- Stop code
    codes[#codes + 1] = 106

    -- Build bars
    local bars = {}
    local x = 0
    local quiet_zone = 10 -- Quiet zone width

    x = quiet_zone
    for _, code in ipairs(codes) do
        local pattern = CODE128_PATTERNS[code]
        if pattern then
            for j = 1, #pattern do
                local w = pattern[j]
                if j % 2 == 1 then
                    -- Bar (odd positions are bars)
                    bars[#bars + 1] = string.format(
                        '<rect x="%d" y="0" width="%d" height="%d" fill="currentColor"/>',
                        x, w, height
                    )
                end
                x = x + w
            end
        end
    end

    local total_width = x + quiet_zone
    return string.format(
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" class="inline-block" style="max-width: 100%%; height: auto;">'
        .. '%s</svg>',
        total_width, height,
        table.concat(bars)
    )
end

-- ---------------------------------------------------------------------------
-- JSON Tree Renderer (recursive, for json-tree shortcode)
-- ---------------------------------------------------------------------------

local function render_json_tree(val, depth, max_expand, path_prefix)
    depth = depth or 0
    max_expand = max_expand or 2
    path_prefix = path_prefix or "root"

    if val == nil then
        return '<span class="text-stone-400">null</span>'
    end

    local t = type(val)

    if t == "string" then
        return '<span class="text-green-700">&quot;' .. html_escape(val) .. '&quot;</span>'
    elseif t == "number" then
        return '<span class="text-blue-700">' .. tostring(val) .. '</span>'
    elseif t == "boolean" then
        return '<span class="text-purple-700">' .. tostring(val) .. '</span>'
    elseif t ~= "table" then
        return '<span class="text-stone-400">' .. html_escape(tostring(val)) .. '</span>'
    end

    -- Determine if array or object
    local is_array = false
    local count = 0
    for k, _ in pairs(val) do
        count = count + 1
        if type(k) ~= "number" then
            is_array = false
            break
        end
        is_array = true
    end

    if count == 0 then
        if is_array then return '<span class="text-stone-400">[]</span>' end
        return '<span class="text-stone-400">{}</span>'
    end

    local expanded = depth < max_expand
    local toggle_id = path_prefix:gsub("[^%w]", "_")
    local open_bracket = is_array and "[" or "{"
    local close_bracket = is_array and "]" or "}"

    local parts = {}
    parts[#parts + 1] = string.format(
        '<span class="cursor-pointer select-none text-stone-500 hover:text-stone-800" '
        .. '@click="%s = !%s">',
        toggle_id, toggle_id
    )
    parts[#parts + 1] = string.format(
        '<span x-text="%s ? \'\\u25BC\' : \'\\u25B6\'" class="inline-block w-3 text-[10px]"></span>',
        toggle_id
    )
    parts[#parts + 1] = '<span class="text-stone-500">' .. open_bracket .. '</span>'
    parts[#parts + 1] = string.format(
        '<span x-show="!%s" class="text-stone-400"> %d items... </span>',
        toggle_id, count
    )
    parts[#parts + 1] = '</span>'
    parts[#parts + 1] = string.format('<div x-show="%s" class="ml-4 border-l border-stone-200 pl-2">', toggle_id)

    -- Sort keys for objects
    local keys = {}
    if is_array then
        for i = 1, #val do keys[#keys + 1] = i end
    else
        for k, _ in pairs(val) do keys[#keys + 1] = k end
        table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
    end

    for _, k in ipairs(keys) do
        local v = val[k]
        local child_path = path_prefix .. "_" .. tostring(k)
        parts[#parts + 1] = '<div>'
        if not is_array then
            parts[#parts + 1] = '<span class="text-stone-800 font-bold">' .. html_escape(tostring(k)) .. '</span>: '
        else
            parts[#parts + 1] = '<span class="text-stone-400">' .. tostring(k - 1) .. ': </span>'
        end
        parts[#parts + 1] = render_json_tree(v, depth + 1, max_expand, child_path)
        parts[#parts + 1] = '</div>'
    end

    parts[#parts + 1] = '</div>'
    parts[#parts + 1] = '<span class="text-stone-500">' .. close_bracket .. '</span>'

    return table.concat(parts)
end

--- Build the x-data initialization object for the JSON tree (expanded state).
local function build_tree_state(val, depth, max_expand, path_prefix)
    depth = depth or 0
    max_expand = max_expand or 2
    path_prefix = path_prefix or "root"
    local entries = {}

    if type(val) == "table" then
        local count = 0
        for _ in pairs(val) do count = count + 1 end
        if count > 0 then
            local toggle_id = path_prefix:gsub("[^%w]", "_")
            local expanded = depth < max_expand
            entries[#entries + 1] = toggle_id .. ": " .. (expanded and "true" or "false")

            for k, v in pairs(val) do
                local child_path = path_prefix .. "_" .. tostring(k)
                local child_entries = build_tree_state(v, depth + 1, max_expand, child_path)
                if child_entries ~= "" then
                    entries[#entries + 1] = child_entries
                end
            end
        end
    end

    return table.concat(entries, ", ")
end

-- ---------------------------------------------------------------------------
-- Error helper
-- ---------------------------------------------------------------------------

local function shortcode_error(name, msg)
    return string.format(
        '<div class="py-1.5"><span class="text-sm text-red-500">%s: %s</span></div>',
        html_escape(name), html_escape(msg)
    )
end

-- ---------------------------------------------------------------------------
-- 1. badge
-- ---------------------------------------------------------------------------

local function render_badge(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("badge", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then return '<div class="py-1.5"></div>' end
    val = tostring(val)

    local values = parse_csv(attrs["values"] or "")
    local colors = parse_csv(attrs["colors"] or "")
    local labels = parse_csv(attrs["labels"] or "")

    -- Find matching index
    local idx = nil
    for i, v in ipairs(values) do
        if v == val then idx = i; break end
    end

    local color = idx and colors[idx] or "#6b7280"
    local label = idx and labels[idx] or val

    return string.format(
        '<div title="Badge: %s" class="py-1.5">'
        .. '<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold border" '
        .. 'style="background-color: %s20; color: %s; border-color: %s">%s</span>'
        .. '</div>',
        html_escape(path), html_escape(color), html_escape(color), html_escape(color), html_escape(label)
    )
end

-- ---------------------------------------------------------------------------
-- 2. format
-- ---------------------------------------------------------------------------

local function render_format(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("format", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    local fmt_type = attrs["type"]
    if not fmt_type then return shortcode_error("format", '"type" attribute is required') end

    local formatted = format_value(val, fmt_type, attrs)
    return string.format(
        '<div title="Formatted: %s (%s)" class="py-1.5"><span class="font-mono text-sm">%s</span></div>',
        html_escape(path), html_escape(fmt_type), html_escape(formatted)
    )
end

-- ---------------------------------------------------------------------------
-- 3. stat-card
-- ---------------------------------------------------------------------------

local function render_stat_card(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("stat-card", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    local label = attrs["label"] or path
    local fmt_type = attrs["type"] or "number"
    local icon_name = attrs["icon"] or "chart"

    local formatted = format_value(val, fmt_type, attrs)
    local icon_svg = get_icon(icon_name)

    return string.format(
        '<div title="Stat: %s (%s)" class="py-1.5">'
        .. '<div class="inline-flex flex-col items-center rounded-lg border border-stone-200 px-6 py-4 text-center">'
        .. '<div class="text-stone-400 mb-1">%s</div>'
        .. '<div class="text-2xl font-bold font-mono">%s</div>'
        .. '<div class="text-xs text-stone-500 mt-0.5">%s</div>'
        .. '</div></div>',
        html_escape(path), html_escape(label), icon_svg, html_escape(formatted), html_escape(label)
    )
end

-- ---------------------------------------------------------------------------
-- 4. meter
-- ---------------------------------------------------------------------------

local function render_meter(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("meter", '"path" attribute is required') end

    local val = tonumber(get_nested(ctx.value, path)) or 0
    local min_val = tonumber(attrs["min"]) or 0
    local max_val = tonumber(attrs["max"]) or 100
    local low = tonumber(attrs["low"]) or 30
    local high = tonumber(attrs["high"]) or 70
    local label = attrs["label"] or path

    local range = max_val - min_val
    if range <= 0 then range = 1 end

    local val_pct = clamp(((val - min_val) / range) * 100, 0, 100)
    local low_pct = clamp(((low - min_val) / range) * 100, 0, 100)
    local high_pct = clamp(((high - min_val) / range) * 100, 0, 100)

    return string.format(
        '<div title="Meter: %s (%s-%s)" class="py-1.5 text-sm">'
        .. '<div class="flex items-center justify-between mb-0.5">'
        .. '<span class="text-stone-600">%s</span>'
        .. '<span class="font-mono font-bold">%s</span>'
        .. '</div>'
        .. '<div class="relative h-3 rounded-full overflow-hidden bg-stone-200">'
        .. '<div class="absolute inset-0 rounded-full" style="background: linear-gradient(to right, '
        .. '#ef4444 0%%, #ef4444 %.1f%%, #f59e0b %.1f%%, #f59e0b %.1f%%, #22c55e %.1f%%, #22c55e 100%%)"></div>'
        .. '<div class="absolute top-0 h-full w-1 bg-stone-800 rounded" style="left: %.1f%%"></div>'
        .. '</div></div>',
        html_escape(path), html_escape(tostring(min_val)), html_escape(tostring(max_val)),
        html_escape(label), html_escape(tostring(val)),
        low_pct, low_pct, high_pct, high_pct,
        val_pct
    )
end

-- ---------------------------------------------------------------------------
-- 5. sparkline
-- ---------------------------------------------------------------------------

local function render_sparkline(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("sparkline", '"path" attribute is required') end

    local data = get_nested(ctx.value, path)
    if type(data) ~= "table" or #data == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    local chart_type = attrs["type"] or "line"
    local height = tonumber(attrs["height"]) or 24
    local width = tonumber(attrs["width"]) or 100
    local color = attrs["color"] or "#d97706"

    -- Convert to numbers
    local values = {}
    for _, v in ipairs(data) do
        local n = tonumber(v)
        if n then values[#values + 1] = n end
    end
    if #values == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    local min_v, max_v = values[1], values[1]
    for _, v in ipairs(values) do
        if v < min_v then min_v = v end
        if v > max_v then max_v = v end
    end
    local v_range = max_v - min_v
    if v_range == 0 then v_range = 1 end

    local padding = 1

    if chart_type == "bar" then
        local bar_w = (width - padding * 2) / #values
        local gap = math.max(0.5, bar_w * 0.1)
        bar_w = bar_w - gap
        local rects = {}
        for i, v in ipairs(values) do
            local h = ((v - min_v) / v_range) * (height - padding * 2)
            if h < 1 then h = 1 end
            local x = padding + (i - 1) * (bar_w + gap)
            local y = height - padding - h
            rects[#rects + 1] = string.format(
                '<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" rx="0.5"/>',
                x, y, bar_w, h, html_escape(color)
            )
        end
        return string.format(
            '<div title="Sparkline: %s (%s)" class="py-1.5"><svg width="%d" height="%d" class="inline-block align-middle">%s</svg></div>',
            html_escape(path), html_escape(chart_type), width, height, table.concat(rects)
        )
    end

    -- Line or area: build points
    local points = {}
    for i, v in ipairs(values) do
        local x = padding + ((i - 1) / math.max(1, #values - 1)) * (width - padding * 2)
        local y = padding + (1 - (v - min_v) / v_range) * (height - padding * 2)
        points[#points + 1] = string.format("%.1f,%.1f", x, y)
    end
    local points_str = table.concat(points, " ")

    if chart_type == "area" then
        -- Close the polygon at the bottom
        local first_x = padding
        local last_x = padding + ((#values - 1) / math.max(1, #values - 1)) * (width - padding * 2)
        local area_points = points_str
            .. string.format(" %.1f,%d %d,%d", last_x, height - padding, first_x, height - padding)
        return string.format(
            '<div title="Sparkline: %s (%s)" class="py-1.5"><svg width="%d" height="%d" class="inline-block align-middle">'
            .. '<polygon points="%s" fill="%s" fill-opacity="0.2" stroke="none"/>'
            .. '<polyline points="%s" fill="none" stroke="%s" stroke-width="1.5"/>'
            .. '</svg></div>',
            html_escape(path), html_escape(chart_type),
            width, height,
            area_points, html_escape(color),
            points_str, html_escape(color)
        )
    end

    -- Default: line
    return string.format(
        '<div title="Sparkline: %s (%s)" class="py-1.5"><svg width="%d" height="%d" class="inline-block align-middle">'
        .. '<polyline points="%s" fill="none" stroke="%s" stroke-width="1.5"/>'
        .. '</svg></div>',
        html_escape(path), html_escape(chart_type), width, height, points_str, html_escape(color)
    )
end

-- ---------------------------------------------------------------------------
-- 6. table
-- ---------------------------------------------------------------------------

local function render_table(ctx)
    local attrs = ctx.attrs or {}
    local entity_type = attrs["type"] or "notes"
    local cols = parse_csv(attrs["cols"] or "name,updated_at")
    local labels = parse_csv(attrs["labels"] or "")
    local limit = tonumber(attrs["limit"]) or 10

    -- Fill missing labels with column names
    for i = #labels + 1, #cols do
        labels[i] = cols[i]
    end

    -- Query entities
    local query_fn
    local type_path
    if entity_type == "resources" then
        query_fn = mah.db.query_resources
        type_path = "resource"
    elseif entity_type == "groups" then
        query_fn = mah.db.query_groups
        type_path = "group"
    else
        query_fn = mah.db.query_notes
        type_path = "note"
    end

    local items = query_fn({
        owner_id = ctx.entity_id,
        limit = limit,
        sort_by = {"updated_at desc"},
    })

    if not items or #items == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    local parts = {
        string.format('<div title="Table: owned %s" class="py-1.5">', html_escape(entity_type)),
        '<table class="w-full text-sm border-collapse">',
        '<thead><tr class="border-b border-stone-200">',
    }
    for _, label in ipairs(labels) do
        parts[#parts + 1] = string.format(
            '<th class="text-left py-1 px-2 font-semibold text-stone-600">%s</th>',
            html_escape(label)
        )
    end
    parts[#parts + 1] = '</tr></thead><tbody>'

    for _, item in ipairs(items) do
        local item_meta = nil
        parts[#parts + 1] = '<tr class="border-b border-stone-100 hover:bg-stone-50">'
        for _, col in ipairs(cols) do
            local cell_val
            if col:sub(1, 5) == "meta." then
                -- Decode meta JSON and navigate path
                if item_meta == nil then
                    if item.meta and item.meta ~= "" then
                        local ok, decoded = pcall(mah.json.decode, item.meta)
                        item_meta = ok and decoded or false
                    else
                        item_meta = false
                    end
                end
                if item_meta then
                    local meta_path = col:sub(6)
                    cell_val = get_nested(item_meta, meta_path)
                end
            else
                cell_val = item[col]
            end

            if type(cell_val) == "table" then
                cell_val = mah.json.encode(cell_val)
            end

            local display = html_escape(tostring(cell_val or ""))

            if col == "name" then
                parts[#parts + 1] = string.format(
                    '<td class="py-1 px-2"><a href="/%s?id=%d" class="text-blue-600 hover:underline">%s</a></td>',
                    type_path, item.id, display
                )
            else
                parts[#parts + 1] = string.format(
                    '<td class="py-1 px-2 text-stone-500">%s</td>',
                    display
                )
            end
        end
        parts[#parts + 1] = '</tr>'
    end

    parts[#parts + 1] = '</tbody></table></div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 7. list
-- ---------------------------------------------------------------------------

local function render_list(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("list", '"path" attribute is required') end

    local data = get_nested(ctx.value, path)
    if type(data) ~= "table" or #data == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No items</span></div>'
    end

    local style = attrs["style"] or "bullet"

    --- Extract display text from an item
    local function item_text(item)
        if type(item) ~= "table" then return tostring(item) end
        -- Try common fields
        for _, key in ipairs({"name", "text", "title", "label"}) do
            if item[key] then return tostring(item[key]) end
        end
        -- Fall back to first string value
        for _, v in pairs(item) do
            if type(v) == "string" then return v end
        end
        return mah.json.encode(item)
    end

    local title_text = string.format("List: %s (%s)", path, style)

    if style == "comma" then
        local texts = {}
        for _, item in ipairs(data) do
            texts[#texts + 1] = html_escape(item_text(item))
        end
        return string.format(
            '<div title="%s" class="py-1.5"><span class="text-sm">%s</span></div>',
            html_escape(title_text), table.concat(texts, ", ")
        )
    end

    if style == "pill" then
        local parts = { string.format('<div title="%s" class="py-1.5"><div class="flex flex-wrap gap-1">', html_escape(title_text)) }
        for _, item in ipairs(data) do
            parts[#parts + 1] = string.format(
                '<span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-stone-100 text-stone-700 border border-stone-200">%s</span>',
                html_escape(item_text(item))
            )
        end
        parts[#parts + 1] = '</div></div>'
        return table.concat(parts)
    end

    -- bullet or numbered
    local tag = style == "numbered" and "ol" or "ul"
    local list_class = style == "numbered" and "list-decimal" or "list-disc"
    local parts = { string.format('<div title="%s" class="py-1.5"><%s class="%s list-inside text-sm space-y-0.5">', html_escape(title_text), tag, list_class) }
    for _, item in ipairs(data) do
        parts[#parts + 1] = '<li>' .. html_escape(item_text(item)) .. '</li>'
    end
    parts[#parts + 1] = string.format('</%s></div>', tag)
    return table.concat(parts)
end

-- ---------------------------------------------------------------------------
-- 8. count-badge
-- ---------------------------------------------------------------------------

local function render_count_badge(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    local entity_type = attrs["type"]
    local label = attrs["label"] or ""
    local icon_name = attrs["icon"]
    local count = 0

    if entity_type then
        -- Entity count mode
        local count_fn
        if entity_type == "resources" then
            count_fn = mah.db.count_resources
        elseif entity_type == "groups" then
            count_fn = mah.db.count_groups
        elseif entity_type == "notes" then
            count_fn = mah.db.count_notes
        end
        if count_fn then
            count = count_fn({ owner_id = ctx.entity_id }) or 0
        end
    elseif path then
        -- Meta array mode
        local data = get_nested(ctx.value, path)
        if type(data) == "table" then
            local count_where = attrs["count-where"]
            local eq_val = attrs["eq"]
            local neq_val = attrs["neq"]

            if count_where then
                for _, item in ipairs(data) do
                    if type(item) == "table" then
                        local field_val = tostring(item[count_where] or "")
                        if eq_val and field_val == eq_val then
                            count = count + 1
                        elseif neq_val and field_val ~= neq_val then
                            count = count + 1
                        end
                    end
                end
            else
                count = #data
            end
        end
    else
        return shortcode_error("count-badge", '"path" or "type" attribute is required')
    end

    local icon_svg = icon_name and get_icon(icon_name) or ""
    local title_text = path and string.format("Count: %s", path) or string.format("Count: owned %s", entity_type)

    return string.format(
        '<div title="%s" class="py-1.5">'
        .. '<span class="inline-flex items-center gap-1 text-sm font-medium text-stone-600">'
        .. '%s<span class="font-mono font-bold">%d</span>'
        .. '<span>%s</span></span></div>',
        html_escape(title_text), icon_svg, count, html_escape(label)
    )
end

-- ---------------------------------------------------------------------------
-- 9. embed
-- ---------------------------------------------------------------------------

local function render_embed(ctx)
    local attrs = ctx.attrs or {}
    local resource_id = tonumber(attrs["resource-id"])
    local max_lines = tonumber(attrs["max-lines"])

    if not resource_id and attrs["path"] then
        resource_id = tonumber(get_nested(ctx.value, attrs["path"]))
    end

    if not resource_id then
        return shortcode_error("embed", '"resource-id" or "path" attribute is required')
    end

    local data = mah.db.get_resource_data(resource_id)
    if not data or not data.data then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">Resource not found</span></div>'
    end

    local content = base64_decode(data.data)

    -- Truncate by lines if needed
    local truncated = false
    if max_lines and max_lines > 0 then
        local lines = {}
        local line_count = 0
        for line in (content .. "\n"):gmatch("(.-)\n") do
            line_count = line_count + 1
            if line_count > max_lines then
                truncated = true
                break
            end
            lines[#lines + 1] = line
        end
        if truncated then
            content = table.concat(lines, "\n")
        end
    end

    local suffix = truncated and '\n<span class="text-stone-400 italic">... (truncated)</span>' or ""
    local title_text = attrs["path"]
        and string.format("Embed: %s", attrs["path"])
        or string.format("Embed: resource %d", resource_id)

    return string.format(
        '<div title="%s" class="py-1.5">'
        .. '<pre class="bg-stone-50 border border-stone-200 rounded p-3 text-xs font-mono overflow-x-auto max-h-80 overflow-y-auto whitespace-pre-wrap">%s%s</pre>'
        .. '</div>',
        html_escape(title_text), html_escape(content), suffix
    )
end

-- ---------------------------------------------------------------------------
-- 10. image
-- ---------------------------------------------------------------------------

local function render_image(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("image", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No image</span></div>'
    end

    local w = tonumber(attrs["width"]) or 100
    local h = tonumber(attrs["height"]) or 100
    local rounded = attrs["rounded"] == "true"
    local alt = html_escape(attrs["alt"] or path)
    local rounded_class = rounded and "rounded-full" or "rounded"

    local src
    if type(val) == "number" or (type(val) == "string" and val:match("^%d+$")) then
        -- Resource ID
        src = string.format("/v1/resource/preview?id=%s&width=%d&height=%d", tostring(val), w, h)
    elseif type(val) == "string" and val:match("^https?://") then
        -- External URL
        src = val
    else
        -- Treat as resource ID string or path
        src = tostring(val)
    end

    return string.format(
        '<div title="Image: %s" class="py-1.5">'
        .. '<img src="%s" width="%d" height="%d" alt="%s" class="object-cover %s" loading="lazy" />'
        .. '</div>',
        html_escape(path), html_escape(src), w, h, alt, rounded_class
    )
end

-- ---------------------------------------------------------------------------
-- 11a. barcode (Code 128)
-- ---------------------------------------------------------------------------

local function render_barcode(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("barcode", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No value</span></div>'
    end

    local text = tostring(val)
    local size = tonumber(attrs["size"]) or 50

    local barcode_svg = code128_svg(text, size)
    if barcode_svg == "" then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">Cannot encode value</span></div>'
    end

    return string.format(
        '<div title="Barcode: %s" class="py-1.5 max-w-full overflow-hidden">'
        .. '<div class="border border-stone-200 rounded p-2 bg-white max-w-full overflow-hidden">%s</div>'
        .. '<div class="text-xs text-stone-500 mt-1 font-mono truncate">%s</div>'
        .. '</div>',
        html_escape(path), barcode_svg, html_escape(text)
    )
end

-- ---------------------------------------------------------------------------
-- 11b. qr-code (actual QR code via client-side JS)
-- ---------------------------------------------------------------------------

local function render_qr_code(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("qr-code", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No value</span></div>'
    end

    local text = tostring(val)
    local size = tonumber(attrs["size"]) or 128
    local fg = attrs["color"] or "#000000"
    local bg = attrs["bg"] or "#ffffff"

    -- Render a placeholder that Alpine fills via the injected QR encoder
    return string.format(
        '<div title="QR Code: %s" class="py-1.5 max-w-full overflow-hidden">'
        .. '<div class="inline-block border border-stone-200 rounded p-2 bg-white max-w-full overflow-hidden"'
        .. ' x-data x-init="if(window.__dvQR){$el.innerHTML=window.__dvQR(%s,%d,%s,%s)}">'
        .. '<span class="text-xs text-stone-400">Loading QR...</span>'
        .. '</div>'
        .. '<div class="text-xs text-stone-500 mt-1 font-mono truncate">%s</div>'
        .. '</div>',
        html_escape(path),
        html_escape(json_value(text)), size, html_escape(json_value(fg)), html_escape(json_value(bg)),
        html_escape(text)
    )
end

-- ---------------------------------------------------------------------------
-- 12. link-preview
-- ---------------------------------------------------------------------------

local function render_link_preview(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("link-preview", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No link</span></div>'
    end

    local url, domain
    if type(val) == "table" then
        url = val.href or val.url or ""
        domain = val.host or val.domain or ""
    else
        url = tostring(val)
        domain = url:match("https?://([^/]+)") or url
    end

    if url == "" then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No link</span></div>'
    end

    return string.format(
        '<div title="Link: %s" class="py-1.5">'
        .. '<a href="%s" target="_blank" rel="noopener" '
        .. 'class="flex items-center gap-3 p-3 border border-stone-200 rounded-lg hover:bg-stone-50 transition-colors text-sm no-underline">'
        .. '<span class="shrink-0 text-stone-400">%s</span>'
        .. '<span class="min-w-0 flex-1">'
        .. '<span class="block font-medium text-stone-800 truncate">%s</span>'
        .. '<span class="block text-xs text-stone-500">%s</span>'
        .. '</span>'
        .. '<span class="shrink-0 ml-auto text-stone-400">%s</span>'
        .. '</a></div>',
        html_escape(path),
        html_escape(url),
        get_icon("globe", "w-8 h-8"),
        html_escape(url),
        html_escape(domain),
        get_icon("external", "w-4 h-4")
    )
end

-- ---------------------------------------------------------------------------
-- 13. json-tree
-- ---------------------------------------------------------------------------

local function render_json_tree_shortcode(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("json-tree", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)
    if val == nil then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    -- If val is a string, try to decode as JSON
    if type(val) == "string" then
        local ok, decoded = pcall(mah.json.decode, val)
        if ok then val = decoded end
    end

    local max_expand = tonumber(attrs["expanded"]) or 2

    local state = build_tree_state(val, 0, max_expand, "root")
    local tree_html = render_json_tree(val, 0, max_expand, "root")

    local title_text = path and string.format("JSON: %s", path) or "JSON: full meta"
    return string.format(
        '<div title="%s" class="py-1.5" x-data="{ %s }"><div class="font-mono text-xs">%s</div></div>',
        html_escape(title_text), state, tree_html
    )
end

-- ---------------------------------------------------------------------------
-- 14. bar-chart
-- ---------------------------------------------------------------------------

local function render_bar_chart(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("bar-chart", '"path" attribute is required') end

    local data = get_nested(ctx.value, path)
    if type(data) ~= "table" then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    local color = attrs["color"] or "#d97706"
    local label_key = attrs["label-key"]
    local value_key = attrs["value-key"]

    -- Extract label/value pairs
    local entries = {}

    -- Check if it's an array of objects or a plain object
    local is_array = #data > 0

    if is_array and label_key and value_key then
        for _, item in ipairs(data) do
            if type(item) == "table" then
                local lbl = tostring(item[label_key] or "")
                local val = tonumber(item[value_key]) or 0
                entries[#entries + 1] = { label = lbl, value = val }
            end
        end
    else
        -- Plain object: keys as labels, values as numbers
        local keys = {}
        for k, _ in pairs(data) do keys[#keys + 1] = k end
        table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
        for _, k in ipairs(keys) do
            local val = tonumber(data[k]) or 0
            entries[#entries + 1] = { label = tostring(k), value = val }
        end
    end

    if #entries == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    -- Find max value for scaling
    local max_val = 0
    for _, e in ipairs(entries) do
        if e.value > max_val then max_val = e.value end
    end
    if max_val == 0 then max_val = 1 end

    local parts = { string.format('<div title="Bar chart: %s" class="py-1.5 max-w-full overflow-hidden"><div class="space-y-1 text-sm">', html_escape(path)) }
    for _, e in ipairs(entries) do
        local pct = (e.value / max_val) * 100
        parts[#parts + 1] = string.format(
            '<div class="flex items-center gap-1">'
            .. '<span class="w-16 shrink-0 text-right text-stone-600 truncate text-xs">%s</span>'
            .. '<div class="flex-1 min-w-0 bg-stone-100 rounded-full h-4 overflow-hidden">'
            .. '<div class="h-full rounded-full" style="width: %.1f%%; background-color: %s"></div>'
            .. '</div>'
            .. '<span class="w-10 shrink-0 text-right font-mono text-xs text-stone-500">%s</span>'
            .. '</div>',
            html_escape(e.label), pct, html_escape(color),
            html_escape(format_number(e.value))
        )
    end
    parts[#parts + 1] = '</div></div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 15. pie-chart
-- ---------------------------------------------------------------------------

local function render_pie_chart(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("pie-chart", '"path" attribute is required') end

    local data = get_nested(ctx.value, path)
    if type(data) ~= "table" then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    local size = tonumber(attrs["size"]) or 120
    local is_donut = attrs["donut"] == "true"
    local label_key = attrs["label-key"]
    local value_key = attrs["value-key"]
    local colors = parse_csv(attrs["colors"] or "")
    if #colors == 0 then colors = DEFAULT_COLORS end

    -- Extract entries
    local entries = {}
    local is_array = #data > 0

    if is_array and label_key and value_key then
        for _, item in ipairs(data) do
            if type(item) == "table" then
                local lbl = tostring(item[label_key] or "")
                local val = tonumber(item[value_key]) or 0
                if val > 0 then
                    entries[#entries + 1] = { label = lbl, value = val }
                end
            end
        end
    else
        local keys = {}
        for k, _ in pairs(data) do keys[#keys + 1] = k end
        table.sort(keys, function(a, b) return tostring(a) < tostring(b) end)
        for _, k in ipairs(keys) do
            local val = tonumber(data[k]) or 0
            if val > 0 then
                entries[#entries + 1] = { label = tostring(k), value = val }
            end
        end
    end

    if #entries == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    -- Calculate total
    local total = 0
    for _, e in ipairs(entries) do total = total + e.value end
    if total == 0 then total = 1 end

    -- For solid pie: r=9, stroke-width=18 → outer edge = 9+9 = 18 (fits viewBox 36x36)
    -- For donut: r=14, stroke-width=6 → outer edge = 14+3 = 17 (fits viewBox 36x36)
    local radius = is_donut and 14 or 9
    local stroke_width = is_donut and 6 or (radius * 2)
    local circumference = 2 * math.pi * radius

    -- Build SVG circles
    local circles = {}
    local offset = 25 -- Start at 12 o'clock (stroke-dashoffset trick)

    for i, e in ipairs(entries) do
        local segment_pct = (e.value / total) * circumference
        local color_idx = ((i - 1) % #colors) + 1
        circles[#circles + 1] = string.format(
            '<circle cx="18" cy="18" r="%.3f" fill="none" stroke="%s" stroke-width="%s" '
            .. 'stroke-dasharray="%.3f %.3f" stroke-dashoffset="%.3f" transform="rotate(-90 18 18)"/>',
            radius, html_escape(colors[color_idx]), stroke_width,
            segment_pct, circumference - segment_pct, offset
        )
        offset = offset - segment_pct
    end

    -- Build legend
    local legend = {}
    for i, e in ipairs(entries) do
        local color_idx = ((i - 1) % #colors) + 1
        legend[#legend + 1] = string.format(
            '<div class="flex items-center gap-1">'
            .. '<span class="w-3 h-3 rounded-sm inline-block shrink-0" style="background:%s"></span>'
            .. '<span class="truncate">%s (%s)</span>'
            .. '</div>',
            html_escape(colors[color_idx]),
            html_escape(e.label),
            html_escape(format_number(e.value))
        )
    end

    return string.format(
        '<div title="Pie chart: %s" class="py-1.5 max-w-full overflow-hidden"><div class="flex flex-wrap items-start gap-3">'
        .. '<svg class="shrink-0" width="%d" height="%d" viewBox="0 0 36 36">%s</svg>'
        .. '<div class="text-xs space-y-1 min-w-0">%s</div>'
        .. '</div></div>',
        html_escape(path), size, size, table.concat(circles), table.concat(legend)
    )
end

-- ---------------------------------------------------------------------------
-- 16. conditional
-- ---------------------------------------------------------------------------

local function render_conditional(ctx)
    local attrs = ctx.attrs or {}
    local path = attrs["path"]
    if not path then return shortcode_error("conditional", '"path" attribute is required') end

    local val = get_nested(ctx.value, path)

    -- Evaluate condition
    local condition_met = false

    if attrs["eq"] then
        condition_met = tostring(val or "") == attrs["eq"]
    elseif attrs["neq"] then
        condition_met = tostring(val or "") ~= attrs["neq"]
    elseif attrs["gt"] then
        condition_met = (tonumber(val) or 0) > (tonumber(attrs["gt"]) or 0)
    elseif attrs["lt"] then
        condition_met = (tonumber(val) or 0) < (tonumber(attrs["lt"]) or 0)
    elseif attrs["contains"] then
        condition_met = tostring(val or ""):find(attrs["contains"], 1, true) ~= nil
    elseif attrs["empty"] then
        condition_met = val == nil or val == ""
    elseif attrs["not-empty"] then
        condition_met = val ~= nil and val ~= ""
    end

    if not condition_met then return "" end

    -- Build title
    local operator = ""
    local cond_value = ""
    if attrs["eq"] then operator = "eq"; cond_value = attrs["eq"]
    elseif attrs["neq"] then operator = "neq"; cond_value = attrs["neq"]
    elseif attrs["gt"] then operator = "gt"; cond_value = attrs["gt"]
    elseif attrs["lt"] then operator = "lt"; cond_value = attrs["lt"]
    elseif attrs["contains"] then operator = "contains"; cond_value = attrs["contains"]
    elseif attrs["empty"] then operator = "empty"; cond_value = ""
    elseif attrs["not-empty"] then operator = "not-empty"; cond_value = ""
    end
    local title_text = string.format("Conditional: %s %s %s", path, operator, cond_value)

    -- Render content
    local output
    if attrs["html"] then
        -- Intentionally unescaped for trusted admin content
        output = attrs["html"]
    elseif attrs["content"] then
        output = html_escape(attrs["content"])
    else
        return ""
    end

    local css_class = attrs["class"]
    if css_class then
        return string.format(
            '<div title="%s" class="py-1.5"><div class="%s">%s</div></div>',
            html_escape(title_text), html_escape(css_class), output
        )
    end

    return string.format('<div title="%s" class="py-1.5">%s</div>', html_escape(title_text), output)
end

-- ---------------------------------------------------------------------------
-- 17. timeline-chart
-- ---------------------------------------------------------------------------

local function render_timeline_chart(ctx)
    local attrs = ctx.attrs or {}
    local entity_type = attrs["type"] or "groups"
    local date_path = attrs["date-path"] or "timeline"
    local limit = tonumber(attrs["limit"]) or 10

    -- Query entities
    local query_fn
    local type_path
    if entity_type == "resources" then
        query_fn = mah.db.query_resources
        type_path = "resource"
    elseif entity_type == "notes" then
        query_fn = mah.db.query_notes
        type_path = "note"
    else
        query_fn = mah.db.query_groups
        type_path = "group"
    end

    local items = query_fn({
        owner_id = ctx.entity_id,
        limit = limit,
        sort_by = {"updated_at desc"},
    })

    if not items or #items == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No data</span></div>'
    end

    -- Parse ISO date to epoch-ish number for comparison (YYYYMMDD as integer)
    local function date_to_num(iso)
        if not iso or type(iso) ~= "string" then return nil end
        local y, m, d = iso:match("(%d%d%d%d)-(%d%d)-(%d%d)")
        if not y then return nil end
        return tonumber(y) * 10000 + tonumber(m) * 100 + tonumber(d)
    end

    -- Extract timeline data from each entity's meta
    local timeline_entries = {}
    local global_min = nil
    local global_max = nil

    for _, item in ipairs(items) do
        local item_meta = nil
        if item.meta and item.meta ~= "" then
            local ok, decoded = pcall(mah.json.decode, item.meta)
            if ok then item_meta = decoded end
        end

        if item_meta then
            local timeline_data = get_nested(item_meta, date_path)
            if type(timeline_data) == "table" then
                local start_date = timeline_data.start or timeline_data["start_date"] or timeline_data[1]
                local end_date = timeline_data["end"] or timeline_data["end_date"] or timeline_data[2]
                local start_num = date_to_num(tostring(start_date or ""))
                local end_num = date_to_num(tostring(end_date or ""))

                if start_num and end_num then
                    timeline_entries[#timeline_entries + 1] = {
                        id = item.id,
                        name = item.name or "(untitled)",
                        start_num = start_num,
                        end_num = end_num,
                        start_str = tostring(start_date),
                        end_str = tostring(end_date),
                    }
                    if not global_min or start_num < global_min then global_min = start_num end
                    if not global_max or end_num > global_max then global_max = end_num end
                end
            end
        end
    end

    if #timeline_entries == 0 then
        return '<div class="py-1.5"><span class="text-sm text-stone-400 italic">No timeline data</span></div>'
    end

    local range = global_max - global_min
    if range <= 0 then range = 1 end

    -- Format range labels
    local min_label = format_date(string.format("%04d-%02d-%02d",
        math.floor(global_min / 10000),
        math.floor((global_min % 10000) / 100),
        global_min % 100
    ))
    local max_label = format_date(string.format("%04d-%02d-%02d",
        math.floor(global_max / 10000),
        math.floor((global_max % 10000) / 100),
        global_max % 100
    ))

    local parts = {
        string.format('<div title="Timeline: owned %s" class="py-1.5 text-sm overflow-hidden max-w-full">', html_escape(entity_type)),
        '<div class="flex text-xs text-stone-400 mb-1">',
        '<span>' .. html_escape(min_label) .. '</span>',
        '<span class="ml-auto">' .. html_escape(max_label) .. '</span>',
        '</div>',
        '<div class="space-y-1">',
    }

    for _, entry in ipairs(timeline_entries) do
        local start_pct = ((entry.start_num - global_min) / range) * 100
        local width_pct = ((entry.end_num - entry.start_num) / range) * 100
        width_pct = math.max(width_pct, 1) -- Minimum 1% width for visibility

        parts[#parts + 1] = string.format(
            '<div class="flex items-center gap-1">'
            .. '<a href="/%s?id=%d" class="w-20 shrink-0 truncate text-blue-600 hover:underline text-xs">%s</a>'
            .. '<div class="flex-1 min-w-0 relative h-4 bg-stone-100 rounded">'
            .. '<div class="absolute h-full rounded bg-amber-500" style="left: %.1f%%; width: %.1f%%"></div>'
            .. '</div>'
            .. '</div>',
            type_path, entry.id, html_escape(entry.name),
            start_pct, width_pct
        )
    end

    parts[#parts + 1] = '</div></div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- Page-bottom injection: QR code generator
-- ---------------------------------------------------------------------------

local function render_qr_script(ctx)
    return [=[<script>
if(!window.__dvQR){
// Minimal QR Code generator - byte mode, EC level L, versions 1-10
// Returns SVG string
window.__dvQR=function(text,size,fg,bg){
fg=fg||'#000';bg=bg||'#fff';size=size||128;
// Galois field tables for GF(256) with polynomial 0x11d
var EXP=new Uint8Array(256),LOG=new Uint8Array(256),v=1;
for(var i=0;i<255;i++){EXP[i]=v;LOG[v]=i;v=(v<<1)^(v>=128?0x11d:0);}
EXP[255]=EXP[0];
function gfMul(a,b){return a&&b?EXP[(LOG[a]+LOG[b])%255]:0;}
// Reed-Solomon
function rsEncode(data,ecLen){
var gen=[1];
for(var i=0;i<ecLen;i++){var ng=new Array(gen.length+1);ng[0]=gen[0];
for(var j=1;j<gen.length;j++)ng[j]=gen[j]^gfMul(gen[j-1],EXP[i]);
ng[gen.length]=gfMul(gen[gen.length-1],EXP[i]);gen=ng;}
var rem=new Uint8Array(ecLen);
for(var i=0;i<data.length;i++){var f=rem[0]^data[i];
for(var j=0;j<ecLen-1;j++)rem[j]=rem[j+1]^gfMul(f,gen[j+1]);
rem[ecLen-1]=gfMul(f,gen[ecLen]);}
return rem;}
// Version info: [total codewords, ec codewords per block, num blocks]
var VERS=[[0],[26,7,1],[44,10,1],[70,15,1],[100,20,1],[134,26,1],[172,18,2],[196,20,2],[242,24,2],[292,30,2],[346,18,2]];
// Find version
var dataBytes=text.length+3;// mode+length+data+terminator overhead
var ver=1;for(;ver<=10;ver++){var vi=VERS[ver];var cap=vi[0]-vi[1]*vi[2];if(dataBytes<=cap)break;}
if(ver>10)return '<span style="color:red;font-size:10px">Text too long for QR</span>';
var modCnt=17+ver*4,ecPerBlk=VERS[ver][1],numBlk=VERS[ver][2],totalCW=VERS[ver][0];
var dataCW=totalCW-ecPerBlk*numBlk;
// Encode data (byte mode)
var bits=[];
function pushBits(val,len){for(var i=len-1;i>=0;i--)bits.push((val>>i)&1);}
pushBits(4,4);// byte mode indicator
pushBits(text.length,ver<=9?8:16);
for(var i=0;i<text.length;i++)pushBits(text.charCodeAt(i),8);
pushBits(0,Math.min(4,dataCW*8-bits.length));// terminator
while(bits.length%8)bits.push(0);// byte align
while(bits.length<dataCW*8){bits.push(1,1,1,0,1,1,0,0);if(bits.length<dataCW*8)bits.push(0,0,0,1,0,0,0,1);}
var dataArr=new Uint8Array(dataCW);
for(var i=0;i<dataCW;i++){var b=0;for(var j=0;j<8;j++)b=(b<<1)|bits[i*8+j];dataArr[i]=b;}
// Split into blocks and compute EC
var blkSize=Math.floor(dataCW/numBlk),extra=dataCW%numBlk;
var dataBlocks=[],ecBlocks=[];
var offset=0;
for(var b=0;b<numBlk;b++){var sz=blkSize+(b>=numBlk-extra?1:0);
dataBlocks.push(dataArr.slice(offset,offset+sz));offset+=sz;
ecBlocks.push(rsEncode(dataBlocks[b],ecPerBlk));}
// Interleave
var codewords=[];
for(var i=0;i<blkSize+1;i++)for(var b=0;b<numBlk;b++)if(i<dataBlocks[b].length)codewords.push(dataBlocks[b][i]);
for(var i=0;i<ecPerBlk;i++)for(var b=0;b<numBlk;b++)codewords.push(ecBlocks[b][i]);
// Create matrix
var N=modCnt,mat=[];for(var i=0;i<N;i++){mat[i]=new Uint8Array(N);}
var reserved=[];for(var i=0;i<N;i++){reserved[i]=new Uint8Array(N);}
// Place finder patterns
function finder(r,c){for(var dr=-1;dr<=7;dr++)for(var dc=-1;dc<=7;dc++){
var rr=r+dr,cc=c+dc;if(rr<0||rr>=N||cc<0||cc>=N)continue;
var v=(dr>=0&&dr<=6&&(dc==0||dc==6))||(dc>=0&&dc<=6&&(dr==0||dr==6))||(dr>=2&&dr<=4&&dc>=2&&dc<=4)?1:0;
mat[rr][cc]=v;reserved[rr][cc]=1;}}
finder(0,0);finder(0,N-7);finder(N-7,0);
// Timing patterns
for(var i=8;i<N-8;i++){if(!reserved[6][i]){mat[6][i]=i%2==0?1:0;reserved[6][i]=1;}
if(!reserved[i][6]){mat[i][6]=i%2==0?1:0;reserved[i][6]=1;}}
// Alignment pattern (version>=2)
if(ver>=2){var aPos=[0,0,0,0,0,0,0];// simplified: single alignment
var ap=ver==2?18:ver==3?22:ver==4?26:ver==5?30:ver==6?34:ver==7?22:ver==8?24:ver==9?26:28;
for(var dr=-2;dr<=2;dr++)for(var dc=-2;dc<=2;dc++){
if(!reserved[ap+dr][ap+dc]){
mat[ap+dr][ap+dc]=(Math.abs(dr)==2||Math.abs(dc)==2||(!dr&&!dc))?1:0;
reserved[ap+dr][ap+dc]=1;}}}
// Format info area reserved
for(var i=0;i<9;i++){if(i<N)reserved[8][i]=1;if(i<N)reserved[i][8]=1;
reserved[8][N-1-i]=1;reserved[N-1-i][8]=1;}
reserved[8][8]=1;mat[N-8][8]=1;reserved[N-8][8]=1;// dark module
// Place data bits
var bitIdx=0,allBits=[];
for(var i=0;i<codewords.length;i++)for(var j=7;j>=0;j--)allBits.push((codewords[i]>>j)&1);
var upward=true;for(var col=N-1;col>=1;col-=2){
if(col==6)col=5;// skip timing
for(var cnt=0;cnt<N;cnt++){var row=upward?N-1-cnt:cnt;
for(var dc=0;dc<=1;dc++){var c=col-dc;
if(!reserved[row][c]){mat[row][c]=bitIdx<allBits.length?allBits[bitIdx]:0;bitIdx++;}}}
upward=!upward;}
// Apply mask 0 (checkerboard)
for(var r=0;r<N;r++)for(var c=0;c<N;c++)if(!reserved[r][c])if((r+c)%2==0)mat[r][c]^=1;
// Format bits for mask 0, EC level L = 0b01, mask 0b000 → format bits = 0x77c4
var fmtBits=0x77c4;
for(var i=0;i<6;i++)mat[8][i]=(fmtBits>>(14-i))&1;
mat[8][7]=(fmtBits>>8)&1;mat[8][8]=(fmtBits>>7)&1;mat[7][8]=(fmtBits>>6)&1;
for(var i=0;i<6;i++)mat[5-i][8]=(fmtBits>>(i))&1;
for(var i=0;i<8;i++)mat[8][N-8+i]=(fmtBits>>(14-i))&1;
for(var i=0;i<7;i++)mat[N-1-i][8]=(fmtBits>>i)&1;
// Render SVG
var cellSize=size/N;
var rects='<rect width="'+size+'" height="'+size+'" fill="'+bg+'"/>';
for(var r=0;r<N;r++)for(var c=0;c<N;c++)if(mat[r][c])
rects+='<rect x="'+(c*cellSize).toFixed(2)+'" y="'+(r*cellSize).toFixed(2)+'" width="'+Math.ceil(cellSize)+'" height="'+Math.ceil(cellSize)+'" fill="'+fg+'"/>';
return '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 '+size+' '+size+'" style="max-width:100%;height:auto;width:'+size+'px"><'+rects+'</svg>';
};}
</script>]=]
end

-- ---------------------------------------------------------------------------
-- Plugin initialization
-- ---------------------------------------------------------------------------

function init()
    -- Inject QR code generator script
    mah.inject("page_bottom", render_qr_script)

    -- 1. badge
    mah.shortcode({
        name = "badge",
        label = "Status Badge",
        render = render_badge,
        description = "Display a colored pill badge based on a meta field value. Map specific values to colors and labels for visual status indicators.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field (e.g. 'status' or 'project.phase')" },
            { name = "values", type = "CSV", description = "Comma-separated values to match against" },
            { name = "colors", type = "CSV", description = "Comma-separated hex colors corresponding to each value" },
            { name = "labels", type = "CSV", description = "Comma-separated display labels corresponding to each value" },
        },
        examples = {
            { title = "Basic status badge", code = '[plugin:data-views:badge path="status"]', notes = "Shows the raw value as a gray badge.",
              example_data = { status = "active" } },
            { title = "Mapped with colors", code = '[plugin:data-views:badge path="status" values="active,archived,draft" colors="#22c55e,#6b7280,#f59e0b" labels="Active,Archived,Draft"]',
              example_data = { status = "active" } },
            { title = "Nested field", code = '[plugin:data-views:badge path="project.phase" values="planning,building,done" colors="#3b82f6,#d97706,#22c55e"]',
              example_data = { project = { phase = "building" } } },
        },
        notes = {
            "Unmatched values display as gray (#6b7280) badges with the raw value as label.",
            "Colors use hex format (#rrggbb).",
            "If the field is nil, an empty spacer is rendered.",
        },
    })

    -- 2. format
    mah.shortcode({
        name = "format",
        label = "Formatted Value",
        render = render_format,
        description = "Display a meta field value with a specific format such as currency, percent, date, filesize, number, or duration.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field" },
            { name = "type", type = "string", required = true, description = "Format type: 'currency', 'percent', 'date', 'filesize', 'number', or 'duration'" },
            { name = "currency", type = "string", default = "$", description = "Currency symbol (only for type=currency)" },
            { name = "decimals", type = "number", description = "Number of decimal places (default 2 for currency, 1 for percent)" },
            { name = "prefix", type = "string", default = "", description = "Text prepended to the formatted value" },
            { name = "suffix", type = "string", default = "", description = "Text appended to the formatted value" },
        },
        examples = {
            { title = "Currency", code = '[plugin:data-views:format path="budget.amount" type="currency"]', notes = "Displays as $1,234.56.",
              example_data = { budget = { amount = 1234.56 } } },
            { title = "Percent with suffix", code = '[plugin:data-views:format path="score" type="percent" suffix=" complete"]',
              example_data = { score = 73.5 } },
            { title = "File size", code = '[plugin:data-views:format path="metrics.disk_usage" type="filesize"]',
              example_data = { metrics = { disk_usage = 1073741824 } } },
        },
        notes = {
            "Supported types: currency, percent, date, filesize, number, duration.",
            "Nil values render as empty string.",
            "The result is displayed in monospace font.",
        },
    })

    -- 3. stat-card
    mah.shortcode({
        name = "stat-card",
        label = "Stat Card",
        render = render_stat_card,
        description = "Display a KPI card with a large formatted number, label, and icon. Useful for dashboards and summary views.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing the value" },
            { name = "label", type = "string", description = "Display label below the value (defaults to the path)" },
            { name = "type", type = "string", default = "number", description = "Format type for the value: 'currency', 'percent', 'date', 'filesize', 'number', or 'duration'" },
            { name = "icon", type = "string", default = "chart", description = "Icon name: 'chart', 'users', 'files', 'clock', 'check', 'file', 'note', 'folder', 'star'" },
            { name = "currency", type = "string", default = "$", description = "Currency symbol (only for type=currency)" },
            { name = "decimals", type = "number", description = "Number of decimal places for the formatted value" },
            { name = "prefix", type = "string", default = "", description = "Text prepended to the formatted value" },
            { name = "suffix", type = "string", default = "", description = "Text appended to the formatted value" },
        },
        examples = {
            { title = "Basic stat card", code = '[plugin:data-views:stat-card path="metrics.total_users" label="Total Users" icon="users"]',
              example_data = { metrics = { total_users = 1284 } } },
            { title = "Revenue card", code = '[plugin:data-views:stat-card path="budget.amount" label="Revenue" type="currency" icon="chart"]',
              example_data = { budget = { amount = 42500 } } },
            { title = "Completion percentage", code = '[plugin:data-views:stat-card path="progress" label="Complete" type="percent" icon="check"]',
              example_data = { progress = 73.5 } },
        },
        notes = {
            "The card renders inline with a border, centered icon, large value, and small label.",
            "Format attrs (currency, decimals, prefix, suffix) are passed through to the format_value function.",
        },
    })

    -- 4. meter
    mah.shortcode({
        name = "meter",
        label = "Meter Gauge",
        render = render_meter,
        description = "Display a horizontal gauge bar with red/yellow/green color zones and a position indicator. Ideal for scores, progress, or threshold values.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the numeric meta field" },
            { name = "min", type = "number", default = "0", description = "Minimum value of the range" },
            { name = "max", type = "number", default = "100", description = "Maximum value of the range" },
            { name = "low", type = "number", default = "30", description = "Threshold below which the zone is red" },
            { name = "high", type = "number", default = "70", description = "Threshold above which the zone is green" },
            { name = "label", type = "string", description = "Display label (defaults to the path)" },
        },
        examples = {
            { title = "Basic meter", code = '[plugin:data-views:meter path="score"]', notes = "Uses default 0-100 range with 30/70 thresholds.",
              example_data = { score = 72 } },
            { title = "Custom range", code = '[plugin:data-views:meter path="metrics.temperature" min="0" max="200" low="60" high="150" label="Temperature"]',
              example_data = { metrics = { temperature = 145 } } },
            { title = "Percentage meter", code = '[plugin:data-views:meter path="progress" label="Progress" high="80"]',
              example_data = { progress = 85 } },
        },
        notes = {
            "The gauge background shows a gradient: red (0 to low), yellow (low to high), green (high to max).",
            "A dark vertical indicator marks the current value position.",
            "Non-numeric values are treated as 0.",
        },
    })

    -- 5. sparkline
    mah.shortcode({
        name = "sparkline",
        label = "Sparkline Chart",
        render = render_sparkline,
        description = "Render a compact inline SVG chart from an array of numbers. Supports line, area, and bar chart types.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to a meta field containing an array of numbers" },
            { name = "type", type = "string", default = "line", description = "Chart type: 'line', 'area', or 'bar'" },
            { name = "height", type = "number", default = "24", description = "SVG height in pixels" },
            { name = "width", type = "number", default = "100", description = "SVG width in pixels" },
            { name = "color", type = "CSS color", default = "#d97706", description = "Stroke or fill color" },
        },
        examples = {
            { title = "Line sparkline", code = '[plugin:data-views:sparkline path="metrics.monthly"]',
              example_data = { metrics = { monthly = {10, 25, 18, 32, 28, 40, 35} } } },
            { title = "Bar sparkline", code = '[plugin:data-views:sparkline path="metrics.monthly" type="bar" color="#3b82f6"]',
              example_data = { metrics = { monthly = {10, 25, 18, 32, 28, 40, 35} } } },
            { title = "Large area chart", code = '[plugin:data-views:sparkline path="metrics.monthly" type="area" width="200" height="40" color="#22c55e"]',
              example_data = { metrics = { monthly = {10, 25, 18, 32, 28, 40, 35} } } },
        },
        notes = {
            "The data must be an array of numbers (e.g. [10, 20, 15, 30]).",
            "Non-numeric array entries are silently skipped.",
            "If all values are equal, the chart renders a flat line/bar.",
        },
    })

    -- 6. table
    mah.shortcode({
        name = "table",
        label = "Entity Table",
        render = render_table,
        description = "Display a table of entities (notes, resources, or groups) owned by the current group. Columns can reference entity fields or meta sub-fields.",
        attrs = {
            { name = "type", type = "string", default = "notes", description = "Entity type to query: 'notes', 'resources', or 'groups'" },
            { name = "cols", type = "CSV", default = "name,updated_at", description = "Comma-separated column names. Use 'meta.path' for meta fields" },
            { name = "labels", type = "CSV", description = "Comma-separated column headers (defaults to column names)" },
            { name = "limit", type = "number", default = "10", description = "Maximum number of rows to display" },
        },
        examples = {
            { title = "Notes table", code = '[plugin:data-views:table type="notes" cols="name,updated_at" limit="5"]' },
            { title = "Resources with meta columns", code = '[plugin:data-views:table type="resources" cols="name,meta.status,meta.score" labels="Name,Status,Score"]' },
            { title = "Groups table", code = '[plugin:data-views:table type="groups" cols="name,meta.budget.amount" labels="Name,Budget" limit="20"]' },
        },
        notes = {
            "Name columns render as clickable links to the entity.",
            "Results are sorted by updated_at descending.",
            "Meta columns use 'meta.' prefix followed by a dot-path into the decoded meta JSON.",
        },
    })

    -- 7. list
    mah.shortcode({
        name = "list",
        label = "List Display",
        render = render_list,
        description = "Render a meta array as a formatted list. Supports bullet, numbered, comma-separated, and pill styles.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to a meta field containing an array" },
            { name = "style", type = "string", default = "bullet", description = "List style: 'bullet', 'numbered', 'comma', or 'pill'" },
        },
        examples = {
            { title = "Bullet list", code = '[plugin:data-views:list path="tags"]',
              example_data = { tags = {"frontend", "backend", "design"} } },
            { title = "Pill badges", code = '[plugin:data-views:list path="tags" style="pill"]',
              example_data = { tags = {"frontend", "backend", "design"} } },
            { title = "Comma-separated", code = '[plugin:data-views:list path="contributors" style="comma"]',
              example_data = { contributors = {"Alice", "Bob", "Carol"} } },
        },
        notes = {
            "For arrays of objects, display text is extracted from 'name', 'text', 'title', or 'label' fields, falling back to the first string value.",
            "Empty arrays render as 'No items'.",
        },
    })

    -- 8. count-badge
    mah.shortcode({
        name = "count-badge",
        label = "Count Badge",
        render = render_count_badge,
        description = "Display a count of owned entities or items in a meta array, with optional filtering. Useful for showing relationship counts or conditional tallies.",
        attrs = {
            { name = "type", type = "string", description = "Entity type to count: 'notes', 'resources', or 'groups'. Counts entities owned by the current group" },
            { name = "path", type = "string", description = "Dot-path to a meta array to count items from (alternative to type)" },
            { name = "count-where", type = "string", description = "Field name within array objects to filter on (requires path)" },
            { name = "eq", type = "string", description = "Count only items where count-where field equals this value" },
            { name = "neq", type = "string", description = "Count only items where count-where field does not equal this value" },
            { name = "label", type = "string", default = "", description = "Text label displayed after the count" },
            { name = "icon", type = "string", description = "Icon name: 'chart', 'users', 'files', 'clock', 'check', 'file', 'note', 'folder', 'star'" },
        },
        examples = {
            { title = "Count owned notes", code = '[plugin:data-views:count-badge type="notes" label="notes" icon="note"]' },
            { title = "Count array items", code = '[plugin:data-views:count-badge path="tags" label="tags"]',
              example_data = { tags = {"lua", "go", "js", "html", "css"} } },
            { title = "Filtered count", code = '[plugin:data-views:count-badge path="tasks" count-where="status" eq="done" label="completed"]',
              example_data = { tasks = {{status = "done"}, {status = "pending"}, {status = "done"}} } },
        },
        notes = {
            "Either 'type' or 'path' is required, but not both.",
            "When using 'type', counts entities owned by the current group via the database.",
            "When using 'path' with 'count-where', items must be objects with the specified field.",
        },
    })

    -- 9. embed
    mah.shortcode({
        name = "embed",
        label = "Resource Embed",
        render = render_embed,
        description = "Embed the content of a resource (text file) in a scrollable code block. The resource can be specified directly by ID or looked up from a meta field.",
        attrs = {
            { name = "resource-id", type = "number", description = "ID of the resource to embed" },
            { name = "path", type = "string", description = "Dot-path to a meta field containing the resource ID (alternative to resource-id)" },
            { name = "max-lines", type = "number", description = "Truncate content after this many lines" },
        },
        examples = {
            { title = "Embed by ID", code = '[plugin:data-views:embed resource-id="42"]' },
            { title = "Embed from meta field", code = '[plugin:data-views:embed path="config_file_id" max-lines="50"]' },
        },
        notes = {
            "Either 'resource-id' or 'path' is required.",
            "Content is base64-decoded from the resource data.",
            "Truncated content shows a '... (truncated)' indicator.",
            "Best suited for text-based resources (code, config, logs).",
        },
    })

    -- 10. image
    mah.shortcode({
        name = "image",
        label = "Image Display",
        render = render_image,
        description = "Display an image from a meta field. The field can contain a resource ID (uses thumbnail preview), an external URL, or a direct image path.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing a resource ID, URL, or image path" },
            { name = "width", type = "number", default = "100", description = "Image display width in pixels" },
            { name = "height", type = "number", default = "100", description = "Image display height in pixels" },
            { name = "rounded", type = "boolean", default = "false", description = "Use circular clipping (true) or standard rounded corners (false)" },
            { name = "alt", type = "string", description = "Alt text for the image (defaults to the path)" },
        },
        examples = {
            { title = "Avatar from resource ID", code = '[plugin:data-views:image path="avatar_id" width="48" height="48" rounded="true"]' },
            { title = "Thumbnail", code = '[plugin:data-views:image path="cover_image" width="200" height="150"]' },
            { title = "External image", code = '[plugin:data-views:image path="logo_url" alt="Company Logo"]',
              example_data = { logo_url = "https://placehold.co/100x100?text=Logo" } },
        },
        notes = {
            "Numeric values and numeric strings are treated as resource IDs and use the preview endpoint.",
            "URLs starting with http:// or https:// are used directly.",
            "Images are lazy-loaded and use object-cover fit.",
        },
    })

    -- 11. barcode
    mah.shortcode({
        name = "barcode",
        label = "Barcode",
        render = render_barcode,
        description = "Generate a Code 128 barcode SVG from a meta field value. Displays the barcode with the encoded text below it.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing the text to encode" },
            { name = "size", type = "number", default = "50", description = "Height of the barcode in pixels" },
        },
        examples = {
            { title = "Product barcode", code = '[plugin:data-views:barcode path="sku"]',
              example_data = { sku = "ABC-12345-XY" } },
            { title = "Larger barcode", code = '[plugin:data-views:barcode path="serial_number" size="80"]',
              example_data = { serial_number = "SN-2024-00042" } },
        },
        notes = {
            "Uses Code 128 encoding, which supports ASCII characters 0-127.",
            "Values that cannot be encoded display a 'Cannot encode value' message.",
            "The barcode SVG scales to fit its container width.",
        },
    })

    -- 12. qr-code
    mah.shortcode({
        name = "qr-code",
        label = "QR Code",
        render = render_qr_code,
        description = "Generate a QR code SVG from a meta field value. Rendered client-side via an injected JavaScript QR encoder.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing the text to encode" },
            { name = "size", type = "number", default = "128", description = "Width and height of the QR code in pixels" },
            { name = "color", type = "CSS color", default = "#000000", description = "Foreground (module) color" },
            { name = "bg", type = "CSS color", default = "#ffffff", description = "Background color" },
        },
        examples = {
            { title = "Basic QR code", code = '[plugin:data-views:qr-code path="url"]',
              example_data = { url = "https://example.com" } },
            { title = "Styled QR code", code = '[plugin:data-views:qr-code path="url" size="200" color="#1e40af" bg="#f0f9ff"]',
              example_data = { url = "https://example.com" } },
            { title = "Small QR", code = '[plugin:data-views:qr-code path="serial_number" size="80"]',
              example_data = { serial_number = "SN-2024-00042" } },
        },
        notes = {
            "Supports byte-mode encoding for versions 1-10 (up to ~174 characters).",
            "Uses error correction level L (7% recovery).",
            "Requires JavaScript to render; shows 'Loading QR...' placeholder until ready.",
            "The encoded text is displayed below the QR code.",
        },
    })

    -- 13. link-preview
    mah.shortcode({
        name = "link-preview",
        label = "Link Preview",
        render = render_link_preview,
        description = "Display a styled link card with a globe icon, URL, domain, and external-link indicator. The meta field can be a URL string or an object with href/url and host/domain fields.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to a meta field containing a URL string or link object ({href, host})" },
        },
        examples = {
            { title = "Simple URL", code = '[plugin:data-views:link-preview path="website"]',
              example_data = { website = "https://github.com/example/project" } },
            { title = "Nested link object", code = '[plugin:data-views:link-preview path="project.homepage"]', notes = "Works with string URLs or objects like {href: '...', host: '...'}.",
              example_data = { project = { homepage = "https://docs.example.com" } } },
        },
        notes = {
            "String values are parsed to extract the domain automatically.",
            "Object values can use 'href' or 'url' for the link, and 'host' or 'domain' for the display domain.",
            "Links open in a new tab with rel='noopener'.",
        },
    })

    -- 14. json-tree
    mah.shortcode({
        name = "json-tree",
        label = "JSON Tree",
        render = render_json_tree_shortcode,
        description = "Render a meta field as an expandable/collapsible JSON tree. Useful for inspecting nested data structures interactively.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field to display as a tree" },
            { name = "expanded", type = "number", default = "2", description = "Number of nesting levels to expand by default" },
        },
        examples = {
            { title = "Expand 2 levels", code = '[plugin:data-views:json-tree path="config"]',
              example_data = { config = { theme = "dark", features = { search = true, export = false }, limits = { max_upload = 50 } } } },
            { title = "Fully collapsed", code = '[plugin:data-views:json-tree path="metrics" expanded="0"]',
              example_data = { metrics = { cpu = 45.2, memory = 2048, requests = { total = 15000, errors = 23 } } } },
            { title = "Deep expansion", code = '[plugin:data-views:json-tree path="nested.data" expanded="5"]',
              example_data = { nested = { data = { level1 = { level2 = { value = "deep" } } } } } },
        },
        notes = {
            "String values that contain valid JSON are automatically decoded.",
            "Uses Alpine.js for interactive expand/collapse toggle.",
            "Arrays show their length; objects show their key count when collapsed.",
        },
    })

    -- 15. bar-chart
    mah.shortcode({
        name = "bar-chart",
        label = "Bar Chart",
        render = render_bar_chart,
        description = "Display a horizontal bar chart from a meta object or array of objects. Keys become labels and values become bar widths, scaled to the maximum.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing data (object or array of objects)" },
            { name = "color", type = "CSS color", default = "#d97706", description = "Bar fill color" },
            { name = "label-key", type = "string", description = "Field name for labels when data is an array of objects" },
            { name = "value-key", type = "string", description = "Field name for values when data is an array of objects" },
        },
        examples = {
            { title = "From object", code = '[plugin:data-views:bar-chart path="scores"]', notes = "Object keys become labels, values become bars (e.g. {math: 90, science: 75}).",
              example_data = { scores = { math = 90, science = 75, english = 88, history = 65 } } },
            { title = "From array of objects", code = '[plugin:data-views:bar-chart path="departments" label-key="name" value-key="budget" color="#3b82f6"]',
              example_data = { departments = {{name = "Engineering", budget = 450000}, {name = "Marketing", budget = 280000}, {name = "Sales", budget = 320000}} } },
            { title = "Custom color", code = '[plugin:data-views:bar-chart path="metrics.breakdown" color="#22c55e"]',
              example_data = { metrics = { breakdown = { frontend = 42, backend = 58, infrastructure = 23 } } } },
        },
        notes = {
            "For plain objects, keys are sorted alphabetically.",
            "For arrays, both label-key and value-key must be specified.",
            "Bars are scaled relative to the maximum value (largest bar = 100% width).",
            "Non-numeric values are treated as 0.",
        },
    })

    -- 16. pie-chart
    mah.shortcode({
        name = "pie-chart",
        label = "Pie Chart",
        render = render_pie_chart,
        description = "Display an SVG pie or donut chart with a color legend from a meta object or array of objects. Segments are proportional to their values.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field containing data (object or array of objects)" },
            { name = "size", type = "number", default = "120", description = "Width and height of the chart in pixels" },
            { name = "donut", type = "boolean", default = "false", description = "Render as a donut chart with a hollow center" },
            { name = "label-key", type = "string", description = "Field name for labels when data is an array of objects" },
            { name = "value-key", type = "string", description = "Field name for values when data is an array of objects" },
            { name = "colors", type = "CSV", description = "Comma-separated hex colors for segments (cycles through a default palette if not specified)" },
        },
        examples = {
            { title = "Pie from object", code = '[plugin:data-views:pie-chart path="budget.breakdown"]', notes = "Object keys become legend labels (e.g. {rent: 1200, food: 400, transport: 200}).",
              example_data = { budget = { breakdown = { rent = 1200, food = 400, transport = 200, utilities = 150 } } } },
            { title = "Donut chart", code = '[plugin:data-views:pie-chart path="budget.breakdown" donut="true" size="150"]',
              example_data = { budget = { breakdown = { rent = 1200, food = 400, transport = 200, utilities = 150 } } } },
            { title = "Custom colors", code = '[plugin:data-views:pie-chart path="categories" label-key="name" value-key="count" colors="#ef4444,#3b82f6,#22c55e"]',
              example_data = { categories = {{name = "Books", count = 45}, {name = "Videos", count = 32}, {name = "Articles", count = 28}} } },
        },
        notes = {
            "Zero and negative values are excluded from the chart.",
            "Default color palette: amber, blue, red, green, violet, pink, cyan, gray.",
            "Colors cycle if there are more segments than colors provided.",
            "Legend shows each segment label with its numeric value.",
        },
    })

    -- 17. conditional
    mah.shortcode({
        name = "conditional",
        label = "Conditional Content",
        render = render_conditional,
        description = "Conditionally render content based on a meta field value. Supports equality, comparison, contains, and empty/not-empty checks.",
        attrs = {
            { name = "path", type = "string", required = true, description = "Dot-path to the meta field to evaluate" },
            { name = "eq", type = "string", description = "Show content when field equals this value" },
            { name = "neq", type = "string", description = "Show content when field does not equal this value" },
            { name = "gt", type = "number", description = "Show content when field is greater than this value" },
            { name = "lt", type = "number", description = "Show content when field is less than this value" },
            { name = "contains", type = "string", description = "Show content when field contains this substring" },
            { name = "empty", type = "boolean", description = "Show content when field is nil or empty string" },
            { name = "not-empty", type = "boolean", description = "Show content when field is not nil and not empty string" },
            { name = "content", type = "string", description = "Text content to display (HTML-escaped)" },
            { name = "html", type = "string", description = "Raw HTML content to display (not escaped, for trusted admin content)" },
            { name = "class", type = "string", description = "CSS class to apply to the wrapper div" },
        },
        examples = {
            { title = "Show when active", code = '[plugin:data-views:conditional path="status" eq="active" content="This item is active"]',
              example_data = { status = "active" } },
            { title = "Warning for high values", code = '[plugin:data-views:conditional path="score" gt="90" html="<span class=\'text-red-600 font-bold\'>High score alert</span>" class="bg-red-50 p-2 rounded"]',
              example_data = { score = 95 } },
            { title = "Show when field exists", code = '[plugin:data-views:conditional path="notes" not-empty="true" content="Has notes attached"]',
              example_data = { notes = "Has some notes" } },
        },
        notes = {
            "Only one condition operator can be used per shortcode.",
            "If neither 'content' nor 'html' is provided, nothing is rendered even when the condition is met.",
            "The 'html' attr is intentionally unescaped for trusted admin content.",
            "Numeric comparisons (gt, lt) treat non-numeric values as 0.",
        },
    })

    -- 18. timeline-chart
    mah.shortcode({
        name = "timeline-chart",
        label = "Timeline Chart",
        render = render_timeline_chart,
        description = "Display a horizontal timeline (Gantt-style) of owned entities. Each entity's meta must contain a date range object with start and end dates.",
        attrs = {
            { name = "type", type = "string", default = "groups", description = "Entity type to query: 'groups', 'notes', or 'resources'" },
            { name = "date-path", type = "string", default = "timeline", description = "Dot-path within each entity's meta to the date range object" },
            { name = "limit", type = "number", default = "10", description = "Maximum number of entities to display" },
        },
        examples = {
            { title = "Project timeline", code = '[plugin:data-views:timeline-chart type="groups" date-path="timeline"]', notes = "Each owned group needs meta like {timeline: {start: \"2025-01-01\", end: \"2025-06-30\"}}." },
            { title = "Note milestones", code = '[plugin:data-views:timeline-chart type="notes" date-path="schedule" limit="20"]' },
            { title = "Resource availability", code = '[plugin:data-views:timeline-chart type="resources" date-path="dates"]' },
        },
        notes = {
            "Date range objects support keys: start/start_date or [1] for start, end/end_date or [2] for end.",
            "Dates must be in ISO format (YYYY-MM-DD).",
            "Entities without valid date ranges in their meta are silently skipped.",
            "Entity names link to their detail pages.",
            "Bars have a minimum 1% width for visibility.",
        },
    })
end
