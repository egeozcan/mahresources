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

    -- A handler that refuses. ActionResult.success has always been produced and
    -- transported; the modal used to ignore it and announce every refusal as
    -- "Action completed successfully" before reloading the page.
    mah.action({
        id = "always-refuses",
        label = "Always Refuses",
        description = "Returns success=false so the modal has to report a failure",
        entity = "resource",
        placement = { "detail", "card", "bulk" },
        handler = function(ctx)
            return { success = false, message = "Refused resource " .. ctx.entity_id .. ": quota exhausted" }
        end,
    })

    -- Refuses exactly the entity named in refuse_id, so a bulk run over two
    -- resources is partly successful and the test controls which one fails —
    -- id parity is not stable when other workers are creating entities.
    mah.action({
        id = "refuses-one",
        label = "Refuses One",
        description = "Fails for the entity named in refuse_id and succeeds for the rest",
        entity = "resource",
        placement = { "detail", "card", "bulk" },
        params = {
            { name = "refuse_id", type = "number", label = "Refuse this id", default = 0 },
        },
        handler = function(ctx)
            local target = tonumber(ctx.params.refuse_id) or 0
            if ctx.entity_id == target then
                return { success = false, message = "Refused resource " .. ctx.entity_id }
            end
            return { success = true, message = "Handled resource " .. ctx.entity_id }
        end,
    })

    -- required combined with show_when: mandatory only in the branch that shows
    -- it. Registration used to reject this pair outright.
    mah.action({
        id = "branch-required",
        label = "Branch Required",
        description = "A mandatory field that exists only in one branch",
        entity = "resource",
        placement = { "detail" },
        params = {
            { name = "mode", type = "select", label = "Mode", default = "simple", options = {"simple", "scheduled"} },
            { name = "publish_at", type = "text", label = "Publish at", required = true,
              show_when = { mode = "scheduled" } },
        },
        handler = function(ctx)
            local at = ctx.params.publish_at
            if at == nil then return { success = true, message = "published now" } end
            return { success = true, message = "scheduled for " .. at }
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
