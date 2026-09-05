-- Project Management plugin for mahresources.
--
-- Multiple projects (Groups in the "PM Project" category), epics under them
-- (Groups in the "PM Epic" category, owned by the project), and tasks (Notes
-- of the "PM Task" type, owned by their epic or by the project directly).
-- Status, priority and a lexicographic order key live in the task's `meta`;
-- due/start dates are the note's native, indexed StartDate/EndDate fields.
--
-- The plugin adds no tables: tasks stay first-class notes, searchable,
-- mass-edit-able, group-exportable and subtree-scoped by the host. The four
-- views (kanban board, backlog, dashboard, timeline) are rendered in the
-- browser (public/pm.js) on top of the host's own /v1/notes endpoint, which is
-- the one read surface that carries a note's Tags and its dates.
--
-- PM writes share validated handlers. Native note writes receive a pure status
-- default through entity hooks; order is allocated only by an explicit PM move.

plugin = {
    name = "project-management",
    version = "1.1.0",
    description = "Project management: kanban boards, epics and tasks built on groups and notes",
    api_version = 1,
    capabilities = { "db:read", "db:write", "render", "pages", "api", "kv", "actions", "hooks", "schedule" },
    settings = {
        { name = "statuses", type = "string", label = "Status names (JSON array)", default = '["backlog","todo","in_progress","blocked","done"]' },
        { name = "status_labels", type = "string", label = "Status labels (JSON object)", default = '{"backlog":"Backlog","todo":"To Do","in_progress":"In Progress","blocked":"Blocked","done":"Done"}' },
        { name = "priorities", type = "string", label = "Priority names (JSON array)", default = '["low","medium","high","urgent"]' },
        { name = "priority_labels", type = "string", label = "Priority labels (JSON object)", default = '{"low":"Low","medium":"Medium","high":"High","urgent":"Urgent"}' },
        { name = "default_status", type = "string", label = "Status new tasks start in", default = "todo" },
        { name = "done_status", type = "string", label = "The status treated as done (progress, overdue)", default = "done" },
    },
}

-- ---------------------------------------------------------------------------
-- Constants
-- ---------------------------------------------------------------------------

-- Colors are code-level: they feed both the taxonomy MetaSchemas (x-color on
-- labeled enums) and the client-rendered pills, so they must agree. The status
-- and priority *sets* are what the settings override.
local STATUS_COLORS = {
    backlog = "#6b7280",
    todo = "#2563eb",
    in_progress = "#d97706",
    blocked = "#dc2626",
    done = "#16a34a",
}
local PRIORITY_COLORS = {
    low = "#6b7280",
    medium = "#6366f1",
    high = "#d97706",
    urgent = "#dc2626",
}
local PRIORITY_FALLBACKS = { "#6b7280", "#6366f1", "#d97706", "#dc2626", "#0d9488", "#9333ea" }

local TAXONOMY = {
    project_category = "PM Project",
    epic_category = "PM Epic",
    task_type = "PM Task",
}

local KV_PREFIX = "cfg"

-- ---------------------------------------------------------------------------
-- Small Lua helpers
-- ---------------------------------------------------------------------------

local function is_empty(s)
    return s == nil or s == ""
end

local function trim(s)
    if s == nil then return "" end
    return (s:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function json_encode(v)
    local ok, out = pcall(mah.json.encode, v)
    if not ok then return nil, tostring(out) end
    return out
end

local function json_decode(s)
    if s == nil or s == "" then return nil, "empty json" end
    local ok, out = pcall(mah.json.decode, s)
    if not ok then return nil, tostring(out) end
    return out
end

-- Decode a JSON string setting; on any failure return nil (caller falls back).
local function decode_setting(value)
    if value == nil then return nil end
    local ok, decoded = pcall(mah.json.decode, tostring(value))
    if not ok or type(decoded) ~= "table" then return nil end
    return decoded
end

-- Merge dst over src: dst wins. Returns a new table.
-- Darken a hex colour for text rendered on a 15% tint of itself (the pills):
-- the raw colour does not clear WCAG AA as small text on that tint.
local function darken_hex(hex)
    if hex == nil then return "#1c1917" end
    local r, g, b = hex:match("^#(%x%x)(%x%x)(%x%x)$")
    if not r then return "#1c1917" end
    local function comp(c)
        return string.format("%02x", math.floor((tonumber(c, 16) or 0) * 0.45))
    end
    return "#" .. comp(r) .. comp(g) .. comp(b)
end

local function merge(src, dst)
    local out = {}
    for k, v in pairs(src or {}) do out[k] = v end
    for k, v in pairs(dst or {}) do out[k] = v end
    return out
end

-- ---------------------------------------------------------------------------
-- Resolved configuration
-- ---------------------------------------------------------------------------

local DEFAULT_STATUSES = { "backlog", "todo", "in_progress", "blocked", "done" }
local DEFAULT_PRIORITIES = { "low", "medium", "high", "urgent" }
local DEFAULT_STATUS_LABELS = {
    backlog = "Backlog", todo = "To Do", in_progress = "In Progress",
    blocked = "Blocked", done = "Done",
}
local DEFAULT_PRIORITY_LABELS = { low = "Low", medium = "Medium", high = "High", urgent = "Urgent" }

-- status/priority names from settings travel into MRQL conditions, MetaQuery
-- filters and CSS attribute selectors, so they are restricted to a safe
-- charset here; anything else is dropped with one log line per resolve pass.
local NAME_OK = "^[A-Za-z0-9_-]+$"
local name_warned = {}

local function clean_name_list(raw_names)
    local out = {}
    for _, name in ipairs(raw_names or {}) do
        if type(name) ~= "string" or name == "" or not name:match(NAME_OK) then
            if not name_warned[name or "?"] then
                name_warned[name or "?"] = true
                mah.log("warning",
                    "project-management: ignoring invalid status/priority name " .. tostring(name)
                    .. " (allowed: letters, digits, - and _)")
            end
        else
            out[#out + 1] = name
        end
    end
    return out
end

local function string_list(setting_value, fallback)
    local decoded = decode_setting(setting_value)
    if not decoded then return fallback end
    local cleaned = clean_name_list(decoded)
    if #cleaned == 0 then return fallback end
    return cleaned
end

local function label_map(setting_value, fallback)
    local decoded = decode_setting(setting_value)
    if not decoded then return fallback end
    local out = {}
    for k, v in pairs(decoded) do
        if type(k) == "string" and type(v) == "string" then out[k] = v end
    end
    return out
end

-- Resolve the status/priority lists from live plugin settings, layering the
-- optional label maps over the code defaults. Read once per request: settings
-- are operator-tuned and rarely changed, and each mah.get_setting is a lookup.
local function resolved_config()
    local statuses = string_list(mah.get_setting("statuses"), DEFAULT_STATUSES)
    local status_labels = merge(DEFAULT_STATUS_LABELS, label_map(mah.get_setting("status_labels"), nil))
    local priorities = string_list(mah.get_setting("priorities"), DEFAULT_PRIORITIES)
    local priority_labels = merge(DEFAULT_PRIORITY_LABELS, label_map(mah.get_setting("priority_labels"), nil))

    local resolved_statuses = {}
    for i, name in ipairs(statuses) do
        resolved_statuses[i] = {
            name = name,
            label = status_labels[name] or name,
            color = STATUS_COLORS[name] or PRIORITY_FALLBACKS[((i - 1) % #PRIORITY_FALLBACKS) + 1],
            text_color = darken_hex(STATUS_COLORS[name] or PRIORITY_FALLBACKS[((i - 1) % #PRIORITY_FALLBACKS) + 1]),
        }
    end
    local resolved_priorities = {}
    for i, name in ipairs(priorities) do
        resolved_priorities[i] = {
            name = name,
            label = priority_labels[name] or name,
            color = PRIORITY_COLORS[name] or PRIORITY_FALLBACKS[((i - 1) % #PRIORITY_FALLBACKS) + 1],
            text_color = darken_hex(PRIORITY_COLORS[name] or PRIORITY_FALLBACKS[((i - 1) % #PRIORITY_FALLBACKS) + 1]),
        }
    end
    local default_status = mah.get_setting("default_status")
    if type(default_status) ~= "string" or default_status == "" then default_status = "todo" end

    -- Which status counts as "done" for progress/overdue. Operator-overridable
    -- through a setting, so removing the word "done" from the status list does
    -- not silently break the numbers.
    local done_status = mah.get_setting("done_status")
    if type(done_status) ~= "string" or done_status == "" then done_status = "done" end

    local known = {}
    for _, st in ipairs(resolved_statuses) do known[st.name] = true end
    if not known[default_status] and #resolved_statuses > 0 then
        default_status = resolved_statuses[1].name
    end
    if not known[done_status] and #resolved_statuses > 0 then
        done_status = resolved_statuses[1].name
    end

    return {
        statuses = resolved_statuses,
        priorities = resolved_priorities,
        default_status = default_status,
        done_status = done_status,
        status_name_set = known,
    }
end

-- KV cache of the taxonomy ids created by setup.
local function cached_taxonomy()
    return mah.kv.get(KV_PREFIX .. "_taxonomy")
end

local function store_taxonomy(tax)
    mah.kv.set(KV_PREFIX .. "_taxonomy", tax)
end

-- ---------------------------------------------------------------------------
-- Taxonomy lookups
-- ---------------------------------------------------------------------------

-- find_by_exact_name returns the first listed entity whose name equals `name`.
-- list_* matches substrings, so exactness must be checked here.
local function find_by_exact_name(list_fn, name)
    local matches, err = list_fn({ name = name, limit = 100 })
    if err then return nil, err end
    for _, item in ipairs(matches or {}) do
        if item.name == name then return item, nil end
    end
    return nil, nil
end

local function find_category(name)
    return find_by_exact_name(mah.db.list_categories, name)
end

local function find_note_type(name)
    return find_by_exact_name(mah.db.list_note_types, name)
end

local function list_groups(category_id, owner_id)
    local out = {}
    local cursor = 0
    while true do
        local opts = { category_id = category_id, limit = 100, mrql = "id > " .. cursor, sort_by = {"id asc"} }
        if owner_id then opts.owner_id = owner_id end
        local page, err = mah.db.query_groups(opts)
        if err then return nil, err end
        for _, group in ipairs(page or {}) do out[#out + 1] = group; cursor = math.max(cursor, group.id) end
        if not page or #page < 100 then break end
    end
    return out, nil
end

local function list_projects()
    local tax = cached_taxonomy()
    if not tax then return {}, nil end
    return list_groups(tax.project_category_id)
end

local function list_epics(project_id)
    local tax = cached_taxonomy()
    if not tax then return {}, nil end
    return list_groups(tax.epic_category_id, project_id)
end

-- ---------------------------------------------------------------------------
-- Schema builders (taxonomy provisioning)
-- ---------------------------------------------------------------------------

local function labeled_enum_entries(list)
    local entries = {}
    for _, item in ipairs(list) do
        entries[#entries + 1] = { const = item.name, title = item.label, ["x-color"] = item.color }
    end
    return entries
end

local function task_schema_json(cfg)
    local schema = json_encode({
        type = "object",
        properties = {
            status = {
                type = "string",
                description = "Task status",
                default = cfg.default_status,
                oneOf = labeled_enum_entries(cfg.statuses),
            },
            priority = {
                type = "string",
                description = "Priority",
                oneOf = labeled_enum_entries(cfg.priorities),
            },
        },
    })
    if not schema then return "" end
    return schema
end

local function project_schema_json(cfg)
    local schema = json_encode({
        type = "object",
        properties = {
            status = {
                type = "string",
                description = "Project status",
                oneOf = labeled_enum_entries(cfg.statuses),
            },
            key = { type = "string", description = "Short project key, e.g. DEV" },
            target_date = { type = "string", description = "Target completion date (YYYY-MM-DD)" },
        },
    })
    if not schema then return "" end
    return schema
end

local function epic_schema_json(cfg)
    local schema = json_encode({
        type = "object",
        properties = {
            status = {
                type = "string",
                description = "Epic status",
                default = cfg.default_status,
                oneOf = labeled_enum_entries(cfg.statuses),
            },
            target_date = { type = "string", description = "Target completion date (YYYY-MM-DD)" },
        },
    })
    if not schema then return "" end
    return schema
end

-- The taxonomy owns the presentation contract for PM entities. These templates
-- are deliberately thin: the reusable behaviour sits behind the plugin
-- shortcodes, while the Custom* slots decide where it appears in the host UI.
-- Setup upgrades the three legacy templates below, but never replaces an
-- operator-authored value.
local LEGACY_PROJECT_HEADER = "<div class=\"pm-embed\">\n"
    .. "[plugin:project-management:view-links]\n"
    .. "[plugin:project-management:progress]\n"
    .. "</div>"
local LEGACY_EPIC_HEADER = "<div class=\"pm-embed\">\n"
    .. "[plugin:project-management:progress]\n"
    .. "[plugin:project-management:task-list limit=\"8\"]\n"
    .. "</div>"
local LEGACY_TASK_SUMMARY = "[plugin:project-management:task-badges]"

-- Bundled v1 presentation values are migration sentinels. Setup may replace
-- these exact strings, but it still leaves every operator-authored variant
-- alone. Keeping the sentinel beside the new value makes that ownership rule
-- explicit and avoids scattering magic historical templates through setup().
local V1_PROJECT_HEADER = '<section class="pm-entity-detail pm-project-detail" data-testid="pm-project-detail">\n'
    .. '<span class="pm-entity-kicker">Project workspace</span>\n'
    .. "[plugin:project-management:group-summary]\n"
    .. "[plugin:project-management:view-links]\n"
    .. "[plugin:project-management:progress]\n"
    .. "</section>"
local V1_EPIC_HEADER = '<section class="pm-entity-detail pm-epic-detail" data-testid="pm-epic-detail">\n'
    .. '<span class="pm-entity-kicker">Epic</span>\n'
    .. "[plugin:project-management:group-summary]\n"
    .. "[plugin:project-management:entity-context]\n"
    .. "[plugin:project-management:progress]\n"
    .. "[plugin:project-management:task-list limit=\"8\"]\n"
    .. "</section>"
local V1_TASK_HEADER = '<section class="pm-entity-detail pm-task-detail" data-testid="pm-task-detail">\n'
    .. '<span class="pm-entity-kicker">Task</span>\n'
    .. "[plugin:project-management:task-badges]\n"
    .. "[plugin:project-management:entity-context]\n"
    .. "</section>"
local V1_EPIC_SUMMARY = '<div class="pm-entity-summary pm-epic-summary" data-testid="pm-epic-summary">'
    .. "[plugin:project-management:group-summary]</div>"
local V1_TASK_SUMMARY = '<div class="pm-entity-summary pm-task-summary" data-testid="pm-task-summary">'
    .. "[plugin:project-management:task-badges]"
    .. '[conditional field="EndDate" not-empty="true"]<span class="pm-entity-date">Due '
    .. '[property path="EndDate" format="date"]</span>[/conditional]</div>'
local V1_TASK_AVATAR = '<span class="pm-entity-avatar pm-task-avatar" aria-hidden="true">&#10003;</span>'
local V1_PROJECT_DETAIL_FOOTER = '<nav class="pm-detail-footer" aria-label="Project management">'
    .. '<a href="/plugins/project-management/board?project=[property path=\'ID\' inline=\'true\']&amp;view=board">'
    .. "Continue in the board &rarr;</a></nav>"
local V1_EPIC_DETAIL_FOOTER = '<div class="pm-detail-footer" aria-label="Epic project context">'
    .. "[plugin:project-management:entity-context]</div>"
local V1_TASK_DETAIL_FOOTER = '<div class="pm-detail-footer" aria-label="Task project context">'
    .. "[plugin:project-management:entity-context]</div>"

local PROJECT_HEADER = '<section class="pm-entity-detail pm-project-detail" data-testid="pm-project-detail" aria-label="Project overview">\n'
    .. '<span class="pm-entity-kicker">Project workspace</span>\n'
    .. "[plugin:project-management:group-summary]\n"
    .. "[plugin:project-management:view-links]\n"
    .. "[plugin:project-management:progress]\n"
    .. "</section>"
local PROJECT_SUMMARY = '<div class="pm-entity-summary pm-project-summary" data-testid="pm-project-summary">'
    .. "[plugin:project-management:group-summary]</div>"
local PROJECT_AVATAR = '<span class="pm-entity-avatar pm-project-avatar" aria-hidden="true">P</span>'
local PROJECT_LIST_HEADER = '<section class="pm-list-intro" data-testid="pm-project-list-intro" aria-label="PM Project list introduction">'
    .. '<strong>Project workspaces</strong><span>Plan in Groups; deliver from the Project Management views.</span>'
    .. '<a href="/plugins/project-management/board">Open project management</a></section>'
local PROJECT_DETAIL_FOOTER = ""
local PROJECT_MRQL_RESULT = '<article class="pm-entity-card pm-project-result" data-testid="pm-project-mrql-result">'
    .. '<span class="pm-entity-kicker">Project</span>'
    .. '<a class="pm-entity-title" href="/group?id=[property path=\'ID\' inline=\'true\']">'
    .. '[property path="Name"]</a>[plugin:project-management:group-summary]'
    .. '<a class="pm-entity-action" href="/plugins/project-management/board?project=[property path=\'ID\' inline=\'true\']&amp;view=board">Board</a>'
    .. "</article>"

local EPIC_HEADER = '<section class="pm-entity-detail pm-epic-detail" data-testid="pm-epic-detail" aria-label="Epic overview">\n'
    .. '<span class="pm-entity-kicker">Epic</span>\n'
    .. "[plugin:project-management:group-summary]\n"
    .. "[plugin:project-management:progress]\n"
    .. "</section>"
local EPIC_SUMMARY = '<div class="pm-entity-summary pm-epic-summary" data-testid="pm-epic-summary">'
    .. "[plugin:project-management:group-summary][plugin:project-management:entity-context]</div>"
local EPIC_HOVER_CARD = '<div class="pm-entity-summary pm-epic-summary" data-testid="pm-epic-hover-card">'
    .. "[plugin:project-management:group-summary][plugin:project-management:entity-context]</div>"
local EPIC_AVATAR = '<span class="pm-entity-avatar pm-epic-avatar" aria-hidden="true">E</span>'
local EPIC_LIST_HEADER = '<section class="pm-list-intro" data-testid="pm-epic-list-intro" aria-label="PM Epic list introduction">'
    .. '<strong>Epics</strong><span>Outcome-sized bodies of work across PM projects.</span>'
    .. '<a href="/plugins/project-management/board">Open project management</a></section>'
local EPIC_DETAIL_FOOTER = '<nav class="pm-detail-footer" aria-label="Epic project context">'
    .. "[plugin:project-management:entity-context]</nav>"
local EPIC_MRQL_RESULT = '<article class="pm-entity-card pm-epic-result" data-testid="pm-epic-mrql-result">'
    .. '<span class="pm-entity-kicker">Epic</span>'
    .. '<a class="pm-entity-title" href="/group?id=[property path=\'ID\' inline=\'true\']">'
    .. '[property path="Name"]</a>[plugin:project-management:group-summary]'
    .. "[plugin:project-management:entity-context]</article>"

local TASK_HEADER = '<section class="pm-entity-detail pm-task-detail" data-testid="pm-task-detail" aria-label="Task overview">\n'
    .. '<span class="pm-entity-kicker">Task</span>\n'
    .. "[plugin:project-management:task-badges]\n"
    .. "</section>"
local TASK_SUMMARY = '<div class="pm-entity-summary pm-task-summary" data-testid="pm-task-summary">'
    .. "[plugin:project-management:task-badges]"
    .. "[plugin:project-management:task-date]"
    .. "[plugin:project-management:entity-context]</div>"
local TASK_HOVER_CARD = '<div class="pm-entity-summary pm-task-summary" data-testid="pm-task-hover-card">'
    .. "[plugin:project-management:task-badges][plugin:project-management:task-date]"
    .. "[plugin:project-management:entity-context]</div>"
local TASK_AVATAR = "[plugin:project-management:task-avatar]"
local TASK_LIST_HEADER = '<section class="pm-list-intro" data-testid="pm-task-list-intro" aria-label="PM Task list introduction">'
    .. '<strong>Task register</strong><span>Every PM task remains a first-class Mahresources note.</span>'
    .. '<a href="/plugins/project-management/board">Open project management</a></section>'
local TASK_DETAIL_FOOTER = '<div class="pm-detail-footer" aria-label="Task project context">'
    .. "[plugin:project-management:entity-context]</div>"
local TASK_MRQL_RESULT = '<article class="pm-entity-card pm-task-result" data-testid="pm-task-mrql-result">'
    .. '<span class="pm-entity-kicker">Task</span>'
    .. '<a class="pm-entity-title" href="/note?id=[property path=\'ID\' inline=\'true\']">'
    .. '[property path="Name"]</a><div class="pm-entity-summary">[plugin:project-management:task-badges]'
    .. "[plugin:project-management:task-date]</div>"
    .. "[plugin:project-management:entity-context]</article>"

-- CustomCSS is the host-page half of the embedded UI. pm.css is intentionally
-- loaded only on the plugin page, while category/type CustomCSS is injected on
-- native detail and list pages where the shortcodes above render.
local EMBED_CSS = [[
.pm-embed-nav{display:inline-flex;flex-wrap:wrap;gap:.5rem;margin-bottom:.5rem}
.pm-view-link{display:inline-flex;align-items:center;min-height:2rem;border:1px solid #d6d3d1;border-radius:.375rem;padding:.25rem .75rem;background:#fff;color:#292524;font-size:.8125rem;text-decoration:none}
.pm-view-link:hover,.pm-view-link:focus-visible{border-color:#b45309;outline:2px solid #b45309;outline-offset:1px}
.pm-progress-wrap{display:flex;align-items:center;gap:.5rem;max-width:24rem}
.pm-progress{display:block;flex:1;min-width:8rem;height:.625rem;background:#e7e5e4;border-radius:9999px;overflow:hidden}
.pm-progress-fill{display:block;height:100%;background:#16a34a}
.pm-progress-text{font-size:.75rem;color:#78716c;white-space:nowrap}
.pm-task-list{list-style:none;margin:.25rem 0 0;padding:0}
.pm-task-item{padding:.125rem 0}.pm-task-item a{color:inherit}
.pm-task-item.pm-task-done a{color:#a8a29e;text-decoration:line-through}
.pm-badges{display:inline-flex;flex-wrap:wrap;gap:.25rem}
.pm-pill{display:inline-block;background:color-mix(in srgb,var(--pm-color,#6366f1) 15%,white);color:var(--pm-color,#4338ca);border-radius:9999px;padding:.0625rem .5rem;font-size:.75rem;font-weight:600;line-height:1.25rem;white-space:nowrap}
/* project-management:presentation:v2 */
.pm-entity-detail{display:grid;gap:.625rem;margin:0 0 1rem;padding:1rem;border:1px solid #e7e5e4;border-left:.3rem solid #b45309;border-radius:.625rem;background:linear-gradient(135deg,#fffbeb,#fff)}
.pm-entity-kicker{color:#92400e;font-size:.75rem;font-weight:700;letter-spacing:.08em;text-transform:uppercase}
.pm-entity-summary{display:flex;flex-wrap:wrap;align-items:center;gap:.35rem .625rem;color:#57534e;font-size:.8125rem}
.pm-entity-fact{display:inline-flex;align-items:center;gap:.25rem}.pm-entity-fact strong{color:#292524}
.pm-entity-avatar{display:inline-flex;width:2rem;height:2rem;align-items:center;justify-content:center;border-radius:.5rem;background:#ffedd5;color:#9a3412;font-size:.75rem;font-weight:800}
.pm-epic-avatar{background:#ede9fe;color:#5b21b6}.pm-task-avatar{border-radius:9999px;background:color-mix(in srgb,var(--pm-avatar-color,#6b7280) 16%,white);color:var(--pm-avatar-text,#374151)}
.pm-entity-date.pm-overdue{color:#b91c1c;font-weight:700}
.pm-context-links,.pm-detail-footer{display:flex;flex-wrap:wrap;align-items:center;gap:.5rem;font-size:.8125rem}
.pm-context-links a,.pm-detail-footer a,.pm-entity-action,.pm-list-intro a{color:#9a3412;font-weight:600;text-decoration:none}
.pm-context-links a:hover,.pm-detail-footer a:hover,.pm-entity-action:hover,.pm-list-intro a:hover{text-decoration:underline}
.pm-entity-card{display:grid;gap:.35rem;min-width:13rem;padding:.75rem;border:1px solid #e7e5e4;border-radius:.625rem;background:#fff}
.pm-entity-title{color:#292524;font-weight:700;text-decoration:none}.pm-entity-title:hover{text-decoration:underline}
.pm-entity-date{color:#57534e}.pm-list-intro{display:flex;flex-wrap:wrap;align-items:center;gap:.5rem 1rem;margin:0 0 1rem;padding:.75rem 1rem;border:1px solid #fed7aa;border-radius:.625rem;background:#fff7ed;color:#57534e}.pm-list-intro strong{color:#9a3412}.pm-list-intro a{margin-left:auto}
.pm-content-block{display:grid;gap:.625rem;padding:.875rem}.pm-content-block h3{margin:0;color:#292524;font-size:.9375rem}.pm-content-block ul{list-style:disc;margin:0;padding-left:1.25rem}.pm-content-block dl{display:grid;grid-template-columns:max-content 1fr;gap:.35rem .75rem;margin:0}.pm-content-block dt{color:#78716c;font-size:.75rem;font-weight:700;text-transform:uppercase}.pm-content-block dd{margin:0;color:#292524}.pm-block-editor{display:grid;gap:.625rem}.pm-block-editor label{display:grid;gap:.25rem;color:#44403c;font-size:.8125rem;font-weight:600}.pm-block-editor input,.pm-block-editor textarea{width:100%;border:1px solid #d6d3d1;border-radius:.375rem;padding:.5rem;font:inherit}.pm-block-editor textarea{min-height:5rem;resize:vertical}
@media(max-width:40rem){.pm-list-intro{align-items:flex-start}.pm-list-intro a{margin-left:0;width:100%}.pm-context-links{align-items:flex-start}.pm-content-block dl{grid-template-columns:1fr}.pm-content-block dd{margin-bottom:.35rem}}
]]

-- Integration defaults; the exact previous values above remain upgrade sentinels.
local EMBED_SCRIPT = '<script defer src="/plugins/project-management/public/pm-core.js"></script><script defer src="/plugins/project-management/public/pm-embed.js"></script>'
-- Block runtimes load through the host even when an operator replaces a header.
local BLOCK_SCRIPTS = {"pm-core.js", "pm-embed.js"}
local V2_PROJECT_HEADER = PROJECT_HEADER
local V2_PROJECT_SUMMARY = PROJECT_SUMMARY
local V2_PROJECT_LIST_HEADER = PROJECT_LIST_HEADER
local V2_PROJECT_MRQL_RESULT = PROJECT_MRQL_RESULT
local V2_EPIC_HEADER = EPIC_HEADER
local V2_EPIC_SUMMARY = EPIC_SUMMARY
local V2_EPIC_HOVER_CARD = EPIC_HOVER_CARD
local V2_EPIC_LIST_HEADER = EPIC_LIST_HEADER
local V2_EPIC_MRQL_RESULT = EPIC_MRQL_RESULT
local V2_TASK_HEADER = TASK_HEADER
local V2_TASK_LIST_HEADER = TASK_LIST_HEADER
local V2_EMBED_CSS = EMBED_CSS

local ROLLUP_SUMMARY = '<span class="pm-rollup">[meta path="pm_done"] done · [meta path="pm_open"] open · [meta path="pm_overdue"] overdue</span>'
PROJECT_HEADER = EMBED_SCRIPT .. PROJECT_HEADER .. '<div class="pm-entity-summary">Key [meta path="key" editable=true] Target [meta path="target_date" editable=true]</div>'
EPIC_HEADER = EMBED_SCRIPT .. EPIC_HEADER
TASK_HEADER = EMBED_SCRIPT .. '<section class="pm-entity-detail pm-task-detail" data-testid="pm-task-detail" aria-label="Task overview"><span class="pm-entity-kicker">Task</span><div class="pm-native-controls">[plugin:project-management:task-controls]<span class="pm-share-fallback">[meta path="status"]</span><span>Priority [meta path="priority" editable=true]</span></div></section>'
PROJECT_SUMMARY = PROJECT_SUMMARY .. ROLLUP_SUMMARY
EPIC_SUMMARY = EPIC_SUMMARY .. ROLLUP_SUMMARY
EPIC_HOVER_CARD = EPIC_HOVER_CARD .. ROLLUP_SUMMARY
PROJECT_MRQL_RESULT = PROJECT_MRQL_RESULT .. ROLLUP_SUMMARY
EPIC_MRQL_RESULT = EPIC_MRQL_RESULT .. ROLLUP_SUMMARY
PROJECT_LIST_HEADER = EMBED_SCRIPT .. PROJECT_LIST_HEADER
EPIC_LIST_HEADER = EMBED_SCRIPT .. EPIC_LIST_HEADER
TASK_LIST_HEADER = EMBED_SCRIPT .. TASK_LIST_HEADER
local GROUP_SECTIONS = '{"ownEntities":{"state":"open"},"relatedEntities":{"state":"collapsed"},"relations":{"state":"collapsed"},"merge":false,"clone":false,"metaJson":false}'
local TASK_SECTIONS = '{"metaJson":false,"noteTypeLink":false,"content":true,"tags":true,"groups":true,"resources":true,"share":true}'
EMBED_CSS = EMBED_CSS .. [[
/* project-management:integration:v2 */
.pm-native-controls{display:flex;flex-wrap:wrap;align-items:center;gap:.75rem}.pm-native-controls:has([data-pm-kind]) .pm-share-fallback{display:none}
[data-pm-kind] label{display:inline-flex;align-items:center;gap:.3rem;font-size:.8125rem}[data-pm-kind] input,[data-pm-kind] select{max-width:18rem;border:1px solid #a8a29e;border-radius:.35rem;padding:.3rem;background:white;color:#292524}
.pm-mini-board{display:grid;grid-template-columns:repeat(auto-fit,minmax(12rem,1fr));gap:.75rem}.pm-mini-column{padding:.6rem;border:1px solid #d6d3d1;border-radius:.5rem}.pm-mini-column h3{font-weight:700}.pm-mini-column article{display:grid;gap:.35rem;padding:.5rem 0;border-top:1px solid #e7e5e4}.pm-rollup{font-size:.8125rem;color:#57534e}.pm-block-row{display:flex;flex-wrap:wrap;gap:.4rem;align-items:center}.pm-block-row input{width:auto;flex:1;min-width:4rem}.pm-block-row button,.pm-block-editor button{border:1px solid #a8a29e;border-radius:.3rem;padding:.25rem .5rem}.pm-block-error{color:#b91c1c}
]]

-- Keep the earlier CSS intact as an upgrade sentinel; append visual fixes to
-- existing taxonomies without replacing operator-authored styles.
local ACCENT_CORNERS_MARKER = "project-management:accent-corners:v1"
local ACCENT_CORNERS_CSS = [[
/* project-management:accent-corners:v1 */
.pm-entity-detail{border-top-left-radius:0;border-bottom-left-radius:0}
]]

local BLOCK_ROW_MARKER = "project-management:block-row:v2"
local BLOCK_ROW_CSS = [[
/* project-management:block-row:v2 */
.pm-block-row{align-items:flex-end}
.pm-block-row>label{flex:1 1 10rem;min-width:0}
.pm-block-row input{width:100%;min-width:0;height:2.375rem}
.pm-block-row-actions{display:flex;flex-wrap:wrap;align-items:center;gap:.4rem;max-width:100%}
.pm-block-row-actions>button{height:2.375rem;white-space:nowrap}
.pm-block-row-actions>a{display:inline-flex;align-items:center;min-height:2.375rem}
.pm-time-entry>label:has([data-pm-field$=".date"]){flex:0 1 10rem}
.pm-time-entry>label:has([data-pm-field$=".hours"]){flex:0 1 4rem}
]]

local SELECT_ARROW_MARKER = "project-management:select-arrow:v1"
local SELECT_ARROW_CSS = [[
/* project-management:select-arrow:v1 */
/* Restore the host base layer's arrow erased by the old background shorthand. */
[data-pm-kind] select:not([multiple]){background:revert-layer;background-color:white;padding-right:2.5rem}
]]

local function presentation_css()
    local css = EMBED_CSS .. ACCENT_CORNERS_CSS .. BLOCK_ROW_CSS .. SELECT_ARROW_CSS
    local cfg = resolved_config()
    for _, entry in ipairs(cfg.statuses) do
        css = css .. '.pm-pill[data-pm-value="' .. entry.name .. '"]{--pm-color:' .. entry.color .. ';color:' .. entry.text_color .. '}'
    end
    for _, entry in ipairs(cfg.priorities) do
        css = css .. '.pm-pill.pm-pill-priority[data-pm-value="' .. entry.name .. '"]{--pm-color:' .. entry.color .. ';color:' .. entry.text_color .. '}'
    end
    return css
end

-- ---------------------------------------------------------------------------
-- Ordering: a port of ordering/position.go (a-z lexicographic position keys),
-- the scheme note blocks already use. No Lua binding exists for it.
-- ---------------------------------------------------------------------------

local function char_code(c)
    return string.byte(c)
end

local function midpoint(a, b)
    return string.char(math.floor((char_code(a) + char_code(b)) / 2))
end

-- generate_between: the lexicographic core. Defined before position_between
-- (which calls it): a Lua name resolves to a local only when the local is
-- declared above the reference, otherwise it is a (nil) global.
local function generate_between(before, after)
    local result = {}
    local i = 0
    local min_char, max_char = "a", "z"
    while true do
        local prev_char, next_char
        if i < #before then
            prev_char = before:sub(i + 1, i + 1)
        else
            prev_char = min_char
        end
        if i < #after then
            next_char = after:sub(i + 1, i + 1)
        else
            next_char = string.char(char_code(max_char) + 1)
        end

        if i >= #before and i < #after then
            if char_code(next_char) > char_code(min_char) then
                local mid = midpoint(min_char, next_char)
                if char_code(mid) >= char_code(next_char) then
                    mid = string.char(char_code(next_char) - 1)
                end
                if char_code(mid) >= char_code(min_char) then
                    result[#result + 1] = mid
                    return table.concat(result)
                end
            end
            result[#result + 1] = min_char
            i = i + 1
        elseif i >= #before and i >= #after then
            if #result == 0 then return min_char end
            return table.concat(result)
        elseif prev_char == next_char then
            result[#result + 1] = prev_char
            i = i + 1
        else
            local mid = midpoint(prev_char, next_char)
            if char_code(mid) > char_code(prev_char) and char_code(mid) < char_code(next_char) then
                result[#result + 1] = mid
                return table.concat(result)
            end
            result[#result + 1] = prev_char
            i = i + 1
            while true do
                if i < #before then
                    prev_char = before:sub(i + 1, i + 1)
                else
                    prev_char = string.char(char_code(min_char) - 1)
                end
                if char_code(prev_char) < char_code(max_char) then
                    local mid = midpoint(string.char(char_code(prev_char) + 1), string.char(char_code(max_char) + 1))
                    result[#result + 1] = mid
                    return table.concat(result)
                end
                result[#result + 1] = prev_char
                i = i + 1
            end
        end
    end
end

-- PositionBetween(before, after): a key strictly between the two ("" = the
-- open end). Returns a key that may equal `after` when the two inputs are
-- adjacent — callers must detect that and rebalance.
local function position_between(before, after)
    if (before == nil or before == "") and (after == nil or after == "") then
        return "n"
    end
    if before == nil or before == "" then before = "a" end
    if after == nil or after == "" then after = "{" end -- char just past 'z'
    return generate_between(before, after)
end

local function first_position()
    return "n"
end

-- even_positions returns n strictly ascending keys spread across the space, so
-- a rebalanced column keeps headroom at both ends. Port of
-- ordering.GenerateEvenPositions / indexToPosition (Go byte arithmetic on the
-- a-z range; Lua needs explicit char_code arithmetic).
local function even_positions(n)
    if n <= 0 then return {} end
    if n == 1 then return { "n" } end
    local min_c, max_c = char_code("a"), char_code("z")
    local alphabet_size = 26
    if n < alphabet_size - 1 then
        local step = (max_c - min_c) / (n + 1)
        local out = {}
        for i = 1, n do
            out[i] = string.char(min_c + math.floor(step * i))
        end
        return out
    end
    -- fixed-width base-26, slots mapped away from the boundaries
    local digits, capacity = 1, alphabet_size
    while capacity < n + 2 do
        digits = digits + 1
        capacity = capacity * alphabet_size
    end
    local out = {}
    for index = 1, n do
        local slot = 1 + math.floor((index * (capacity - 2)) / (n + 1))
        local chars = {}
        for d = digits, 1, -1 do
            chars[d] = string.char(min_c + (slot % alphabet_size))
            slot = math.floor(slot / alphabet_size)
        end
        out[index] = table.concat(chars)
    end
    return out
end

-- ---------------------------------------------------------------------------
-- Meta handling
-- ---------------------------------------------------------------------------

-- meta_object parses a JSON meta string into a Lua table, or returns a fresh
-- one when empty/absent. Invalid JSON is an error — a task whose meta is
-- corrupt must not be silently rewritten from scratch.
local function meta_object(meta_str)
    if meta_str == nil or meta_str == "" then return {} end
    local ok, obj = pcall(mah.json.decode, meta_str)
    if not ok then return nil, "stored meta is not valid JSON" end
    if type(obj) ~= "table" then return nil, "stored meta is not an object" end
    return obj
end

local function meta_string(obj)
    return mah.json.encode(obj)
end

local function meta_get(note_or_group, key)
    local obj = meta_object(note_or_group.meta)
    if not obj then return nil end
    return obj[key]
end

-- ---------------------------------------------------------------------------
-- Task reads (server side)
-- ---------------------------------------------------------------------------

-- task_scope_clause returns the MRQL owner clause for a project subtree
-- (owner.id = P OR ancestors.id = P) or an epic (owner.id = E).
local function task_scope_clause(container)
    if container.owner_epic then
        return string.format("owner.id = %d", container.epic_id)
    end
    return string.format("(owner.id = %d OR ancestors.id = %d)", container.project_id, container.project_id)
end

local function status_filter_clause(status, cfg)
    cfg = cfg or resolved_config()
    local exact = string.format("meta.status = %q", status)
    if status == cfg.default_status then
        return "(" .. exact .. " OR meta.status IS EMPTY)"
    end
    return exact
end

-- mrql_flat_tasks runs a flat MRQL task query over a container, returning the
-- item array. meta comes back as a parsed table; tags are not included.
local function mrql_flat_tasks(container, conditions, opts)
    local tax = cached_taxonomy()
    if not tax then return nil, "plugin not set up" end
    local limit = (opts and opts.limit) or 200
    local conds = { string.format("noteType = %d", tax.task_type_id), task_scope_clause(container) }
    if conditions then
        for _, c in ipairs(conditions) do conds[#conds + 1] = c end
    end
    local query = "type = note AND " .. table.concat(conds, " AND ")
    if opts and opts.order_by then
        query = query .. " ORDER BY " .. opts.order_by
    end
    if limit and limit > 0 then
        query = query .. " LIMIT " .. tostring(limit)
    end
    local res, err = mah.db.mrql_query(query, { limit = limit })
    if err then return nil, err end
    if not res or not res.items then return {}, nil end
    return res.items, nil
end

-- column_tasks returns the tasks in one status column of a container, in board
-- order (meta.order ascending; tasks without an order key come first, matching
-- the host's NULLS-FIRST sort on meta->>'order').
local function column_tasks(container, status, limit)
    local tax = cached_taxonomy()
    if not tax then return nil, "plugin not set up" end
    return mrql_flat_tasks(container, { status_filter_clause(status) },
        { limit = limit, order_by = "meta.order ASC" })
end

-- container_key is the canonical identity of an ordering container: a
-- project subtree, or an orphaned epic itself. Comparing project ids alone
-- conflates two orphaned epics (both nil), which must order independently.
local function container_key(container)
    if container.owner_epic then
        return "epic:" .. tostring(container.epic_id)
    end
    return "project:" .. tostring(container.project_id)
end

-- Serialize mutations that derive an order key for one column. Each such
-- transaction writes the same plugin_kvs row first: on SQLite that write takes
-- the single writer lock, on Postgres a row lock, and every other ordering
-- transaction for the same (container, status) blocks on it until this one
-- commits — so the column reads that follow (neighbour orders, the tail, or a
-- rebalance fetch) observe the previous transaction's insert. Without it, two
-- concurrent creates both read the same tail and both commit the same key.
-- Lock key for notes that are not in any status column yet
local NO_STATUS_LOCK_KEY = "__none__"
local NO_CONTAINER = { owner_epic = true, epic_id = 0 }

-- A per-task serialization row. Every plugin write to a task takes it (as
-- one of the sorted first writes of its transaction), so two of our endpoints
-- touching the same task serialize on it — on Postgres they otherwise lock
-- different columns (a metadata update locks the source column, a move the
-- destination's) and can commit a lost update between each other's snapshot
-- reads. Host-native note edits (the core note form) are outside this lock, as
-- they are outside the plugin entirely.
-- Entry builder for the sorted up-front lock set.
local function col_entry(container, status)
    return { key = "col:" .. container_key(container) .. ":" .. tostring(status) }
end

-- lock_columns acquires several column locks in one canonical order. An
-- owner-changing update may need two locks (the task's current column and the
-- destination's); two such updates in opposite directions would otherwise take
-- them in opposite order and deadlock on Postgres (SQLSTATE 40P01 aborts one
-- of them — a valid request fails). Sorting by the lock key makes the order
-- order-independent.
local function lock_columns(locks)
    table.sort(locks, function(a, b) return a.key < b.key end)
    for _, l in ipairs(locks) do
        mah.kv.set(l.key, "1")
    end
end

-- order_taken probes whether any task other than except_id already holds
-- `order` in the column. Two requests that computed the same key from the same
-- stale neighbours both see "free" here only if they run truly concurrently
-- (Postgres); the probe catches every case where the other commit is visible.
local function order_taken(container, status, order, except_id)
    local items, err = mrql_flat_tasks(container, {
        status_filter_clause(status),
        string.format("meta.order = %q", order),
    }, { limit = 2 })
    if err then return false, err end
    for _, it in ipairs(items or {}) do
        if it.id ~= except_id then return true, nil end
    end
    return false, nil
end

-- Transient-failure marker: an ordering transaction that discovers, after
-- taking its locks, that the task moved in a way its pre-transaction plan did
-- not anticipate raises this to roll back and retry with fresh pre-reads,
-- rather than acquiring an additional lock out of the canonical order.
local RETRY_MARKER = "pm_retry_transient"

local function is_transient_err(err)
    local msg = tostring(err)
    return msg:match("database is locked") or msg:match("database is busy")
        or msg:match("deadlock") or msg:match("40P01")
        or msg:find(RETRY_MARKER, 1, true) ~= nil
end

-- attempt_loop runs attempt_fn up to four times. attempt_fn performs its own
-- pre-transaction decision reads (so a retry re-reads and re-plans) and
-- returns (true, nil, result) when its transaction committed, or (false, err)
-- when it failed. Failures that match is_transient_err — database lock
-- contention, a Postgres deadlock abort (40P01), or the RETRY_MARKER raised
-- after a locked snapshot disagreed with the plan — are retried; deterministic
-- failures propagate on the first attempt.
local function attempt_loop(attempt_fn)
    local last_err
    for _ = 1, 4 do
        local ok, err, result = attempt_fn()
        if ok then return true, nil, result end
        last_err = err
        if not is_transient_err(err) then return false, err end
    end
    return false, last_err
end



-- column_tail_order returns the order key of the last task in a status column,
-- or nil when the column is empty. MRQL DESC puts rows without an order key
-- last, so the first row of a DESC fetch is the true maximum key.
local function column_tail_order(container, tax, status)
    local items, err = mrql_flat_tasks(container, { status_filter_clause(status) },
        { limit = 1, order_by = "meta.order DESC" })
    if err then return nil, err end
    if not items or #items == 0 then return nil end
    local meta = items[1].meta or {}
    return meta.order or nil
end

-- status_counts runs an aggregated COUNT over tasks matching conditions,
-- grouped by meta.status. Returns { [status_or_nil] = count, total = n }.
local function status_counts(container, conditions, tax)
    local conds = { string.format("noteType = %d", tax.task_type_id), task_scope_clause(container) }
    if conditions then
        for _, c in ipairs(conditions) do conds[#conds + 1] = c end
    end
    local query = "type = note AND " .. table.concat(conds, " AND ")
        .. " GROUP BY meta.status COUNT()"
    local res, err = mah.db.mrql_query(query, { limit = 100 })
    if err then return nil, err end
    local counts, total = {}, 0
    local cfg = resolved_config()
    if res and res.rows then
        for _, row in ipairs(res.rows) do
            local status = row["meta.status"]
            if status == nil or status == "" then status = cfg.default_status end
            local count = tonumber(row["count"]) or 0
            counts[status] = (counts[status] or 0) + count
            total = total + count
        end
    end
    counts.total = total
    return counts, nil
end

-- owner_container resolves the ordering container for a task owned by
-- `owner_group` (a PM Project or PM Epic group). The board's columns span the
-- whole project, so a task under an epic must take its position key from the
-- project's column, not the epic's — otherwise two tasks created under
-- different epics both start at key "n" and their column order is arbitrary.
-- Only an orphaned epic (no project group left) orders within itself.
local function owner_container(owner_group)
    if owner_group.category == TAXONOMY.epic_category and owner_group.owner_id and owner_group.owner_id > 0 then
        return { owner_epic = false, project_id = owner_group.owner_id }
    end
    if owner_group.category == TAXONOMY.epic_category then
        return { owner_epic = true, epic_id = owner_group.id }
    end
    return { owner_epic = false, project_id = owner_group.id }
end

-- A container is a project group or a single epic (for unassigned epics whose
-- project is gone).
local function container_for(project_id, epic_id)
    if epic_id and epic_id > 0 then
        local epic, err = mah.db.get_group(epic_id)
        if err then return nil, err end
        if not epic then return nil, "epic not found" end
        return { owner_epic = true, epic_id = epic_id, group = epic }, nil
    end
    local project, err = mah.db.get_group(project_id)
    if err then return nil, err end
    if not project then return nil, "project not found" end
    return { owner_epic = false, project_id = project_id, group = project }, nil
end

-- ---------------------------------------------------------------------------
-- API helpers
-- ---------------------------------------------------------------------------

local function api_error(ctx, status, message)
    ctx.status(status)
    ctx.json({ error = message })
end

local function parse_body(ctx)
    if ctx.body == nil or ctx.body == "" then return {}, "request body is required" end
    local ok, data = pcall(mah.json.decode, ctx.body)
    if not ok then return nil, "invalid JSON body" end
    if type(data) ~= "table" then return nil, "body must be a JSON object" end
    return data, nil
end

local function query_number(ctx, key)
    local v = ctx.query and ctx.query[key]
    if v == nil then return 0 end
    if type(v) == "table" then v = v[1] end
    local n = tonumber(v)
    if not n or n < 0 or n ~= math.floor(n) then return 0 end
    return n
end

-- Require the caller to be an administrator (or auth disabled: no principal).
local function require_admin(ctx)
    local p = ctx.principal
    if p == nil then return true, nil end
    if p.isAdmin or p.superUser then return true, nil end
    return false, "setting up project management requires an administrator"
end

-- ---------------------------------------------------------------------------
-- Validated date handling
-- ---------------------------------------------------------------------------

-- Dates must use the host's naive YYYY-MM-DDTHH:MM format. Reject zone
-- suffixes and impossible dates instead of letting the host silently store NULL.
local function normalize_datetime(value)
    if value == nil or value == "" then return nil, nil end
    local s = tostring(value)
    -- Two anchored alternatives (minutes, or minutes:seconds); Lua patterns
    -- have no non-capturing groups, so the seconds branch is spelled out.
    local y, mo, d, h, mi = s:match("^(%d%d%d%d)-(%d%d)-(%d%d)T(%d%d):(%d%d)$")
    local se
    if not y then
        local y2, mo2, d2, h2, mi2, se2 = s:match("^(%d%d%d%d)-(%d%d)-(%d%d)T(%d%d):(%d%d):(%d%d)$")
        y, mo, d, h, mi = y2, mo2, d2, h2, mi2
        se = se2
    end
    if not y then
        return nil, "dates must use YYYY-MM-DDTHH:MM format (RFC3339 zones and fractional seconds are not accepted)"
    end
    local ny, nmo, nd, nh, nmi = tonumber(y), tonumber(mo), tonumber(d), tonumber(h), tonumber(mi)
    if nmo < 1 or nmo > 12 or nd < 1 or nh > 23 or nmi > 59 then
        return nil, "date is not a real calendar date: " .. s
    end
    if se and tonumber(se) and tonumber(se) > 59 then
        return nil, "date is not a real calendar date: " .. s
    end
    local days_in_month = { 31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31 }
    if nmo == 2 and (ny % 400 == 0 or (ny % 4 == 0 and ny % 100 ~= 0)) then
        days_in_month[2] = 29
    end
    if nd > days_in_month[nmo] then
        return nil, "date is not a real calendar date: " .. s
    end
    return string.format("%04d-%02d-%02dT%02d:%02d", ny, nmo, nd, nh, nmi), nil
end

-- SQLite stores native dates with a space separator. MRQL string literals
-- must use that representation for chronological comparisons on the same day;
-- PostgreSQL accepts the same format. Keep seconds for precise boundaries.
local function query_datetime(value)
    local normalized, err = normalize_datetime(value)
    if not normalized then return nil, err end
    local seconds = tostring(value):match(":(%d%d)$")
    if #tostring(value) == 16 then seconds = "00" end
    return normalized:gsub("T", " ") .. ":" .. seconds, nil
end

-- ---------------------------------------------------------------------------
-- Native entity presentation and PM Task blocks
-- ---------------------------------------------------------------------------

local function config_entry(entries, name)
    if not name then return nil end
    for _, entry in ipairs(entries or {}) do
        if entry.name == name then return entry end
    end
    return nil
end

local function render_pill(value, entry, extra_class)
    if not value then return "" end
    local label = entry and entry.label or value
    local color = entry and entry.color or "#6366f1"
    return '<span class="pm-pill' .. (extra_class or "") .. '" data-pm-value="' .. mah.html_escape(value)
        .. '">'
        .. mah.html_escape(label) .. "</span>"
end

local function effective_status(value, cfg)
    if type(value) == "table" and type(value.status) == "string" and value.status ~= "" then
        return value.status
    end
    return cfg.default_status
end

local function render_task_badges(value)
    if type(value) ~= "table" then return "" end
    local cfg = resolved_config()
    local pills = {}
    local status = effective_status(value, cfg)
    pills[#pills + 1] = render_pill(status, config_entry(cfg.statuses, status), "")
    if value.priority then
        pills[#pills + 1] = render_pill(value.priority, config_entry(cfg.priorities, value.priority), " pm-pill-priority")
    end
    if #pills == 0 then return "" end
    return '<span class="pm-badges">'
        .. table.concat(pills) .. "</span>"
end

local TASK_STATUS_GLYPHS = {
    backlog = "\226\128\162", -- bullet
    todo = "\226\151\139", -- open circle
    in_progress = "\226\150\182", -- play
    blocked = "!",
    done = "\226\156\147", -- check
}

local function render_task_avatar(value)
    if type(value) ~= "table" then value = {} end
    local cfg = resolved_config()
    local status = effective_status(value, cfg)
    local entry = config_entry(cfg.statuses, status)
    local label = entry and entry.label or status
    local color = entry and entry.color or "#6b7280"
    local glyph = TASK_STATUS_GLYPHS[status] or string.upper(status:sub(1, 1))
    return '<span class="pm-entity-avatar pm-task-avatar" role="img" aria-label="'
        .. mah.html_escape(label .. " task") .. '" data-pm-status="' .. mah.html_escape(status)
        .. '" style="--pm-avatar-color:' .. color .. ';--pm-avatar-text:' .. darken_hex(color) .. '">'
        .. mah.html_escape(glyph) .. "</span>"
end

local function render_task_date(ctx)
    if ctx.entity_type ~= "note" or type(ctx.entity) ~= "table" then return "" end
    local due = ctx.entity.EndDate
    if type(due) ~= "string" or due == "" then return "" end
    local date = due:sub(1, 10)
    if not date:match("^%d%d%d%d%-%d%d%-%d%d$") then return "" end
    local cfg = resolved_config()
    local overdue = effective_status(ctx.value, cfg) ~= cfg.done_status
        and date < mah.util.now_iso():sub(1, 10)
    return '<span class="pm-entity-date' .. (overdue and " pm-overdue" or "") .. '">Due '
        .. mah.html_escape(date) .. (overdue and " (overdue)" or "") .. "</span>"
end

local function render_group_summary(ctx)
    if ctx.entity_type ~= "group" or not ctx.entity_id or ctx.entity_id == 0 then return "" end
    local value = ctx.value
    if type(value) ~= "table" then
        local group, err = mah.db.get_group(ctx.entity_id)
        if err or not group then return "" end
        if group.category ~= TAXONOMY.project_category and group.category ~= TAXONOMY.epic_category then return "" end
        value = meta_object(group.meta) or {}
    end
    local cfg = resolved_config()
    local facts = {}
    if value.status then
        facts[#facts + 1] = render_pill(value.status, config_entry(cfg.statuses, value.status), "")
    end
    if value.key and value.key ~= "" then
        facts[#facts + 1] = '<span class="pm-entity-fact"><span>Key</span><strong>'
            .. mah.html_escape(value.key) .. "</strong></span>"
    end
    if value.target_date and value.target_date ~= "" then
        facts[#facts + 1] = '<span class="pm-entity-fact"><span>Target</span><strong>'
            .. mah.html_escape(value.target_date) .. "</strong></span>"
    end
    if #facts == 0 then return "" end
    return '<span class="pm-entity-summary" data-testid="pm-group-summary">'
        .. table.concat(facts) .. "</span>"
end

local function render_entity_context(ctx)
    if not ctx.entity_id or ctx.entity_id == 0 then return "" end
    if ctx.entity_type ~= "group" and ctx.entity_type ~= "note" then return "" end

    -- The host resolves these names in batches for list and MRQL surfaces.
    -- Falling back to the plugin DB keeps preview/legacy render paths working,
    -- but the normal card path now performs no per-card lookup.
    local presentation = type(ctx.presentation) == "table" and ctx.presentation or {}
    local scope = type(presentation.scope) == "table" and presentation.scope or nil
    local parent = type(presentation.parent) == "table" and presentation.parent or nil
    local group
    if ctx.entity_type == "group" then
        group = {
            id = ctx.entity_id,
            name = type(ctx.entity) == "table" and ctx.entity.Name or "",
            category = scope and scope.category or nil,
            owner_id = parent and parent.id or nil,
        }
        if is_empty(group.category) then
            local loaded, err = mah.db.get_group(ctx.entity_id)
            if err or not loaded then return "" end
            group = loaded
        end
    else
        if not scope or not scope.id then
            local note, nerr = mah.db.get_note(ctx.entity_id)
            if nerr or not note or note.note_type ~= TAXONOMY.task_type or not note.owner_id then return "" end
            local loaded, gerr = mah.db.get_group(note.owner_id)
            if gerr or not loaded then return "" end
            group = loaded
        else
            group = { id = scope.id, name = scope.name or "", category = scope.category, owner_id = parent and parent.id or nil }
        end
    end

    local board_query
    local links = {}
    if group.category == TAXONOMY.project_category then
        board_query = "project=" .. tostring(group.id)
        if ctx.entity_type == "note" then
            links[#links + 1] = '<a href="/group?id=' .. tostring(group.id) .. '">'
                .. mah.html_escape(group.name) .. "</a>"
        end
    elseif group.category == TAXONOMY.epic_category then
        if ctx.entity_type == "note" then
            links[#links + 1] = '<a href="/group?id=' .. tostring(group.id) .. '">'
                .. mah.html_escape(group.name) .. "</a>"
        end
        if group.owner_id and group.owner_id > 0 then
            local project = parent
            if not project or not project.name or project.name == "" then
                local loaded, perr = mah.db.get_group(group.owner_id)
                if not perr then project = loaded end
            end
            if project and (project.category == nil or project.category == TAXONOMY.project_category) then
                board_query = "project=" .. tostring(project.id)
                links[#links + 1] = '<a href="/group?id=' .. tostring(project.id) .. '">'
                    .. mah.html_escape(project.name) .. "</a>"
            end
        end
        if not board_query then board_query = "epic=" .. tostring(group.id) end
    else
        return ""
    end
    links[#links + 1] = '<a href="/plugins/project-management/board?' .. board_query
        .. '&amp;view=board">View on board</a>'
    return '<span class="pm-context-links" data-testid="pm-entity-context">'
        .. table.concat(links, '<span aria-hidden="true">&middot;</span>') .. "</span>"
end

local function nonempty_lines(value)
    local out = {}
    for line in tostring(value or ""):gmatch("[^\r\n]+") do
        local cleaned = trim(line)
        if cleaned ~= "" then out[#out + 1] = cleaned end
    end
    return out
end

local function editor_field(block_id, label, field, value, multiline)
    local attrs = ' data-pm-block="' .. tostring(block_id) .. '" data-pm-field="' .. field .. '" data-pm-block-field="' .. field .. '"'
    local control = multiline and ('<textarea' .. attrs .. '>' .. mah.html_escape(value or "") .. '</textarea>')
        or ('<input' .. attrs .. ' value="' .. mah.html_escape(value or "") .. '">')
    return '<label><span>' .. mah.html_escape(label) .. '</span>' .. control .. '</label>'
end

local function block_button(ctx, action, label, index, collection)
    return '<button type="button" data-pm-block="' .. ctx.block.id .. '" data-pm-note="' .. ctx.note.id
        .. '" data-pm-block-action="' .. action .. '" data-pm-index="' .. tostring(index or 0)
        .. '" data-pm-collection="' .. (collection or 'items') .. '">' .. mah.html_escape(label) .. '</button>'
end

local function block_field(ctx, label, path, value, numeric)
    local out = editor_field(ctx.block.id,label,path,tostring(value or ''),false)
    if numeric then out = out:gsub('<input ', '<input type="number" min="0" step="any" ') end
    return out
end

local function row_buttons(ctx,index,collection,extra)
    return '<div class="pm-block-row-actions">' .. block_button(ctx,'up','Move up',index,collection)
        .. block_button(ctx,'down','Move down',index,collection)
        .. block_button(ctx,'remove','Remove row',index,collection) .. (extra or '') .. '</div>'
end

local function register_work_blocks(filters)
    local item_schema = {type='object',required={'id','label'},additionalProperties=false,properties={
        id={type='string',minLength=1,maxLength=100},label={type='string',maxLength=2000},task_id={type='integer',minimum=1}}}
    mah.block_type({type='subtasks',label='Subtasks',icon='ST',filters=filters,scripts=BLOCK_SCRIPTS,
        content_schema={type='object',required={'items'},additionalProperties=false,properties={items={type='array',maxItems=100,items=item_schema}}},
        state_schema={type='object',properties={checked={type='array',maxItems=100,uniqueItems=true,items={type='string'}}},additionalProperties=false},
        default_content={items=mah.json.array({})},default_state={checked=mah.json.array({})},
        render_view=function(ctx)
            local checked, rows = {}, {}
            for _, id in ipairs(ctx.block.state.checked or {}) do checked[id]=true end
            for i,item in ipairs(ctx.block.content.items or {}) do
                local mark = checked[item.id] and '☑ ' or '☐ '
                local label = mah.html_escape(item.label)
                if item.task_id then label = '<a href="/note?id=' .. item.task_id .. '">' .. label .. '</a>' end
                local toggle = ctx.can_write and block_button(ctx,'toggle',mark .. item.label,i-1) or mark .. label
                rows[#rows+1] = '<li>' .. toggle .. (item.task_id and (' <a href="/note?id=' .. item.task_id .. '">Promoted task</a>') or '') .. '</li>'
            end
            return '<section class="pm-content-block" data-testid="pm-subtasks"><h3>Subtasks</h3><ul>' .. table.concat(rows) .. '</ul></section>'
        end,
        render_edit=function(ctx)
            local rows = {}
            for i,item in ipairs(ctx.block.content.items or {}) do
                rows[#rows+1] = '<div class="pm-block-row">' .. block_field(ctx,'Subtask','items.' .. (i-1) .. '.label',item.label)
                    .. row_buttons(ctx,i-1,nil,not item.task_id and block_button(ctx,'promote','Promote to task',i-1) or '<a href="/note?id=' .. item.task_id .. '">Open task</a>') .. '</div>'
            end
            return '<section class="pm-content-block pm-block-editor" data-testid="pm-subtasks-editor"><h3>Subtasks</h3>' .. table.concat(rows) .. block_button(ctx,'add','Add subtask') .. '</section>'
        end})
    local ids = {type='array',maxItems=50,uniqueItems=true,items={type='integer',minimum=1}}
    mah.block_type({type='dependencies',label='Dependencies',icon='DP',filters=filters,scripts=BLOCK_SCRIPTS,
        content_schema={type='object',additionalProperties=false,properties={blocked_by=ids,blocks=ids},required={'blocked_by','blocks'}},
        default_content={blocked_by=mah.json.array({}),blocks=mah.json.array({})},default_state={},
        render_view=function(ctx)
            local rows, open, cfg = {}, 0, resolved_config()
            for _, key in ipairs({'blocked_by','blocks'}) do
                local links = {}
                for _, id in ipairs(ctx.block.content[key] or {}) do
                    local note = mah.db.get_note(id)
                    if note then
                        local status = effective_status(meta_object(note.meta),cfg)
                        if key == 'blocked_by' and status ~= cfg.done_status then open=open+1 end
                        links[#links+1] = '<li><a href="/note?id=' .. id .. '">' .. mah.html_escape(note.name) .. '</a> ' .. render_pill(status,config_entry(cfg.statuses,status),'') .. '</li>'
                    else links[#links+1] = '<li>Unavailable task</li>' end
                end
                rows[#rows+1] = '<h4>' .. (key == 'blocked_by' and 'Blocked by' or 'Blocks') .. '</h4><ul>' .. table.concat(links) .. '</ul>'
            end
            return '<section class="pm-content-block" data-testid="pm-dependencies"><h3>Dependencies</h3><p>Blocked by ' .. open .. ' open</p>' .. table.concat(rows) .. '</section>'
        end,
        render_edit=function(ctx)
            local rows = {}
            for _, key in ipairs({'blocked_by','blocks'}) do
                for i,id in ipairs(ctx.block.content[key] or {}) do
                    rows[#rows+1] = '<div class="pm-block-row">' .. block_field(ctx,key == 'blocked_by' and 'Blocked by note ID' or 'Blocks note ID',key .. '.' .. (i-1),id,true) .. row_buttons(ctx,i-1,key) .. '</div>'
                end
                rows[#rows+1] = '<label>Note ID<input type="number" min="1" data-pm-new-reference="' .. key .. '"></label>' .. block_button(ctx,'add',key == 'blocked_by' and 'Add blocker' or 'Add blocked task',0,key)
            end
            return '<section class="pm-content-block pm-block-editor" data-testid="pm-dependencies-editor"><h3>Dependencies</h3>' .. table.concat(rows) .. '</section>'
        end})
    mah.block_type({type='time-log',label='Time log',icon='TM',filters=filters,scripts=BLOCK_SCRIPTS,
        content_schema={type='object',required={'estimate_hours','entries'},additionalProperties=false,properties={
            estimate_hours={type='number',minimum=0,maximum=100000},entries={type='array',maxItems=200,items={type='object',required={'date','hours','note'},additionalProperties=false,properties={
                date={type='string',pattern='^\\d{4}-\\d{2}-\\d{2}$'},hours={type='number',minimum=0,maximum=10000},note={type='string',maxLength=2000}}}}}},
        default_content={estimate_hours=0,entries=mah.json.array({})},default_state={},
        render_view=function(ctx)
            local total, rows = 0, {}
            for _, entry in ipairs(ctx.block.content.entries or {}) do
                total=total+entry.hours
                rows[#rows+1] = '<li>' .. mah.html_escape(entry.date) .. ' · ' .. entry.hours .. ' h · ' .. mah.html_escape(entry.note) .. '</li>'
            end
            local estimate = ctx.block.content.estimate_hours or 0
            return '<section class="pm-content-block" data-testid="pm-time-log"><h3>Time log</h3><p>' .. total .. ' / ' .. estimate .. ' hours</p><progress aria-label="Logged hours versus estimate" max="' .. math.max(estimate,total,1) .. '" value="' .. total .. '"></progress><ul>' .. table.concat(rows) .. '</ul></section>'
        end,
        render_edit=function(ctx)
            local rows = {block_field(ctx,'Estimate hours','estimate_hours',ctx.block.content.estimate_hours,true)}
            for i,entry in ipairs(ctx.block.content.entries or {}) do
                local prefix='entries.' .. (i-1) .. '.'
                rows[#rows+1] = '<div class="pm-block-row pm-time-entry">' .. block_field(ctx,'Date (YYYY-MM-DD)',prefix .. 'date',entry.date)
                    .. block_field(ctx,'Hours',prefix .. 'hours',entry.hours,true) .. block_field(ctx,'Note',prefix .. 'note',entry.note) .. row_buttons(ctx,i-1,'entries') .. '</div>'
            end
            return '<section class="pm-content-block pm-block-editor" data-testid="pm-time-log-editor"><h3>Time log</h3>' .. table.concat(rows) .. block_button(ctx,'add','Add time entry',0,'entries') .. '</section>'
        end})
end

local pm_block_types_registered = false

local function register_pm_block_types(tax)
    if pm_block_types_registered or not tax or not tax.task_type_id then return end
    local filters = { note_type_ids = { tax.task_type_id } }

    mah.block_type({
        type = "acceptance-criteria",
        scripts = BLOCK_SCRIPTS,
        label = "Acceptance criteria",
        icon = "AC",
        description = "Outcome-focused acceptance criteria and their verification method.",
        filters = filters,
        content_schema = {
            type = "object",
            properties = {
                criteria = { type = "string", maxLength = 10000 },
                verification = { type = "string", maxLength = 2000 },
            },
            required = { "criteria" },
            additionalProperties = false,
        },
        default_content = { criteria = "Describe one observable outcome per line", verification = "" },
        default_state = {},
        render_view = function(ctx)
            local lines = nonempty_lines(ctx.block.content.criteria)
            local items = {}
            for _, line in ipairs(lines) do items[#items + 1] = "<li>" .. mah.html_escape(line) .. "</li>" end
            local body = #items > 0 and ("<ul>" .. table.concat(items) .. "</ul>")
                or "<p>No acceptance criteria yet.</p>"
            local verification = ctx.block.content.verification
            if verification and verification ~= "" then
                body = body .. '<p><strong>Verify:</strong> ' .. mah.html_escape(verification) .. "</p>"
            end
            return '<section class="pm-content-block" data-testid="pm-acceptance-criteria" aria-label="Acceptance criteria">'
                .. "<h3>Acceptance criteria</h3>" .. body .. "</section>"
        end,
        render_edit = function(ctx)
            return '<section class="pm-content-block pm-block-editor" data-testid="pm-acceptance-criteria-editor" aria-label="Edit acceptance criteria">'
                .. "<h3>Acceptance criteria</h3>"
                .. editor_field(ctx.block.id, "Criteria (one per line)", "criteria", ctx.block.content.criteria, true)
                .. editor_field(ctx.block.id, "Verification", "verification", ctx.block.content.verification, false)
                .. "</section>"
        end,
    })

    mah.block_type({
        type = "status-update",
        scripts = BLOCK_SCRIPTS,
        label = "Status update",
        icon = "UP",
        description = "A concise progress update with next step and blocker.",
        filters = filters,
        content_schema = {
            type = "object",
            properties = {
                summary = { type = "string", maxLength = 10000 },
                next_step = { type = "string", maxLength = 4000 },
                blocker = { type = "string", maxLength = 4000 },
            },
            required = { "summary" },
            additionalProperties = false,
        },
        default_content = { summary = "What changed?", next_step = "", blocker = "" },
        default_state = {},
        render_view = function(ctx)
            local content = ctx.block.content
            local rows = { "<dt>Update</dt><dd>" .. mah.html_escape(content.summary or "") .. "</dd>" }
            if content.next_step and content.next_step ~= "" then
                rows[#rows + 1] = "<dt>Next</dt><dd>" .. mah.html_escape(content.next_step) .. "</dd>"
            end
            if content.blocker and content.blocker ~= "" then
                rows[#rows + 1] = "<dt>Blocker</dt><dd>" .. mah.html_escape(content.blocker) .. "</dd>"
            end
            return '<section class="pm-content-block" data-testid="pm-status-update" aria-label="Status update">'
                .. "<h3>Status update</h3><dl>" .. table.concat(rows) .. "</dl></section>"
        end,
        render_edit = function(ctx)
            local content = ctx.block.content
            return '<section class="pm-content-block pm-block-editor" data-testid="pm-status-update-editor" aria-label="Edit status update">'
                .. "<h3>Status update</h3>"
                .. editor_field(ctx.block.id, "Update", "summary", content.summary, true)
                .. editor_field(ctx.block.id, "Next step", "next_step", content.next_step, false)
                .. editor_field(ctx.block.id, "Blocker", "blocker", content.blocker, false)
                .. "</section>"
        end,
    })

    register_work_blocks(filters)
    pm_block_types_registered = true
end

-- All write entry points share these handlers, including host actions.
local task_handlers = {}
local function register_task_api(name, handler)
    task_handlers[name] = handler
    mah.api("POST", "api/task/" .. name, handler)
end

local function full_task(id)
    local task, err = mah.db.get_note(id)
    if not task then error(err or "task not found") end
    task.meta = meta_object(task.meta) or {}
    task.status = effective_status(task.meta, resolved_config())
    return task
end

local function call_task(name, body)
    local result, status = nil, 200
    task_handlers[name]({body = meta_string(body), status = function(n) status = n end,
        json = function(value) result = value end})
    if status >= 400 or not result then error(result and result.error or "Task operation failed") end
    return result
end

local pm_actions_registered = false
local function register_pm_actions(tax)
    if pm_actions_registered or not tax or not tax.task_type_id then return end
    local cfg = resolved_config()
    local statuses, priorities = {}, {}
    for _, entry in ipairs(cfg.statuses) do statuses[#statuses + 1] = entry.name end
    for _, entry in ipairs(cfg.priorities) do priorities[#priorities + 1] = entry.name end
    local actions = {
        {id="pm-set-status", label="Set task status", param={name="status",type="select",label="Status",options=statuses,required=true}},
        {id="pm-set-priority", label="Set task priority", param={name="priority",type="select",label="Priority",options=priorities,required=true}},
        {id="pm-set-due", label="Set task due date", param={name="due",type="text",label="Due (YYYY-MM-DDTHH:MM)",required=true}},
        {id="pm-clear-due", label="Clear task due date"},
        {id="pm-move-to-epic", label="Move task to epic or project", param={name="owner_id",type="entity_ref",entity="group",label="Epic or project",required=true,filters={category_ids={tax.epic_category_id,tax.project_category_id}}}},
    }
    for _, spec in ipairs(actions) do
        mah.action({id=spec.id,label=spec.label,entity="note",placement={"detail","card","bulk"},
            filters={note_type_ids={tax.task_type_id}},bulk_max=50,async=false,
            description="Applies to PM Tasks only. Each selected task saves separately; a later failure does not undo earlier changes.",
            params=spec.param and {spec.param} or {}, handler=function(ctx)
                local body = merge(ctx.params or {}, {id=ctx.entity_id})
                if spec.id == "pm-clear-due" then body.due = "" end
                call_task("update",body)
                return {success=true,message="Task updated"}
            end})
    end
    local filters = {category_ids={tax.project_category_id,tax.epic_category_id}}
    mah.action({id="pm-new-task",label="New task",entity="group",placement={"detail","card"},filters=filters,
        params={{name="name",type="text",label="Task name",required=true}}, handler=function(ctx)
            local task = call_task("create",{owner_id=ctx.entity_id,name=ctx.params.name})
            return {success=true,redirect="/note?id=" .. task.id}
        end})
    mah.action({id="pm-open-board",label="Open board",entity="group",placement={"detail"},filters=filters,
        handler=function(ctx)
            local group = mah.db.get_group(ctx.entity_id)
            if not group or (group.category_id ~= tax.project_category_id and group.category_id ~= tax.epic_category_id) then error("Not a PM project or epic") end
            return {success=true,redirect="/plugins/project-management/board?" .. (group.category_id == tax.epic_category_id and "epic=" or "project=") .. ctx.entity_id}
        end})
    pm_actions_registered = true
end

local function control_html(kind, id, value, placeholder, read_only, options)
    local attrs = ' data-pm-kind="' .. kind .. '" data-pm-id="' .. tostring(id) .. '"'
    if read_only then return '<span' .. attrs .. '>' .. placeholder .. '</span>' end
    return '<pm-' .. kind .. '-control' .. attrs .. ' data-morph-client-owned data-value="'
        .. mah.html_escape(tostring(value or "")) .. '" data-options="' .. mah.html_escape(meta_string(options or {})) .. '">'
        .. placeholder .. '</pm-' .. kind .. '-control>'
end

local function render_task_controls(ctx)
    if ctx.entity_type ~= "note" then return "" end
    local cfg = resolved_config()
    local entity = ctx.entity or {}
    local status = effective_status(ctx.value, cfg)
    local owner = ctx.presentation and ctx.presentation.scope or {}
    local due = entity.EndDate or ""
    local rendered = control_html("status",ctx.entity_id,status,render_pill(status,config_entry(cfg.statuses,status),""),ctx.read_only,cfg.statuses)
    rendered = rendered .. control_html("due",ctx.entity_id,due,render_task_date(ctx),ctx.read_only)
    rendered = rendered .. control_html("owner",ctx.entity_id,entity.OwnerId or owner.id,mah.html_escape(owner.name or "No owner"),ctx.read_only)
    return rendered
end

local function group_container(group, tax)
    if group.category_id == tax.epic_category_id or group.category == TAXONOMY.epic_category then
        return {owner_epic=true,epic_id=group.id,project_id=group.owner_id or 0}
    end
    return {owner_epic=false,project_id=group.id}
end

local function render_mini_board(ctx)
    local tax = cached_taxonomy()
    if not tax or ctx.entity_type ~= "group" then return "" end
    local entity = ctx.entity or {}
    local group = {id=ctx.entity_id,category_id=entity.CategoryId,owner_id=entity.OwnerId}
    local container = group_container(group,tax)
    local counts = ctx.value and ctx.value.pm_counts
    if type(counts) ~= "table" then counts = status_counts(container,nil,tax) end
    if not counts then return '<p>Task counts are temporarily unavailable.</p>' end
    local cfg, columns = resolved_config(), {}
    for _, status in ipairs(cfg.statuses) do
        local tasks, err = column_tasks(container,status.name,5)
        if err then return '<p>Tasks are temporarily unavailable.</p>' end
        local cards = {}
        for _, task in ipairs(tasks or {}) do
            cards[#cards+1] = '<article data-pm-id="' .. task.id .. '"><a href="/note?id=' .. task.id .. '">' .. mah.html_escape(task.name) .. '</a>'
                .. control_html("status",task.id,status.name,render_pill(status.name,status,""),ctx.read_only,cfg.statuses) .. '</article>'
        end
        columns[#columns+1] = '<section class="pm-mini-column" data-status="' .. status.name .. '"><h3>' .. mah.html_escape(status.label)
            .. ' <span class="pm-mini-count">' .. tostring(counts[status.name] or 0) .. '</span></h3>' .. table.concat(cards) .. '</section>'
    end
    return '<div class="pm-mini-board" data-testid="pm-mini-board">' .. table.concat(columns) .. '</div>'
end

-- Cached counters are presentation data. A complete periodic reconciliation
-- also covers deletions, bulk edits and skipped after-hooks. Dirty hints never
-- decide whether a group is eligible for repair.
local function reconcile_rollups()
    local tax = cached_taxonomy()
    if not tax then return end
    local cfg = resolved_config()
    for _, category in ipairs({tax.project_category_id,tax.epic_category_id}) do
        local groups, err = list_groups(category)
        if err then error(err) end
        for _, group in ipairs(groups or {}) do
            local container = group_container(group,tax)
            local counts, cerr = status_counts(container,nil,tax)
            if not counts then error(cerr) end
            local not_done = string.format("meta.status != %q",cfg.done_status)
            if cfg.default_status ~= cfg.done_status then not_done = '(' .. not_done .. ' OR meta.status IS EMPTY)' end
            local overdue, oerr = status_counts(container,{not_done,'endDate < NOW()'},tax)
            if not overdue then error(oerr) end
            local next_tasks, nerr = mrql_flat_tasks(container,{not_done,'endDate IS NOT NULL'},{limit=1,order_by='endDate ASC'})
            if nerr then error(nerr) end
            local next_due = ""
            if next_tasks and next_tasks[1] then
                local note, err = mah.db.get_note(next_tasks[1].id)
                if not note then error(err or "Task disappeared during rollup") end
                next_due = note.end_date or ""
            end
            local subtasks, checked, cursor = 0, 0, 0
            while true do
                local notes, err = mah.db.query_notes({note_type_id=tax.task_type_id,include_blocks=true,limit=100,
                    mrql=task_scope_clause(container) .. ' AND id > ' .. cursor,sort_by={'id asc'}})
                if not notes then error(err) end
                for _, note in ipairs(notes) do
                    cursor = math.max(cursor,note.id)
                    for _, block in ipairs(note.blocks or {}) do
                        if block.type == 'plugin:project-management:subtasks' then
                            local done = {}
                            for _, id in ipairs(block.state.checked or {}) do done[id]=true end
                            for _, row in ipairs(block.content.items or {}) do
                                subtasks=subtasks+1
                                if done[row.id] then checked=checked+1 end
                            end
                        end
                    end
                end
                if #notes < 100 then break end
            end
            -- Serialize the read-modify-write with other plugin rollup writers.
            local ok, txerr = mah.db.transaction(function()
                mah.kv.set('rollup:' .. group.id,'1')
                local current, err = mah.db.get_group(group.id)
                if not current then error(err or "Group disappeared during rollup") end
                local meta = meta_object(current.meta)
                if not meta then error("Group metadata is not an object") end
                meta.pm_counts = counts
                meta.pm_subtasks = subtasks
                meta.pm_subtasks_done = checked
                meta.pm_done = counts[cfg.done_status] or 0
                meta.pm_open = counts.total - meta.pm_done
                meta.pm_overdue = overdue.total
                meta.pm_next_due = next_due
                meta.pm_rollup_at = mah.util.now_iso()
                local saved, err = mah.db.patch_group(group.id,{meta=meta_string(meta)})
                if not saved then error(err) end
            end)
            if not ok then error(txerr) end
        end
    end
    mah.kv.set('pm_rollup_last',mah.util.now_iso())
end

local function register_pm_hooks()
    local function stamp(data)
        local tax = cached_taxonomy()
        if not tax or data.note_type_id ~= tax.task_type_id then return data end
        local meta = meta_object(data.meta)
        if meta and (meta.status == nil or meta.status == "") then
            meta.status = resolved_config().default_status
            data.meta = meta_string(meta)
        end
        return data
    end
    mah.on('before_note_create',stamp)
    mah.on('before_note_update',stamp)
    for _, event in ipairs({'after_note_create','after_note_update','after_note_delete'}) do
        mah.on(event,function(data)
            local tax = cached_taxonomy()
            if tax and data.note_type_id == tax.task_type_id then mah.kv.set('pm_rollup_dirty',true) end
        end)
    end
    mah.schedule({id='rollup',every='10m',overlap='skip',handler=reconcile_rollups})
end

-- ---------------------------------------------------------------------------
-- init()
-- ---------------------------------------------------------------------------

-- The plugin page shell. The views themselves are rendered by pm.js; this is
-- only the mount point plus the static asset links. One handler serves all
-- four views; the client switches on the ?view= parameter.
local function build_page_html()
    local base = "/plugins/project-management/public"
    return '<div class="pm-app" id="pm-app">'
        .. '<h2 class="pm-app-title">Project Management</h2>'
        .. '<div id="pm-root" class="pm-root"></div>'
        .. '<noscript><p>Project Management needs JavaScript for its views. '
        .. 'The task notes stay fully usable through the regular Notes pages.</p></noscript>'
        .. '</div>'
        .. '<link rel="stylesheet" href="' .. base .. '/pm.css?v=1.1.0-select-arrows">'
        .. EMBED_SCRIPT .. '<style>' .. presentation_css() .. '</style>'
        .. '<script defer src="' .. base .. '/pm.js"></script>'
end

function init()
    local page_html = build_page_html()

    -- A first install provisions the dynamic note-type id from api/setup, so
    -- setup registers these too. On later boots the cached id lets init restore
    -- the registrations immediately.
    register_pm_block_types(cached_taxonomy())
    register_pm_actions(cached_taxonomy())
    register_pm_hooks()
    mah.shortcode({name="task-controls",label="Task controls",render=render_task_controls})
    mah.shortcode({name="mini-board",label="Project mini board",render=render_mini_board})

    -- ------------------------------------------------------------------
    -- Page shell. One handler serves all four views; the client switches on
    -- the ?view= parameter and re-renders in place.
    -- ------------------------------------------------------------------
    for _, view in ipairs({"board", "backlog", "dashboard", "timeline"}) do
        mah.page(view, function(ctx) return page_html end, { hide_sidebar = true })
        mah.menu("PM " .. view:sub(1,1):upper() .. view:sub(2), view)
    end

    -- ------------------------------------------------------------------
    -- Shortcodes used by the taxonomy's Custom* slots
    -- ------------------------------------------------------------------
    mah.shortcode({
        name = "view-links",
        label = "Project Management view links",
        description = "Links to the board, backlog, dashboard and timeline for the group being viewed.",
        render = function(ctx)
            if ctx.entity_type ~= "group" or not ctx.entity_id or ctx.entity_id == 0 then
                return ""
            end
            local group, err = mah.db.get_group(ctx.entity_id)
            if err or not group then return "" end
            local tax = cached_taxonomy()
            if not tax then return "" end
            local project_id = ctx.entity_id
            if group.category and group.category == TAXONOMY.epic_category then
                project_id = group.owner_id or 0
            end
            if project_id == 0 then return "" end
            if group.category and group.category ~= TAXONOMY.project_category
                and group.category ~= TAXONOMY.epic_category then
                return ""
            end
            local base = "/plugins/project-management/board?project=" .. project_id
            local views = {
                { v = "board", label = "Board" },
                { v = "backlog", label = "Backlog" },
                { v = "dashboard", label = "Dashboard" },
                { v = "timeline", label = "Timeline" },
            }
            local links = {}
            for _, view in ipairs(views) do
                links[#links + 1] = '<a class="pm-view-link" href="' .. base .. "&view=" .. view.v .. '">'
                    .. mah.html_escape(view.label) .. "</a>"
            end
            return '<div class="pm-embed-nav" aria-label="Project views">' .. table.concat(links) .. "</div>"
        end,
    })

    mah.shortcode({
        name = "group-summary",
        label = "Project or epic summary",
        description = "Renders configured status, key and target date for a PM Project or PM Epic.",
        render = render_group_summary,
    })

    mah.shortcode({
        name = "entity-context",
        label = "Project Management entity context",
        description = "Links a PM task or epic to its owning entities and board.",
        render = function(ctx)
            if ctx.entity_type ~= "note" then return render_entity_context(ctx) end
            return '<span data-pm-region="context" data-pm-id="' .. ctx.entity_id .. '">' .. render_entity_context(ctx) .. '</span>'
        end,
    })

    mah.shortcode({
        name = "progress",
        label = "Task progress bar",
        description = "Renders done/total task progress for the project or epic being viewed (or a project=\"id\" attribute).",
        attrs = {
            { name = "project", type = "string", required = false, description = "Project group id" },
        },
        render = function(ctx)
            local target = ctx.attrs and ctx.attrs.project or nil
            local group_id = 0
            if target then
                group_id = tonumber(target) or 0
            elseif ctx.entity_type == "group" then
                group_id = ctx.entity_id or 0
            end
            if group_id == 0 then return "" end
            local tax = cached_taxonomy()
            if not tax then return "" end
            local group, gerr = mah.db.get_group(group_id)
            if gerr or not group then return "" end
            local project_id, epic_id = group_id, nil
            if group.category == TAXONOMY.project_category then
                epic_id = nil
            elseif group.category == TAXONOMY.epic_category then
                project_id = group.owner_id or 0
                epic_id = group_id
            else
                return ""
            end
            if project_id == 0 and not epic_id then return "" end
            local container = { owner_epic = epic_id ~= nil, project_id = project_id, epic_id = epic_id }
            local value = meta_object(group.meta) or {}
            local counts, cerr = value.pm_counts, nil
            if type(counts) ~= "table" then counts, cerr = status_counts(container, nil, tax) end
            if cerr then return "" end
            local cfg = resolved_config()
            local done = counts[cfg.done_status] or 0
            local total = counts.total
            if total == 0 then
                return '<div class="pm-progress" role="progressbar" aria-label="Task completion" '
                    .. 'aria-valuemin="0" aria-valuemax="1" aria-valuenow="0" aria-valuetext="No tasks yet">'
                    .. '<span class="pm-progress-fill" style="width:0%"></span></div>'
            end
            local pct = math.floor((done / total) * 100)
            return '<div class="pm-progress-wrap">'
                .. '<div class="pm-progress" role="progressbar" aria-label="Task completion" aria-valuemin="0" '
                .. 'aria-valuemax="' .. total .. '" aria-valuenow="' .. done .. '" aria-valuetext="'
                .. done .. " of " .. total .. " tasks done (" .. pct .. "%)" .. '">'
                .. '<span class="pm-progress-fill" style="width:' .. pct .. '%"></span></div>'
                .. '<span class="pm-progress-text">' .. done .. " / " .. total .. " done</span></div>"
        end,
    })

    mah.shortcode({
        name = "task-list",
        label = "Recent tasks",
        description = "Lists the most recently updated tasks of the project or epic being viewed.",
        attrs = {
            { name = "limit", type = "number", required = false, description = "How many tasks to list", default = "8" },
        },
        render = function(ctx)
            local limit = tonumber(ctx.attrs and ctx.attrs.limit or "8") or 8
            if limit > 25 then limit = 25 end
            local group_id = 0
            if ctx.entity_type == "group" then group_id = ctx.entity_id or 0 end
            if group_id == 0 then return "" end
            local tax = cached_taxonomy()
            if not tax then return "" end
            local group, gerr = mah.db.get_group(group_id)
            if gerr or not group then return "" end
            local container
            if group.category == TAXONOMY.project_category then
                container = { owner_epic = false, project_id = group_id, epic_id = nil }
            elseif group.category == TAXONOMY.epic_category then
                container = { owner_epic = true, project_id = 0, epic_id = group_id }
            else
                return ""
            end
            local items, err = mrql_flat_tasks(container, nil, { limit = limit, order_by = "updated DESC" })
            if err then return "" end
            if not items or #items == 0 then return "" end
            local cfg = resolved_config()
            local lines = {}
            for _, item in ipairs(items) do
                local meta = item.meta or {}
                local done = effective_status(meta, cfg) == cfg.done_status
                lines[#lines + 1] = '<li class="pm-task-item' .. (done and " pm-task-done" or "") .. '">'
                    .. '<a href="/note?id=' .. tostring(item.id) .. '">' .. mah.html_escape(item.name) .. "</a>"
                    .. "</li>"
            end
            return '<ul class="pm-task-list" aria-label="Tasks">' .. table.concat(lines) .. "</ul>"
        end,
    })

    mah.shortcode({
        name = "task-badges",
        label = "Task status and priority badges",
        description = "Renders the status and priority pills for a PM Task note.",
        render = function(ctx)
            if ctx.entity_type ~= "note" then return "" end
            return '<span data-pm-region="badges" data-pm-id="' .. ctx.entity_id .. '">' .. render_task_badges(ctx.value) .. '</span>'
        end,
    })

    mah.shortcode({
        name = "task-avatar",
        label = "Status-aware task avatar",
        description = "Renders a task avatar whose glyph and colour reflect the effective status.",
        render = function(ctx)
            if ctx.entity_type ~= "note" then return "" end
            return '<span data-pm-region="avatar" data-pm-id="' .. ctx.entity_id .. '">' .. render_task_avatar(ctx.value) .. '</span>'
        end,
    })

    mah.shortcode({
        name = "task-date",
        label = "Task due date",
        description = "Renders a due date and marks unfinished past-due tasks as overdue.",
        render = function(ctx) return '<span data-pm-region="date" data-pm-id="' .. ctx.entity_id .. '">' .. render_task_date(ctx) .. '</span>' end,
    })

    -- ------------------------------------------------------------------
    -- API endpoints
    -- ------------------------------------------------------------------

    mah.api('POST','api/task/promote',function(ctx)
        local body, err = parse_body(ctx)
        if not body then api_error(ctx,400,err) return end
        local tax = cached_taxonomy()
        local parent, perr = mah.db.get_note(tonumber(body.id) or 0)
        if not tax or not parent or parent.note_type_id ~= tax.task_type_id then api_error(ctx,400,perr or 'Not a PM Task') return end
        local item
        for _, block in ipairs(parent.blocks or {}) do
            if block.id == tonumber(body.block_id) and block.type == 'plugin:project-management:subtasks' then
                for _, row in ipairs(block.content.items or {}) do if row.id == body.item_id then item=row break end end
            end
        end
        if not item then api_error(ctx,400,'Subtask row not found') return end
        local key = 'promoted:' .. tostring(body.block_id) .. ':' .. item.id
        local promoted
        local ok, txerr = mah.db.transaction(function()
            -- Claim before reading so repeated clicks/retries create exactly one note.
            local existing = mah.kv.get(key)
            if existing then
                local previous = mah.db.get_note(existing)
                if not previous then error('Previously promoted task is unavailable') end
                promoted = full_task(existing)
                return
            end
            -- The VM serializes promotion requests; the transaction makes the
            -- note and durable idempotency record commit together.
            promoted = call_task('create',{owner_id=parent.owner_id,name=item.label})
            mah.kv.set(key,promoted.id)
        end)
        if not ok then api_error(ctx,400,tostring(txerr)) return end
        ctx.json(promoted)
    end)

    -- GET api/config — resolved ids, status and priority lists.
    mah.api("GET", "api/config", function(ctx)
        local tax = cached_taxonomy()
        local cfg = resolved_config()
        if not tax then
            ctx.json({ configured = false, plugin = "project-management", statuses = cfg.statuses,
                priorities = cfg.priorities, default_status = cfg.default_status })
            return
        end
        ctx.json({
            configured = true,
            task_type_id = tax.task_type_id,
            project_category_id = tax.project_category_id,
            epic_category_id = tax.epic_category_id,
            statuses = cfg.statuses,
            priorities = cfg.priorities,
            default_status = cfg.default_status,
            done_status = cfg.done_status,
        })
    end)

    -- POST api/setup — idempotent taxonomy provisioning (admin gesture).
    mah.api("POST", "api/setup", function(ctx)
        local allowed, reason = require_admin(ctx)
        if not allowed then api_error(ctx, 403, reason) return end

        local cfg = resolved_config()
        local tax = cached_taxonomy() or {}

        -- Find or create "PM Project".
        local project_category = nil
        if tax.project_category_id then
            local got, _ = mah.db.get_category(tax.project_category_id)
            if got then project_category = got end
        end
        if not project_category then
            local found, ferr = find_category(TAXONOMY.project_category)
            if ferr then api_error(ctx, 500, ferr) return end
            if found then
                project_category = found
            else
                local created, cerr = mah.db.create_category({
                    name = TAXONOMY.project_category,
                    description = "Groups managed as projects by the Project Management plugin",
                })
                if not created then api_error(ctx, 500, "creating category: " .. tostring(cerr)) return end
                project_category = mah.db.get_category(created.id)
            end
        end

        -- Find or create "PM Epic".
        local epic_category = nil
        if tax.epic_category_id then
            local got, _ = mah.db.get_category(tax.epic_category_id)
            if got then epic_category = got end
        end
        if not epic_category then
            local found, ferr = find_category(TAXONOMY.epic_category)
            if ferr then api_error(ctx, 500, ferr) return end
            if found then
                epic_category = found
            else
                local created, cerr = mah.db.create_category({
                    name = TAXONOMY.epic_category,
                    description = "Epics within PM Project groups, managed by the Project Management plugin",
                })
                if not created then api_error(ctx, 500, "creating category: " .. tostring(cerr)) return end
                epic_category = mah.db.get_category(created.id)
            end
        end

        -- Find or create the "PM Task" note type.
        local task_type = nil
        if tax.task_type_id then
            local got, _ = mah.db.get_note_type(tax.task_type_id)
            if got then task_type = got end
        end
        if not task_type then
            local found, ferr = find_note_type(TAXONOMY.task_type)
            if ferr then api_error(ctx, 500, ferr) return end
            if found then
                task_type = found
            else
                local created, cerr = mah.db.create_note_type({ name = TAXONOMY.task_type, description = "Tasks" })
                if not created then api_error(ctx, 500, "creating note type: " .. tostring(cerr)) return end
                task_type = mah.db.get_note_type(created.id)
            end
        end

        -- Repair slots and schemas on rows we own. Empty fields get the plugin
        -- default; a value equal to the previous bundled default is upgraded.
        -- Any other value belongs to the operator and survives setup. CSS is
        -- additive so an existing customization gains the new presentation
        -- rules without being replaced.
        local function fields_to_patch(carrier, fields)
            local need = nil
            for k, spec in pairs(fields) do
                local current = carrier[k]
                local matches_legacy = false
                if type(spec.legacy) == "table" then
                    for _, legacy in ipairs(spec.legacy) do
                        if current == legacy then matches_legacy = true break end
                    end
                elseif spec.legacy and current == spec.legacy then
                    matches_legacy = true
                end
                if (is_empty(current) and not spec.allow_empty) or matches_legacy or spec.force then
                    need = need or {}
                    need[k] = spec.value
                elseif spec.add_css and not tostring(current):find("project-management:integration:v2", 1, true) then
                    need = need or {}
                    need[k] = tostring(current) .. "\n" .. presentation_css()
                elseif spec.add_css and not tostring(current):find(ACCENT_CORNERS_MARKER, 1, true) then
                    need = need or {}
                    need[k] = tostring(current) .. "\n" .. ACCENT_CORNERS_CSS
                end
                if spec.add_css then
                    local css = (need and need[k]) or tostring(current)
                    if not css:find(BLOCK_ROW_MARKER, 1, true) then
                        need = need or {}
                        need[k] = css .. "\n" .. BLOCK_ROW_CSS
                    end
                    css = (need and need[k]) or css
                    if not css:find(SELECT_ARROW_MARKER, 1, true) then
                        need = need or {}
                        need[k] = css .. "\n" .. SELECT_ARROW_CSS
                    end
                end
            end
            return need
        end

        local function repair_category_fields(cat, fields)
            local need = fields_to_patch(cat, fields)
            if need then
                local ok, uerr = mah.db.patch_category(cat.id, need)
                if not ok then api_error(ctx, 500, "updating category: " .. tostring(uerr)) return false end
            end
            return true
        end

        local project_fields = {
            section_config = { value = GROUP_SECTIONS, legacy = "{}" },
            custom_own_entities = { value = "[plugin:project-management:mini-board]" },
            meta_schema = { value = project_schema_json(cfg) },
            custom_header = { value = PROJECT_HEADER, legacy = { V2_PROJECT_HEADER, LEGACY_PROJECT_HEADER, V1_PROJECT_HEADER } },
            custom_summary = { value = PROJECT_SUMMARY, legacy = V2_PROJECT_SUMMARY },
            custom_avatar = { value = PROJECT_AVATAR },
            custom_hover_card = { value = PROJECT_SUMMARY, legacy = V2_PROJECT_SUMMARY },
            custom_list_header = { value = PROJECT_LIST_HEADER, legacy = V2_PROJECT_LIST_HEADER },
            custom_detail_footer = { value = PROJECT_DETAIL_FOOTER, legacy = V1_PROJECT_DETAIL_FOOTER, allow_empty = true },
            custom_mrql_result = { value = PROJECT_MRQL_RESULT, legacy = V2_PROJECT_MRQL_RESULT },
            custom_css = { value = presentation_css(), legacy = V2_EMBED_CSS, add_css = true },
        }
        if not repair_category_fields(project_category, project_fields) then return end
        if not repair_category_fields(epic_category, {
            section_config = { value = GROUP_SECTIONS, legacy = "{}" },
            custom_own_entities = { value = "[plugin:project-management:mini-board]" },
            meta_schema = { value = epic_schema_json(cfg) },
            custom_header = { value = EPIC_HEADER, legacy = { V2_EPIC_HEADER, LEGACY_EPIC_HEADER, V1_EPIC_HEADER } },
            custom_summary = { value = EPIC_SUMMARY, legacy = { V1_EPIC_SUMMARY, V2_EPIC_SUMMARY } },
            custom_avatar = { value = EPIC_AVATAR },
            custom_hover_card = { value = EPIC_HOVER_CARD, legacy = V2_EPIC_HOVER_CARD },
            custom_list_header = { value = EPIC_LIST_HEADER, legacy = V2_EPIC_LIST_HEADER },
            custom_detail_footer = { value = EPIC_DETAIL_FOOTER, legacy = V1_EPIC_DETAIL_FOOTER },
            custom_mrql_result = { value = EPIC_MRQL_RESULT, legacy = V2_EPIC_MRQL_RESULT },
            custom_css = { value = presentation_css(), legacy = V2_EMBED_CSS, add_css = true },
        }) then return end

        do
            local share_template_migration_key = KV_PREFIX .. "_share_templates_v1"
            local enable_share_templates = mah.kv.get(share_template_migration_key) ~= true
            local need = fields_to_patch(task_type, {
                section_config = { value = TASK_SECTIONS, legacy = "{}" },
                meta_schema = { value = task_schema_json(cfg) },
                custom_header = { value = TASK_HEADER, legacy = { V1_TASK_HEADER, V2_TASK_HEADER } },
                custom_summary = { value = TASK_SUMMARY, legacy = { LEGACY_TASK_SUMMARY, V1_TASK_SUMMARY } },
                custom_avatar = { value = TASK_AVATAR, legacy = V1_TASK_AVATAR },
                custom_hover_card = { value = TASK_HOVER_CARD },
                -- One migration enables the safe, restricted share rendering
                -- path. Subsequent setup runs respect an operator who turns it
                -- back off; plugin shortcodes remain suppressed on public shares.
                apply_templates_to_shares = { value = true, force = enable_share_templates },
                custom_list_header = { value = TASK_LIST_HEADER, legacy = V2_TASK_LIST_HEADER },
                custom_detail_footer = { value = TASK_DETAIL_FOOTER, legacy = V1_TASK_DETAIL_FOOTER },
                custom_mrql_result = { value = TASK_MRQL_RESULT },
                custom_css = { value = presentation_css(), legacy = V2_EMBED_CSS, add_css = true },
            })
            if need then
            local ok, uerr = mah.db.patch_note_type(task_type.id, need)
            if not ok then api_error(ctx, 500, "updating note type: " .. tostring(uerr)) return end
            end
            if enable_share_templates then mah.kv.set(share_template_migration_key, true) end
        end

        local tax_new = {
            project_category_id = project_category.id,
            epic_category_id = epic_category.id,
            task_type_id = task_type.id,
        }
        store_taxonomy(tax_new)
        tax = tax_new
        register_pm_block_types(tax_new)
        register_pm_actions(tax_new)
        mah.kv.set("cfg_presentation_v2", true)

        ctx.json({
            ok = true,
            project_category_id = project_category.id,
            epic_category_id = epic_category.id,
            task_type_id = task_type.id,
        })
    end)

    -- GET api/projects — project groups plus orphaned epics.
    mah.api("GET", "api/projects", function(ctx)
        local tax = cached_taxonomy()
        if not tax then
            ctx.json({ configured = false, projects = mah.json.array({}), unassigned = mah.json.array({}) })
            return
        end
        local projects, perr = list_projects()
        if perr then api_error(ctx, 500, perr) return end
        -- Groups whose category was deleted mid-flight can come back with a
        -- blank category; those cannot be project rows. They are filtered by
        -- category in SQL, so this guard is defensive only.
        local out = {}
        for _, p in ipairs(projects or {}) do
            out[#out + 1] = {
                id = p.id, name = p.name, status = meta_get(p, "status"),
                key = meta_get(p, "key"), target_date = meta_get(p, "target_date"),
            }
        end
        -- Orphaned epics: PM Epic groups whose owner is gone or is not a
        -- project. listing by owner would miss ownerless rows, so list all
        -- epics and filter in Lua (cap 1000 via paging is not needed: epics
        -- are few, and the taxonomy is required for the query to make sense).
        local all_epics, aerr = list_groups(tax.epic_category_id)
        if aerr then api_error(ctx, 500, aerr) return end
        local unassigned = {}
        for _, g in ipairs(all_epics) do
            local is_unassigned = false
            if not g.owner_id or g.owner_id == 0 then
                is_unassigned = true
            else
                local owner, oerr = mah.db.get_group(g.owner_id)
                if oerr or not owner or owner.category ~= TAXONOMY.project_category then
                    is_unassigned = true
                end
            end
            if is_unassigned then
                unassigned[#unassigned + 1] = { id = g.id, name = g.name, owner_id = g.owner_id }
            end
        end
        ctx.json({ configured = true, projects = mah.json.array(out), unassigned = mah.json.array(unassigned) })
    end)

    -- GET api/epics?project=N — epics of one project.
    mah.api("GET", "api/epics", function(ctx)
        local project_id = query_number(ctx, "project")
        if project_id == 0 then api_error(ctx, 400, "project is required") return end
        local project, perr = mah.db.get_group(project_id)
        if perr then api_error(ctx, 500, perr) return end
        if not project then api_error(ctx, 404, "project not found") return end
        local epics, eerr = list_epics(project_id)
        if eerr then api_error(ctx, 500, eerr) return end
        local out = {}
        for _, e in ipairs(epics or {}) do
            out[#out + 1] = {
                id = e.id, name = e.name, status = meta_get(e, "status"),
                target_date = meta_get(e, "target_date"),
            }
        end
        ctx.json({ project = { id = project.id, name = project.name }, epics = mah.json.array(out) })
    end)

    -- GET api/epic?epic=N — resolve a direct epic URL without guessing from
    -- the project picker (which intentionally contains only orphaned epics).
    mah.api("GET", "api/epic", function(ctx)
        local epic_id = query_number(ctx, "epic")
        if epic_id == 0 then api_error(ctx, 400, "epic is required") return end
        local epic, eerr = mah.db.get_group(epic_id)
        if eerr then api_error(ctx, 500, eerr) return end
        if not epic then api_error(ctx, 404, "epic not found") return end
        if epic.category ~= TAXONOMY.epic_category then
            api_error(ctx, 400, "group is not a PM Epic")
            return
        end
        local result = { id = epic.id, name = epic.name, owner_id = epic.owner_id }
        if epic.owner_id and epic.owner_id > 0 then
            local project, _ = mah.db.get_group(epic.owner_id)
            if project and project.category == TAXONOMY.project_category then
                result.project = { id = project.id, name = project.name }
            end
        end
        ctx.json({ epic = result })
    end)

    -- GET api/stats?project=N[&epic=N][&now=...][&week_start=...][&week_end=...]
    mah.api("GET", "api/stats", function(ctx)
        local project_id = query_number(ctx, "project")
        local epic_id = query_number(ctx, "epic")
        if project_id == 0 and epic_id == 0 then
            api_error(ctx, 400, "project or epic is required")
            return
        end
        local tax = cached_taxonomy()
        if not tax then api_error(ctx, 400, "plugin not set up yet") return end
        local container, cerr = container_for(project_id, epic_id)
        if cerr then api_error(ctx, 400, cerr) return end

        local by_status, err1 = status_counts(container, nil, tax)
        if err1 then api_error(ctx, 500, err1) return end

        -- Priority distribution.
        local conds = {}
        local by_priority = {}
        do
            local query = string.format("type = note AND noteType = %d AND %s GROUP BY meta.priority COUNT()",
                tax.task_type_id, task_scope_clause(container))
            local res, qerr = mah.db.mrql_query(query, { limit = 100 })
            if qerr then api_error(ctx, 500, qerr) return end
            local total = 0
            if res and res.rows then
                for _, row in ipairs(res.rows) do
                    local priority = row["meta.priority"] or "none"
                    local count = tonumber(row["count"]) or 0
                    by_priority[priority] = count
                    total = total + count
                end
            end
            by_priority.total = total
        end

        -- Overdue: endDate in the past and not in the done status.
        local cfg = resolved_config()
        local not_done = string.format("meta.status != %q", cfg.done_status)
        if cfg.default_status ~= cfg.done_status then
            not_done = "(" .. not_done .. " OR meta.status IS EMPTY)"
        end
        local overdue = 0
        local now = ctx.query and (ctx.query.now or (type(ctx.query.now) == "table" and ctx.query.now[1])) or nil
        local date_filter = "endDate < "
        if now and now ~= "" then
            local bound, err = query_datetime(now)
            if not bound then api_error(ctx, 400, err) return end
            date_filter = date_filter .. string.format("%q", bound)
        else
            date_filter = date_filter .. "NOW()"
        end
        do
            local c = status_counts(container, { not_done, date_filter }, tax)
            if not c then
                -- fallthrough: no count for overdue
            else
                overdue = c.total
            end
        end

        -- Due this week (requires client-supplied week bounds, in the same
        -- instant format the rest of the app stores).
        local due_this_week = nil
        local week_start = ctx.query and ctx.query.week_start or nil
        local week_end = ctx.query and ctx.query.week_end or nil
        if week_start and week_end and week_start ~= "" and week_end ~= "" then
            local start_bound, start_err = query_datetime(week_start)
            local end_bound, end_err = query_datetime(week_end)
            if not start_bound or not end_bound then api_error(ctx, 400, start_err or end_err) return end
            local c = status_counts(container, {
                not_done,
                string.format("endDate >= %q", start_bound),
                string.format("endDate < %q", end_bound),
            }, tax)
            if c then due_this_week = c.total end
        end

        ctx.json({
            project = project_id,
            epic = epic_id,
            total = by_status.total,
            by_status = by_status,
            by_priority = by_priority,
            overdue = overdue,
            due_this_week = due_this_week,
        })
    end)

    -- POST api/epic/create { project_id, name }
    mah.api("POST", "api/epic/create", function(ctx)
        local body, err = parse_body(ctx)
        if not body then api_error(ctx, 400, err) return end
        local project_id = tonumber(body.project_id) or 0
        local name = trim(body.name)
        if project_id <= 0 then api_error(ctx, 400, "project_id is required") return end
        if name == "" then api_error(ctx, 400, "name is required") return end
        local tax = cached_taxonomy()
        if not tax then api_error(ctx, 400, "plugin not set up yet") return end
        local project, perr = mah.db.get_group(project_id)
        if perr then api_error(ctx, 500, perr) return end
        if not project or project.category ~= TAXONOMY.project_category then
            api_error(ctx, 400, "project_id must be a PM Project group")
            return
        end
        local cfg = resolved_config()
        local epic, cerr = mah.db.create_group({
            name = name,
            owner_id = project_id,
            category_id = tax.epic_category_id,
            meta = meta_string({ status = cfg.default_status }),
        })
        if not epic then api_error(ctx, 400, "creating epic: " .. tostring(cerr)) return end
        ctx.json({ id = epic.id, name = name })
    end)

    -- POST api/task/create { owner_id, name, status?, priority?, due?, start?, description? }
    register_task_api("create", function(ctx)
        local body, err = parse_body(ctx)
        if not body then api_error(ctx, 400, err) return end
        local owner_id = tonumber(body.owner_id) or 0
        local name = trim(body.name)
        if name == "" then api_error(ctx, 400, "name is required") return end
        local tax = cached_taxonomy()
        if not tax then api_error(ctx, 400, "plugin not set up yet") return end
        local cfg = resolved_config()

        -- Owner must be a PM Project or PM Epic group.
        local owner, oerr = mah.db.get_group(owner_id)
        if oerr then api_error(ctx, 500, oerr) return end
        if not owner or (owner.category ~= TAXONOMY.project_category and owner.category ~= TAXONOMY.epic_category) then
            api_error(ctx, 400, "owner_id must be a PM Project or PM Epic group")
            return
        end

        local status = body.status
        if status == nil or status == "" then status = cfg.default_status end
        if not cfg.status_name_set[status] then
            api_error(ctx, 400, "unknown status: " .. tostring(status))
            return
        end
        local priority = body.priority
        if priority ~= nil and priority ~= "" then
            local known = false
            for _, p in ipairs(cfg.priorities) do if p.name == priority then known = true break end end
            if not known then api_error(ctx, 400, "unknown priority: " .. tostring(priority)) return end
        end

        local due, derr = normalize_datetime(body.due)
        if derr then api_error(ctx, 400, "due: " .. derr) return end
        local start, serr = normalize_datetime(body.start)
        if serr then api_error(ctx, 400, "start: " .. serr) return end

        -- Append at the end of the destination status column. The tail read
        -- happens inside the transaction: on SQLite writers serialize, so the
        -- order is distinct across concurrent creates in one process.
        local created
        local ok_tx, txerr = attempt_loop(function()
            local container = owner_container(owner)
            local attempt_result
            local ok_t, terr_t = mah.db.transaction(function()
                lock_columns({ col_entry(container, status) })
                local tail, terr = column_tail_order(container, tax, status)
                if terr then error("reading column: " .. terr) end
                local order = position_between(tail, nil)
                local meta = { status = status, order = order }
                if priority then meta.priority = priority end
                local note, nerr = mah.db.create_note({
                    name = name,
                    description = body.description or "",
                    owner_id = owner_id,
                    note_type_id = tax.task_type_id,
                    start_date = start,
                    end_date = due,
                    meta = meta_string(meta),
                })
                if not note then error("creating note: " .. tostring(nerr)) end
                attempt_result = note
            end)
            if not ok_t then return false, terr_t end
            created = attempt_result
            return true, nil, attempt_result
        end)
        if not ok_tx then api_error(ctx, 400, tostring(txerr)) return end
        ctx.json(full_task(created.id))
    end)

    -- POST api/task/update { id, name?, status?, priority?, owner_id?/epic_id?, due?, start?, description? }
    -- Partial update. Every date write is validated here — the host silently
    -- NULLs a date it cannot parse, and this plugin refuses instead.
    register_task_api("update", function(ctx)
        local body, err = parse_body(ctx)
        if not body then api_error(ctx, 400, err) return end
        local id = tonumber(body.id) or 0
        if id <= 0 then api_error(ctx, 400, "id is required") return end
        local tax = cached_taxonomy()
        if not tax then api_error(ctx, 400, "plugin not set up yet") return end
        local cfg = resolved_config()

        local pre, perr = mah.db.get_note(id)
        if perr then api_error(ctx, 500, perr) return end
        if not pre then api_error(ctx, 404, "task not found") return end
        if pre.note_type ~= TAXONOMY.task_type then
            api_error(ctx, 400, "not a PM Task note")
            return
        end

        local patch_opts = {}

        if body.name ~= nil then
            local nm = trim(body.name)
            if nm == "" then api_error(ctx, 400, "name cannot be empty") return end
            patch_opts.name = nm
        end
        if body.description ~= nil then patch_opts.description = tostring(body.description) end

        if body.status ~= nil and body.status ~= "" then
            if not cfg.status_name_set[body.status] then
                api_error(ctx, 400, "unknown status: " .. tostring(body.status))
                return
            end
            patch_opts.status = body.status
        end

        if body.priority ~= nil then
            if body.priority == "" then
                -- explicit clear is handled through the meta rewrite below
                patch_opts.clear_priority = true
            else
                local known = false
                for _, p in ipairs(cfg.priorities) do if p.name == body.priority then known = true break end end
                if not known then api_error(ctx, 400, "unknown priority: " .. tostring(body.priority)) return end
                patch_opts.priority = body.priority
            end
        end

        if body.due ~= nil then
            local ndue, derr = normalize_datetime(body.due)
            if derr then api_error(ctx, 400, "due: " .. derr) return end
            patch_opts.end_date = ndue or ""
        end
        if body.start ~= nil then
            local nstart, serr = normalize_datetime(body.start)
            if serr then api_error(ctx, 400, "start: " .. serr) return end
            patch_opts.start_date = nstart or ""
        end

        local new_owner = tonumber(body.owner_id or body.epic_id) or 0
        if new_owner > 0 then
            local ng, ng_err = mah.db.get_group(new_owner)
            if ng_err then api_error(ctx, 500, ng_err) return end
            if not ng or (ng.category ~= TAXONOMY.project_category and ng.category ~= TAXONOMY.epic_category) then
                api_error(ctx, 400, "owner_id must be a PM Project or PM Epic group")
                return
            end
            patch_opts.owner_id = new_owner
        end

        -- The task's meta (status/priority/order) is not a patch_note key: it
        -- is rewritten in full here, merged over the stored note. Decision
        -- reads run per attempt, before the transaction; inside it the first
        -- statements are writes — the plan's column locks, then the task's own
        -- row (columns-then-task everywhere; no column is ever acquired after
        -- the task lock). If the locked snapshot disagrees with the plan, the
        -- attempt raises RETRY_MARKER and attempt_loop re-plans from fresh
        -- reads.
        local ok_tx, txerr, updated = attempt_loop(function()
            local pre_task, terr = mah.db.get_note(id)
            if terr then return false, terr end
            if not pre_task then return false, "task not found" end
            if pre_task.note_type ~= TAXONOMY.task_type then
                return false, "not a PM Task note"
            end
            local pre_meta, pre_merr = meta_object(pre_task.meta)
            if not pre_meta then return false, "task meta is not a JSON object" end
            local current_owner_pre = pre_task.owner_id or 0
            local new_owner_id = patch_opts.owner_id or current_owner_pre

            -- Source and destination containers/statuses for this attempt.
            local src_container = nil
            local src_status = pre_meta.status or cfg.default_status
            if current_owner_pre > 0 then
                local src_g, sg_err = mah.db.get_group(current_owner_pre)
                if sg_err then return false, sg_err end
                if src_g and (src_g.category == TAXONOMY.project_category or src_g.category == TAXONOMY.epic_category) then
                    src_container = owner_container(src_g)
                end
            end
            local dst_container = src_container
            local dst_status = patch_opts.status or src_status
            local owner_moved = false
            if new_owner_id ~= current_owner_pre then
                local ng2, ng2err = mah.db.get_group(new_owner_id)
                if ng2err then return false, ng2err end
                if not ng2 then return false, "owner group not found" end
                dst_container = owner_container(ng2)
                owner_moved = true
            end

            local attempt_result
            local ok_t, terr_t = mah.db.transaction(function()
                -- First statements: writes — column locks in canonical order
                -- (current column, target status, destination container when
                -- it differs), then the task's own row.
                local locks = {}
                if src_container then
                    locks[#locks + 1] = col_entry(src_container, src_status or NO_STATUS_LOCK_KEY)
                else
                    locks[#locks + 1] = col_entry(NO_CONTAINER, NO_STATUS_LOCK_KEY)
                end
                if patch_opts.status and src_container and patch_opts.status ~= src_status then
                    locks[#locks + 1] = col_entry(src_container, patch_opts.status)
                end
                if owner_moved and dst_container
                    and not (src_container and container_key(src_container) == container_key(dst_container)) then
                    locks[#locks + 1] = col_entry(dst_container, dst_status)
                end
                lock_columns(locks)
                mah.kv.set("task:" .. tostring(id), "1")

                -- Authoritative re-read. Divergence from this attempt's plan
                -- means the locks were for the wrong state: abort and re-plan.
                local current, cerr = mah.db.get_note(id)
                if cerr or not current then error("reading note: " .. tostring(cerr)) end
                local cur_owner = current.owner_id or 0
                if cur_owner ~= current_owner_pre then
                    error(RETRY_MARKER)
                end
                if patch_opts.status and (current.meta == nil or meta_object(current.meta) == nil) then
                    error("reading note meta: invalid json")
                end
                local meta, merr = meta_object(current.meta)
                if not meta then error("reading note meta: " .. tostring(merr or "invalid json")) end
                local cur_status = meta.status or cfg.default_status
                if cur_status ~= (pre_meta.status or cfg.default_status) then
                    -- The committed status differs from what this attempt
                    -- planned against — whether or not this update changes it.
                    -- An owner-only update that lands on a task whose status
                    -- changed concurrently would compute its destination tail
                    -- against the wrong status column. Abort and re-plan.
                    error(RETRY_MARKER)
                end
                local eff_status = patch_opts.status or cur_status
                local eff_owner = patch_opts.owner_id or cur_owner

                if eff_status ~= cur_status then
                    if eff_owner > 0 then
                        -- Status change inside a PM column: append at the
                        -- destination column's tail (destination lock already
                        -- held up front).
                        meta.status = eff_status
                        local tail, terr5 = column_tail_order(dst_container, tax, eff_status)
                        if terr5 then error("reading column: " .. terr5) end
                        meta.order = position_between(tail, nil)
                    else
                        -- No container to order in (ownerless task).
                        meta.status = eff_status
                    end
                elseif eff_owner ~= cur_owner and eff_owner > 0 and cur_owner > 0 then
                    -- Owner moved to another container: re-seat the order key
                    -- at the destination column's tail (a key unique in the
                    -- source project may already exist there). The destination
                    -- column lock was taken up front.
                    local sg3, serr3 = mah.db.get_group(cur_owner)
                    if serr3 or not sg3 then error("owner group not found") end
                    local dg3, derr3 = mah.db.get_group(eff_owner)
                    if derr3 or not dg3 then error("owner group not found") end
                    if container_key(owner_container(sg3)) ~= container_key(owner_container(dg3)) then
                        local tail, terr6 = column_tail_order(dst_container, tax, eff_status)
                        if terr6 then error("reading column: " .. terr6) end
                        meta.order = position_between(tail, nil)
                    end
                end

                if patch_opts.priority then meta.priority = patch_opts.priority end
                if patch_opts.clear_priority then meta.priority = nil end

                local note_opts = { meta = meta_string(meta) }
                if patch_opts.name then note_opts.name = patch_opts.name end
                if patch_opts.description ~= nil then note_opts.description = patch_opts.description end
                if patch_opts.owner_id then note_opts.owner_id = patch_opts.owner_id end
                if patch_opts.start_date ~= nil then note_opts.start_date = patch_opts.start_date end
                if patch_opts.end_date ~= nil then note_opts.end_date = patch_opts.end_date end

                local res, perr2 = mah.db.patch_note(id, note_opts)
                if not res then error("saving task: " .. tostring(perr2)) end
                attempt_result = res
            end)
            if not ok_t then return false, terr_t end
            return true, nil, attempt_result
        end)
        if not ok_tx then api_error(ctx, 400, tostring(txerr)) return end
        ctx.json(full_task(updated.id))
    end)


    register_task_api("move", function(ctx)
        local body, err = parse_body(ctx)
        if not body then api_error(ctx, 400, err) return end
        local id = tonumber(body.id) or 0
        local status = body.status
        if id <= 0 then api_error(ctx, 400, "id is required") return end
        if status == nil or status == "" then api_error(ctx, 400, "status is required") return end
        local tax = cached_taxonomy()
        if not tax then api_error(ctx, 400, "plugin not set up yet") return end
        local cfg = resolved_config()
        if not cfg.status_name_set[status] then
            api_error(ctx, 400, "unknown status: " .. tostring(status))
            return
        end
        local before_id = tonumber(body.before_id) or 0
        local after_id = tonumber(body.after_id) or 0

        local ok_tx, txerr, updated = attempt_loop(function()
            -- Pre-reads are per attempt: a retry re-plans from fresh data.
            local pre_task, terr = mah.db.get_note(id)
            if terr then return false, terr end
            if not pre_task then return false, "task not found" end
            if pre_task.note_type ~= TAXONOMY.task_type then
                return false, "only PM Task notes can be moved"
            end
            local pre_owner_id = pre_task.owner_id or 0
            local pre_group, gerr = mah.db.get_group(pre_owner_id)
            if gerr then return false, gerr end
            if not pre_group then return false, "task has no PM owner group" end
            if pre_group.category ~= TAXONOMY.project_category and pre_group.category ~= TAXONOMY.epic_category then
                return false, "task is not owned by a PM Project or PM Epic group"
            end
            local pre_container = owner_container(pre_group)
            local pre_meta_r, pre_meta_err = meta_object(pre_task.meta)
            if not pre_meta_r then return false, "task meta unreadable: " .. tostring(pre_meta_err) end
            local pre_task_status = pre_meta_r.status or cfg.default_status

            local attempt_updated
            local ok_t, terr_t = mah.db.transaction(function()
                -- First statements: writes only — the source column (where the
                -- task currently sits, per this attempt's pre-read), the
                -- destination column, then the task's own row. Locking the
                -- source column too makes rebalances and departures of a
                -- column mutually exclusive: a member cannot leave a column
                -- while it is being rebalanced (departures hold the source
                -- column lock), and two opposing collision-triggering moves
                -- serialize instead of deadlocking. Columns-then-task
                -- everywhere; no column is acquired after the task lock —
                -- divergence aborts the attempt and re-plans instead.
                lock_columns({
                    col_entry(pre_container, pre_task_status),
                    col_entry(pre_container, status),
                })
                mah.kv.set("task:" .. tostring(id), "1")

                local locked, lerr = mah.db.get_note(id)
                if lerr or not locked then error("task not found: " .. tostring(lerr)) end
                -- The locked snapshot is authoritative for this attempt. If
                -- the task left the container or status the pre-read planned
                -- for, the column locks were for the wrong state: abort and
                -- retry with a fresh plan.
                local actual_owner = locked.owner_id or 0
                if actual_owner ~= pre_owner_id then
                    error(RETRY_MARKER)
                end
                local meta, merr = meta_object(locked.meta)
                if not meta then error("task meta unreadable: " .. tostring(merr)) end
                if (meta.status or cfg.default_status) ~= pre_task_status then
                    error(RETRY_MARKER)
                end

                -- Neighbour validation + order derivation, from the locked
                -- snapshot and this attempt's container.
                local function neighbour_order(note_id, side)
                    if not note_id or note_id <= 0 then return nil end
                    local n, nerr = mah.db.get_note(note_id)
                    if nerr or not n then error(side .. " task not found") end
                    if n.note_type ~= TAXONOMY.task_type then
                        error(side .. " task is not a PM Task note")
                    end
                    local m, m2err = meta_object(n.meta)
                    if not m then error(side .. " task meta unreadable") end
                    if (m.status or cfg.default_status) ~= status then
                        error(side .. " task is not in the destination status")
                    end
                    local og2, o2err = mah.db.get_group(n.owner_id or 0)
                    if o2err or not og2 then error(side .. " task has no PM owner group") end
                    if container_key(owner_container(og2)) ~= container_key(pre_container) then
                        error(side .. " task is not in the same project")
                    end
                    return m.order
                end

                local before_order = neighbour_order(before_id, "before")
                local after_order = neighbour_order(after_id, "after")

                local order
                if before_id <= 0 and after_id <= 0 then
                    -- Append at the destination column's true tail (under the
                    -- column lock), so a column of any depth is handled
                    -- without the client paging to find its end.
                    local tail, terr3 = column_tail_order(pre_container, tax, status)
                    if terr3 then error("reading column: " .. terr3) end
                    order = position_between(tail, nil)
                else
                    order = position_between(before_order, after_order)
                end
                local collision = (after_order ~= nil and order >= after_order)
                    or (before_order ~= nil and order <= before_order)
                -- A key can also be taken without being one of the boundaries.
                if not collision then
                    local taken, terr4 = order_taken(pre_container, status, order, id)
                    if terr4 then error("reading column: " .. terr4) end
                    collision = taken
                end

                local function patch_task(note_id, new_meta)
                    local res, perr = mah.db.patch_note(note_id, { meta = meta_string(new_meta) })
                    if not res then error("saving task: " .. tostring(perr)) end
                    return res
                end

                if not collision then
                    meta.status = status
                    meta.order = order
                    attempt_updated = patch_task(id, meta)
                    return
                end

                -- Collision: rebalance the destination column. Fetch its
                -- members in order (the move itself is not committed yet, so
                -- this task is only in the list when it was already here),
                -- drop this task, insert it between its neighbours, and write
                -- fresh even positions to every member that still belongs.
                local items, rerr = mrql_flat_tasks(pre_container, {
                    status_filter_clause(status, cfg),
                }, { limit = 1000, order_by = "meta.order ASC" })
                if rerr then error("reading column: " .. rerr) end
                local seq = {}
                for _, it in ipairs(items or {}) do
                    if it.id ~= id then seq[#seq + 1] = it.id end
                end
                local function find_index(sid)
                    for idx, s2 in ipairs(seq) do if s2 == sid then return idx end end
                    return nil
                end
                local insert_at
                if before_id > 0 and after_id > 0 then
                    local bi, ai = find_index(before_id), find_index(after_id)
                    if bi and ai and bi < ai then insert_at = bi + 1
                    elseif bi then insert_at = bi + 1
                    elseif ai then insert_at = ai
                    else insert_at = #seq + 1 end
                elseif before_id > 0 then
                    insert_at = (find_index(before_id) or #seq) + 1
                elseif after_id > 0 then
                    insert_at = find_index(after_id) or 1
                else
                    insert_at = #seq + 1
                end
                insert_at = math.min(insert_at, #seq + 1)
                table.insert(seq, insert_at, id)

                local positions = even_positions(#seq)
                for idx, sid in ipairs(seq) do
                    -- Member lock before its snapshot (the column lock is
                    -- already held; no column is taken after this point).
                    mah.kv.set("task:" .. tostring(sid), "1")
                    local n, nerr2 = mah.db.get_note(sid)
                    if nerr2 or not n then error("reading task during rebalance") end
                    local m, merr3 = meta_object(n.meta)
                    if not m then error("task meta unreadable during rebalance") end
                    if (m.status or cfg.default_status) ~= status and sid ~= id then
                        -- This member left the column between the fetch and
                        -- its lock (its own move owns the task row now). It is
                        -- no longer this column's to renumber.
                    else
                        if sid == id then
                            m.status = status
                            m.order = positions[idx]
                        else
                            m.order = positions[idx]
                        end
                        local res = patch_task(sid, m)
                        if sid == id then attempt_updated = res end
                    end
                end
                if not attempt_updated then
                    -- The moved task was not a member of this column (it came
                    -- from another status): write it at its planned slot.
                    meta.status = status
                    meta.order = positions[insert_at]
                    attempt_updated = patch_task(id, meta)
                end
            end)
            if not ok_t then return false, terr_t end
            return true, nil, attempt_updated
        end)
        if not ok_tx then api_error(ctx, 400, tostring(txerr)) return end
        ctx.json(full_task(updated.id))
    end)

end
