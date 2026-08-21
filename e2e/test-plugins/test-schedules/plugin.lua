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
    -- It no longer means the firing path is untested from the browser. The
    -- run-now control fires this schedule on demand, so
    -- plugin-schedule-run-now.spec.ts asserts an actual run — an outcome
    -- recorded, `runs` moved, and `nextDueAt` deliberately not moved. A
    -- not-yet-due row is exactly what that control exists for, so the hour is
    -- now part of what is under test rather than a way of avoiding it.
    mah.schedule({
        id = "nightly-rollup",
        every = "1h",
        handler = function()
            mah.kv.set("last-run", "fired")
        end,
    })
end
