-- Ketchup: draw new images and edit existing ones in the Ketchup editor
-- (https://github.com/egeozcan/ketchup), with this library as the place the
-- images live.
--
-- The editor runs in the browser on the plugin's "edit" page. It loads the
-- image from /v1/resource/view and saves through the ordinary resource API
-- as the person using it: an edit uploads a new version of the resource, a
-- new drawing creates a resource. The Lua side only renders the page and the
-- ways into it, so it holds no write capability of its own.
--
-- public/ketchup.js is Ketchup's embeddable build, stamped on its first line
-- with the Ketchup commit it came from. Update it with
-- scripts/update-ketchup.sh <ref>.

plugin = {
    api_version = 1,
    name = "ketchup",
    version = "1.0",
    description = "Draw new images and edit existing ones in the Ketchup editor. Edits are saved as new versions; drawings as new resources.",
    capabilities = { "db:read", "pages", "inject", "actions" },
    settings = {
        { name = "new_width", type = "number", label = "New image width (px)", default = 1200 },
        { name = "new_height", type = "number", label = "New image height (px)", default = 800 },
    },
}

local PUBLIC = "/plugins/ketchup/public"
local MAX_DIMENSION = 16384

-- What the editor can open, and what it saves each as. GIF is left out on
-- purpose: only its first frame would survive a save. BMP saves as PNG,
-- since browsers cannot encode BMP.
local SAVE_TYPES = {
    ["image/png"] = "image/png",
    ["image/jpeg"] = "image/jpeg",
    ["image/webp"] = "image/webp",
    ["image/bmp"] = "image/png",
}

local function editable_types()
    local list = {}
    for t, _ in pairs(SAVE_TYPES) do list[#list + 1] = t end
    table.sort(list)
    return list
end

local function edit_url(resource_id)
    return "/plugins/ketchup/edit?id=" .. tostring(resource_id)
end

local function new_url(owner_id)
    if owner_id and owner_id > 0 then
        return "/plugins/ketchup/edit?owner=" .. tostring(owner_id)
    end
    return "/plugins/ketchup/edit"
end

-- Ids arrive as query strings: decimal digits only, so "0x10" and "1e3" are
-- not ids, and small enough to stay exact.
local function positive_int(value)
    if type(value) == "string" and not value:match("^%d+$") then return nil end
    local n = tonumber(value)
    if n == nil or n ~= n or n < 1 or n ~= math.floor(n) or n > 2^53 then return nil end
    return n
end

-- A setting as a canvas dimension, falling back when unset or out of range.
local function dimension_setting(name, fallback)
    local n = positive_int(mah.get_setting(name))
    if n == nil or n > MAX_DIMENSION then return fallback end
    return n
end

local function notice(title, body)
    return '<div class="bg-amber-50 border border-amber-200 rounded p-4" role="alert">'
        .. '<h1 class="font-semibold mb-1">' .. mah.html_escape(title) .. '</h1>'
        .. '<p>' .. body .. '</p></div>'
end

-- JSON placed inside <script type="application/json">: "</" must not appear,
-- or a resource named "</script>..." would end the element early.
local function script_json(value)
    return (mah.json.encode(value):gsub("</", "<\\/"))
end

local function render_editor(config)
    return '<link rel="stylesheet" href="' .. PUBLIC .. '/editor.css">'
        .. '<section class="ketchup-host" aria-labelledby="ketchup-title">'
        .. '<div class="ketchup-bar">'
        .. '<h2 id="ketchup-title" class="ketchup-title"></h2>'
        .. '<label class="ketchup-name-label" hidden>Name <input class="ketchup-name" type="text" autocomplete="off"></label>'
        .. '<span class="ketchup-dirty" hidden>Unsaved changes</span>'
        .. '<p class="ketchup-status" role="status" aria-live="polite"></p>'
        .. '<div class="ketchup-actions">'
        .. '<a class="ketchup-back" hidden></a>'
        .. '<button type="button" class="ketchup-save-copy" hidden disabled>Save as new resource</button>'
        .. '<button type="button" class="ketchup-save" disabled>Save</button>'
        .. '</div></div>'
        .. '<drawing-app class="ketchup-app" embedded tabindex="0" aria-label="Image editor"></drawing-app>'
        .. '</section>'
        .. '<script type="application/json" id="ketchup-config">' .. script_json(config) .. '</script>'
        .. '<script type="module" src="' .. PUBLIC .. '/editor.js"></script>'
end

local function edit_page(id)
    local resource, err = mah.db.get_resource(id)
    if err then
        return notice("Resource could not be read", mah.html_escape(err))
    end
    if not resource then
        return notice("Resource not found", "There is no resource " .. tostring(id) .. " you can open.")
    end
    local save_type = SAVE_TYPES[resource.content_type or ""]
    if not save_type then
        return notice("Ketchup cannot edit this resource",
            mah.html_escape(resource.name or "") .. " is " .. mah.html_escape(resource.content_type or "of an unknown type")
            .. '. Ketchup edits PNG, JPEG, WebP and BMP images. <a class="underline" href="/resource?id='
            .. tostring(id) .. '">Back to the resource</a>')
    end
    return render_editor({
        mode = "edit",
        resource = {
            id = resource.id,
            name = resource.name or "",
            contentType = resource.content_type,
            ownerId = resource.owner_id or 0,
        },
        saveType = save_type,
        -- The hash keeps a browser from answering with the version before the last save.
        fileUrl = "/v1/resource/view?id=" .. tostring(resource.id) .. "&v=" .. ((resource.hash or ""):gsub("[^%w]", "")),
    })
end

local function new_page(owner_id)
    local owner = nil
    if owner_id then
        local group, err = mah.db.get_group(owner_id)
        if err then
            return notice("Group could not be read", mah.html_escape(err))
        end
        if not group then
            return notice("Group not found", "There is no group " .. tostring(owner_id) .. " you can add to.")
        end
        owner = { id = group.id, name = group.name or "" }
    end
    return render_editor({
        mode = "new",
        owner = owner,
        saveType = "image/png",
        width = dimension_setting("new_width", 1200),
        height = dimension_setting("new_height", 800),
    })
end

-- A link styled like the sidebar's own action buttons.
local function sidebar_link(title, href, label)
    return '<div class="sidebar-section" role="group" aria-label="' .. title .. '">'
        .. '<h2 class="sidebar-section-title">' .. title .. '</h2>'
        .. '<a class="sidebar-action-btn block" href="' .. href .. '">' .. label .. '</a>'
        .. '</div>'
end

function init()
    mah.page("edit", function(ctx)
        local query = ctx.query or {}
        if query.id ~= nil then
            local id = positive_int(query.id)
            if not id then return notice("Invalid resource id", "The id must be a positive whole number.") end
            return edit_page(id)
        end
        local owner_id = nil
        if query.owner ~= nil and query.owner ~= "" then
            owner_id = positive_int(query.owner)
            if not owner_id then return notice("Invalid group id", "The group id must be a positive whole number.") end
        end
        return new_page(owner_id)
    end, { hide_sidebar = true })

    mah.menu("New Image", "edit")

    mah.inject("resource_detail_sidebar", function(ctx)
        local id = positive_int(ctx.entity_id)
        if not id then return "" end
        local resource = mah.db.get_resource(id)
        if not resource or not SAVE_TYPES[resource.content_type or ""] then return "" end
        return sidebar_link("Ketchup", edit_url(id), "Edit in Ketchup")
    end)

    mah.inject("group_detail_sidebar", function(ctx)
        local id = positive_int(ctx.entity_id)
        if not id then return "" end
        return sidebar_link("Ketchup", new_url(id), "New image in this group")
    end)

    -- Cards have no sidebar slot, so a card's way in is an action.
    mah.action({
        id = "edit",
        label = "Edit in Ketchup",
        description = "Opens the image in the Ketchup editor. Saving there adds a new version.",
        entity = "resource",
        placement = { "card" },
        filters = { content_types = editable_types() },
        handler = function(ctx)
            return { success = true, message = "Opening Ketchup", redirect = edit_url(ctx.entity_id) }
        end,
    })
end
