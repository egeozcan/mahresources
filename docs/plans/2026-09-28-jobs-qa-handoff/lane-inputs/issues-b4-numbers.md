**N1 (P2). Progress numbers are raw or mislabelled on /jobs and the detail page**
/jobs shows "8051532 / 20971520 bytes" where the drawer says "6.5 MB of 20.0 MB"; similarity counts read "11 B of 11 B"; finished exports show "10 B of 5.0 KB" beside "Completed"; stopped jobs with no known total draw a full bar next to a unitless number; an unknown-size download shows no amount received.
Likely cause: job_template_context.go jobRowProgressBar and jobCenter.js:365 print raw counts instead of jobProgress.js formatAmount; job_queue_bridge.go:984 labels any byte counter "bytes"; :958 projects the export estimate as the total
QA sources: L4-2, L4-5, L4-6, L6-14, L4-7, L5-14, L5-6

**N2 (P2). HLS downloads finish as "0 B of 673 B" and lose their segment history**
While the video is assembled the bar drops from 93 percent back to 0. No speed, time left or graph while running.
Likely cause: downloadJobProgress (job_download_adapter.go ~866) tests TotalSize before PhaseTotal, so the playlist's Content-Length flips the unit to bytes and each unit flip resets the series
QA sources: L1-2, L4-21, L5-19, L6-5

**N3 (P3). Stats wording: "about under 1 s left", "1003 KB of 1.0 MB", "average 0 shares/s", a speed graph that ends in a plunge to 0**
Likely cause: jobs/progress_series.go:92-107 averages from the first point with an amount rather than the job's start
QA sources: L1-16, L4-24, L5-32, L6-28

**N4 (P3). Pills, kind names and command wording are system vocabulary and differ by surface**
Kinds read "remote-download", "plugin-command"; commands read "Pin visible lineage", "Forget replay input"; fields read "Origin: api", "Class: internal".
QA sources: L4-19, L4-26
