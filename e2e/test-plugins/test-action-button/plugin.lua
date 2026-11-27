-- Fixture for the [action] shortcode E2E specs (109-action-shortcode.spec.ts,
-- accessibility/action-shortcode-a11y.spec.ts, auth/action-shortcode-roles.spec.ts).
-- Kept apart from test-actions so the action counts other specs assert on do
-- not change.
plugin = {
    name = "test-action-button",
    version = "1.0",
    description = "Actions run from [action] template buttons",
    settings = {}
}

function init()
    -- A synchronous write with a confirmation: the template shows the
    -- Description in an [autorefresh] block, so the change is visible in place.
    mah.action({
        id = "mark-reviewed",
        label = "Mark reviewed",
        entity = "resource",
        placement = { "detail" },
        confirm = "Mark as reviewed?",
        handler = function(ctx)
            local _, err = mah.db.patch_resource(ctx.entity_id, { description = "reviewed" })
            if err then return { success = false, message = err } end
            return { success = true, message = "Marked resource " .. ctx.entity_id .. " reviewed" }
        end,
    })

    -- A group action that only reports, for the role checks.
    mah.action({
        id = "ping-group",
        label = "Ping group",
        entity = "group",
        handler = function(ctx)
            return { success = true, message = "Pinged group " .. ctx.entity_id }
        end,
    })
end
