-- Meta Editors plugin for mahresources
-- Provides 17 shortcodes for inline editing of entity meta JSON fields.
-- Usage in templates: [plugin:meta-editors:shortcode-name path="meta.field" ...]
-- Each shortcode renders an Alpine.js component that saves via the editMeta API.

plugin = {
    name = "meta-editors",
    version = "1.0",
    description = "Provides 17 interactive shortcodes for editing entity meta fields inline.\n"
        .. "\n"
        .. "[plugin:meta-editors:slider] — Numeric slider input. Attrs: path, min, max, step, label.\n"
        .. "[plugin:meta-editors:stepper] — Increment/decrement buttons. Attrs: path, min, max, step.\n"
        .. "[plugin:meta-editors:star-rating] — Clickable star rating. Attrs: path, max.\n"
        .. "[plugin:meta-editors:toggle] — Boolean toggle switch. Attrs: path, label.\n"
        .. "[plugin:meta-editors:multi-select] — Toggleable pill chips. Attrs: path, options, labels.\n"
        .. "[plugin:meta-editors:button-group] — Single-select button group. Attrs: path, options, labels.\n"
        .. "[plugin:meta-editors:color-picker] — Color swatch palette. Attrs: path, colors.\n"
        .. "[plugin:meta-editors:tags-input] — Add/remove string tags. Attrs: path, placeholder.\n"
        .. "[plugin:meta-editors:textarea] — Auto-saving textarea. Attrs: path, rows, placeholder.\n"
        .. "[plugin:meta-editors:date-picker] — Date input. Attrs: path, label.\n"
        .. "[plugin:meta-editors:date-range] — Start/end date pair. Attrs: path, start-label, end-label.\n"
        .. "[plugin:meta-editors:status-badge] — Cycling status badge. Attrs: path, options, colors, labels.\n"
        .. "[plugin:meta-editors:progress-input] — Clickable progress bar. Attrs: path, label.\n"
        .. "[plugin:meta-editors:key-value] — Key-value pair editor. Attrs: path.\n"
        .. "[plugin:meta-editors:checklist] — Checklist with checkboxes. Attrs: path.\n"
        .. "[plugin:meta-editors:url-input] — URL input with validation. Attrs: path, placeholder, label.\n"
        .. "[plugin:meta-editors:markdown] — Monospace textarea. Attrs: path, rows, placeholder.",
}

-- ---------------------------------------------------------------------------
-- Helpers
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
    if type(tbl) ~= "table" or not path or path == "" then return nil end
    local current = tbl
    for segment in path:gmatch("[^%.]+") do
        if type(current) ~= "table" then return nil end
        current = current[segment]
    end
    return current
end

--- Convert a Lua value to a JSON string safe for embedding in HTML attributes.
local function json_value(val)
    if val == nil then return "null" end
    local t = type(val)
    if t == "boolean" then return val and "true" or "false" end
    if t == "number" then return tostring(val) end
    if t == "string" then return mah.json.encode(val) end
    if t == "table" then return mah.json.encode(val) end
    return "null"
end

--- Generate a short random ID for unique element references.
local function make_id()
    local chars = "abcdefghijklmnopqrstuvwxyz0123456789"
    local parts = {}
    for _ = 1, 8 do
        local i = math.random(1, #chars)
        parts[#parts + 1] = chars:sub(i, i)
    end
    return "me_" .. table.concat(parts)
end

--- Parse a comma-separated string into an ordered array of trimmed strings.
local function parse_csv(str)
    local arr = {}
    if not str or str == "" then return arr end
    for item in str:gmatch("[^,]+") do
        arr[#arr + 1] = item:match("^%s*(.-)%s*$")
    end
    return arr
end

--- Build the x-data JSON object for a shortcode.
--- Returns the string to place inside x-data="...".
local function build_xdata(ctx, initial_value)
    return string.format(
        '{ val: %s, saving: false, saved: false, async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(initial_value),
        html_escape(ctx.entity_type),
        ctx.entity_id,
        html_escape(ctx.attrs.path)
    )
end

--- Build save status indicator spans.
local function save_indicators()
    return '<span x-show="saving" class="text-xs text-stone-400 ml-1">Saving...</span>'
        .. '<span x-show="saved" x-transition class="text-xs text-green-600 ml-1">&#10003;</span>'
end

--- Return an error message if the path attribute is missing.
local function require_path(name, ctx)
    if not ctx.attrs or not ctx.attrs.path or ctx.attrs.path == "" then
        return string.format(
            '<p class="text-sm text-red-500">[%s]: &quot;path&quot; attribute is required</p>',
            html_escape(name)
        )
    end
    return nil
end

-- ---------------------------------------------------------------------------
-- Page-bottom injection: save helper script
-- ---------------------------------------------------------------------------

local function render_save_helper(ctx)
    return [[<script>
if (!window.__metaEditorsSave) {
  window.__metaEditorsSave = async function(entityType, entityId, path, value) {
    const form = new URLSearchParams();
    form.set('path', path);
    form.set('value', JSON.stringify(value));
    const resp = await fetch('/v1/' + entityType + '/editMeta?id=' + entityId, {
      method: 'POST', body: form,
      headers: {'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json'}
    });
    if (!resp.ok) throw new Error(await resp.text());
    return resp.json();
  };
}
</script>]]
end

-- ---------------------------------------------------------------------------
-- 1. slider
-- ---------------------------------------------------------------------------

local function render_slider(ctx)
    local err = require_path("slider", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = 0 end
    local min = ctx.attrs.min or "0"
    local max = ctx.attrs.max or "100"
    local step = ctx.attrs.step or "1"
    local label = ctx.attrs.label

    local xdata = build_xdata(ctx, tonumber(val) or 0)
    local title_text = string.format("Edit: %s (range %s-%s)", ctx.attrs.path, min, max)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="flex items-center gap-2 text-sm py-1.5 max-w-full">', html_escape(title_text), html_escape(xdata))
    if label then
        parts[#parts + 1] = string.format('<span class="text-stone-600 shrink-0">%s</span>', html_escape(label))
    end
    parts[#parts + 1] = '<span x-text="val" class="font-mono font-bold w-8 text-right shrink-0"></span>'
    parts[#parts + 1] = string.format(
        '<input type="range" :min="%s" :max="%s" :step="%s" x-model.number="val" @change="save(val)" class="flex-1 min-w-0 accent-amber-700">',
        html_escape(min), html_escape(max), html_escape(step)
    )
    parts[#parts + 1] = string.format('<span class="text-stone-400 text-xs">/ %s</span>', html_escape(max))
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 2. stepper
-- ---------------------------------------------------------------------------

local function render_stepper(ctx)
    local err = require_path("stepper", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = 0 end
    local min = ctx.attrs.min or "0"
    local max = ctx.attrs.max or "100"
    local step = ctx.attrs.step or "1"

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, min: %s, max: %s, step: %s, async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(tonumber(val) or 0),
        html_escape(min), html_escape(max), html_escape(step),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (range %s-%s)", ctx.attrs.path, min, max)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex items-center gap-1 text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<button @click="val = Math.max(min, val - step); save(val)" :disabled="val <= min" '
        .. 'class="w-7 h-7 rounded border border-stone-300 text-stone-600 hover:bg-stone-100 disabled:opacity-30">'
        .. '&#8722;</button>'
    parts[#parts + 1] = '<span x-text="val" class="font-mono font-bold w-10 text-center"></span>'
    parts[#parts + 1] = '<button @click="val = Math.min(max, val + step); save(val)" :disabled="val >= max" '
        .. 'class="w-7 h-7 rounded border border-stone-300 text-stone-600 hover:bg-stone-100 disabled:opacity-30">'
        .. '+</button>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 3. star-rating
-- ---------------------------------------------------------------------------

local function render_star_rating(ctx)
    local err = require_path("star-rating", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = 0 end
    local max = ctx.attrs.max or "5"

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, hover: 0, max: %s, async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(tonumber(val) or 0), html_escape(max),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local star_svg = '<svg class="w-5 h-5" viewBox="0 0 20 20" '
        .. ':fill="i <= (hover || val) ? \'#f59e0b\' : \'none\'" '
        .. ':stroke="i <= (hover || val) ? \'#f59e0b\' : \'#d6d3d1\'" stroke-width="1.5">'
        .. '<path d="M10 1l2.39 4.84L17.3 6.7l-3.65 3.56.86 5.02L10 13.07l-4.51 2.21.86-5.02L2.7 6.7l4.91-.86L10 1z"/>'
        .. '</svg>'

    local title_text = string.format("Edit: %s (1-%s stars)", ctx.attrs.path, max)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex items-center gap-0.5 py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="i in max" :key="i">'
    parts[#parts + 1] = '<button @click="save(i === val ? 0 : i)" @mouseenter="hover = i" @mouseleave="hover = 0" '
        .. 'class="p-0 focus:outline-none" :aria-label="\'Rate \' + i + \' of \' + max">'
    parts[#parts + 1] = star_svg
    parts[#parts + 1] = '</button>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 4. toggle
-- ---------------------------------------------------------------------------

local function render_toggle(ctx)
    local err = require_path("toggle", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = false end
    local label = ctx.attrs.label

    local xdata = build_xdata(ctx, val)

    local title_text = string.format("Edit: %s (toggle)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex items-center gap-2 text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    if label then
        parts[#parts + 1] = string.format('<span class="text-stone-600">%s</span>', html_escape(label))
    end
    parts[#parts + 1] = '<button @click="save(!val)" role="switch" :aria-checked="val" '
        .. ':class="val ? \'bg-amber-600\' : \'bg-stone-300\'" '
        .. 'class="relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500 focus:ring-offset-1">'
        .. '<span :class="val ? \'translate-x-6\' : \'translate-x-1\'" '
        .. 'class="inline-block h-4 w-4 rounded-full bg-white transition-transform shadow"></span>'
        .. '</button>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 5. multi-select
-- ---------------------------------------------------------------------------

local function render_multi_select(ctx)
    local err = require_path("multi-select", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = {} end
    local options = parse_csv(ctx.attrs.options or "")
    local labels = parse_csv(ctx.attrs.labels or "")

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, options: %s, labels: %s, '
        .. 'toggle(opt) { let a = [...this.val]; let i = a.indexOf(opt); if(i>=0) a.splice(i,1); else a.push(opt); this.save(a); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val), json_value(options), json_value(labels),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (multi-select)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="flex flex-wrap gap-1 py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="(opt, idx) in options" :key="opt">'
    parts[#parts + 1] = '<button @click="toggle(opt)" '
        .. ':class="val.includes(opt) ? \'bg-amber-100 text-amber-800 border-amber-300\' : \'bg-white text-stone-600 border-stone-300\'" '
        .. 'class="px-2 py-0.5 rounded-full text-xs font-medium border transition-colors" '
        .. 'x-text="labels[idx] || opt"></button>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 6. button-group
-- ---------------------------------------------------------------------------

local function render_button_group(ctx)
    local err = require_path("button-group", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local options = parse_csv(ctx.attrs.options or "")
    local labels = parse_csv(ctx.attrs.labels or "")

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, options: %s, labels: %s, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val), json_value(options), json_value(labels),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (select one)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex rounded-md shadow-sm text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="(opt, idx) in options" :key="opt">'
    parts[#parts + 1] = '<button @click="save(opt)" '
        .. ':class="val === opt ? \'bg-amber-700 text-white border-amber-700 z-10\' : \'bg-white text-stone-700 border-stone-300 hover:bg-stone-50\'" '
        .. 'class="px-3 py-1 border font-medium first:rounded-l-md last:rounded-r-md -ml-px first:ml-0 transition-colors focus:outline-none focus:ring-2 focus:ring-amber-500 focus:z-10" '
        .. 'x-text="labels[idx] || opt"></button>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 7. color-picker
-- ---------------------------------------------------------------------------

local DEFAULT_COLORS = "#ef4444,#f59e0b,#22c55e,#3b82f6,#8b5cf6,#ec4899,#6b7280,#000000"

local function render_color_picker(ctx)
    local err = require_path("color-picker", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local colors = parse_csv(ctx.attrs.colors or DEFAULT_COLORS)

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, colors: %s, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val), json_value(colors),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (color)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="flex flex-wrap gap-1 py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="c in colors" :key="c">'
    parts[#parts + 1] = '<button @click="save(c)" '
        .. ':style="\'background-color:\' + c" '
        .. ':class="val === c ? \'ring-2 ring-offset-1 ring-amber-500\' : \'\'" '
        .. 'class="w-6 h-6 rounded-full border border-stone-200 flex items-center justify-center transition-shadow focus:outline-none" '
        .. ':aria-label="\'Select color \' + c">'
        .. '<svg x-show="val === c" class="w-3 h-3 text-white" fill="none" stroke="currentColor" stroke-width="3" viewBox="0 0 24 24"><path d="M5 13l4 4L19 7"/></svg>'
        .. '</button>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 8. tags-input
-- ---------------------------------------------------------------------------

local function render_tags_input(ctx)
    local err = require_path("tags-input", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = {} end
    local placeholder = ctx.attrs.placeholder or "Add tag..."

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, input: \'\',' .. ' '
        .. 'add() { let t = this.input.trim(); if (t && !this.val.includes(t)) { let a = [...this.val, t]; this.save(a); } this.input = \'\'; }, '
        .. 'remove(i) { let a = [...this.val]; a.splice(i, 1); this.save(a); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (tags)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<div class="flex flex-wrap gap-1 mb-1">'
    parts[#parts + 1] = '<template x-for="(tag, i) in val" :key="i">'
    parts[#parts + 1] = '<span class="inline-flex items-center gap-0.5 px-2 py-0.5 bg-stone-100 text-stone-700 rounded text-xs">'
        .. '<span x-text="tag"></span>'
        .. '<button @click="remove(i)" class="text-stone-400 hover:text-red-600 font-bold">&times;</button>'
        .. '</span>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = '</div>'
    parts[#parts + 1] = string.format(
        '<input type="text" x-model="input" @keydown.enter.prevent="add()" :placeholder="\'%s\'" '
        .. 'class="px-2 py-1 border border-stone-300 rounded text-sm w-40 focus:outline-none focus:ring-1 focus:ring-amber-500">',
        html_escape(placeholder)
    )
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 9. textarea
-- ---------------------------------------------------------------------------

local function render_textarea(ctx)
    local err = require_path("textarea", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local rows = ctx.attrs.rows or "3"
    local placeholder = ctx.attrs.placeholder or ""

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, timer: null, '
        .. 'debounced() { clearTimeout(this.timer); this.timer = setTimeout(() => this.save(this.val), 500); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (text)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = string.format(
        '<textarea x-model="val" @input="debounced()" rows="%s" placeholder="%s" '
        .. 'class="w-full px-2 py-1 border border-stone-300 rounded text-sm resize-y focus:outline-none focus:ring-1 focus:ring-amber-500"></textarea>',
        html_escape(rows), html_escape(placeholder)
    )
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 10. date-picker
-- ---------------------------------------------------------------------------

local function render_date_picker(ctx)
    local err = require_path("date-picker", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local label = ctx.attrs.label

    local xdata = build_xdata(ctx, val)

    local title_text = string.format("Edit: %s (date)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex items-center gap-2 text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    if label then
        parts[#parts + 1] = string.format('<span class="text-stone-600">%s</span>', html_escape(label))
    end
    parts[#parts + 1] = '<input type="date" :value="val || \'\'" @change="save($event.target.value)" '
        .. 'class="px-2 py-1 border border-stone-300 rounded text-sm font-mono focus:outline-none focus:ring-1 focus:ring-amber-500">'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 11. date-range
-- ---------------------------------------------------------------------------

local function render_date_range(ctx)
    local err = require_path("date-range", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = {} end
    local start_label = ctx.attrs["start-label"] or "From"
    local end_label = ctx.attrs["end-label"] or "To"

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, '
        .. 'saveRange(key, v) { let obj = Object.assign({}, this.val || {}); obj[key] = v; this.save(obj); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (date range)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="flex flex-wrap items-center gap-2 text-sm py-1.5 max-w-full">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = string.format('<span class="text-stone-600 shrink-0">%s</span>', html_escape(start_label))
    parts[#parts + 1] = '<input type="date" :value="(val && val.start) || \'\'" @change="saveRange(\'start\', $event.target.value)" '
        .. 'class="px-2 py-1 border border-stone-300 rounded text-sm font-mono focus:outline-none focus:ring-1 focus:ring-amber-500 min-w-0">'
    parts[#parts + 1] = string.format('<span class="text-stone-600 shrink-0">%s</span>', html_escape(end_label))
    parts[#parts + 1] = '<input type="date" :value="(val && val.end) || \'\'" @change="saveRange(\'end\', $event.target.value)" '
        .. 'class="px-2 py-1 border border-stone-300 rounded text-sm font-mono focus:outline-none focus:ring-1 focus:ring-amber-500 min-w-0">'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 12. status-badge
-- ---------------------------------------------------------------------------

local function render_status_badge(ctx)
    local err = require_path("status-badge", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local options = parse_csv(ctx.attrs.options or "")
    local colors = parse_csv(ctx.attrs.colors or "#9ca3af,#f59e0b,#22c55e")
    local labels = parse_csv(ctx.attrs.labels or "")

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, options: %s, colors: %s, labels: %s, '
        .. 'next() { let o = Array.isArray(this.options) ? this.options : []; let i = o.indexOf(this.val); let n = (i + 1) %% o.length; if (o[n] != null) this.save(o[n]); }, '
        .. 'getColor() { let o = Array.isArray(this.options) ? this.options : []; let c = Array.isArray(this.colors) ? this.colors : []; let i = o.indexOf(this.val); return i >= 0 ? (c[i] || c[0] || "#9ca3af") : (c[0] || "#9ca3af"); }, '
        .. 'getLabel() { let o = Array.isArray(this.options) ? this.options : []; let l = Array.isArray(this.labels) ? this.labels : []; let i = o.indexOf(this.val); return i >= 0 ? (l[i] || o[i]) : (this.val || l[0] || o[0] || ""); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val), json_value(options), json_value(colors), json_value(labels),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (status)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="inline-flex items-center gap-1 py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<button @click="next()" '
        .. ':style="\'background-color:\' + getColor() + \'20; color:\' + getColor() + \'; border-color:\' + getColor()" '
        .. 'class="px-2 py-0.5 rounded-full text-xs font-semibold border cursor-pointer transition-colors" '
        .. 'x-text="getLabel()"></button>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 13. progress-input
-- ---------------------------------------------------------------------------

local function render_progress_input(ctx)
    local err = require_path("progress-input", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = 0 end
    local label = ctx.attrs.label or "Progress"

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, '
        .. 'setFromClick(e) { let rect = e.currentTarget.getBoundingClientRect(); let pct = Math.round(((e.clientX - rect.left) / rect.width) * 100); pct = Math.max(0, Math.min(100, pct)); this.save(pct); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(tonumber(val) or 0),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (progress %%)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<div class="flex items-center gap-2 mb-0.5">'
    parts[#parts + 1] = string.format('<span class="text-stone-600">%s</span>', html_escape(label))
    parts[#parts + 1] = '<span class="font-mono font-bold text-xs" x-text="(val || 0) + \'%\'"></span>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    parts[#parts + 1] = '<div @click="setFromClick($event)" class="bg-stone-200 rounded-full h-3 cursor-pointer overflow-hidden">'
        .. '<div class="bg-amber-600 h-full rounded-full transition-all" :style="\'width:\' + (val || 0) + \'%\'"></div>'
        .. '</div>'
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 14. key-value
-- ---------------------------------------------------------------------------

local function render_key_value(ctx)
    local err = require_path("key-value", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = {} end

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, newKey: \'\', newVal: \'\', '
        .. 'addPair() { if (!this.newKey.trim()) return; let obj = Object.assign({}, this.val || {}); obj[this.newKey.trim()] = this.newVal; this.save(obj); this.newKey = \'\'; this.newVal = \'\'; }, '
        .. 'removePair(k) { let obj = Object.assign({}, this.val || {}); delete obj[k]; this.save(obj); }, '
        .. 'entries() { return Object.entries(this.val || {}); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (key-value pairs)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="[k,v] in entries()" :key="k">'
    parts[#parts + 1] = '<div class="flex items-center gap-1 mb-1">'
        .. '<span class="font-mono text-xs bg-stone-100 px-1.5 py-0.5 rounded" x-text="k"></span>'
        .. '<span class="text-stone-400">=</span>'
        .. '<span class="font-mono text-xs" x-text="v"></span>'
        .. '<button @click="removePair(k)" class="text-stone-400 hover:text-red-600 text-xs">&times;</button>'
        .. '</div>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = '<div class="flex items-center gap-1 mt-1">'
        .. '<input type="text" x-model="newKey" placeholder="key" class="px-1.5 py-0.5 border border-stone-300 rounded text-xs w-20 focus:outline-none focus:ring-1 focus:ring-amber-500">'
        .. '<input type="text" x-model="newVal" placeholder="value" @keydown.enter.prevent="addPair()" class="px-1.5 py-0.5 border border-stone-300 rounded text-xs w-28 focus:outline-none focus:ring-1 focus:ring-amber-500">'
        .. '<button @click="addPair()" class="px-1.5 py-0.5 bg-amber-700 text-white rounded text-xs hover:bg-amber-800">+</button>'
        .. '</div>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 15. checklist
-- ---------------------------------------------------------------------------

local function render_checklist(ctx)
    local err = require_path("checklist", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = {} end

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, newItem: \'\', '
        .. 'addItem() { if (!this.newItem.trim()) return; let a = [...(this.val || []), {text: this.newItem.trim(), done: false}]; this.save(a); this.newItem = \'\'; }, '
        .. 'toggleItem(i) { let a = JSON.parse(JSON.stringify(this.val || [])); a[i].done = !a[i].done; this.save(a); }, '
        .. 'removeItem(i) { let a = [...(this.val || [])]; a.splice(i, 1); this.save(a); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (checklist)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<template x-for="(item, i) in (val || [])" :key="i">'
    parts[#parts + 1] = '<div class="flex items-center gap-1.5 mb-1">'
        .. '<input type="checkbox" :checked="item.done" @change="toggleItem(i)" '
        .. 'class="rounded border-stone-300 text-amber-600 focus:ring-amber-500 h-4 w-4">'
        .. '<span :class="item.done ? \'line-through text-stone-400\' : \'text-stone-700\'" x-text="item.text" class="flex-1"></span>'
        .. '<button @click="removeItem(i)" class="text-stone-400 hover:text-red-600 text-xs">&times;</button>'
        .. '</div>'
    parts[#parts + 1] = '</template>'
    parts[#parts + 1] = '<div class="flex items-center gap-1 mt-1">'
        .. '<input type="text" x-model="newItem" @keydown.enter.prevent="addItem()" placeholder="Add item..." '
        .. 'class="px-2 py-0.5 border border-stone-300 rounded text-xs flex-1 focus:outline-none focus:ring-1 focus:ring-amber-500">'
        .. '<button @click="addItem()" class="px-1.5 py-0.5 bg-amber-700 text-white rounded text-xs hover:bg-amber-800">+</button>'
        .. '</div>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 16. url-input
-- ---------------------------------------------------------------------------

local function render_url_input(ctx)
    local err = require_path("url-input", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local placeholder = ctx.attrs.placeholder or "https://..."
    local label = ctx.attrs.label

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, timer: null, valid: false, '
        .. 'debounced() { clearTimeout(this.timer); try { new URL(this.val); this.valid = true; } catch { this.valid = false; } this.timer = setTimeout(() => { if (this.valid) this.save(this.val); }, 500); }, '
        .. 'checkValid() { try { new URL(this.val); this.valid = true; } catch { this.valid = false; } }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (URL)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" x-init="checkValid()" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = '<div class="flex items-center gap-1">'
    if label then
        parts[#parts + 1] = string.format('<span class="text-stone-600">%s</span>', html_escape(label))
    end
    parts[#parts + 1] = string.format(
        '<input type="url" x-model="val" @input="debounced()" placeholder="%s" '
        .. 'class="px-2 py-1 border border-stone-300 rounded text-sm font-mono flex-1 focus:outline-none focus:ring-1 focus:ring-amber-500">',
        html_escape(placeholder)
    )
    parts[#parts + 1] = '<a x-show="valid" :href="val" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-800" title="Open link">'
        .. '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6M15 3h6v6M10 14L21 3"/></svg>'
        .. '</a>'
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- 17. markdown (monospace textarea)
-- ---------------------------------------------------------------------------

local function render_markdown(ctx)
    local err = require_path("markdown", ctx)
    if err then return err end

    local val = get_nested(ctx.value, ctx.attrs.path)
    if val == nil then val = "" end
    local rows = ctx.attrs.rows or "5"
    local placeholder = ctx.attrs.placeholder or "Write markdown..."

    local xdata = string.format(
        '{ val: %s, saving: false, saved: false, timer: null, '
        .. 'debounced() { clearTimeout(this.timer); this.timer = setTimeout(() => this.save(this.val), 500); }, '
        .. 'async save(v) { '
        .. 'this.saving = true; '
        .. 'try { await window.__metaEditorsSave(\'%s\', %d, \'%s\', v); this.val = v; this.saved = true; setTimeout(() => this.saved = false, 1500); } '
        .. 'catch(e) { console.error(e); } '
        .. 'this.saving = false; } }',
        json_value(val),
        html_escape(ctx.entity_type), ctx.entity_id, html_escape(ctx.attrs.path)
    )

    local title_text = string.format("Edit: %s (markdown)", ctx.attrs.path)
    local parts = {}
    parts[#parts + 1] = string.format('<div title="%s" x-data="%s" class="text-sm py-1.5">', html_escape(title_text), html_escape(xdata))
    parts[#parts + 1] = string.format(
        '<textarea x-model="val" @input="debounced()" rows="%s" placeholder="%s" '
        .. 'class="w-full px-2 py-1 border border-stone-300 rounded text-sm font-mono resize-y focus:outline-none focus:ring-1 focus:ring-amber-500"></textarea>',
        html_escape(rows), html_escape(placeholder)
    )
    parts[#parts + 1] = save_indicators()
    parts[#parts + 1] = '</div>'
    return table.concat(parts, "\n")
end

-- ---------------------------------------------------------------------------
-- Plugin initialization
-- ---------------------------------------------------------------------------

function init()
    -- Inject the shared save helper script once per page
    mah.inject("page_bottom", render_save_helper)

    -- Register all 17 shortcodes
    mah.shortcode({ name = "slider",         label = "Slider",           render = render_slider })
    mah.shortcode({ name = "stepper",        label = "Stepper",          render = render_stepper })
    mah.shortcode({ name = "star-rating",    label = "Star Rating",      render = render_star_rating })
    mah.shortcode({ name = "toggle",         label = "Toggle",           render = render_toggle })
    mah.shortcode({ name = "multi-select",   label = "Multi Select",     render = render_multi_select })
    mah.shortcode({ name = "button-group",   label = "Button Group",     render = render_button_group })
    mah.shortcode({ name = "color-picker",   label = "Color Picker",     render = render_color_picker })
    mah.shortcode({ name = "tags-input",     label = "Tags Input",       render = render_tags_input })
    mah.shortcode({ name = "textarea",       label = "Textarea",         render = render_textarea })
    mah.shortcode({ name = "date-picker",    label = "Date Picker",      render = render_date_picker })
    mah.shortcode({ name = "date-range",     label = "Date Range",       render = render_date_range })
    mah.shortcode({ name = "status-badge",   label = "Status Badge",     render = render_status_badge })
    mah.shortcode({ name = "progress-input", label = "Progress Input",   render = render_progress_input })
    mah.shortcode({ name = "key-value",      label = "Key-Value Editor", render = render_key_value })
    mah.shortcode({ name = "checklist",      label = "Checklist",        render = render_checklist })
    mah.shortcode({ name = "url-input",      label = "URL Input",        render = render_url_input })
    mah.shortcode({ name = "markdown",       label = "Markdown Editor",  render = render_markdown })
end
