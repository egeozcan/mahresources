# A retry creates a new immutable Job

A terminal Job keeps the success, failure, cancellation, or interruption it actually reached. Retry creates a new Job with unchanged replayable input, current authorization and policy validation, a new owner equal to the requester, and a typed `retry-of` link to the finished Job. Repeat does the same for safe reruns of successful work through `repeat-of`; neither operation rewrites its source.

## Considered options

- **Reset the terminal Job and run it in place.** This is compact and matches parts of the current download queue, but it erases which execution produced each outcome, makes old links change meaning, and lets a later success overwrite the failure someone is analyzing.
- **Keep one Job with mutable current state and nested attempt history.** This preserves more evidence, but the public identity still ambiguously names both the original outcome and the latest run. Controls, outputs, retention, ownership, and authorization become attempt-indexed throughout the interface.
- **Create a new linked Job.** Every execution has one identity and one immutable terminal outcome. Lineage supplies the workflow view without making individual rows mutable.

## Consequences

Retry lineage is linear by default: only its latest terminal leaf advertises Retry, and at most one active recovery Job exists in the chain unless a Kind explicitly permits branches. Successful Repeat may branch because each rerun is independent. Internal transient recovery before a Job terminates remains inside that Job and does not create user-visible retry Jobs. Changing input is a fresh submission rather than Retry or Repeat.

During compatibility, a legacy ID is a separate durable handle projecting the current linear Retry leaf. Creating a successor and advancing its handle are atomic; legacy get/list/events/control resolve that leaf under current authorization. Legacy Retry retains its status response and adds the successor's canonical ID. Active leaves refuse another Retry; unsuccessful terminal leaves may be retried again. Canonical UUIDs never redirect or change meaning. The shared SSE route explicitly versions representations: unversioned traffic retains legacy handles, while `version=2` emits canonical UUIDs with separate cursors. This isolates the old mutable-identity contract at the compatibility boundary rather than weakening Job immutability.
