plugin = {
    name = "test-api",
    version = "1.0",
    description = "E2E test plugin for JSON API endpoints"
}

function init()
    -- GET endpoint that echoes query params and method
    mah.api("GET", "echo", function(ctx)
        ctx.json({ query = ctx.query, method = ctx.method, path = ctx.path })
    end)

    -- POST endpoint that echoes parsed body
    mah.api("POST", "echo", function(ctx)
        local body = mah.json.decode(ctx.body)
        ctx.status(201)
        ctx.json({ received = body })
    end)

    -- PUT endpoint
    mah.api("PUT", "echo", function(ctx)
        local body = mah.json.decode(ctx.body)
        ctx.json({ updated = body })
    end)

    -- DELETE endpoint with no body
    mah.api("DELETE", "echo", function(ctx)
        ctx.status(204)
    end)

    -- Endpoint that uses KV store
    mah.api("POST", "store", function(ctx)
        local body = mah.json.decode(ctx.body)
        mah.kv.set("api_data", body)
        ctx.status(201)
        ctx.json({ stored = true })
    end)

    mah.api("GET", "store", function(ctx)
        local data = mah.kv.get("api_data")
        if data then
            ctx.json(data)
        else
            ctx.status(404)
            ctx.json({ error = "no data" })
        end
    end)

    -- Endpoint that calls mah.abort
    mah.api("POST", "validate", function(ctx)
        mah.abort("validation failed")
    end)

    -- Endpoint that errors
    mah.api("GET", "crash", function(ctx)
        error("intentional crash")
    end)

    -- Nested path
    mah.api("GET", "nested/deep/path", function(ctx)
        ctx.json({ nested = true })
    end)
end
