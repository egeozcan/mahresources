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
