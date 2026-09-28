# b4-detail: scope notes and routed follow-ups

- **L7, the detail page half, is yours:** the timeline is not live, stops at 100 events (it fetches `/events?limit=100` once and ignores `nextSequence`), and the header keeps the old phase after the Job finished (the live merge keeps `detail.phase` when the terminal snapshot omits it). b4-list owns the `/jobs` half.
- **X5 (double announcements) is b4-list's on both pages.** Do not change live-region code on the detail page; if your fix needs to, tell me.
- **X10 and X11 touch the drawer template.** Keep to landmarks, labels and small-viewport layout; b4-list and b4-numbers also edit `jobPanel.js`/`jobPanel.tpl` (list reads; progress and states).
- Follow-up (from batch 1): the global search input has no focus indicator in normal mode. Hide the outline only with `focus:outline-hidden`, never `outline-none` (`internal/arch/forced_colors_focus_test.go`).
- Batch 3 facts you will meet: the detail page reads through the preference-fenced `refreshJobPreference` (`rereadJob` calls it), keeps focus with `keepFocusWithin`, and confirms only a command that stops work or cannot be undone (`commandConfirmOptions`). A Job page whose read failed offers Try again and All jobs.
