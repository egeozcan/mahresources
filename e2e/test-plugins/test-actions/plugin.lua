plugin = {
    name = "test-actions",
    version = "1.0",
    description = "Test plugin for E2E action tests",
    settings = {}
}

function init()
    mah.action({
        id = "sync-greet",
        label = "Greet Resource",
        description = "Greets a resource with a message",
        entity = "resource",
        placement = { "detail", "card" },
        params = {
            { name = "greeting", type = "text", label = "Greeting", required = true, default = "Hello" },
        },
        handler = function(ctx)
            return { success = true, message = "Greeted resource " .. ctx.entity_id .. " with: " .. ctx.params.greeting }
        end,
    })

    mah.action({
        id = "group-action",
        label = "Group Action",
        description = "Performs an action on a group",
        entity = "group",
        placement = { "detail", "bulk" },
        handler = function(ctx)
            return { success = true, message = "Ran on group " .. ctx.entity_id }
        end,
    })

    mah.action({
        id = "conditional-demo",
        label = "Conditional Demo",
        description = "Action with show_when params for testing conditional visibility",
        entity = "resource",
        placement = { "detail" },
        params = {
            { name = "mode", type = "select", label = "Mode", default = "a", options = {"a", "b"} },
            { name = "extra_a", type = "text", label = "Extra A", default = "alpha",
              show_when = { mode = "a" } },
            { name = "extra_b", type = "text", label = "Extra B", default = "beta",
              show_when = { mode = "b" } },
            { name = "advanced", type = "boolean", label = "Advanced", default = false },
            -- Lua boolean (not "true" string): modal binds the checkbox to a JS
            -- boolean via x-model, so isParamVisible compares with === against
            -- the literal Go bool plumbed through the plugin manifest.
            { name = "tuning", type = "text", label = "Tuning", default = "deep",
              show_when = { advanced = true } },
            -- Static help block. Renders only when mode=a; should never appear
            -- in the submission body since type=info has no input value.
            { name = "info_for_a", type = "info",
              label = "About Mode A",
              description = "Mode A is the default. Select B to see extra fields.",
              show_when = { mode = "a" } },
        },
        handler = function(ctx)
            local p = ctx.params or {}
            local parts = {}
            for k, v in pairs(p) do parts[#parts + 1] = k .. "=" .. tostring(v) end
            table.sort(parts)
            return { success = true, message = "params: " .. table.concat(parts, ",") }
        end,
    })

    mah.action({
        id = "async-demo",
        label = "Async Demo",
        description = "Demonstrates async action with progress",
        entity = "resource",
        placement = { "detail" },
        async = true,
        params = {
            { name = "steps", type = "number", label = "Steps", default = 3 },
        },
        handler = function(ctx)
            mah.job_progress(ctx.job_id, 50, "Working...")
            mah.job_complete(ctx.job_id, { message = "Done!", steps_completed = ctx.params.steps })
        end,
    })
end
