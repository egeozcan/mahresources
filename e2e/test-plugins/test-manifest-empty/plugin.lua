-- E2E fixture for the two "declared but unrestricted/ungranted" branches of the
-- Access panel: a manifest that asks for no capabilities gets none of the mah
-- APIs, and a manifest with no network list may reach any public host.
--
-- The second one is the reading that is easy to get backwards, so it is worth a
-- fixture that is not also a legacy plugin.
plugin = {
    api_version = 1,
    name = "test-manifest-empty",
    version = "1.0",
    description = "E2E test plugin that declares a manifest but asks for nothing"
}

function init()
    -- Nothing to register: no capability is granted, so no mah API exists here.
end
