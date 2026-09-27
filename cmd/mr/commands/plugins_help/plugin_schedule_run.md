---
outputShape: Object with ok (bool), name (string), scheduleId (string) and started (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: plugin schedules, plugin enable, plugin disable
---

# Long

Run one of a plugin's schedules straight away, without waiting for it to
come due. This is the supported way to exercise a schedule you are
developing: the alternatives are editing `next_due_at` in the database
directly, or waiting out an interval that may be an hour.

The command returns as soon as the run has *started*, not when it has
finished. A schedule handler may run for the full asynchronous job
allowance, so the request is not held open for it; the run reports
itself through the same job events as any other plugin job, and the
outcome lands on the row, where `mr plugin schedules` will show it as
`lastStatus` with an incremented `runs`. Starting takes at most the
10-second dispatch wait: the run needs its plugin to be free of other
background work, one of the server's plugin job slots and room in the
deployment's job budget.

Two things are deliberately unchanged by a manual run. It executes as
the operator who enabled the plugin, exactly as a scheduled run does,
rather than as whoever asked for it. And `nextDueAt` does not move: this
is an extra run, not a re-phasing, so the schedule stays on the cadence
it was already on.

A run is refused rather than started in six cases, each with its own
message and a non-zero exit. There is no such stored schedule. The
plugin no longer declares that id, which is what a disabled plugin and a
renamed schedule both look like. The row has no owner, so the schedule
has stopped. A run is already in flight, which is the `skip` overlap
policy doing exactly what it promises. The run could not start within
the dispatch wait because the plugin, the job slots or the job budget
stayed busy: nothing ran, nothing is recorded on the row and no job is
kept, so running the command again later is safe. Or the account the
schedule runs as may no longer run it, because it is disabled, lost the
role to write or may no longer use the plugin: the message says which,
the row records `refused` without counting a run, and no job is kept.
If the server cannot tell within 5 seconds past the dispatch wait
whether the run started, for example because its database is slow to
answer, the message says it had not started by then. It may still
start, so check the jobs panel before running the command again.

# Example

  # Run a schedule you are developing, without waiting for it to come due
  mr plugin schedule-run my-plugin nightly-rollup

  # Start it, then read back what it recorded
  mr plugin schedule-run my-plugin nightly-rollup
  mr plugin schedules my-plugin --json | jq -r '.[] | select(.scheduleId=="nightly-rollup") | .lastStatus'

  # mr-doctest: a manual run executes the handler without re-phasing the cadence, timeout=60s
  mr plugin disable test-schedules --json > /dev/null
  mr plugin enable test-schedules --json > /dev/null
  DUE() { mr plugin schedules test-schedules --json | jq -r '.[] | select(.scheduleId=="manual-only") | .nextDueAt'; }
  RUNS() { mr plugin schedules test-schedules --json | jq -r '.[] | select(.scheduleId=="manual-only") | .runs'; }
  BEFORE=$(DUE)
  mr plugin schedule-run test-schedules manual-only --json | jq -e '.started == true' > /dev/null
  N=0
  for _ in $(seq 1 20); do
    N=$(RUNS); N=${N:-0}
    if [ "$N" -ge 1 ]; then break; fi
    sleep 0.25
  done
  test "$N" -ge 1
  test "$BEFORE" = "$(DUE)"
  mr plugin disable test-schedules --json > /dev/null

  # mr-doctest: a schedule the server has never stored is refused rather than silently ignored
  ! mr plugin schedule-run no-such-plugin no-such-schedule
