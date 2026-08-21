plugin = {
    name = "test-schedules",
    version = "1.0",
    description = "Test plugin that declares a durable schedule",
}

function init()
    -- One hour, deliberately: MinScheduleInterval (30s) makes "fires quickly"
    -- and "does not disturb anything" mutually exclusive, and a schedule firing
    -- on its own during a run would put a background job into every other
    -- spec's jobs panel.
    --
    -- This row is the one plugin-schedules.spec.ts asserts still reads "never
    -- run", so nothing may fire it. That assertion is durable rather than
    -- incidental: rows are deliberately never deleted on disable, so a run
    -- against this id would survive the disable/enable both specs do and leave
    -- the other one asserting against history it did not create.
    mah.schedule({
        id = "nightly-rollup",
        every = "1h",
        handler = function()
            mah.kv.set("last-run", "fired")
        end,
    })

    -- The row the run-now control is exercised against, kept separate from the
    -- one above for exactly the reason that one names. Same one-hour interval,
    -- because "not yet due" is the state the control exists for: a manual run
    -- has to claim a row the ticker would refuse.
    mah.schedule({
        id = "manual-only",
        every = "1h",
        handler = function()
            mah.kv.set("manual-last-run", "fired")
        end,
    })
end
