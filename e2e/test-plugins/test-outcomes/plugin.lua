-- Plugin actions whose Jobs end in ways a person has to be told about: a
-- failure with the plugin's own reason, and a running handler that may be
-- cancelled. A plugin of its own, because a plugin runs one async handler at a
-- time: a long-running action in a shared test plugin would hold every other
-- spec's actions of that plugin behind it.
plugin = {
    name = "test-outcomes",
    version = "1.0",
    api_version = 1,
    description = "Plugin action outcomes for E2E tests",
    capabilities = { "actions" },
}

function init()
    -- Reports its own failure: the Job's Failure section shows this message.
    mah.action({
        id = "fail-demo",
        label = "Fail Demo",
        description = "Reports a failure with the plugin's own reason",
        entity = "resource",
        placement = { "detail" },
        async = true,
        handler = function(ctx)
            mah.job_fail(ctx.job_id, "Upstream rejected chunk 6: HTTP 502 Bad Gateway")
        end,
    })

    -- Waits for up to a minute and declares that it may be stopped partway, so a
    -- running Job offers Cancel.
    mah.action({
        id = "cancellable-wait",
        label = "Cancellable Wait",
        description = "Waits until it is cancelled",
        entity = "resource",
        placement = { "detail" },
        async = true,
        cancel = true,
        handler = function(ctx)
            for i = 1, 300 do
                mah.job_progress(ctx.job_id, i % 100, "Waiting " .. i)
                mah.sleep(0.2)
            end
            mah.job_complete(ctx.job_id, { message = "Waited a minute" })
        end,
    })

    -- Declares nothing about cancelling; queued behind cancellable-wait, it can
    -- still be cancelled before it starts.
    mah.action({
        id = "quick",
        label = "Quick",
        description = "Completes at once",
        entity = "resource",
        placement = { "detail" },
        async = true,
        handler = function(ctx)
            mah.job_complete(ctx.job_id, { message = "Done" })
        end,
    })
end
