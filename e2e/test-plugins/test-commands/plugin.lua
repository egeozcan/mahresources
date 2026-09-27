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

-- refuse answers a refused command. A runtime the host is still recovering
-- comes with the seconds until it tries again: that is "try later" (503), not
-- a bad request.
local function refuse(ctx, err, retry_after, status)
    if retry_after then
        ctx.status(503)
        ctx.json({ error = err, retry_after = retry_after })
        return
    end
    ctx.status(status)
    ctx.json({ error = err })
end

function init()
    mah.api("POST", "run", function(ctx)
        local body = mah.json.decode(ctx.body)
        local run_id, err, retry_after = mah.commands.run("fixture", { mode = body.mode })
        if err then
            refuse(ctx, err, retry_after, 500)
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
        local run_id, err, retry_after = mah.commands.run("read_input", {}, nil, options)
        if err then
            refuse(ctx, err, retry_after, 400)
            return
        end
        ctx.status(202)
        ctx.json({ run_id = run_id })
    end)
end
