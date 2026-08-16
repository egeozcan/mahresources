-- E2E fixture for the private-network warning on /plugins/manage.
--
-- allow_private_hosts only parses alongside a network list that names its hosts
-- exactly, so this declares one CIDR block and one host name. That mix is what
-- the warning distinguishes: the block lifts the dial-time deny, the name does
-- not.
plugin = {
    api_version = 1,
    capabilities = { "http" },
    network = { "192.168.0.0/16", "printer.local" },
    allow_private_hosts = true,
    name = "test-manifest-private",
    version = "1.0",
    description = "E2E test plugin that may reach private network addresses"
}

function init()
    -- Nothing to register: the specs only read the manage page.
end
