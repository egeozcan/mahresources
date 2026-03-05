plugin = {
    name = "test-kvstore",
    version = "1.0",
    description = "Test plugin for KV store E2E tests",
    settings = {}
}

function init()
    -- Page: set a key-value pair via POST form params
    -- POST /plugins/test-kvstore/set with params: key, value (JSON string)
    mah.page("set", function(ctx)
        local params = ctx.params or {}
        local key = params.key
        local value_json = params.value
        if not key or not value_json then
            return '<div data-testid="kv-error">Missing key or value</div>'
        end
        local decoded = mah.json.decode(value_json)
        mah.kv.set(key, decoded)
        return '<div data-testid="kv-result">OK</div>'
    end)

    -- Page: get a value by key via query param
    -- GET /plugins/test-kvstore/get?key=mykey
    mah.page("get", function(ctx)
        local key = (ctx.query or {}).key
        if not key then
            return '<div data-testid="kv-error">Missing key</div>'
        end
        local val = mah.kv.get(key)
        if val == nil then
            return '<div data-testid="kv-value" data-found="false">nil</div>'
        end
        local json_val = mah.json.encode(val)
        return '<div data-testid="kv-value" data-found="true">' .. json_val .. '</div>'
    end)

    -- Page: delete a key via POST
    -- POST /plugins/test-kvstore/delete with params: key
    mah.page("delete", function(ctx)
        local params = ctx.params or {}
        local key = params.key
        if not key then
            return '<div data-testid="kv-error">Missing key</div>'
        end
        mah.kv.delete(key)
        return '<div data-testid="kv-result">OK</div>'
    end)

    -- Page: list keys via query param
    -- GET /plugins/test-kvstore/list?prefix=optional
    mah.page("list", function(ctx)
        local prefix = (ctx.query or {}).prefix or ""
        local keys = mah.kv.list(prefix)
        local json_keys = mah.json.encode(keys)
        return '<div data-testid="kv-keys">' .. json_keys .. '</div>'
    end)
end
