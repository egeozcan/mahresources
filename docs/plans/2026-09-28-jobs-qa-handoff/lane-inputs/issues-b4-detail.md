**J4 (P2). The detail page shows no accepted, started or finished time, no duration and no expiry**
Elsewhere, times use a different format on each surface, none relative, and the drawer shows none.
Likely cause: templates/displayJob.tpl:30-46 renders three fields
QA sources: L5-5, L4-18, L5-30

**J5 (P2). Job outputs download without a file extension, and the drawer has no download for a finished export**
Every group export saves as "Exported archive" and every summary export as "Job summary export". The drawer offers "Export again" but not the archive; only the detail page links it, as "Open output".
Likely cause: application_context/job_output_access.go:290-307 safeJobOutputFilename uses the display label; jobCenter.js resultOutput returns only entity outputs
QA sources: L5-12, L6-8, L6-9, L4-17

**J8 (P3). Error pages give no way back**
A bad filter in the URL gives "Error 400" with an internal message and no filter form; an unknown or malformed job id gives "job not found" with HTTP 200; an id containing a slash shows a routing error.
QA sources: L5-21, L5-17

**X7 (P2). Output links show "Open output" but are named "Open Exported archive" (SC 2.5.3)**
QA sources: L3-21, L6-10

**X9 (P3). Every detail page has the same document title and two h1 headings, so Retry lands on a page that sounds identical**
QA sources: L3-17, L5-20

**X10 (P3). At 400% zoom the drawer leaves 57 px for the list; at 320 px titles and metric labels are cut off with no way to read them**
QA sources: L3-18, L4-25

**X11 (P3). Semantics polish: duplicate landmarks inside the drawer, an aria-label on a plain div, "Advertised controls" jargon, screen-reader text shown to everyone**
QA sources: L3-16

**X12 (P3). The Jobs shortcut inside global search is refused with a spoken-only message from a region outside that dialog**
QA sources: L3-20

**U11 (P3). Lineage lists bare, identical titles with no relation, state or time, so a retry chain is unreadable**
QA sources: L2-5, L5-24
