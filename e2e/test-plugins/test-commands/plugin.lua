plugin = {
    api_version = 1,
    capabilities = { "api", "commands" },
    commands = {
        {
            name = "fixture",
            argv = { "test-command-helper", "{{mode}}" },
            timeout = 60,
        },
    },
    name = "test-commands",
    version = "1.0",
    description = "Deterministic E2E fixture for plugin command lifecycle tests",
}

function init()
    mah.api("POST", "run", function(ctx)
        local body = mah.json.decode(ctx.body)
        local run_id, err = mah.commands.run("fixture", { mode = body.mode })
        if err then
            ctx.status(500)
            ctx.json({ error = err })
            return
        end
        ctx.status(202)
        ctx.json({ run_id = run_id })
    end)
end
