plugin = {
    api_version = 1,
    capabilities = { "api", "commands" },
    commands = {
        {
            name = "fixture",
            argv = { "test-command-helper", "{{mode}}" },
            timeout = 60,
        },
        {
            name = "read_input",
            argv = { "test-command-helper", "read-input" },
            inputs = { "cookies.txt" },
            timeout = 60,
        },
    },
    name = "test-commands",
    version = "1.0",
    description = "Deterministic E2E fixture for plugin command lifecycle tests",
}

-- refuse answers a refused command. An unavailable command runtime is "try
-- later" (503), with the seconds until the host tries again when it has a
-- retry scheduled; any other refusal is the request's.
local function refuse(ctx, err, info, status)
    if info and info.unavailable then
        ctx.status(503)
        ctx.json({ error = err, retry_after = info.retry_after })
        return
    end
    ctx.status(status)
    ctx.json({ error = err })
end

function init()
    mah.api("POST", "run", function(ctx)
        local body = mah.json.decode(ctx.body)
        local run_id, err, info = mah.commands.run("fixture", { mode = body.mode })
        if err then
            refuse(ctx, err, info, 500)
            return
        end
        ctx.status(202)
        ctx.json({ run_id = run_id })
    end)

    -- Supplied input files. `files` is a list of { name, content } pairs, so a
    -- test can drive the declared-name refusal as well as the happy path.
    mah.api("POST", "read-input", function(ctx)
        local body = mah.json.decode(ctx.body)
        local options = nil
        if body.files then
            local inputs = {}
            for _, file in ipairs(body.files) do
                inputs[file.name] = file.content
            end
            options = { inputs = inputs }
        end
        local run_id, err, info = mah.commands.run("read_input", {}, nil, options)
        if err then
            refuse(ctx, err, info, 400)
            return
        end
        ctx.status(202)
        ctx.json({ run_id = run_id })
    end)
end
