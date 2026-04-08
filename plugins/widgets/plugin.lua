-- Widgets plugin for mahresources
-- Provides 5 shortcodes for group/resource category custom template slots:
--   summary, gallery, progress, activity, tree
-- Usage in templates: [plugin:widgets:shortcode-name attr="value"]

plugin = {
    name = "widgets",
    version = "1.0",
    description = "Adds 5 shortcodes for use in category CustomHeader, CustomSidebar, and CustomSummary slots.\n"
        .. "\n"
        .. "[plugin:widgets:summary] — Entity counts (resources, notes, sub-groups). Attrs: show, style.\n"
        .. "[plugin:widgets:gallery] — Thumbnail grid of images (owned, then related). Attrs: count, cols, content-type.\n"
        .. "[plugin:widgets:progress] — Progress bar from meta field values. Attrs: field, complete, type, label.\n"
        .. "[plugin:widgets:activity] — Timeline of recently updated owned entities. Attrs: count, type.\n"
        .. "[plugin:widgets:tree] — Group hierarchy (ancestors and children). Attrs: direction, depth.",
}

-- ---------------------------------------------------------------------------
-- Helpers
-- ---------------------------------------------------------------------------

--- Escape HTML special characters to prevent XSS.
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

--- Navigate a dot-separated path inside a table (e.g. "status.phase").
local function get_nested(tbl, path)
    local current = tbl
    for segment in path:gmatch("[^%.]+") do
        if type(current) ~= "table" then return nil end
        current = current[segment]
    end
    return current
end

--- Parse a comma-separated string into a set (table with trimmed keys = true).
local function parse_csv_set(str)
    local set = {}
    if not str or str == "" then return set end
    for item in str:gmatch("[^,]+") do
        set[item:match("^%s*(.-)%s*$")] = true
    end
    return set
end

--- Clamp a number between min and max.
local function clamp(n, lo, hi)
    if n < lo then return lo end
    if n > hi then return hi end
    return n
end

-- Simple inline SVG icons (small, 16x16 viewBox).
local ICON_FILE = '<svg xmlns="http://www.w3.org/2000/svg" class="inline-block w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 1h7l3 3v11H3V1z"/><path d="M10 1v3h3"/></svg>'
local ICON_NOTE = '<svg xmlns="http://www.w3.org/2000/svg" class="inline-block w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 1h10v14H3V1z"/><path d="M5 5h6M5 8h6M5 11h3"/></svg>'
local ICON_FOLDER = '<svg xmlns="http://www.w3.org/2000/svg" class="inline-block w-4 h-4" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M1 3h5l2 2h7v9H1V3z"/></svg>'

-- ---------------------------------------------------------------------------
-- 1. summary -- Entity Stats Dashboard
-- ---------------------------------------------------------------------------

local function render_summary(ctx)
    local attrs = ctx.attrs or {}
    local show_str = attrs["show"] or "resources,notes,groups"
    local style = attrs["style"] or "compact"
    local show = parse_csv_set(show_str)
    local eid = ctx.entity_id

    -- Gather counts for requested sections.
    local stats = {}
    if show["resources"] then
        local c = mah.db.count_resources({ owner_id = eid })
        stats[#stats + 1] = { icon = ICON_FILE, count = c or 0, label = "Resources" }
    end
    if show["notes"] then
        local c = mah.db.count_notes({ owner_id = eid })
        stats[#stats + 1] = { icon = ICON_NOTE, count = c or 0, label = "Notes" }
    end
    if show["groups"] then
        local c = mah.db.count_groups({ owner_id = eid })
        stats[#stats + 1] = { icon = ICON_FOLDER, count = c or 0, label = "Groups" }
    end

    if #stats == 0 then
        return '<p class="text-sm text-gray-400 italic">No stats to display</p>'
    end

    -- Compact style: single flex row.
    if style == "compact" then
        local parts = { '<div class="flex items-center gap-4 text-sm text-gray-600">' }
        for _, s in ipairs(stats) do
            parts[#parts + 1] = string.format(
                '<span class="flex items-center gap-1">%s <strong>%d</strong> %s</span>',
                s.icon, s.count, html_escape(s.label)
            )
        end
        parts[#parts + 1] = '</div>'
        return table.concat(parts, "\n")
    end

    -- Cards style: grid of rounded cards.
    local cols = #stats
    local parts = { string.format('<div class="grid grid-cols-%d gap-3">', cols) }
    for _, s in ipairs(stats) do
        parts[#parts + 1] = string.format(
            '<div class="rounded-lg border border-gray-200 p-4 text-center">'
            .. '<div class="flex justify-center mb-1 text-gray-500">%s</div>'
            .. '<div class="text-2xl font-bold">%d</div>'
            .. '<div class="text-xs text-gray-500">%s</div>'
            .. '</div>',
            s.icon, s.count, html_escape(s.label)
        )
    end
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 2. gallery -- Resource Thumbnail Grid
-- ---------------------------------------------------------------------------

local function render_gallery(ctx)
    local attrs = ctx.attrs or {}
    local count = tonumber(attrs["count"]) or 8
    local cols = tonumber(attrs["cols"]) or 4
    local ct = attrs["content-type"] or "image/"

    count = clamp(count, 1, 100)
    cols = clamp(cols, 1, 12)

    -- Try owned resources first, fall back to related resources.
    local resources = mah.db.query_resources({
        owner_id = ctx.entity_id,
        content_type = ct,
        limit = count,
        sort_by = { "updated_at desc" },
    })

    if (not resources or #resources == 0) and ctx.entity_type == "group" then
        resources = mah.db.query_resources({
            groups = { ctx.entity_id },
            content_type = ct,
            limit = count,
            sort_by = { "updated_at desc" },
        })
    end

    if not resources or #resources == 0 then
        return '<p class="text-sm text-gray-400 italic">No images found</p>'
    end

    local parts = { string.format('<div class="grid grid-cols-%d gap-2">', cols) }
    for _, r in ipairs(resources) do
        local name = html_escape(r.name or "")
        parts[#parts + 1] = string.format(
            '<a href="/resource?id=%d">'
            .. '<img src="/v1/resource/thumbnail?id=%d&width=200&height=200" '
            .. 'loading="lazy" alt="%s" '
            .. 'class="rounded object-cover w-full aspect-square" />'
            .. '</a>',
            r.id, r.id, name
        )
    end
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 3. progress -- Completion Progress Bar
-- ---------------------------------------------------------------------------

local function render_progress(ctx)
    local attrs = ctx.attrs or {}
    local field = attrs["field"]
    local complete_val = attrs["complete"] or "done"
    local entity_type = attrs["type"] or "notes"
    local custom_label = attrs["label"]

    if not field or field == "" then
        return '<p class="text-sm text-red-500">progress: "field" attribute is required</p>'
    end

    -- Query owned entities.
    local items
    if entity_type == "resources" then
        items = mah.db.query_resources({ owner_id = ctx.entity_id, limit = 100 })
    elseif entity_type == "groups" then
        items = mah.db.query_groups({ owner_id = ctx.entity_id, limit = 100 })
    else
        items = mah.db.query_notes({ owner_id = ctx.entity_id, limit = 100 })
    end

    items = items or {}
    local total = #items
    local done = 0

    for _, item in ipairs(items) do
        if item.meta and item.meta ~= "" then
            local ok, meta = pcall(mah.json.decode, item.meta)
            if ok and type(meta) == "table" then
                local val = get_nested(meta, field)
                if tostring(val) == complete_val then
                    done = done + 1
                end
            end
        end
    end

    local percent = 0
    if total > 0 then
        percent = math.floor((done / total) * 100)
    end

    local label_text = custom_label or string.format("%d/%d complete", done, total)

    return string.format(
        '<div class="bg-gray-200 rounded-full h-4 overflow-hidden">'
        .. '<div class="bg-blue-500 h-full rounded-full transition-all" style="width: %d%%"></div>'
        .. '</div>'
        .. '<p class="text-sm text-gray-600 mt-1">%s</p>',
        percent, html_escape(label_text)
    )
end

-- ---------------------------------------------------------------------------
-- 4. activity -- Recent Activity Timeline
-- ---------------------------------------------------------------------------

local function render_activity(ctx)
    local attrs = ctx.attrs or {}
    local count = tonumber(attrs["count"]) or 5
    count = clamp(count, 1, 20)
    local atype = attrs["type"] or "all"

    -- Collect items as {icon, type_path, id, name, updated_at}.
    local items = {}

    local function collect(type_path, icon, query_fn)
        local results = query_fn({
            owner_id = ctx.entity_id,
            limit = count,
            sort_by = { "updated_at desc" },
        })
        if results then
            for _, r in ipairs(results) do
                items[#items + 1] = {
                    icon = icon,
                    type_path = type_path,
                    id = r.id,
                    name = r.name or "(untitled)",
                    updated_at = r.updated_at or "",
                }
            end
        end
    end

    if atype == "all" or atype == "resources" then
        collect("resource", ICON_FILE, mah.db.query_resources)
    end
    if atype == "all" or atype == "notes" then
        collect("note", ICON_NOTE, mah.db.query_notes)
    end
    if atype == "all" or atype == "groups" then
        collect("group", ICON_FOLDER, mah.db.query_groups)
    end

    if #items == 0 then
        return '<p class="text-sm text-gray-400 italic">No recent activity</p>'
    end

    -- Sort by updated_at descending (ISO8601 strings are lexicographically sortable).
    table.sort(items, function(a, b) return a.updated_at > b.updated_at end)

    -- Trim to requested count.
    local trimmed = {}
    for i = 1, math.min(count, #items) do
        trimmed[#trimmed + 1] = items[i]
    end

    local parts = { '<div class="space-y-2">' }
    for _, item in ipairs(trimmed) do
        -- Extract YYYY-MM-DD HH:MM from the ISO8601 string.
        local date_str = ""
        if item.updated_at and #item.updated_at >= 16 then
            date_str = item.updated_at:sub(1, 16):gsub("T", " ")
        end

        parts[#parts + 1] = string.format(
            '<div class="flex items-center gap-2 text-sm">'
            .. '<span class="text-gray-400">%s</span>'
            .. '<a href="/%s?id=%d" class="text-blue-600 hover:underline truncate">%s</a>'
            .. '<span class="text-gray-400 text-xs whitespace-nowrap ml-auto">%s</span>'
            .. '</div>',
            item.icon,
            item.type_path,
            item.id,
            html_escape(item.name),
            html_escape(date_str)
        )
    end
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 5. tree -- Group Hierarchy Tree
-- ---------------------------------------------------------------------------

local function render_tree(ctx)
    if ctx.entity_type ~= "group" then
        return '<p class="text-sm text-gray-400 italic">Tree view is only available for groups</p>'
    end

    local attrs = ctx.attrs or {}
    local direction = attrs["direction"] or "both"
    local max_depth = tonumber(attrs["depth"]) or 3
    max_depth = clamp(max_depth, 1, 10)

    local current_id = ctx.entity_id

    -- Track visited IDs to prevent infinite loops.
    local visited = {}

    --- Render a single group link or bold name for the current entity.
    local function render_node(group)
        if group.id == current_id then
            return string.format('<span class="font-bold">%s</span>', html_escape(group.name or "(untitled)"))
        end
        return string.format(
            '<a href="/group?id=%d" class="text-blue-600 hover:underline">%s</a>',
            group.id, html_escape(group.name or "(untitled)")
        )
    end

    --- Recursively render children as nested <ul> lists.
    local function render_children(parent_id, depth)
        if depth > max_depth then return "" end
        if visited[parent_id] then return "" end
        visited[parent_id] = true

        local children = mah.db.query_groups({ owner_id = parent_id, limit = 20 })
        if not children or #children == 0 then return "" end

        local parts = { '<ul class="ml-4 mt-1 space-y-1 border-l border-gray-200 pl-2">' }
        for _, child in ipairs(children) do
            if not visited[child.id] or child.id == current_id then
                parts[#parts + 1] = '<li>' .. render_node(child)
                if not visited[child.id] then
                    parts[#parts + 1] = render_children(child.id, depth + 1)
                end
                parts[#parts + 1] = '</li>'
            end
        end
        parts[#parts + 1] = '</ul>'
        return table.concat(parts, "\n")
    end

    --- Walk up the owner chain to collect ancestors (root first).
    local function collect_ancestors()
        local chain = {}
        local id = current_id
        local seen = {}
        for _ = 1, max_depth + 1 do
            if seen[id] then break end
            seen[id] = true
            local g = mah.db.get_group(id)
            if not g then break end
            chain[#chain + 1] = g
            if not g.owner_id or g.owner_id == 0 then break end
            id = g.owner_id
        end
        -- Reverse so root is first.
        local reversed = {}
        for i = #chain, 1, -1 do
            reversed[#reversed + 1] = chain[i]
        end
        return reversed
    end

    -- Build the tree HTML.
    local html_parts = { '<ul class="text-sm space-y-1">' }

    if direction == "up" or direction == "both" then
        local ancestors = collect_ancestors()
        -- Render ancestors as a nested structure (root first).
        -- Each ancestor wraps the next level.
        local indent_parts = {}
        local close_parts = {}
        for i, g in ipairs(ancestors) do
            -- Mark non-current ancestors as visited to prevent re-traversal,
            -- but do NOT mark the current entity yet — render_children needs to enter it.
            if g.id ~= current_id then
                visited[g.id] = true
            end
            indent_parts[#indent_parts + 1] = '<li>' .. render_node(g)
            if i < #ancestors then
                indent_parts[#indent_parts + 1] = '<ul class="ml-4 mt-1 space-y-1 border-l border-gray-200 pl-2">'
                close_parts[#close_parts + 1] = '</ul></li>'
            else
                -- Last item is the current entity; render children if direction == "both".
                if direction == "both" then
                    indent_parts[#indent_parts + 1] = render_children(current_id, 2)
                end
                close_parts[#close_parts + 1] = '</li>'
            end
        end
        for _, p in ipairs(indent_parts) do
            html_parts[#html_parts + 1] = p
        end
        -- Close in reverse order.
        for i = #close_parts, 1, -1 do
            html_parts[#html_parts + 1] = close_parts[i]
        end

    elseif direction == "down" then
        -- Just render current node + children.
        local g = mah.db.get_group(current_id)
        if g then
            visited[current_id] = true
            html_parts[#html_parts + 1] = '<li>' .. render_node(g)
            html_parts[#html_parts + 1] = render_children(current_id, 1)
            html_parts[#html_parts + 1] = '</li>'
        end
    end

    html_parts[#html_parts + 1] = '</ul>'
    return table.concat(html_parts, "\n")
end

-- ---------------------------------------------------------------------------
-- Plugin initialization
-- ---------------------------------------------------------------------------

function init()
    mah.shortcode({ name = "summary",  label = "Entity Summary",     render = render_summary })
    mah.shortcode({ name = "gallery",  label = "Resource Gallery",   render = render_gallery })
    mah.shortcode({ name = "progress", label = "Progress Bar",       render = render_progress })
    mah.shortcode({ name = "activity", label = "Recent Activity",    render = render_activity })
    mah.shortcode({ name = "tree",     label = "Group Hierarchy",    render = render_tree })
end
