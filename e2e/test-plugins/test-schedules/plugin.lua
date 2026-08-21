plugin = {
    name = "test-schedules",
    version = "1.0",
    description = "Test plugin that declares a durable schedule",
}

function init()
    -- One hour, deliberately. What this fixture exists to cover is the stored
    -- row and the table that renders it; a schedule that actually fired during
    -- a run would put a background job into every other spec's jobs panel, and
    -- MinScheduleInterval (30s) makes "fires quickly" and "does not disturb
    -- anything" mutually exclusive anyway. The firing path has Go coverage.
    mah.schedule({
        id = "nightly-rollup",
        every = "1h",
        handler = function()
            mah.kv.set("last-run", "fired")
        end,
    })
end
