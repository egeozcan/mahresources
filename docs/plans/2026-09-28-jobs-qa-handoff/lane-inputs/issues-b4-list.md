**J1 (P2). Downloads cannot be told apart or found**
Every title is "Download from <host>", and no surface shows the file or path. Search matches the summary's JSON text instead: a lone " matches every job and "scheme" every download, but no URL or file name is found, so the legacy /downloads?URL= link always returns nothing. Screen readers hear the same name on every row.
Likely cause: Titles are reduced to scheme and host on purpose (download-queue.md), a rule written for the query string; jobs/query.go:599-617 searches CAST(summary AS TEXT); server/legacy_job_compatibility.go:57-59 maps URL to search
QA sources: L4-8, L5-1, L3-15

**J2 (P2). /jobs cards print each job's summary as raw JSON**
It is the tallest block on most cards, breaks mid-token, and shows notation such as targets:["owner:1"].
Likely cause: job_template_context.go jobSummaryText (~584) falls back to json.Compact for any non-string summary
QA sources: L4-1, L5-25

**J9 (P3). Filter details**
Back shows the previous page's checkboxes; a leading or trailing space in search matches nothing; the Origin filter is a free-text box for an exact value; with auth off, Owner and Actor are set on some jobs only; /downloads same-day date ranges list nothing.
QA sources: L5-7, L5-8, L5-28, L5-23, L5-27

**J10 (P3). Summaries: no summary figures or summary export in the Job Center, exports do not record their filter, and window=91d gets a misleading error**
QA sources: L5-31, L5-13, L5-22

**U9 (P2). Bulk commands on /jobs act on jobs no longer shown, and Cancel and Retry are never offered in bulk**
Likely cause: A card's destroy() does not run for every card removed by the list refresh, so selectedIds is never pruned
QA sources: L2-11, L2-12

**U10 (P2). Between about 1024 and 1120 px wide, the header pushes the Jobs badges and the settings gear off-screen**
They are clipped and cannot be scrolled to.
QA sources: L4-12

**U14 (P3). Bulk results name jobs by UUID and command key ("1 of 1 job: pin."), and selection says "item"**
QA sources: L2-13, L3-9

**L7 (P2). On /jobs, running rows freeze; on the detail page the timeline is not live and stops at 100 events**
Progress, speed and time left on /jobs move only when some other job changes state. The detail timeline showed only "accepted" after the job finished, has no paging past 100 events, and the header kept the old phase after the job finished.
Likely cause: src/components/jobList.js:223-247 never listens for job-progress; jobCenter.js:481 fetches /events?limit=100 once and ignores nextSequence; the live merge keeps detail.phase when the terminal snapshot omits it
QA sources: L5-9, L2-4, L5-11, L5-10, L4-22

**X5 (P2). On /jobs and the detail page every state change is announced twice, by two page-level live regions**
The /jobs copy also drops the failure reason.
QA sources: L3-11

**X8 (P3). Under reduced motion, /jobs and /downloads cards still pulse their indeterminate bar**
Likely cause: The server-rendered card uses animate-pulse without the motion-safe: variant the other surfaces use
QA sources: L3-13
