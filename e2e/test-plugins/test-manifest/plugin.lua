-- E2E fixture for the Access panel on /plugins/manage.
--
-- Every other plugin in this directory is legacy (no manifest), so nothing
-- exercised the declared-manifest branches of the panel. This one declares each
-- of them: a capability list, a network allowlist, a dependency, and a minimum
-- app version. It is never enabled by the specs — the panel renders for a
-- discovered plugin whether or not it is running.
--
-- db:write is declared and db:read is not, on purpose: the panel must show the
-- effective set, and db:write implies db:read.
plugin = {
    api_version = 1,
    capabilities = { "db:write", "hooks", "inject" },
    -- Deliberately out of sorted order: NetworkDisplay sorts what it renders.
    network = { "api.example.com", "*.cdn.example.org" },
    dependencies = { "test-banner" },
    min_app_version = "1.2.0",
    name = "test-manifest",
    version = "1.0",
    description = "E2E test plugin that declares a full manifest"
}

function init()
    -- Nothing to register: the specs read the manage page, and enabling this
    -- plugin is refused anyway until its dependency is enabled.
end
