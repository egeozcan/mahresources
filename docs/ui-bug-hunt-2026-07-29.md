# Mahresources UI bug hunt — 2026-07-29

Seeded ephemeral dev server on `:8200` (87 resources including real images, multi-version files, non-image and edge-case files; 64 notes with blocks; 87 groups in a 6-level hierarchy; 78 tags; 71 categories; 63 note types; 60 SQL queries; 4 saved MRQL queries; 4 relations; 1 series), driven through a real browser with playwright-cli by 8 parallel explorer agents across 8 areas.

**160 distinct findings.**

## How to read the provenance labels

| Label | Meaning |
|---|---|
| ✅ **VERIFIED** (13) | I personally re-ran this against the live server after the hunt. The exact request and response are quoted on the finding. |
| ⚠️ **DISPUTED** (4) | My own re-check did not reproduce it as described, or could not decide (client-rendered). Treat with suspicion. |
| **verified-run** | Found by an explorer agent and confirmed by a second, independent agent that re-tested it in a fresh browser session. Not re-checked by me. |
| **recovered** | Reconstructed from the transcripts of agents that were killed mid-run. Every entry quotes evidence and step numbers from the transcript, but nobody re-ran it. Treat as a strong lead, not a confirmed defect. |

A note on the verification pass: the second-opinion agents confirmed 97 of 99 findings, which on its own would be too clean to trust. They did however **downgrade the severity of 34** and upgrade 1, so they were not merely agreeing. Of the four claims my own spot-check could decide independently, three held and one did not.

| Severity | Count |
|---|---|
| high | 24 |
| medium | 77 |
| low | 59 |

| Kind | Count |
|---|---|
| bug | 52 |
| ux | 57 |
| design | 20 |
| a11y | 31 |

---

## HIGH (24)

### 1. Background remote download silently discards the user-entered Name; the resource is named after the URL's last path segment ✅ **VERIFIED**

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource/new`

**Steps**

1. Go to /resource/new
2. Type a name into the Name field, e.g. 'hunt-admin NAMED bg download'
3. Paste a remote URL into the URL field, e.g. https://picsum.photos/seed/dl-test2/400/301
4. Tick the 'Download in background (track...)' checkbox that appears
5. Submit the form
6. Open the resource that was created (jobs panel -> 'View resource ->', or /v1/resource?id=<n>)

**Expected:** The created resource keeps the Name the user typed (as it does for a foreground remote download).

**Actual:** The Name is thrown away and replaced with the last path segment of the URL ('300', '301', '302'). The Description the user typed IS preserved, so only Name is lost.

**Evidence:** #78/#79 submitted with Name 'hunt-admin dl test' -> #84/#85 `curl /v1/resource?id=89` returned `"Name": "300"`. #90/#91 submitted with Name 'hunt-admin NAMED bg download' -> resource list showed `[(90, '301'), (89, '300'), ...]`. #92-#95 submitted Name 'hunt-admin desc test' + Description 'HUNTADMIN-DESCRIPTION-MARKER' -> `Name= '302' Desc= 'HUNTADMIN-DESCRIPTION-MARKER'`. Contrast #96/#97, the same form WITHOUT the background checkbox: `Name= 'hunt-admin FOREGROUND name' orig= 'https://picsum.photos/seed/dl-test4/400/303'`.

**Verified by me (re-run against the live server):** `POST /v1/resource/remote?background=true with name='MY CHOSEN NAME' -> resource named '200' (last URL path segment); Description preserved`

**Screenshot:** `jobs-panel-names.png`

### 2. A paused download job can never be cancelled: cancel returns 404 'already finished' and the jobs panel offers only Resume

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (jobs panel) and POST /v1/jobs/cancel?id=<jobId>`

**Steps**

1. Start a large remote download: POST /v1/download/submit with {"URL":"https://proof.ovh.net/files/1Gb.dat"} (or use /resource/new with 'Download in background')
2. Open the jobs panel (button 'Open jobs panel', or Cmd+Shift+D) and click Pause on the running job
3. Try to cancel it: from the panel there is no Cancel button, only Resume
4. Try the API: POST http://localhost:8200/v1/jobs/cancel?id=<jobId>

**Expected:** A paused job can be cancelled and removed from the queue, both from the panel and via the API.

**Actual:** The API rejects it with HTTP 404 `{"error":"job <id> already finished"}` — a paused job is treated as finished by the cancel handler — and the panel renders no Cancel control for a paused job, so a paused 1 GB download is stuck in the queue with no way to abandon it.

**Evidence:** #134/#135 (step described as 'Reproduce cancel-while-paused'): submit -> `{"jobs":[{"id":"22aaa378",...}]}`, then `--- pause --- {"status":"paused"} HTTP 200`, then `--- cancel while paused --- {"error":"job 22aaa378 already finished"} HTTP 404`. Same result earlier for a different job at #126/#127: `{"error":"job 268cbe7f already finished"}` `HTTP 404`. #136/#137 the panel for that job shows only `- generic "Paused" [ref=e214]: ⏸` ... `- button "Resume" [ref=e221]` with no Cancel button (compare the failed job above it, which does get `- button "Retry"`).

**Screenshot:** `jobs-paused-no-cancel.png`

### 3. Mobile nav menu cannot be closed: Escape is a no-op and the full-screen panel covers the hamburger toggle so clicking it is intercepted

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard and http://localhost:8200/notes at 390x844`

**Steps**

1. Resize the browser to a phone viewport (390x844)
2. Load /dashboard
3. Click the hamburger button (aria-label 'Toggle menu')
4. Press Escape
5. Try to click the hamburger again to close the menu
6. Repeat on /notes

**Expected:** Escape closes the mobile menu (and/or the panel has a visible close button), and the hamburger toggle stays clickable to close it.

**Actual:** Escape leaves aria-expanded='true' and the panel fully painted (display block, visibility visible, opacity 1, 390x844). The panel is the top element at the hamburger's coordinates, so the toggle click is intercepted by the panel. The panel contains zero buttons, so there is no close affordance at all — the only escape is following one of the nav links.

**Evidence:** #104/#105 after Escape: `{"exp":"true","panelVis":false}`; #106/#107 shows the panel is actually still painted: `{"exists":true,"display":"block","visibility":"visible","opacity":"1","rect":844,"topEl":"navbar-mobile-panel"}`. #102/#103: `{"hdrTag":"HEADER","hdrZ":"40","hdrPos":"sticky","topElement":"DIV|navbar-mobile-panel"}` — the panel, not the toggle, is hit-testable at (42,18). #110/#111 clicking the toggle hangs on actionability: `- waiting for locator('button[aria-label=\'Toggle menu\']') - locator resolved to <button aria-expanded="true" class="navbar-toggle" ...`. #112/#113 reproduced on /notes: `{"expanded":"true","rect":[0,0,390,844],"topElAtToggle":"navbar-mobile-panel","closeButtons":0}`. #100/#101 confirms the panel has no buttons: `{"buttons":[],"pos":"fixed","z":"39",...}`.

**Screenshot:** `nav-mobile-menu-notoclose.png`

### 4. Global search dialog declares aria-modal=true but does not trap focus — Tab walks into the page behind it while it is open

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (any page — header search dialog)`

**Steps**

1. Load /dashboard
2. Press Cmd+K (or click 'Open search dialog')
3. Type 'arch' and wait for results
4. Press Tab repeatedly and watch document.activeElement

**Expected:** Focus stays inside the modal dialog and cycles between its controls until the dialog is dismissed.

**Actual:** After one or two Tabs focus leaves the dialog and lands on page chrome and page links (Settings button, 'View All →', card links) while the dialog remains open, visible and aria-modal='true'. A screen reader / keyboard user is left interacting with content that is visually behind the overlay.

**Evidence:** Digest a497cf2b #67/#68: four consecutive Tabs give `{"tag":"BUTTON","label":"Settings","inDialog":false}`, `{"tag":"A",...,"inDialog":false}` x3; #69/#70 confirms the dialog is still up: `{"modal":"true","visible":true,"box":[384,86,512,312]}`. Repeated at #73-#76 with 10 results loaded: focus goes 'Clear search' (inDialog true) -> UL (inDialog true) -> ... -> `{"tag":"A","txt":"","inDialog":false}` while `{"open":true,"modal":"true","h":540}`. Independently reproduced in digest abe06580 #40/#41 (`"BUTTON|Settings"`, `"A|View All →"`, `"A|"`, `"A|3000"`, `"A|Default"`) with #42/#43 confirming `{"dlgs":[{"label":"Search","modal":"true","vis":true}...]`.

**Screenshot:** `nav-search-no-focus-trap.png`

### 5. Expanding a tree node throws keyboard focus to <body> — arrow-key navigation dies after every expand

- **Area:** Group tree (accessibility) · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/group/tree?root=65`

**Steps**

1. Go to http://localhost:8200/group/tree?root=65.
2. Focus the "Photography" treeitem (Tab to it, or run `document.querySelector('[role=treeitem]').focus()`).
3. Press ArrowRight to expand it (standard tree keyboard behaviour, and the widget does expand).
4. Inspect document.activeElement.
5. Press ArrowDown — nothing happens.
6. Repeat, this time focusing the expand button ("Expand Photography, 3 children") and pressing Enter.

**Expected:** After expanding, focus stays on the treeitem (or on the toggle button) so the user can keep navigating with ArrowDown/ArrowRight.

**Actual:** Focus is moved to <body>. The subsequent ArrowDown does nothing (activeElement stays BODY, text "Skip to main content"). A keyboard user has to Tab back in from the top of the page after every single expansion — which makes walking the 6-level Landscapes chain require re-entering the tree five times.

**Evidence:** eval after ArrowRight: {"active":"BODY","itemsNow":4,"tabindexes":["0","-1","-1","-1"]}; after Enter on the toggle button: "BODY|". Reproduced 3 times (2x ArrowRight, 1x Enter on button). No console errors.

### 6. "Block types" listbox has no arrow-key navigation — only the first option is reachable by keyboard

- **Area:** Note blocks / block editor (a11y) · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=61`

**Steps**

1. Open a note, click "Edit Blocks".
2. Tab to (or focus) the "+ Add Block" button and press Enter. The listbox opens and focus lands on the first option.
3. Press ArrowDown several times, then ArrowUp, then End.
4. Press Tab.
5. Press Enter to see which block type is inserted.

**Expected:** An element with role=listbox must implement the ARIA listbox keyboard pattern: ArrowDown/ArrowUp move the focused option, Home/End jump to first/last, Enter selects the focused option. A keyboard user must be able to reach all 8 block types.

**Actual:** ArrowDown, ArrowUp, Home and End do nothing — focus never leaves the first option. All options carry tabindex=-1, so Tab does not step through them either: Tab jumps straight back to the "+ Add Block" button and then out of the block editor entirely (next stop was the "New" button in the Resources card). Enter inserts the first option. Combined with the randomised ordering (separate finding), a keyboard-only user can only ever insert whichever block type randomly landed first. (Escape does correctly close the popup and return focus.)

**Evidence:** After opening the listbox: {"vis":true,"activeIdx":0,"active":"📅 Calendar","tabs":"-1,-1,-1,-1,-1,-1,-1,0"}
After ArrowDown x2: focusedIdx still 0.
Second clean run: options [Divider,Gallery,Heading,References,Table,Text,Todos,Calendar], focusedIdx 0, after 2x ArrowDown focusedIdx still 0.
Tab sequence from inside listbox: BUTTON "+ Add Block" -> BUTTON "New" -> BUTTON "0" (all 8 options skipped).
Pressing Enter on the focused "Calendar" option added a calendar block (blocks went from "heading text divider todos references gallery table" to "... table calendar").

**Screenshot:** `notes-blocktypes-order-B.png`

### 7. Note "Share Note" creates a share link that 404s because the share server is not running ✅ **VERIFIED**

- **Area:** Notes / blocks / groups / relations · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/note?id=66 (share panel) -> http://localhost:8200/s/d09fbe678c0ba10c50083a86d1f535a7`

**Steps**

1. Start the server the way this instance was started: ./mahresources -ephemeral -bind-address=:8200 -max-db-connections=4 (no -share-port, i.e. the share feature is disabled).
2. Open any note, e.g. http://localhost:8200/note?id=66.
3. Click the "Share Note" button in the sidebar. A share token/link is created and shown.
4. Copy the share link and open it (http://localhost:8200/s/<token>), or curl it.
5. Also open http://localhost:8200/admin/shares - the share is listed there.

**Expected:** Either the Share Note action is unavailable/disabled with an explanation when no share server is configured, or the generated link resolves to the shared note.

**Actual:** The note is marked shared, the share appears in /admin/shares ("Shared Notes"), but the link returns HTTP 404 on the main server and no share server is listening on any other port, so the link is dead for everyone it is given to.

**Evidence:** #107 probing the token: "main8200: 404 / 8201: 000 fail / 8202: 000 fail / 8181: 000 fail / 8080: 000 fail". #109 confirms the launch args have no share port: "./mahresources -ephemeral -bind-address=:8200 -max-db-connections=4". #113 re-verified: "s-path on main server: 404". #115 opening the share URL in a new tab renders the ordinary app chrome (the app 404 page): "Skip to main content\nDASHBOARD\nNOTES\nRESOURCES\nTAGS\nGROUPS\n". #111 shows the share is nevertheless recorded: "Page Title: Shared Notes - mahresources".

**Verified by me (re-run against the live server):** `POST /v1/note/share -> HTTP 200 {"shareToken":"747a...","shareUrl":"/s/747a..."} while the share server is disabled; GET /s/<token> -> 404`

**Screenshot:** `notes-share-404.png`

### 8. Group tree on mobile pushes deep nodes to negative x where they cannot be scrolled to

- **Area:** Notes / blocks / groups / relations · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/group/tree?root=65`

**Steps**

1. Resize the browser to 390x844 (mobile).
2. Open http://localhost:8200/group/tree?root=65 (Photography).
3. Expand the deepest expandable node repeatedly (Photography -> Landscapes -> Landscapes / Sub-level 1 -> Sub-level 2 -> Sub-level 3), i.e. 4-5 levels.
4. Try to bring the deep nodes into view by scrolling the tree horizontally (drag/scroll left).

**Expected:** Every node in the expanded tree can be reached by scrolling the tree container horizontally.

**Actual:** From level 3 down, node boxes are laid out at negative x (18px to the left of the viewport origin, 34px left of the tree container's own left edge). The container only scrolls right (scrollWidth 424 vs clientWidth 358) and scrollLeft cannot go below 0, so the left-clipped nodes are permanently unreachable and partly invisible.

**Evidence:** #55 mobile measurement: "{\"docSW\":390,\"docCW\":390,\"bodySW\":390,\"treeChart\":{\"sw\":424,\"cw\":358}}". #61 confirms scrolling cannot reach them: "{\"sl\":0,\"sw\":424,\"cw\":358,\"afterNeg\":0,\"boxX\":-18.3125,\"chartX\":16}" (setting scrollLeft=-500 leaves scrollLeft at 0). #63 reproduces after a reload and re-expansion: "[{\"n\":\"Photography\",\"x\":144},{\"n\":\"Landscapes\",\"x\":21},{\"n\":\"Landscapes / Sub-level 1\",\"x\":-18},{\"n\":\"Landscapes / Sub-level 2\",\"x\":-18},{\"n\":\"Landscapes / Sub-level 3\",\"x\":-18}". Desktop is fine for comparison (#45: tree-chart sw 824 = cw 824).

**Screenshot:** `tree-mobile-clipped-left.png`

### 9. Every bulk operation on /resources/details pops a false "Bulk operation failed" alert and leaves the list stale — although the operation succeeded

- **Area:** Resource list — bulk operations (details view) · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Go to http://localhost:8200/resources/details (no filters needed).
2. Tick one row's checkbox.
3. Click 'Add Tag' in the bulk toolbar, pick any tag from the autocompleter, click 'Add'.
4. A native browser alert appears: "Bulk operation failed: Could not find refreshed list".
5. Dismiss it — the table row is unchanged (stale Updated timestamp), the checkbox is still ticked and the editor is still open with the value, so the natural reaction is to retry.
6. Verify server-side that it DID work: curl -s 'http://localhost:8200/v1/resource?id=<id>' → the tag is present.
7. Repeat with 'Remove Tag' and with '+ Add Field' (addMeta) on /resources/details — the same alert appears every time.
8. Do the identical operations on /resources (Thumbnails view) — no alert, and the grid refreshes correctly.

**Expected:** The bulk operation reports success and the list refreshes to show the new tag/meta (as it does in the Thumbnails view).

**Actual:** On /resources/details the server-side mutation succeeds (POST /v1/resources/addTags, /removeTags, /addMeta all apply), but the client-side refresh step cannot find the list container, so it throws a native alert("Bulk operation failed: Could not find refreshed list") and leaves the stale table + still-checked rows + still-open editor on screen. Bulk Delete on the same page is NOT affected (it refreshes fine), so only the non-delete actions break.

**Evidence:** playwright-cli output after clicking the addTags submit on /resources/details:
### Modal state
- ["alert" dialog with message "Bulk operation failed: Could not find refreshed list"]

Reproduced 4x on /resources/details: addTags (id 105+106), addTags (id 104), addTags (id 108), addTags (id 109 on unfiltered /resources/details), removeTags (id 105), addMeta (id 109).
After each, the API confirmed success, e.g.
  109 hunt-reslist bulk T {'hrlKey2': 'v2'}
  108 hunt-reslist bulk S ['hunt-reslist-tag']
Same actions on /resources (Thumbnails): no dialog, grid refreshed, e.g. removeTags → 109 hunt-reslist bulk T []
JS console: 0 errors (the failure is only surfaced through alert()).

**Screenshot:** `reslist-v2-details-stale-after-bulk.png`

### 10. HTTP 500 from /v1/resource/recalculateDimensions on an SVG resource ✅ **VERIFIED**

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=97 (any image/svg+xml resource)`

**Steps**

1. Upload an SVG as a resource (e.g. a 120x60 <svg> with a rect and a circle) at /resource/new.
2. Open its resource page at /resource?id=<id>.
3. Click the sidebar button 'Recalculate Dimensions'.

**Expected:** The SVG's intrinsic width/height are parsed and stored, or the app reports a clean, handled message that SVG dimensions are not supported.

**Actual:** The POST returns HTTP 500 and the browser lands on a full-page error screen titled 'Error 500' reading 'An error has occurred / encountered errors during dimension calculation / Go back'. The resource stays at 0x0.

**Evidence:** #179 before the click: 'dims 0 0 image/svg+xml'. #181 console: '[ERROR] Failed to load resource: the server responded with a status of 500 (Internal Server Error) @ http://localhost:8200/v1/resource/recalculateDimensions?redirect=%2Fresource%3Fid%3D97:0'. #183: url 'http://localhost:8200/v1/resource/recalculateDimensions?redirect=%2Fresource%3Fid%3D97', title 'Error 500', body 'An error has occurred\nencountered errors during dimension calculation\n\nGo back'. #189, driven through the UI a second time: title 'Error 500' again on the same URL. The same button on PNG resource 88 succeeds (#193 returns to 'http://localhost:8200/resource?id=88'). The endpoint is POST-only, so a plain GET returns 404 (#189: 'recalc97: 404') - reproduce by clicking the button, not by curl.

**Verified by me (re-run against the live server):** `POST /v1/resource/recalculateDimensions (SVG id=97) -> HTTP 500 {"error":"encountered errors during dimension calculation"}`

**Screenshot:** `recalc-500-svg.png`

### 11. Rotate 90 Degrees on an SVG resource returns HTTP 500 'image: unknown format' ✅ **VERIFIED**

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=97 (any image/svg+xml resource)`

**Steps**

1. Open an SVG resource page at /resource?id=<id>.
2. Click the sidebar button 'Rotate' (Rotate 90 Degrees).

**Expected:** Either the rotate control is not offered for vector formats, or rotation is handled (or refused) without a 500.

**Actual:** The browser is navigated to /v1/resources/rotate and shown a full-page 'Error 500' reading 'An error has occurred / image: unknown format / Go back'. The image decoder is handed an SVG it cannot decode. The resource is left unchanged (still 0x0).

**Evidence:** #195: url 'http://localhost:8200/v1/resources/rotate', title 'Error 500', txt 'An error has occurred\nimage: unknown format\n\nGo back'; the follow-up API read prints 'image/svg+xml 0 0 178'. The rotate/crop/recalculate controls are rendered for SVG resources - #178 and #194 both located them by accessible name on /resource?id=97 and both reached the server.

**Verified by me (re-run against the live server):** `POST /v1/resources/rotate {"ID":97,"Angle":90} -> HTTP 500 {"error":"image: unknown format"}`

### 12. Rotate silently transcodes PNG to JPEG, destroying transparency and inflating file size ✅ **VERIFIED**

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Upload a PNG at /resource/new (e.g. 400x300, 123089 bytes).
2. On the resource page, click 'Rotate' (Rotate 90 Degrees).
3. Inspect the resource: GET /v1/resource?id=<id>.
4. Repeat with an RGBA PNG that has a transparent background.

**Expected:** Rotation is format-preserving: a PNG stays a PNG and keeps its alpha channel.

**Actual:** The stored file becomes a JPEG (ContentType image/jpeg, .jpg on disk) with no alpha. For the transparent PNG the file also grew 7.5x (1194 -> 9001 bytes) because the transparent region is flattened and re-encoded as JPEG. Crop on the same resource preserves PNG, so the conversion is specific to rotate.

**Evidence:** #59 after rotating the uploaded PNG: ID 88, Name 'hunt-resdetail test image A', OriginalName 'hunt_a.png', ContentType 'image/jpeg', Location '/resources/0a/7b/5e/0a7b5e49a94b3b987fa8e89d4b53d19ace735372.jpg', FileSize 37082. #77 version list: '89 v2 37082 image/jpeg 300 x 400 'Rotated 90 degrees'' against '88 v1 123089 image/png 400 x 300 'Initial version''. #147 a second rotate: 'res: image/jpeg 80 100 3158 /resources/3d/7a/b0/3d7ab06cd150e3ef197bb661ab87'. #153 with the RGBA transparent PNG: 'before: image/png 1194' then 'after: image/jpeg 9001 /resources/42/8b/66/428b666507d53b53402aed72f66e2f2c312ee6d4.jpg'. Contrast #145 after a crop: 'image/png 100 80 285 99' - still PNG.

**Verified by me (re-run against the live server):** `resource 71 before: image/png / .png / 1807 bytes; after rotate: image/jpeg / .jpg / 1289 bytes`

**Screenshot:** `rotate-png-to-jpeg.png`

### 13. Details table is 2147px wide inside an 822px scroller that keyboard users cannot scroll

- **Area:** Resources — list & detail · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open /resources/details at 1280px wide.
2. Tab through the controls in a table row and watch the horizontal scroll position of .detail-table-wrap.
3. Check the wrapper for tabindex / role / aria-label.

**Expected:** A horizontally scrollable region is keyboard operable - either tabindex="0" with role="region" and a label (so arrow keys scroll it), or focusing a cell control scrolls it into view.

**Actual:** The scroll container has no tabindex, no role and no aria-label, and focusing each focusable element in a row never moves scrollLeft off 0 while 1325px of the table remain off-screen. A keyboard-only user cannot reach the right-hand columns. WCAG 2.1.1 (Keyboard).

**Evidence:** #95: '{"tableSW":2147,"tableCW":2147,"chain":[{"tag":"DIV","cls":"detail-table-wrap","ox":"auto","cw":822,"sw":2147},{"tag":"MAIN","cls":"main","ox":"visible","cw":824,"sw":824}, ...]}'. #97: '{"tabindex":null,"role":null,"label":null}'. #99: '{"maxScroll":1325,"res":[{"tag":"INPUT","txt":"","scrollLeft":0},{"tag":"A","txt":"92","scrollLeft":0},{"tag":"A","txt":"hunt-admin FOREGROUN","scrollLeft":0}, ...]}'. Also present on mobile, #189: '{"docSW":390,"iw":390,"wrapCW":356,"wrapSW":2005}'. The header set is wide by design, #89: '["SELECT","ID","NAME","PREVIEW","SIZE","CREATED","UPDATED","ORIGINAL NAME","ORIG..."]'.

**Screenshot:** `reslist-details-scrolled-right.png`

### 14. Details-view row checkboxes have no accessible name

- **Area:** Resources — list & detail · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open /resources/details.
2. Inspect the row checkboxes (.detail-table-checkbox) for aria-label and for an associated <label>.

**Expected:** Each row checkbox is named after its row, exactly as the grid does ('Select <resource name>'), so a screen-reader user knows what they are selecting.

**Actual:** Every visible row checkbox has aria-label null and no associated label element; the only text is a single sr-only 'Select' in the column header, which does not name the individual rows. WCAG 4.1.2 (Name, Role, Value) Level A failure on the primary bulk-selection control.

**Evidence:** #255: '[{"cls":"","aria":null,"vis":false,"checked":true},{"cls":"","aria":null,"vis":false,"checked":true},{"cls":"focus:ring-1 focus:ring-amber-600 h-3.5 ","aria":null,"vis":true,"checked":false} ...]'. #263: '{"parentTag":"TD","parentCls":"","parentBox":{"w":30,"h":45},"hasLabel":false,"cs":"0px"}'. #251 shows the header's only text: '{"selHeader":"<span class=\"sr-only\">Select</span>","rowCbs":4}'. Contrast the grid view, where each checkbox is properly named (#115: '- checkbox "Select hunt-reslist throwaway B" [ref=e142]', '- checkbox "Select hunt-reslist throwaway A" [ref=e158]').

**Screenshot:** `reslist-details-tiny-checkbox.png`

### 15. Inline description edit is silently discarded when you leave the field with the keyboard

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/tag?id=85`

**Steps**

1. Open a tag detail page (e.g. /tag?id=85).
2. Double-click the description block (title="Double-click to edit") to enter edit mode.
3. Type a new description into the textarea.
4. Press Tab (do not click anywhere on the page).
5. Reload the page.

**Expected:** Leaving the editor by keyboard commits the edit, the same way clicking away does; or the UI offers a Save control.

**Actual:** Tab moves focus out of the region (to the 'Open jobs panel' button) without saving, and the typed text is gone after reload. The save is bound only to @click.away, and there is no Save button anywhere in the editor.

**Evidence:** #30/#32 show the handler: `<textarea @click.away=" ... fetch(descriptionEditUrl, { method: 'POST', body: formData })">` - the only save trigger is click.away. #36 fills 'EDITED VIA KEYBOARD' then presses Tab and reports focus is `"BUTTON|Open jobs panel"`. #38 reloads and reads the description back: `"hunt-tax test description"` (the original value). The agent repeated the sequence at #40 and captured #42 'unsaved keyboard edit - no save button anywhere'.

**Screenshot:** `tags-inline-desc-no-save.png`

### 16. Tag merge with nothing selected confirms, then dumps the user on a raw HTTP 400 error page

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/tag?id=85`

**Steps**

1. Open a tag detail page (e.g. /tag?id=85).
2. Do not select anything in the 'Tags To Merge' picker.
3. Click the 'Merge' button in the sidebar.
4. Accept the confirm dialog.

**Expected:** The Merge button is disabled while the selection is empty, or the click is a no-op with an inline message.

**Actual:** A destructive confirm ('Selected tags will be deleted and merged to hunt-tax-tag1. Are you sure?') is shown even with an empty selection; accepting navigates the whole page to /v1/tags/merge?redirect=... which renders an unstyled 'An error has occurred / one or more losers required / Go back' page. The user loses the page context.

**Evidence:** #50/#51 `["confirm" dialog with message "Selected tags will be deleted and merged to hunt-tax-tag1. Are you sure?"]` with nothing selected. #52/#53 `- Page Title: Error 400` and `[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/tags/merge?redirect=%2Ftag%3Fid%3D85`. Reproduced at #56-#59: `"http://localhost:8200/v1/tags/merge?redirect=%2Ftag%3Fid%3D85 :: An error has occurred\none or more losers required\n\nGo back"`.

**Screenshot:** `tags-merge-empty-400.png`

### 17. Invalid Meta JSON Schema is accepted and persisted with no lint, no client validation and no server rejection ✅ **VERIFIED**

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=73`

**Steps**

1. Open a category edit page, e.g. /category/edit?id=73.
2. Focus the 'Meta JSON Schema' editor and type: { not valid json ][
3. Wait a few seconds for lint to settle, then click Save.
4. Reopen /category/edit?id=73.

**Expected:** Either a lint marker in the schema editor, or a rejected save with an error naming the parse failure - the same JSON parser that the Visual Editor uses already produces a precise message.

**Actual:** No lint marker appears, the save succeeds and redirects to the category page, and the invalid text is stored verbatim and reloaded into the editor. Groups in that category then render with no error and no indication their schema is broken.

**Evidence:** #112/#113 editor state after typing: `{"header":"HUNT-TAX-HEADER-CONTENT-DO-NOT-LOSE","schema":"{ not valid json ][]}"}`. #114/#115 lint query returns `[]`; #116/#117 after a 3s wait `{"lint":0, ...}`. #124/#125 Save navigates to `http://localhost:8200/category?id=73` (no error). #130/#131 reopening the edit page reads back `"schema":"{ not valid json ][]}"`. #132-#137: a group created in that category renders /group?id=89 and /group/edit?id=89 with `Total messages: 0 (Errors: 0, Warnings: 0)` - the broken schema is silently ignored downstream. The parser error the UI *could* have shown is quoted at #151: `Expected property name or '}' in JSON at position 2 (line 1 column 3)`.

**Verified by me (re-run against the live server):** `POST /v1/category with MetaSchema='{not valid json' -> HTTP 200, stored verbatim; /group/new?categoryId=75 still 200`

**Screenshot:** `cat-invalid-json-schema.png`

### 18. Visual Editor opens completely blank when the stored schema is invalid; the parse error is hidden behind the Raw JSON tab

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=73`

**Steps**

1. Open /category/edit?id=73 whose Meta JSON Schema is invalid JSON.
2. Click 'Visual Editor'.
3. Look at the default 'Edit Schema' tab.
4. Click the 'Raw JSON' tab.

**Expected:** The dialog opens on an explanatory state ('This schema is not valid JSON: <parse error>') rather than an empty panel.

**Actual:** The 'Edit Schema' tab is entirely empty - no fields, no empty state, no error - and 'Apply Schema' is rendered in a washed-out state. Only after switching to 'Raw JSON' does the parse error appear.

**Evidence:** #142/#143 dialog text: `"Visual Editor\nMeta JSON Schema\nEdit Schema\nPreview Form\nRaw JSON\n\u00d7\nCancel\nApply Schema\nSection Visibil..."` (no error). #150/#151 after clicking Raw JSON: `"Meta JSON Schema\nEdit Schema\nPreview Form\nRaw JSON\n\u00d7\n\nExpected property name or '}' in JSON at position 2 (line 1 column 3)\n\nCancel\nA..."`. The screenshot at #146 shows the blank Edit Schema panel (verified: the modal body is empty white space between the tab strip and the Cancel / Apply Schema footer).

**Screenshot:** `cat-visualeditor-blank-invalidjson.png`

### 19. Category / note type / resource category edit pages overflow a 390px viewport and the overflow is clipped and unreachable

- **Area:** Taxonomy / templates / MRQL · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=72 (also /noteType/edit?id=1, /resourceCategory/edit?id=1)`

**Steps**

1. Resize the browser to 390x844 (iPhone-class viewport).
2. Open /category/edit?id=72 and scroll down to the 'Custom Templates' fieldset.
3. Try to reach the right-hand side of any CodeMirror editor or the per-slot 'Generate' button.

**Expected:** The form reflows to the viewport, or the wide parts (CodeMirror) scroll inside their own container.

**Actual:** The page is ~4.5x wider than the viewport (body scrollWidth 1778 vs 390) while html/body have overflow-x:hidden, so the document cannot be scrolled horizontally at all - everything past x=390 is permanently unreachable. The CodeMirror editors are laid out at 948px, the 'Custom Templates' fieldset at 984px, and the 'Generate' buttons start at x=373 (only 17px visible). Typing a long line makes it worse (fieldset grows to 1185px).

**Evidence:** #198/#199 `{"docW":390,"winW":390,"bodyW":1778}`. #202/#203 `{"bodyOverflowX":"hidden","htmlOverflowX":"hidden","canScroll":"390/390"}`. #200/#201 overflowing elements e.g. `FIELDSET... right=483 w=467`. #220/#221 `["Reuse & Presets w=467 right=483","Custom Templates w=984 right=1000","Section Visibility w=358 right=374"]`. #226/#227 `["cm-editor w=948 sw=948","cm-scroller w=948 sw=948","cm-content w=907 sw=907", ...]`. #210/#211 `{"count":9,"first":{"left":373,"right":466,"width":93},"viewport":390}`. #214/#215 same on the other taxonomy editors: `== noteType/edit?id=1 "maxRight=483 vw=390"` / `== resourceCategory/edit?id=1 "maxRight=483 vw=390"`. #228/#229 after typing one long line: `"Custom Templates w=1185"`. (The /templatePartial/edit page is clean: #212/#213 returns `[]`.)

### 20. The categories list offers a 'Custom Property' sort that the server rejects with Error 400 'invalid sort column' ✅ **VERIFIED**

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/categories`

**Steps**

1. Open /categories.
2. In the SORT row choose column 'Custom Property'.
3. Type a property name (e.g. 'camera') into the property field.
4. Click 'Apply Filters'.

**Expected:** The sort applies (it does on /tags), or the option is not offered on this list.

**Actual:** The page navigates to /categories?SortBy=meta->>'camera' desc and renders a full-page 'Error 400 / invalid sort column'. The identical SortBy value works on /tags.

**Evidence:** #248/#249 the Apply navigates to `"http://localhost:8200/categories?SortBy=meta-%3E...`. #252/#253 direct comparison: /tags with `SortBy=meta-%3E%3E%27camera%27+desc` -> `"no error"` and a normal result list (`["hunt-tax-tag1","tutorial","portrait", ...]`), while /categories with the same SortBy -> `"Error 400\ninvalid sort column"`.

**Verified by me (re-run against the live server):** `GET /categories?sortBy=__meta__ -> HTTP 400 {"error":"invalid sort column"}`

**Screenshot:** `categories-custom-sort-400.png`

### 21. Failed inline rename gives sighted users no feedback at all - the only error is a 1x1 clipped live region

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/tag?id=85`

**Steps**

1. Open /tag?id=85 and click the 'Edit name' pencil.
2. Clear the field (or type only spaces) and press Enter.
3. Watch the page.

**Expected:** A visible inline error next to the field, and the field stays in edit mode with the user's text.

**Actual:** POST /v1/tag/editName returns 400, the editor closes, the old name is restored, and nothing visible changes. The 'Could not save name' message exists only in a visually-hidden live region (1x1 px, clip rect(0,0,0,0)), so it reaches screen readers but nobody else.

**Evidence:** #326/#327 `[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/tag/editName?id=85:0` and `[ERROR] Error posting data: Error: Server responded with 400 at .../public/dist/main.js`. #330/#331 measuring the element that holds the message: `{"cls":"","w":1,"h":1,"clip":"rect(0px, 0px, 0px, 0px)","pos":"absolute"}`. Reproduced with whitespace-only input at #332/#333: same two console errors plus `{"name":"hunt-tax-tag1-renamed","visibleAlerts":["false","Could not save name"]}`.

**Screenshot:** `tag-inline-name-empty-nofeedback.png`

### 22. MRQL autocomplete shows descriptions instead of field names, including four indistinguishable 'relation count' rows

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open /mrql.
2. Type: type = resource AND 
3. Press Ctrl+Space and read the completion list.

**Expected:** Each row shows the token that will be inserted (fts, tags.count, groups.count, notes.count, ...), with the human description as secondary detail.

**Actual:** Several rows show the server's `label` (a description) rather than its `value`: 'any ancestor group', 'any descendant group', 'entity type filter', 'full-text search', 'perceptual similarity', and four consecutive rows all reading 'relation count' with nothing to tell them apart. Picking one inserts something whose text never appeared in the list, and filtering matches the description text rather than the field name.

**Evidence:** #98/#99 the list: `"any ancestor group\nany descendant group\ncategory\ncontentType\ncreated\ndescription\nentity type filter\nfileSize\nfull-text search\ngroup\ngroups\nguid\nhash\nheight\nid\nname\nnotes\noriginalLocation\noriginalName\nowner\nperceptual similarity\nrelation count\nrelation count\nrelation count\nrelation count\nsimilarImages\ntags\nupdated\nwidth"`. #102/#103 the DOM confirms the description is the completion label: `<span class="cm-completionLabel">any ancestor group`. #118/#119 indexed: `"0:any ancestor group","1:any descendant group","2:category",...,"6:entity type filter","8:full-text search"`. #120/#121 activating row 22 inserts `"type = resource AND groups.count"` - a string that was never displayed. #116/#117 the server payload it is built from: `{"suggestions":[{"value":"count","type":"field","label":"relation count"}]}`. #114/#115 typing 'anc' filters on the description: `"any ancestor group\nany descendant group\nrelation count\nrelation count\nrelation count\nrelation count"`.

### 23. MRQL results region can show a previous query's entity and cards after a new query runs

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open /mrql and run a few different queries, including a group query and an Explain on a resource query.
2. Select all in the editor, type: type = note LIMIT 3
3. Press Cmd+Enter and inspect the Query plan and Query results regions immediately afterwards.

**Expected:** After a run, the plan and the results both describe the query that just ran.

**Actual:** The two panels disagreed with each other and with the executed query. Immediately after the run the Query plan heading still read 'Explain (resource)' from an earlier query; a moment later the plan showed the correct 'Explain (note)' / SELECT * FROM `notes` WHERE 1 = 1 LIMIT 3, while the results region alongside it read 'Entity: group' and listed three group cards (Engineering Backend / Engineering Frontend / Engineering DevOps, 'Group for ... related items').

**Evidence:** #66/#67, taken 1.5s after running `type = note LIMIT 3`: `- region "Query plan" ... - heading "Explain (resource)" [level=2]`. The screenshot captured at #68 shows, on one screen: `Explain (note)` with `SELECT * FROM \`notes\` WHERE 1 = 1 LIMIT 3`, and beside it `Results (3 items)` ... `Entity: group` with three group cards. Console had no failed render at that point (#83 `Total messages: 1`, the one 400 being the earlier bogusField query), so the stale content was not a failed request left on screen.

**Screenshot:** `mrql-stale-explain.png`

### 24. SQL query results table crushes every column to a few pixels instead of scrolling, while half the page sits empty

- **Area:** Taxonomy / templates / MRQL · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/query?id=60`

**Steps**

1. Open /query?id=60 ('Categories Ordered By Name', SELECT * FROM categories ORDER BY name ASC).
2. Click Run.
3. Read the results table at a 1280px-wide window.

**Expected:** The table scrolls horizontally inside its container (or uses the full page width), so headers and values stay readable.

**Actual:** 16 columns are squeezed into a 790px table inside an 824px container with overflow-x: visible, so nothing scrolls: header cells wrap to one character per line ('c/r/e/a/t/e/d/_/b/y/_/u/s/e/r/_/i/d' running vertically for ~20 lines) and values break mid-word. Meanwhile the right ~35% of the 1280px page is empty.

**Evidence:** #160/#161 `{"tableW":790,"tableSW":789,"parentTag":"DIV","parentCls":"query-results output mt-2","parentW":824,"parentSW":822,"overflowX":"visible","gpOverflow":"visible"}`. #164/#165 `{"tableW":790,"colWidths":[77,36,35,34,69,36,36,35,44,69,69,32,69,37,35,76],"parentOverflowX":"visible"}` - reproduced after reloading the page and re-running. The screenshot shows the vertical one-letter-per-line headers.

**Screenshot:** `query-run-results.png`

---

## MEDIUM (77)

### 25. Mobile: groups list is buried ~1745px below the always-expanded filter sidebar

- **Area:** /groups list (mobile) · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/groups`

**Steps**

1. Resize to 390x844.
2. Go to http://localhost:8200/groups.
3. Observe what is on screen; scroll down and count how far you must go to see the first group card.
4. Reload and re-measure.

**Expected:** On a narrow viewport the filter panel is collapsed behind a toggle (or moved below the results) so the list itself is the first thing the user sees.

**Actual:** The full sidebar — a 20-chip tag cloud, the Sort block and ~15 filter fields — is stacked above the results. The first group card sits at y=1745, i.e. more than two full viewport heights of scrolling before any group is visible. The same pattern on a group detail page pushes the group's own Notes/Sub-Groups/Resources to y=980–1039, below the Merge and Clone tools.

**Evidence:** eval at 390x844 on /groups (twice): first article offsetTop = 1745, viewport height 844, document height 13508. On /group?id=65: main offsetTop = 980; on /group?id=1: 1039.

**Screenshot:** `groups-list-mobile.png`

### 26. Log entry "Details" field renders a raw Go struct (`<types.JSON Value>`) — empty box on screen plus an Alpine JS error ⚠️ **DISPUTED**

- **Area:** Admin / Logs · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/log?id=658`

**Steps**

1. Go to http://localhost:8200/admin/settings and change any runtime setting (e.g. "Docs site base URL") and click Save — this writes a log entry that has a non-empty Details payload.
2. Go to http://localhost:8200/logs and click the timestamp link of the newest `runtime_setting` row (e.g. /log?id=658 or /log?id=657).
3. Look at the "Details" row of the Log Entry Details table.
4. Open the browser console.

**Expected:** The Details row shows the log entry's JSON payload, pretty-printed by the page's `x-init` JSON.parse/stringify. No console errors.

**Actual:** The server emits the literal text `<types.JSON Value>` inside the `<pre>`, which the browser parses as an unknown HTML element. The `<pre>` therefore has empty textContent, the Details row renders as an empty grey bar (no data at all), and Alpine throws `Alpine Expression Error: Unexpected end of JSON input`. Reproduced on log ids 657 and 658; log entries without Details do not render the row at all, so every entry that HAS details is broken.

**Evidence:** Console: `[WARNING] Alpine Expression Error: Unexpected end of JSON input / Expression: "(() => { const parsed = JSON.parse($el.textContent); $el.textContent = JSON.stringify(parsed, null, 2); })()" ... SyntaxError: Unexpected end of JSON input at JSON.parse`.
Raw HTML from `curl -s http://localhost:8200/log?id=658 | grep -A3 '>Details<'`:
`<pre class="bg-stone-100 ..." x-data x-init="(() => { const parsed = JSON.parse($el.textContent); ... })()"><types.JSON Value></pre>`
Same output for /log?id=657; ids 650/653/654/655/656/600/500/400 have no Details row and no error.

**My re-check:** UNDECIDED — no `types.JSON Value` string appears in the server HTML for /logs; if it happens it is client-rendered and my curl check cannot see it.

**Screenshot:** `log-658-details-broken.png`

### 27. Logs filter dropdowns are missing values that actually occur, and the Entity Type control desyncs from the applied filter

- **Area:** Admin / Logs · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/logs?EntityType=runtime_setting`

**Steps**

1. Open http://localhost:8200/logs. Open the "Entity Type" dropdown — options are: All Types, Tag, Category, Note, Note Type, Resource, Resource Category, Resource Version, Group, Series, Query, Relation, Relation Type, Plugin. Open "Action" — options are: All Actions, Create, Update, Delete, System, Progress, Plugin.
2. Note that the log table itself contains rows with Entity `runtime_setting` and Action `reset` (created by any /admin/settings change) — neither value is offered in the dropdowns, so the settings audit trail cannot be filtered from the UI.
3. Navigate directly to http://localhost:8200/logs?EntityType=runtime_setting.
4. Compare the result table to the "Entity Type" dropdown.
5. Click "Apply Filters".

**Expected:** (a) The filter dropdowns offer every entity type / action the log actually records. (b) When a filter is active the control reflects it, so re-submitting the form keeps it.

**Actual:** (a) `runtime_setting`, `templatePartial` and `mrql_query` are missing from Entity Type; `reset` is missing from Action. (b) At /logs?EntityType=runtime_setting the table correctly shows 6 filtered rows, but the Entity Type select reads "All Types" (value `""`). Clicking "Apply Filters" therefore silently discards the filter and returns to all logs. With a supported value (?EntityType=resource) the select correctly shows "resource", so the desync is specific to values the dropdown does not know.

**Evidence:** `document.querySelector('select[name=EntityType]').value` → `""` while `document.querySelectorAll('tbody tr').length` → 6 at /logs?EntityType=runtime_setting.
Same eval at /logs?EntityType=resource → value `"resource"`, 50 rows.
Select options via eval: EntityType = ["","tag","category","note","noteType","resource","resourceCategory","resource_version","group","series","query","relation","relationType","plugin"]; Action = ["","create","update","delete","system","progress","plugin"].
Actual distinct values in the log (from /v1/logs?limit=1000): actions {update, create, delete, reset}; types {resource, group, note, query, resource_version, runtime_setting, category, tag, templatePartial, mrql_query}.

**Screenshot:** `hunt-admin-logs-filter-desync.png`

### 28. 'Copy from existing' dropdown is silently truncated to 51 entries per group — 22 categories and 16 note types cannot be copied from

- **Area:** Category / Note Type template authoring — Reuse & Presets · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/category/new`

**Steps**

1. Go to http://localhost:8200/category/new (repeat on /noteType/new — same result). 2. Open the 'Copy from existing' select. 3. Count the entries per optgroup: Category = 51, Resource Category = 2, Note Type = 51. 4. Verify the real totals: /categories.json?page=1 returns 50 and ?page=2 returns 23 (73 categories); /noteTypes.json?page=1 returns 50 and ?page=2 returns 17 (67 note types). 5. Search the option list for 'Person', 'Vendor' or 'Project' (all real categories visible on /categories?page=2) — none of them are present.

**Expected:** Every existing category / note type should be offered as a copy source, or the control should page/search and tell the user the list is partial.

**Actual:** Only 51 of 73 categories and 51 of 67 note types appear. The remaining ones (Partner, Customer, Vendor, Contractor, Employee, Specification, Prototype, Mockup, Diagram, Spreadsheet, Presentation, Report, Document, Portfolio, Program, Initiative, Project, Team, Department, Business, Location, Person …) can never be used as a copy source and nothing indicates the list is cut off. The backing calls /v1/categories and /v1/note/noteTypes return only 50 rows (adding ?maxResults=500 still returns 50).

**Evidence:** eval on /category/new: {"groups":["Category (this type)=51","Resource Category=2","Note Type=51"],"hasPerson":false,"hasVendor":false,"total":102}. Same result on /noteType/new. fetch('/categories.json?page=2') -> 23 more categories including "Person","Vendor","Project".

**Screenshot:** `category-preset-apply-overwrite.png`

### 29. Live preview is a blank 384px box with no empty state when the category has no groups yet — a brand-new category can never preview its templates

- **Area:** Category template authoring — Live preview · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/category/edit?id=74`

**Steps**

1. Create a category with no groups (e.g. /category/new, name it, Save). 2. Open /category/edit?id=<that category>. 3. Type any template into 'Custom Header', e.g. `<b>Hello [property path="Name"]</b>`. 4. Look at the Live preview panel: it is a large empty white box, permanently. 5. Click the 'Preview against' box and type any real group name, e.g. 'Photography' — no options are offered. 6. Check the network tab: the lookup is /v1/groups?name=Photography&maxResults=8&categoryId=<this category>, i.e. scoped to groups already in this category, of which there are none. 7. Same on /category/new.

**Expected:** When there is no entity to preview against, the panel should say so ('No groups in this category yet — pick a group or save first'), or fall back to a synthetic sample entity so the author can still see their template render.

**Actual:** The preview iframe's srcdoc is empty (length 0) and the panel renders as a 384px-tall blank box with no message, no spinner, no explanation. Because the 'Preview against' search is filtered by categoryId, an author creating a brand-new category — exactly when the preview is most useful — can never make it render anything. Reproduced on both /category/new and /category/edit?id=74.

**Evidence:** eval: {"srcdocLen":0,"previewBoxH":384}. Network request captured by playwright-cli requests: 'GET http://localhost:8200/v1/groups?name=Photography&maxResults=8&categoryId=74 => [200] OK' returning no options.

**Screenshot:** `category-preview-no-entity.png`

### 30. Escape from the global search dialog drops focus onto <body> instead of restoring it to the button that opened it

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (any page — header search dialog)`

**Steps**

1. Load /dashboard
2. Click the 'Open search dialog' button in the header (focus is now on that button)
3. Press Escape to dismiss the dialog
4. Inspect document.activeElement

**Expected:** Focus returns to the 'Open search dialog' button (this is what the jobs panel does correctly).

**Actual:** activeElement is BODY, so keyboard users are dumped back to the top of the document and must Tab from scratch.

**Evidence:** Digest abe06580 #34/#35: `{"active":"BODY|site bg-stone-50","dialogOpen":true}` (dialog element present but `display:"none"` per #36/#37). #38/#39 opened via the button (`"Search"` focused), Escape -> `{"tag":"BODY","label":null,...}`; #40/#41 'attempt2:' -> `"BODY"`. Digest a497cf2b #61/#62 `{"active":"<body class=\"site bg-stone-50\">...","dialogVisible":false}`, #63/#64 `"{\"tag\":\"BODY\",\"label\":null}"`, and #65/#66 ('Repro #2 focus loss after search Escape') `{"tag":"BODY","label":null}`. Contrast the jobs panel, digest a7e14caa #60/#61: `{"dialog":false,"active":"Open jobs panel"}`.

**Screenshot:** `nav-search-focus-escape.png`

### 31. Global search shows 'No results found' for one-character queries even though the API returns matches

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (search dialog) vs GET /v1/search?q=a`

**Steps**

1. Open the search dialog (Cmd+K)
2. Type a single character, e.g. 'a' (or 'P')
3. Wait ~2s and read the dialog
4. Compare with curl -G http://localhost:8200/v1/search --data-urlencode 'q=a'

**Expected:** Either show the matches the API can return for a single character, or tell the user to keep typing ('type at least 2 characters'). Not a false 'no results'.

**Actual:** The client never issues the request for a 1-character query (no /v1/search call in the network log) and renders the definitive-sounding empty state 'No results found for "a" / Try a different search term', while the API returns 14 hits for the same query. The same happens for a whitespace-only query.

**Evidence:** Digest abe06580 #176/#177: `" No results found for \"a\" Try a different search term ..."`; #178/#179 the network log after typing 'ar' shows only `29. [GET] http://localhost:8200/v1/search?q=ar&limit=15 => [200] OK` (no request was made for 'a'), while the API for q=a returns `14` results (`category Person`, `resource A resource with a really extremely long ...`). #182/#183 ('Reproduce single-char no-results bug') on /notes with 'P': `" No results found for \"P\" ..."` then `API for P: total 1`. #172/#173 whitespace query: `[   ] => {"n":0,"txt":" No results found for \" \" ..."}`.

**Screenshot:** `nav-search-1char-noresults.png`

### 32. Global search silently truncates to 15 results with no 'more results' indicator and no full search-results page

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (search dialog); /search returns 404`

**Steps**

1. Open the search dialog (Cmd+K)
2. Type a broad term such as 'Group'
3. Count the rendered options and look for any 'showing N of M' / 'see all results' affordance
4. Try to open a full results page at http://localhost:8200/search

**Expected:** Show how many matches exist beyond the visible list and give a way to see them all.

**Actual:** The dialog renders 15 options while the backend reports total=50 (and itself caps the payload at 20). Nothing in the dialog says results were truncated, and there is no /search page to fall back to (404).

**Evidence:** Digest a497cf2b #89/#90: `{"opts":15,"txt":"roup Group for Design UX related items 🔍 Nested Groups Query 🔍 Groups Last 30 Days Query No results found for \"Group\" Try a different search term Start typing to search Search across all you..."}` (the trailing text is the hidden empty-state markup, not a count). #87/#88: `Group -> total 50 len 20`, `note -> total 50 len 20`, `Seeded -> total 21 len 20`. #85/#86: `/search -> 404`. The live request seen in digest abe06580 #179 is `/v1/search?q=ar&limit=15`.

### 33. Pressing Enter in a runtime-settings field does nothing — no save, no feedback — because the fields are not inside a form

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/settings`

**Steps**

1. Open /admin/settings
2. Type a new value into a setting field, e.g. 'Hash aHash threshold' -> 6 (or 'MRQL page query budget' -> 150)
3. Press Enter
4. Check GET /v1/admin/settings for that key

**Expected:** Enter either submits that setting (like the Save button) or the UI makes clear that Save must be clicked.

**Actual:** Nothing happens. The value stays in the box looking edited, but the setting is unchanged and not overridden; the control is not inside a <form>, so Enter has nothing to submit and no message is shown.

**Evidence:** #258-#261: filled 'Hash aHash threshold' with 6 and pressed Enter -> `hash_ahash_threshold current= 5 overridden= False`, and `{"inForm":false}`. Reproduced on a second field at #264/#265 ('Reproduce Enter no-op on second setting field'): filled 'MRQL page query budget' with 150, pressed Enter -> `mrql_page_query_budget current= 200 overridden= False`. (The Save button does work: #32-#43 setting MRQL default LIMIT to 3 took effect — `Results (3 items) ... Default limit applied (3 rows)`.)

**Screenshot:** `settings-enter-noop.png`

### 34. Admin 'create user' validation failures replace the page with a bare error screen at /v1/users and wipe everything the admin typed

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/users`

**Steps**

1. Open /admin/users
2. Fill username 'hunt-admin-tmpuser', password 'abc', role 'guest', leave scope group empty
3. Submit the form
4. Click 'Go back' and inspect the form fields
5. Repeat with a valid scope group id but a 3-character password

**Expected:** Validation errors render inline on the users page with the entered values preserved (password excepted).

**Actual:** The browser navigates to /v1/users and shows a generic 'An error has occurred' page containing only the raw message; going back leaves username and password empty (only the role select survives), so the admin retypes everything. For a guest with no scope the message is 'scope group does not exist', which does not say that a guest REQUIRES a scope group.

**Evidence:** #226/#227: `- heading "An error has occurred" [level=1]` / `- code: scope group does not exist`. #228/#229 after 'Go back': `{"url":"http://localhost:8200/admin/users","u":"","p":"","role":"guest"}`. #230/#231 second case: `{"url":"http://localhost:8200/v1/users","txt":"An error has occurred\npassword must be at least 8 characters\n\nGo back"}`.

**Screenshot:** `users-create-error.png`

### 35. Export group picker: choosing a group with the keyboard throws focus back to <body>

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/export`

**Steps**

1. Open /admin/export
2. Focus the 'Search groups to add' box and type 'Portraits' (or 'Family')
3. Press Tab to reach the first result button
4. Press Enter to add the group
5. Inspect document.activeElement

**Expected:** After adding a chip, focus returns to the search box (or moves to the new chip) so the keyboard user can keep going.

**Actual:** The group is added but focus is reset to BODY, so the user must Tab from the top of the page to add a second group.

**Evidence:** #282/#283: focused element before Enter `"BUTTON:Family"`, after Enter `{"sel":1,"active":"BODY:Skip to main content"}`. #284/#285 ('Reproduce focus loss in export picker') with 'Portraits': `"BUTTON:Portraits"` then `{"active":"BODY","chips":["Remove Portraits"]}`.

### 36. Export group picker is an autocomplete with no combobox ARIA (no role, aria-expanded, aria-controls or aria-activedescendant)

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/export`

**Steps**

1. Open /admin/export
2. Focus the 'Search groups to add' input and type 'Family'
3. Inspect the input's ARIA attributes
4. Press ArrowDown and observe that focus does not enter the result list (only Tab reaches it)

**Expected:** Same pattern as the global search box, which exposes role=combobox, aria-expanded, aria-controls=search-results, aria-activedescendant and aria-autocomplete=list, so screen readers announce that results appeared and which one is active.

**Actual:** Every one of those attributes is null on the export picker, so the appearance of results is not announced and there is no active-descendant model; results are only reachable by Tab.

**Evidence:** #278/#279: `{"role":null,"expanded":null,"controls":null,"activedesc":null,"autocomplete":null}` for the input whose accessible name is `"Search groups to add"`. Compare the global search input, digest abe06580 #28/#29: `{"aad":"search-result-2","ac":"search-results","role":"combobox","exp":"true","label":"Search"}`. #280-#283 show ArrowDown left focus in the input and only Tab reached `BUTTON:Family`.

### 37. /logs: entity types that exist in the data are missing from the EntityType dropdown, and clicking 'Apply Filters' silently wipes a URL-supplied filter

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/logs?EntityType=runtime_setting`

**Steps**

1. Open http://localhost:8200/logs?EntityType=runtime_setting — the table correctly filters to 2 rows
2. Look at the EntityType select: it shows the empty option, not 'runtime_setting'
3. Click 'Apply Filters'
4. Observe the URL and the row count

**Expected:** The dropdown offers every entity type that actually appears in the audit log, and Apply Filters preserves the current filter.

**Actual:** 'runtime_setting' (and other real values such as mrql_query, templatePartial, resource_version, relation) are not options, so the select falls back to '' and Apply Filters posts an empty filter — the URL becomes ?Level=&Action=&EntityType=&... and the result set jumps back to all 50 rows. A supported value like 'resource' round-trips correctly, which confirms the mechanism.

**Evidence:** #198/#199: `{"rows":2,"sel":"","txt":"TIME\tLEVEL\tACTION\tENTITY\tMESSAGE..."}` — filter applied, select empty. #200/#201 after clicking Apply Filters: `{"url":"http://localhost:8200/logs?Level=&Action=&EntityType=&EntityID=&Message=&CreatedBefore=&CreatedAfter=","rows":50}`. #202/#203 with a supported value: `{"sel":"resource","rows":50}`. The real value set is from #196/#197: `entityTypes: ['category', 'group', 'mrql_query', 'note', 'relation', 'resource', 'resource_version', 'runtime_setting', 'tag', 'templatePartial']`.

### 38. GET /v1/resources?seriesId=N appears to ignore the series filter ✅ **VERIFIED**

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/v1/resources?seriesId=1`

**Steps**

1. curl 'http://localhost:8200/v1/resources?seriesId=1' and count the items
2. curl 'http://localhost:8200/v1/resources' and count the items
3. Compare

**Expected:** The filtered call returns only resources belonging to series 1.

**Actual:** Both calls return the same 50 items, i.e. the filter has no effect on the result set (ResourceQueryBase does declare `SeriesId uint`).

**Evidence:** #182/#183: `--- count with seriesId filter vs none --- 50 / 50`, and `seriesId= None series= None` for /v1/resource?id=96. #184/#185: `models/query_models/resource_query.go:19: SeriesId uint`. Caveat for whoever verifies: 50 is also the default page size, so confirm against a series with fewer than 50 members before filing.

**Verified by me (re-run against the live server):** `GET /v1/resources?seriesId=1 and ?seriesId=99999 both return the same 50 rows as the unfiltered list`

### 39. Keyboard focus on /admin/settings controls computes outline-style: none, unlike the same interaction on /admin/users

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/settings`

**Steps**

1. Open /admin/settings
2. Focus the 'Max import size' field, then press Tab a few times
3. At each stop read getComputedStyle(document.activeElement).outline
4. Do the same on /admin/users starting from the username field

**Expected:** Every keyboard-focused control shows a visible focus indicator, as the user-form inputs do.

**Actual:** On the settings page the focused reason input computes '2px none rgba(0, 0, 0, 0)' and the focused Save button computes '1px none rgb(0, 95, 204)' — outline-style none, i.e. no outline is painted. The equivalent controls on /admin/users compute '2px solid' with a visible ring. Worth confirming a box-shadow ring is not standing in: the settings probe only captured the outline property.

**Evidence:** #306/#307 (/admin/settings): `{"tag":"INPUT","label":"Reason for Max import size","outline":"2px none rgba(0, 0, 0, 0)"}` and `{"tag":"BUTTON","label":"Save","outline":"1px none rgb(0, 95, 204)"}`. #276/#277 (/admin/users): `{"tag":"INPUT","name":"displayName","outline":"2px solid","ring":"rgb(255, 255, 255) 0px 0px 0px 0px, oklc..."}`.

### 40. Jobs panel lists jobs oldest-first with no auto-scroll, so a download you just started is off-screen; no way to clear finished jobs

- **Area:** Download queue / Jobs cockpit · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/dashboard`

**Steps**

1. Open http://localhost:8200/resource/new.
2. Fill Name = `hunt-admin big dl`, URL = `https://proof.ovh.net/files/100Mb.dat`, tick "Download in background (track progress in download cockpit)", click Save.
3. You are redirected to /resources with no confirmation that anything was queued.
4. Click the orange floating "Open jobs panel" button (bottom right).
5. Look at what is visible in the panel without scrolling.
6. Look for any control to clear/dismiss finished or failed jobs.

**Expected:** The job you just started is visible immediately (newest first, or the list auto-scrolls to the active job), and finished/failed jobs can be dismissed so the panel does not grow unbounded.

**Actual:** The panel renders oldest-first and opens scrolled to the top, showing completed jobs from hours ago. The running download is the LAST item in the list, roughly 1340px below the fold; there is no badge or hint that an active job exists further down. There is also no "clear completed" / dismiss control anywhere in the panel — after a few sessions it had accumulated 20 permanent entries (completed downloads, completed exports, failed imports).

**Evidence:** Immediately after submitting, eval on the panel: `{"n":20, "scrollTop":0, "scrollHeight":1973, "clientHeight":634, "last":[..., "⬇ 100Mb.dat Downloading https://proof.ovh.net/files/100Mb.dat 959.8 KB / 100 MB (0.9%) 255.8 KB/s Pause Cancel"]}`.
Panel header/controls eval: `allBtns` = ["Close jobs panel", "Retry", "Resume"] — no clear/dismiss action; `nJobs` = 20.

**Screenshot:** `jobs-panel-newest-at-bottom.png`

### 41. A paused download loses its progress readout entirely and offers no way to cancel it

- **Area:** Download queue / Jobs cockpit · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/dashboard`

**Steps**

1. Queue a slow background download (e.g. /resource/new with URL `https://proof.ovh.net/files/100Mb.dat` and "Download in background" ticked).
2. Open the jobs panel (orange floating button) and scroll to the bottom to find the running job. While running it shows `5.1 MB / 100 MB (5.1%) 219.6 KB/s` plus "Pause" and "Cancel".
3. Click "Pause".
4. Inspect the row: what information and which buttons remain?

**Expected:** A paused job keeps showing how far it got (bytes / total / percent / progress bar) and can still be cancelled, so it does not sit in the queue forever.

**Actual:** On pause the entire progress readout disappears — the row collapses to `⏸ 100Mb.dat  Paused  https://proof.ovh.net/files/100Mb.dat  Resume`. The bytes, percentage, speed and progress bar are all gone, and the "Cancel" button is removed too, so the only escape is Resume (and then Cancel). A pre-existing paused job from another session (`1Gb.dat`) shows the identical state, so paused jobs accumulate in the panel with no way to dismiss them.

**Evidence:** Running row eval: `"⬇ 100Mb.dat Downloading https://proof.ovh.net/files/100Mb.dat 5.1 MB / 100 MB (5.1%) 219.6 KB/s Pause Cancel"`, buttons `["Pause","Cancel"]`.
After clicking Pause: `{"btns":["Resume"], "txt":"⏸ 100Mb.dat Paused https://proof.ovh.net/files/100Mb.dat Resume"}`.
Independent second instance in the same panel: `{"txt":"⏸ 1Gb.dat Paused https://proof.ovh.net/files/1Gb.dat Resume", "btns":["Resume"]}`.
(Resume and Cancel themselves work correctly: after Resume the row returns to `Pause`/`Cancel` and continues from 5.2% → 6.9%; Cancel yields `⛔ ... Cancelled ... Retry`.)

**Screenshot:** `jobs-download-paused-no-progress.png`

### 42. Group compare marks CREATED/UPDATED as changed while rendering identical values, and the timestamp wraps mid-number

- **Area:** Group compare · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/group/compare?g1=65&g2=70`

**Steps**

1. Go to http://localhost:8200/group/compare?g1=65&g2=70 (Photography vs Landscapes).
2. Read the summary banner and then the CREATED cell in the Metadata section.
3. Repeat with any freshly-cloned pair, e.g. clone a group and compare the original with the clone — the CREATED and UPDATED cells are the only two "changes" reported.

**Expected:** A field is only flagged as changed when the two rendered values differ; and the rendered value should not be broken across lines inside a time token.

**Actual:** CREATED renders "Jul 29, 2026 11:27 → Jul 29, 2026 11:27" — visually identical — but is highlighted with the red "changed" border and counted in "Core 2 changed" / "Groups differ". Because timestamps are formatted to the minute but compared at full precision, any two distinct groups will almost always report at least these two phantom differences. Additionally the cell text wraps inside the clock value, printing "Jul 29, 2026 1" on one line and "2:12" on the next.

**Evidence:** g1=93&g2=92 (a clone and its source): summary "Groups differ | Sections 1 changed | Core 2 changed", CREATED "Jul 29, 2026 12:12 → Jul 29, 2026 12:12", UPDATED "Jul 29, 2026 12:12 → Jul 29, 2026 12:12". g1=65&g2=70: "CREATED | Jul 29, 2026 11:27 → Jul 29, 2026 11:27". Control: /group/compare?g1=92&g2=92 correctly says "Groups are identical".

**Screenshot:** `group-compare-identical-timestamps.png`

### 43. "Show in Tree" silently fails to reveal groups deeper than 4 levels — target node is absent and nothing is highlighted

- **Area:** Group tree · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/group/tree?containing=82`

**Steps**

1. Open http://localhost:8200/group?id=82 ("Landscapes / Sub-level 4", the 6th level of the Photography > Landscapes chain).
2. Click the "Show in Tree" link in the left sidebar.
3. You land on /group/tree?containing=82.
4. Look at the rendered tree and at the sidebar text "HIGHLIGHTED PATH — Teal nodes show the path from root to the selected group."
5. Repeat with ?containing=81 (Sub-level 3, level 5) — same result. Then with ?containing=79 (Sub-level 1, level 3) — that one works and is highlighted.

**Expected:** The tree expands down to the requested group and highlights it (as it correctly does for containing=70 and containing=79), or at minimum tells the user the group is deeper than the rendered slice.

**Actual:** The tree always renders the same fixed 4-level slice (Photography > Landscapes > Sub-level 1 > Sub-level 2) regardless of which descendant was requested. "Landscapes / Sub-level 4" does not appear anywhere on the page, no node carries the `tree-node-box--focused` class, and the sidebar still claims teal nodes show the path to the selected group. The teal path just stops at Sub-level 2 with a collapsed "▼ 1" expander.

**Evidence:** eval on /group/tree?containing=82: {"url":"http://localhost:8200/group/tree?containing=82","hasTarget":false,"focused":0}. Per-id comparison: containing=79 -> focused=["Landscapes / Sub-level 1Media"]; containing=81 -> focused=[]; containing=82 -> focused=[]. Deepest aria-level rendered is 4 for all four ids (79/80/81/82).

**Screenshot:** `tree-show-in-tree-missing-target.png`

### 44. Tree root-selection list is silently capped at 50 groups — root groups with children (Travel 2026, Work Projects) are unreachable ⚠️ **DISPUTED**

- **Area:** Group tree · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/group/tree`

**Steps**

1. Go to http://localhost:8200/group/tree (no query string).
2. Scroll to the bottom of the "Select a root group to view its tree" list.
3. Count the entries and note the last one.
4. Search for "Travel 2026" or "Work Projects" in the list — neither is present, although both are root groups (breadcrumb on /group?id=67 and /group?id=68 is just Groups > name) and both have 2 sub-groups.
5. Reload and repeat.

**Expected:** All root groups are listed, or the list is paginated / searchable / labelled "showing 50 of N".

**Actual:** Exactly 50 entries, alphabetical, ending abruptly at "Project Beta AddOn". No pagination, no search, no truncation notice. Every root group sorting after "Project Beta" (Research Papers, Reykjavik Studio, Street Photography, Travel 2026, Vendor *, Work Projects, …) cannot be picked as a tree root from this page — /group/tree?root=67 and ?root=68 work fine if you know the id.

**Evidence:** eval on /group/tree (twice): {"count":50,"last":"Project Beta AddOn","hasTravel":false,"hasWork":false}; document.querySelector('nav[aria-label=Pagination]') === null. /group?id=67 subgroups: ["Iceland March","Japan Autumn"]; /group?id=68 subgroups: ["Web App Redesign","API Migration"]. Total groups = 91 (50 on page 1 + 41 on page 2 of /groups).

**My re-check:** UNDECIDED — the tree root list is client-rendered, so my server-side check could not confirm the 50-item cap.

**Screenshot:** `tree-rootlist-truncated.png`

### 45. Breadcrumb "Groups" link on /group/tree uses a relative href and 404s (/group/groups) ⚠️ **DISPUTED**

- **Area:** Group tree / navigation · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/group/tree?root=65`

**Steps**

1. Go to http://localhost:8200/group/tree?root=65 (or reach it via a group page's "Show in Tree" link, e.g. /group/tree?containing=70).
2. Click the first breadcrumb item "Groups" (the home-icon crumb at the top left).
3. Repeat on /group/tree?containing=70.

**Expected:** Navigates to /groups (the Groups list). The identical breadcrumb on /group?id=65 does exactly that.

**Actual:** Navigates to http://localhost:8200/group/groups and renders "404 Not Found — Page not found". The anchor is `<a href="groups">` (relative), which resolves correctly under /group?id=... but resolves to /group/groups under /group/tree.

**Evidence:** eval: breadcrumb anchor .href === "http://localhost:8200/group/groups"; after click: {"url":"http://localhost:8200/group/groups","title":"404 Not Found - mahresources"}. Reproduced from both ?root=65 and ?containing=70.

**My re-check:** DID NOT REPRODUCE — the rendered /group/tree breadcrumb href is the absolute `/groups`. `/group/groups` does 404, but nothing links there in the server HTML.

**Screenshot:** `tree-breadcrumb-404.png`

### 46. MRQL Explain panel is never invalidated — it keeps showing the plan of a previous, different query while Results show another

- **Area:** MRQL editor (/mrql) — Explain · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open http://localhost:8200/mrql (fresh load).
2. Click into the query editor and type: type = note LIMIT 3
3. Press Cmd+Shift+Enter (Explain). The panel shows "Explain (note)" with SELECT * FROM `notes` WHERE 1 = 1 LIMIT 3.
4. Select all in the editor and type: type = group LIMIT 3
5. Press Cmd+Enter (Run).
6. Look at the Explain panel above the Results panel.

**Expected:** Running a different query should clear (or refresh) the Explain panel, or the panel should be visibly marked as belonging to a different query, so the plan and the results can never disagree.

**Actual:** The Explain panel still reads "Explain (note) / SELECT * FROM `notes` WHERE 1 = 1 LIMIT 3" while the Results panel directly beneath reads "Results (3 items) / Entity: group" and lists groups. The reverse also happens: run a GROUP BY query, then Explain a different query, and the fresh plan sits on top of stale result rows. Nothing distinguishes which query each panel belongs to.

**Evidence:** Reproduced 3 times. eval of [aria-label="Query plan"] returned "Explain (note) ... SELECT * FROM `notes` WHERE 1 = 1 LIMIT 3" while [aria-label="Query results"] returned "Results (3 items) ... Entity: group ... Engineering Backend".

**Screenshot:** `mrql-stale-explain.png`

### 47. "+ Add Block" menu lists the 8 block types in a different (randomly rotated) order on every page load

- **Area:** Note blocks / block editor · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=61`

**Steps**

1. Open http://localhost:8200/note?id=61 (any note).
2. Click "Edit Blocks", then click "+ Add Block" and note the order of the 8 options.
3. Reload the page (F5) and repeat step 2. Repeat 3-4 times.
4. Alternatively hit the backing endpoint directly: `for i in $(seq 1 10); do curl -s http://localhost:8200/v1/note/block/types | python3 -c "import json,sys;print(','.join(t['type'] for t in json.load(sys.stdin)))"; done`

**Expected:** The block-type menu has a stable, deterministic order (alphabetical or a curated order), so users build muscle memory and "the 2nd item" always means the same block type.

**Actual:** The order is a random rotation of the list on every request. 10 consecutive API calls produced 4 distinct orders. In the browser, two consecutive loads of the same note gave first-option "Text" and first-option "Divider". The list appears to be built by ranging over a Go map, whose single-bucket iteration starts at a random offset.

**Evidence:** 10x GET /v1/note/block/types returned:
   4  text,todos,calendar,divider,gallery,heading,references,table
   3  gallery,heading,references,table,text,todos,calendar,divider
   2  divider,gallery,heading,references,table,text,todos,calendar
   1  references,table,text,todos,calendar,divider,gallery,heading
Browser DOM read of [role=listbox][aria-label="Block types"] options across reloads:
  load 1: Text,Todos,Calendar,Divider,Gallery,Heading,References,Table
  load 2: Divider,Gallery,Heading,References,Table,Text,Todos,Calendar
Console: 0 errors.

**Screenshot:** `notes-blocktypes-order-A.png`

### 48. Block editor: todo-item text inputs have no accessible name, and item/chip remove buttons are 10x24 / 16x16 px named only "×"

- **Area:** Note blocks / block editor (a11y) · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=62`

**Steps**

1. Open http://localhost:8200/note?id=62 (API v2 Migration Plan) and click "Edit Blocks".
2. Look at the TODOS card: two text inputs holding "Dual-write enabled" and "Shadow-read comparison green", each followed by a small red "×".
3. Inspect those inputs for aria-label / aria-labelledby / title / <label for> / wrapping <label> — all absent, and there is no placeholder either.
4. Measure the "×" buttons in the todos block and in the REFERENCES chips.

**Expected:** Every text input needs a programmatic name (aria-label like "To-do item 1" or a visually-hidden label). Destructive icon buttons need a descriptive name ("Remove to-do item: Dual-write enabled") and a target of at least 24x24 CSS px (WCAG 2.2 SC 2.5.8).

**Actual:** The todo-item inputs expose no accessible name at all — a screen reader announces only "edit text". The remove buttons are named "×" (no indication of what is removed) and are far under the minimum target size: todo-item "×" measures 10x24 px, reference-chip "×" measures 16x16 px.

**Evidence:** Input audit: [{"val":"Dual-write enabled","ariaLabel":null,"ariaLabelledby":null,"title":null,"id":"","hasLabel":false,"inLabel":false},{"val":"Shadow-read comparison gr",...same...}]
Button audit: [{"name":"×","w":16,"h":16},{"name":"×","w":16,"h":16},{"name":"×","w":16,"h":16},{"name":"×","w":10,"h":24},{"name":"×","w":10,"h":24}]
Accessibility snapshot shows them as unnamed: `textbox [ref=e230]: Dual-write enabled` and `button "×" [ref=e231]`.

**Screenshot:** `notes-todo-input-unlabeled.png`

### 49. Calendar block: all 35 day cells are click-only <div>s — not focusable, no role, unusable by keyboard

- **Area:** Note blocks / calendar block (a11y) · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=67`

**Steps**

1. Open a note, click "Edit Blocks", "+ Add Block" -> Calendar, then click "Done".
2. Scroll to the calendar grid. Hover a day cell — the cursor becomes a pointer and the cell highlights, so it is clearly interactive (it opens the "New Event" dialog pre-filled with that date).
3. Try to reach a day cell with Tab, or activate one with Enter/Space.
4. Inspect: document.querySelectorAll('div') filtered on @click containing 'openEventModalForDay'.

**Expected:** An interactive day cell should be a <button> (or have role="button" + tabindex="0" + key handlers) so keyboard and screen-reader users can create an event on a specific day. Today only the mouse can do it.

**Actual:** All 35 day cells are bare <div class="... cursor-pointer hover:bg-amber-50" @click="openEventModalForDay(day.date)"> with no role and no tabindex. 0 of 35 are focusable, 0 have a role. Tab skips the whole grid. The only keyboard-reachable way to add an event is the generic "+ Add Event" button, which cannot target a specific day.

**Evidence:** eval on the calendar block: {"n":35,"anyFocusable":0,"anyRole":0}
Parent element HTML: <div class="bg-white min-h-[80px] p-1 relative cursor-pointer hover:bg-amber-50 transition-colors" @click="openEventModalForDay(day.date)" ...> with role=null, tabindex=null.
Accessibility snapshot renders them as: generic [ref=e136] [cursor=pointer]: "28" ... (35 of them, none exposed as buttons).

**Screenshot:** `notes-calendar-block.png`

### 50. Table-block column labels and cell values are silently lost when you navigate away (blur-only save, no autosave, no warning)

- **Area:** Note blocks / table & todos blocks · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=67`

**Steps**

1. Create a note, open it, click "Edit Blocks", "+ Add Block" -> Table, then "+ Add column".
2. Click into the "Column label" input and type a new label (e.g. "Revenue"). Do NOT click anywhere else.
3. While the input still has focus, click a top-nav link ("Notes" or "Dashboard").
4. Go back to the note and re-open "Edit Blocks", or check GET /v1/note/blocks?noteId=<id>.

**Expected:** Either the edit autosaves while typing (as the Heading and Text blocks do), or the user is warned before leaving with unsaved changes. Typed content must not vanish silently.

**Actual:** The typed label is discarded. The server still holds the previous value; no toast, no confirm-before-leave, no error. Reproduced twice with different strings. Root cause is visible in the DOM: the table column input is `<input x-model="col.label" @blur="saveContent()">` — blur only — whereas heading is `<input x-model="text" @input="onInput()" @blur="save()">`. Todo-item label inputs have the same blur-only binding, so they lose edits the same way.

**Evidence:** Run 1: DOM value after typing = "LostEditTestRevenueNew Column"; after clicking nav link /notes, GET /v1/note/blocks?noteId=67 -> {"columns":[{"label":"RevenueNew Column"}]}.
Run 2: DOM value after typing = "SECOND-LOSS-TESTRevenueNew Column"; after clicking nav link /dashboard, server still {"label":"RevenueNew Column"}.
Binding audit of block-editor inputs:
  Heading text...  @input=onInput()      @blur=save()
  Enter text...    @input=onBlockInput() @blur=saveBlock()
  Column label     @input=(none)         @blur=saveContent()
  todo item label  @input=(none)         @blur=saveContent()
Console: 0 errors (fails silently).

**Screenshot:** `notes-blockedit-62.png`

### 51. Note sharing hands out share URLs that are dead: the share server never started and nothing in the UI or the app log says so

- **Area:** Note sharing · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=61`

**Steps**

1. Create a note (or open any note) at http://localhost:8200/note?id=<id>.
2. In the right sidebar under "Sharing", click "Share Note". A token path such as /s/747a77ebc33760be5bc43947ecf03ded appears, with a "Shared" badge.
3. Open http://localhost:8200/admin/settings, expand "Boot-only settings" — it reports Share port 8383.
4. Try the link on the main port:  curl -i http://localhost:8200/s/747a77ebc33760be5bc43947ecf03ded  -> 404.
5. Try it on the advertised share port: curl -i http://localhost:8383/s/747a77ebc33760be5bc43947ecf03ded  -> 404 (that port is held by an unrelated process).
6. Check the listeners of the running instance: lsof -nP -iTCP -sTCP:LISTEN -a -p <pid of ./mahresources -bind-address=:8200>  -> only *:8200.
7. Check http://localhost:8200/logs — no entry at all about the share server failing to bind.

**Expected:** If the share server could not start (SHARE_PORT=8383 in .env was already in use), the failure should be surfaced: an error in /logs, and the note UI should either hide/disable "Share Note" or say the share server is not running. A user must not be told a note is "Shared" when the link can never resolve.

**Actual:** The instance silently continued without a share listener. /admin/settings still advertises "Share port 8383". The note page happily mints share tokens, shows a "Shared" badge, and the only warning shown misdiagnoses the problem — it talks about SHARE_PUBLIC_URL ("append the token path to your server's public URL manually") when the real problem is that no server serves /s/<token> at all. /admin/shares lists the note as reachable ("1 shared note"). Nothing in the application log mentions it.

**Evidence:** lsof for the :8200 instance (pid 80997): only `TCP *:8200 (LISTEN)`.
lsof -nP -iTCP:8383 -sTCP:LISTEN -> `mahresour 37929 ... 127.0.0.1:8383 (LISTEN)` (a different, unrelated mahresources process).
curl -o /dev/null -w '%{http_code}' http://localhost:8200/s/747a77ebc33760be5bc43947ecf03ded -> 404
curl http://localhost:8383/s/747a77ebc33760be5bc43947ecf03ded -> "Not found"
/admin/settings boot-only table: "Bind address :8200", "Share port 8383".
.env contains SHARE_PORT=8383, SHARE_BIND_ADDRESS=127.0.0.1.
/v1/logs shows "Created share token" but no share-server error.

**Screenshot:** `notes-admin-shareport.png`

### 52. Group tree root picker silently caps at 50 roots, so later groups cannot be reached from the UI

- **Area:** Notes / blocks / groups / relations · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/group/tree`

**Steps**

1. Open http://localhost:8200/group/tree (the "Select a root group to view its tree:" picker).
2. Count the entries and look for "Work Projects" or "Travel 2026" (both exist as groups).
3. Look for pagination or a search box in the picker.
4. Navigate directly to http://localhost:8200/group/tree?root=68 to confirm the page exists.

**Expected:** The root picker either lists all root groups, or paginates/searches/says "showing 50 of N".

**Actual:** Exactly 50 alphabetically-first roots are listed, with no pagination and no indication that the list is truncated. Groups later in the alphabet (Work Projects id=68, Travel 2026) are simply absent from the picker even though their tree pages work when hit directly.

**Evidence:** #57: "{\"liCount\":50,\"hasWork\":false,\"hasTravel\":false, ...}". #61 after a reload: "{\"liCount\":50,\"pagination\":false,\"hasWork\":false,\"hasTravel\":false,\"mainTextStart\":\"Select a root group to view its tree:...\"}" and "--- direct root=68 works? --- \"Tree: Work Projects - mahresources\"". #59 confirms the group exists via API: '[{"ID":68,...,"Name":"Work Projects"...}]'. #67 confirms the cap in code: "group_template_context.go-418- roots, err := context.GetGroupTreeRoots(50)".

### 53. Inline description edit cannot be committed with the keyboard: Tab leaves the editor open and the change unsaved

- **Area:** Notes / blocks / groups / relations · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/notes`

**Steps**

1. Open http://localhost:8200/notes.
2. Double-click a note card's description to enter inline edit (the textarea has aria-label "Edit description").
3. Type some text.
4. Press Tab to move focus out of the textarea.
5. Check the note via the API (GET /v1/note?id=66) and look at the card.

**Expected:** Tabbing out of the inline editor either commits the edit (like clicking away does) or cancels it; the editor should not stay open holding uncommitted text.

**Actual:** Tab moves focus to the next control but the editor stays open with the edited text and nothing is sent to the server. Only a mouse click elsewhere (@click.away) commits, so a keyboard-only user has no way to save and the typed text is silently stranded.

**Evidence:** #215 after typing and pressing Tab: "{\"activeNow\":\"INPUT\",\"firstCard\":\"H | hunt-notes block playground v2\",\"textareas\":1}" and #217 the textarea still holds "CHANGED BY TAB TESThunt-notes description text". Second run #221: "{\"stillEditing\":1,\"val\":\"SECOND RUN hunt-notes description text\"}" with "server desc: hunt-notes description text" (unchanged). #223 shows a mouse click away does save: "server desc: SECOND RUN hunt-notes description text". The handler is mouse-only - #211: '<textarea @click.away="... fetch(descriptionEditUrl, { method: POST ..." @keydown.escape="editing = false"'.

**Screenshot:** `notes-inline-edit-tab.png`

### 54. Notes list shows no empty state when a filter matches nothing

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/notes?mrql=name%20~%20%22*zzzznope*%22`

**Steps**

1. Open http://localhost:8200/notes.
2. Apply a filter that matches nothing - e.g. tick "Shared Only" and click Apply Filters, or navigate to /notes?mrql=name ~ "*zzzznope*".
3. Look at the main content area.

**Expected:** A "no notes match this filter" empty state (ideally with a way to clear the filter).

**Actual:** The list area is completely blank below the view toggles and filter bar - no message, no count, no hint that the filter is the reason. The user cannot tell an empty result from a broken page.

**Evidence:** #137: "{\"n\":0,\"main\":\"List\\nTimeline\\nSelect All\\nFilter these notes with an MRQL expression\\nFilter\\nEdit in MRQL editor\"}" - zero articles and the entire main text is just the chrome. The agent ran this as its second zero-result filter specifically to confirm the missing empty state (#136 description: "Second zero-result filter to confirm missing empty state"), after the "Shared Only" filter at #134.

**Screenshot:** `notes-empty-state-missing2.png`

### 55. Calendar block controls overflow off-screen on mobile with no way to scroll to them

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/note?id=66`

**Steps**

1. Create (or open) a note that contains a calendar block - the agent used http://localhost:8200/note?id=66.
2. Resize the browser to 390x844.
3. Scroll down to the calendar block.
4. Try to reach the "Agenda" view toggle, and try to scroll the page or the block horizontally.

**Expected:** All calendar view controls (Month/Week/Agenda) are reachable at mobile width, either by wrapping or by a horizontally scrollable strip.

**Actual:** The Agenda button is laid out entirely outside the 390px viewport (x 407-479). The document does not scroll horizontally (scrollWidth == clientWidth == 390) and the nearest scrollable ancestor has overflow-x: hidden, so the control can never be reached or clicked on a phone.

**Evidence:** #165: "{\"agendaRect\":[407,479],\"viewport\":390,\"chain\":[\"DIV|ox=hidden|w=112|sw=136|cw=110\",\"DIV|ox=visible|w=205|sw=205|cw=205\",\"DIV|ox=visible|w=324|sw=421|cw=324\", ...]}". #167 re-verified after a full reload: "{\"left\":407,\"right\":479,\"vw\":390,\"docScrollW\":390}". #155 also lists block content overflowing the viewport: "[\"DIV.flex items-center gap-2 right=454\", \"DIV.flex border border-stone-200 rounded overflow-h...\"]".

**Screenshot:** `notes-note66-mobile-calendar.png`

### 56. Submitting the group's Add Tags form with no tag selected navigates to a bare HTTP 400 error page

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/group?id=88 -> http://localhost:8200/v1/groups/addTags?redirect=%2Fgroup%3Fid%3D88`

**Steps**

1. Open any group detail page, e.g. http://localhost:8200/group?id=88.
2. Without picking anything in the "Add Tag" autocompleter, click the "Add Tags" button.

**Expected:** The button is disabled until a tag is chosen, or the submit is a no-op with an inline message; the user stays on the group page.

**Actual:** The browser is navigated away from the group to the raw API endpoint, which renders a full-page "Error 400" screen whose only recovery is a javascript:history.back() link. All page context is lost.

**Evidence:** #119: body text "An error has occurred\nat least one tag ID is required\n\nGo back" plus console "[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/groups/addTags?redirect=%2Fgroup%3Fid%3D88:0". #121: "{\"url\":\"http://localhost:8200/v1/groups/addTags?redirect=%2Fgroup%3Fid%3D88\",\"links\":[\"Go back -> javascript:history.back()\"],\"title\":\"Error 400\"}". The same 400 was hit earlier at #107: "[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ .../v1/groups/addTags...".

**Screenshot:** `group-addtags-empty-error.png`

### 57. Relation type form keeps showing "Please select at least 1 value" after the category is chosen

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/relationType/new`

**Steps**

1. Open http://localhost:8200/relationType/new.
2. Fill in Name (e.g. "hunt-groups RT Test") but leave From Category and To Category empty, then click Save.
3. The form comes back with "Please select at least 1 value" under From Category and To Category.
4. Now type "Person" in From Category and pick the option; type "Location" in To Category and pick the option.
5. Look at the From Category field again.

**Expected:** The validation message clears as soon as a value is selected.

**Actual:** The error text stays under the field even though a value ("Person") is now selected and shown as a chip, so the form looks invalid after the user has already fixed it.

**Evidence:** #149 the error element after the failed submit: "msgs":[{"tag":"SPAN","cls":"block text-sm font-medium text-red-400 mt-3","id":"input_autocompleter_1-error",...}]. #159 after selecting both categories, the form still reads: "Name * | Required | Description | From Category | Please select at least 1 value | Person | Remove Person | Added Person | To ...".

**Screenshot:** `relationtype-stale-error.png`

### 58. From/To Category are required on the relation type form but nothing says so until submit, and the fields never get aria-required/aria-invalid

- **Area:** Notes / blocks / groups / relations · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/relationType/new`

**Steps**

1. Open http://localhost:8200/relationType/new.
2. Inspect the From Category / To Category labels - only Name carries the "*" / "Required" marker.
3. Fill only Name and click Save.
4. Inspect the two category comboboxes' ARIA attributes after the error is rendered.

**Expected:** Required fields are marked as required before submit (visually and via aria-required), and after a failed submit the offending combobox is aria-invalid="true" and receives focus.

**Actual:** Only Name is marked ("Name * | Required"). The category comboboxes have aria-required=null and, even after the server rejects the submit and renders their error text, aria-invalid=null; focus stays on the Save button. Screen reader users get no indication the fields are required or invalid.

**Evidence:** #151: "[{\"id\":\"input_autocompleter_1\",\"al\":null,\"inv\":null,\"desc\":\"input_autocompleter_1-error\",\"req\":null}, ...]" (inv = aria-invalid, req = aria-required). #149 focus after submit: "active":"BUTTON|ml-3 inline-flex justify-center py-2 px-4 border border-tran". #159 shows only Name is marked required: "Name * | Required | Description | From Category | Please select at least 1 value | ...".

**Screenshot:** `relationtype-required-after-submit.png`

### 59. Relation detail pages render an empty <h1>

- **Area:** Notes / blocks / groups / relations · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/relation?id=1`

**Steps**

1. Open http://localhost:8200/relation?id=1 (a relation with no Name - the Create Relation form does not require one).
2. Inspect the heading structure of the page (h1..h4) and the h1's markup.

**Expected:** The page's h1 names the page (e.g. the relation name, or "Relation: Anna Lindqvist -> Reykjavik Studio" as the <title> already does).

**Actual:** The h1 is empty - it contains only an empty <inline-edit> element. The first meaningful heading is an h2 "Relation", so heading navigation and the document outline start with a blank top-level heading.

**Evidence:** #135 heading dump: "[\"H1: \",\"H2: Relation\",\"H3: FROM GROUP\",\"H3: Anna Lindqvist\",\"H3: TO GROUP\",\"H3: Reykjavik Studio\",\"H2: Edit Tags\",...]" while document.title is "Relation from An...". #137 shows the empty element: '<h1 class="..."> <span class="break-words"><inline-edit post="/v1/relation/editName?id=1" name="name"></inline-e' (opening tag immediately followed by the closing tag). #199 on a relation that does have a name shows the same empty h1 with the name demoted to h2: "[\"H1:\",\"H2:hunt-groups rel test\",...]".

**Screenshot:** `relation-detail-empty-h1.png`

### 60. Relation "category mismatch" rejection is a terse, non-announced error and round-trips through the URL

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/relation/new?FromGroupId=87&ToGroupId=85&GroupRelationTypeId=1`

**Steps**

1. Open http://localhost:8200/relation/new.
2. Pick relation type "Address" (its category flow is Person -> Location).
3. Pick a From Group whose category is not Person (e.g. "Northwind Labs (Business)") and any To Group.
4. Click Save.
5. Or reproduce directly: http://localhost:8200/relation/new?FromGroupId=87&ToGroupId=85&GroupRelationTypeId=1 and click Save.

**Expected:** A clear message naming the mismatch ("Address requires a Person as From Group; Northwind Labs is a Business"), announced to assistive tech, ideally caught before submit since both categories are known client-side.

**Actual:** The form reloads with the raw string "category mismatch" rendered in a plain <h3> with no role=alert (nothing is announced), and the failure is carried in the query string as Error=category+mismatch. Nothing tells the user which side is wrong or what is expected. The mismatch is only detectable by submitting.

**Evidence:** #189 the error element: "{\"tag\":\"H3\",\"cls\":\"text-sm font-medium text-red-800\",\"role\":null,\"parentRole\":null}". #193 the re-run: "http://localhost:8200/relation/new?FromGroupId=87&ToGroupId=85&GroupRelationTypeId=1&Name=&Description=&Error=category+mis...". #193 also shows the type's declared flow: "DependsOn\n\nA depends on B\n\nCATEGORY FLOW\nPerson\nLocation".

**Screenshot:** `relation-category-mismatch.png`

### 61. Relation types (and note types / categories) can be created but never deleted - no UI action and no API endpoint

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/relationType?id=4`

**Steps**

1. Create a relation type at http://localhost:8200/relationType/new (e.g. "hunt-groups RT Test").
2. Open its detail page http://localhost:8200/relationType?id=4 and look for a Delete action.
3. Open http://localhost:8200/relationType/edit?id=4 and look again.
4. Check http://localhost:8200/relationTypes, /noteTypes and /categories for delete controls.
5. Try the API: curl -X DELETE http://localhost:8200/v1/relationType -d '{"ID":9999}'.

**Expected:** A mistyped or obsolete taxonomy entry can be removed (or at least archived), the way tags and entities can.

**Actual:** There is no delete affordance anywhere for relation types, note types or categories, and the DELETE endpoint does not exist (returns the HTML 404 page). Every accidental relation type is permanent.

**Evidence:** #167 detail page: "btns":["Edit","Edit Tags"]. #169 edit page: "[\"Save\"]", and the list page items are plain links only. #171: "DELETE /v1/relationType -> 404" plus empty delete-button arrays for /noteTypes and /categories ("[]", "[]"). #173 shows the response body is the HTML 404 page: "<title>404 Not Found - mahresources</title>".

### 62. Mobile list pages bury the first result under ~2 screens of filter UI

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/notes`

**Steps**

1. Resize the browser to 390x844.
2. Open http://localhost:8200/notes.
3. Measure/observe how far down the first note card starts. Repeat on /resources and /groups.

**Expected:** On a phone the first result is visible (or one short scroll away); filters collapse behind a control.

**Actual:** The filter form is fully expanded above the list, so the first card starts at y=1574px on /notes (viewport height 844) - almost two full screens of scrolling before any content. /groups is 1745px and /resources is 2155px.

**Evidence:** #177: "{\"firstArticleTop\":1574,\"pageHeight\":11955,\"viewportH\":844}". #179: "/resources: 2155" and "/groups: 1745". (#173 confirms there is no horizontal overflow: "{\"docW\":390,\"vw\":390,\"over\":[]}", so this is purely vertical burial.)

**Screenshot:** `notes-list-mobile.png`

### 63. Mobile 390x844: the note's own content starts ~900px down, and on /notes the first note card is ~1574px down, behind the filter sidebar

- **Area:** Notes list + note detail (mobile layout) · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/notes`

**Steps**

1. Resize the viewport to 390x844.
2. Open http://localhost:8200/notes. The whole filter sidebar (20 tag chips, Sort controls, Name/Text/Tags/Groups/Owner/Note Type/Meta/date filters, Shared Only, Apply Filters) renders first; no note card is visible on the first screen.
3. Measure: document.querySelector('article').getBoundingClientRect().top + scrollY -> 1574.
4. Open http://localhost:8200/note?id=61 at the same size. The whole metadata sidebar (Updated, Created, Owner, Note Type, Tags + Add Tag form, Meta Data, Sharing, GUID) renders before the note's blocks.
5. Measure: document.querySelector('main').getBoundingClientRect().top + scrollY -> 902 (viewport is 844).

**Expected:** On a phone the primary content (the note list / the note body) should come first, with the filter and metadata panels collapsed behind a disclosure or moved below the content. A user should not have to scroll ~2 screens to see the first note.

**Actual:** The sidebar always precedes <main> in source order and simply stacks above it at narrow widths. On /notes the first note card sits at y=1574 on an 844px-tall viewport (~1.9 screens of filter UI first). On a note detail page the note body sits at y=902 — the entire first screen is metadata, even for a note whose Meta Data is empty. Same pattern on /resources (2155), /groups (1745). No horizontal overflow (scrollWidth == clientWidth == 390), so this is purely vertical burial of the content.

**Evidence:** 390x844: /notes -> {"firstNoteTop":1574,"docH":11929,"vh":844}; scrollWidth 390 == clientWidth 390 (no h-scroll).
/note?id=61 -> mainTop=902 docH=1947; /note?id=64 -> mainTop=874; /note?id=62 -> mainTop=902.
Comparison: /resources 2155, /groups 1745, /tags 693.

**Screenshot:** `notes-list-mobile.png`

### 64. Relation detail page renders an empty <h1> for relations without a name — page header is a bare pencil icon

- **Area:** Relations · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/relation?id=1`

**Steps**

1. Go to http://localhost:8200/relation?id=1 (a seeded relation whose Name field is empty — confirm via /relation/edit?id=1, name="").
2. Look at the page header area, left of the Edit/Delete buttons.
3. Inspect document.querySelector('h1').textContent.
4. Repeat for /relation?id=2 and /relation?id=3.

**Expected:** The heading falls back to something meaningful — the browser tab already does exactly that ("Relation from Anna Lindqvist to Reykjavik Studio").

**Actual:** The <h1> is empty (contains only an <inline-edit> custom element with no value). Visually the header row is blank except for a floating pencil icon. Screen-reader users get an empty top-level heading, and sighted users get a page that does not identify itself.

**Evidence:** eval on ids 1/2/3: h1='""' for all three, while document.title is "Relation from Anna Lindqvist to Reykjavik Studio - mahresources" etc. h1 outerHTML: `<h1 …><span class="break-words"><inline-edit post="/v1/relation/editName?id=1" name="name"></inline-edit></span></h1>`. Heading list on the page: [H1:"", H2:"Relation", H3:"From Group", …].

**Screenshot:** `relation-empty-title.png`

### 65. Relation form offers category-incompatible groups and then fails with the bare message "category mismatch"

- **Area:** Relations · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/relation/new?FromGroupId=83`

**Steps**

1. From /group?id=83 (Anna Lindqvist, a Person) click "New" in the Relations section — you land on /relation/new?FromGroupId=83.
2. Pick Type = "Address" (which is defined as Person → Location).
3. Open the "To Group" picker: it lists every group in the system, including "Northwind Labs (Business)" and other categories that Address cannot accept, with no filtering or hint.
4. Choose "Northwind Labs (Business)", give it a name and click Save.
5. Repeat via the direct URL /relation/new?FromGroupId=83&ToGroupId=87&GroupRelationTypeId=1&Name=x and click Save.

**Expected:** Either the To Group picker is constrained to the type's To Category (and the Type picker shows each type's Person→Location flow), or the error explains it: e.g. "Address requires a From group in category Person and a To group in category Location; Northwind Labs is a Business."

**Actual:** The picker offers all 90+ groups regardless of the selected type. On submit the server rejects it and the page reloads with a red alert containing only the raw string "category mismatch", with no indication of which categories are required or which side is wrong. The failed query string (including the error) is also pushed into the address bar.

**Evidence:** Result URL both times: /relation/new?FromGroupId=83&ToGroupId=87&GroupRelationTypeId=1&Name=…&Description=&Error=category+mismatch, alert text "category mismatch". To Group listbox contents with Type=Address included "Northwind Labs (Business)", "Marco Reyes (Person)", "Landscapes / Sub-level 4 (Media)", etc.

**Screenshot:** `relation-category-mismatch.png`

### 66. Keyboard focus is dropped to <body> after 'Select All' / 'Deselect All' in the bulk toolbar

- **Area:** Resource list — bulk selection accessibility · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open http://localhost:8200/resources/details.
2. Activate the 'Select All' button (click or keyboard).
3. Immediately inspect document.activeElement → it is BODY.
4. The button that was activated is still in the DOM but its container is hidden (offsetParent === null), because the toolbar swaps between a 'no selection' and a 'has selection' variant.
5. Repeat on /resources (Thumbnails view) — same result.
6. Then activate the now-visible 'Deselect All' — activeElement is BODY again.

**Expected:** After a toolbar action that re-renders the toolbar, focus should move to the equivalent control in the new toolbar (e.g. 'Deselect All'), or at minimum stay within the toolbar region.

**Actual:** Because the activated button's container is hidden rather than updated in place, the browser resets focus to <body>. A keyboard-only user who selects all 50 rows then has to tab from the very top of the document again to reach the bulk actions they just revealed.

**Evidence:** /resources/details: after clicking Select All → {"active":"BODY/Skip to main content\nDASH","checked":50}; the original button: {"exists":true,"visible":false}
/resources: after Select All → {"active":"BODY","checked":50}; after Deselect All → {"active":"BODY","checked":0}
Reproduced on both views and in both directions.

### 67. Details table: one long-named resource stretches the Name column to ~1220px and pushes Preview/Size/Created/Updated out of view with no scroll affordance

- **Area:** Resource list — details/table view · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open http://localhost:8200/resources/details at a 1280x900 viewport.
2. Page 1 contains the seeded resource 'A resource with a really extremely long descriptive name that ought to test how the interface truncates things in cards, tables, breadcrumbs and the browser title bar' (ids 74 and 85).
3. Observe: the table shows only the Select, ID and Name columns. Preview, Size, Created, Updated, Original Name and Original Location are all off-screen to the right, with no visible scrollbar or any other hint that they exist.
4. Compare page 2 (/resources/details?page=2), which has no long names: the table is 1012px wide and most columns are visible.
5. Same effect at 390x844: table scrollWidth 2012 inside a 356px container.

**Expected:** Long names should be truncated (CSS ellipsis with a title tooltip) or the Name column capped, so the other columns stay visible; if the table must scroll, the overflow needs a visible affordance.

**Actual:** The Name column is sized by its longest cell, so a single 174-character name makes the table 2212px wide inside an 822px overflow-x:auto container. The columns that carry the actual metadata are invisible unless the user discovers the horizontal scroll.

**Evidence:** On /resources/details (page 1, 1280x900):
{"tableW":2212,"tableClient":2212,"parentW":822,"parentScrollW":2212,"parentOverflowX":"auto","bodyScrollW":1280,"winW":1280,"headers":["SELECT:17-47","ID:47-93","NAME:93-1312","PREVIEW:1312-1581","SIZE:1581-1662","CREATED:1662-1801","UPDATED:1801-1941","ORIGINAL NAME:1941-2085","ORIGINAL LOCATION:2085-2229"]}
On page 2 (no long names): {"tableW":1012,"parentW":822,...}
At 390x844: {"doc":390,"win":390,"tableW":2012,"parentW":356}

**Screenshot:** `reslist-details-longname-overflow.png`

### 68. No empty state on the resource list — a zero-result filter or an out-of-range page renders a silently blank content area

- **Area:** Resource list — empty states · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources?mrql=name%20~%20%22*zzzznotexisting*%22`

**Steps**

1. Open http://localhost:8200/resources?mrql=name ~ "*zzzznotexisting*" — the whole content column is blank; main's text is only 'Thumbnails | Details | Simple | Timeline | Select All | Filter these resources with an MRQL expression | Filter | Edit in MRQL editor'.
2. Do the same with the sidebar form: /resources?name=zzzznotexisting → same blank area.
3. Switch to Details: /resources/details?mrql=name ~ "*zzzznotexisting*" → an empty table header row and nothing else.
4. Open an out-of-range page: /resources?page=99 → blank content, and the 'Previous' link points to /resources?page=98 which is also blank.
5. Compare with /tags?name=zzzznotexisting, which does show 'No tags found. Create one.'

**Expected:** A 'No resources found' empty state with a hint to clear the filter (matching the /tags list, which already does this).

**Actual:** Every zero-result state in the resources list (MRQL filter, sidebar filter, out-of-range page, both Thumbnails and Details views) renders a blank area with no message. The 'Select All' button is still shown even though there is nothing to select. /notes and /groups behave the same way, while /tags has a proper empty state, so the app is also internally inconsistent.

**Evidence:** /resources?mrql=name ~ "*zzzznotexisting*" → {"count":0,"main":"Thumbnails\nDetails\nSimple\nTimeline\nSelect All\nFilter these resources with an MRQL expression\nFilter\nEdit in MRQL editor"}
/resources/details?mrql=... → "...SELECT\tID\tNAME\tPREVIEW\tSIZE\tCREATED\tUPDATED\tORIGINAL NAME\tORIGINAL LOCATION" and no rows.
/resources?page=99 → {"arts":0, footer "Previous|1|2|3"}, Previous -> /resources?page=98.
/tags?name=zzzznotexisting → "List | Timeline | Select All | No tags found. Create one."

**Screenshot:** `reslist-empty-noresults.png`

### 69. SVG resources render a broken image in the resource grid and details table (0x0 JPEG served as the preview)

- **Area:** Resource list — grid + details thumbnails · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources?mrql=contentType%20~%20%22*svg*%22`

**Steps**

1. Open http://localhost:8200/resources?mrql=contentType ~ "*svg*" (two seeded/other-agent SVGs: id 78 'Logo Mark (SVG)' and id 97).
2. Look at the two cards — both show the browser's broken-image icon with the alt text spilling over the thumbnail area.
3. Same on the plain grid: open /resources, scroll to the 'Logo Mark (SVG)' card.
4. Confirm from the server: curl -s -o /tmp/p.jpg 'http://localhost:8200/v1/resource/preview?id=78&height=300' ; file /tmp/p.jpg
5. Contrast with a non-previewable file (id 100, text/plain): curl -D - -o /dev/null 'http://localhost:8200/v1/resource/preview?id=100&height=300' → 307 to /public/placeholders/file.jpg, which renders fine.

**Expected:** An SVG that cannot be rasterised should fall back to the same /public/placeholders/file.jpg placeholder that .txt/.csv/.json/.md resources get, so the card shows a clean file icon.

**Actual:** /v1/resource/preview for image/svg+xml returns HTTP 200, Content-Type image/jpeg, 591 bytes, and `file` reports "JPEG image data, baseline, precision 8, 0x0". A zero-dimension JPEG cannot be decoded, so every SVG card/table row shows a broken-image icon (img.naturalWidth === 0 && img.complete === true).

**Evidence:** $ curl -s -o /tmp/svgprev.jpg 'http://localhost:8200/v1/resource/preview?id=78&height=300'; file /tmp/svgprev.jpg
/tmp/svgprev.jpg: JPEG image data, baseline, precision 8, 0x0, components 3

In-page: [...document.querySelectorAll('article img')].map(i=>({alt:i.alt,w:i.naturalWidth,h:i.naturalHeight,complete:i.complete}))
→ [{"alt":"Preview of hunt-resdetail svg test","w":0,"h":0,"complete":true},{"alt":"Preview of Logo Mark (SVG)","w":0,"h":0,"complete":true}]

Contrast (text/plain id=100): HTTP/1.1 307 Temporary Redirect / Location: /public/placeholders/file.jpg

No console errors (0 messages).

**Screenshot:** `reslist-v2-svg-broken-thumbs.png`

### 70. Switching to the Simple view while on page 2 lands on a completely blank page (Simple view ignores pagination) ✅ **VERIFIED**

- **Area:** Resource list — view switcher + pagination · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources/simple?page=2`

**Steps**

1. Open http://localhost:8200/resources/details?page=2 (or /resources?page=2) — 50 rows are shown.
2. Click the 'Simple' tab in the Display options group. The link is /resources/simple?page=2 (page is preserved).
3. The resulting page is entirely blank: no page <h1>, no sidebar, no resources — only the view tabs, the MRQL bar and a pagination footer showing '1'.
4. Repeat from /resources?page=2 → same blank page.
5. Compare: /resources/simple (no page param) renders all 111 resources on a single page (111 unique /resource?id= links, pagination footer shows only '1'), i.e. the Simple view never paginates.

**Expected:** Either the Simple view paginates like the other views so page=2 shows the second batch, or the view switcher drops the page param when it is meaningless — never a blank dead end.

**Actual:** Simple renders every resource on page 1 and returns zero items for page>=2, so preserving the current page number when switching views produces an empty page with no message and no way to tell what went wrong. Additionally, rendering all resources on one page will not scale (the project explicitly targets deployments with millions of resources).

**Evidence:** On /resources/details?page=2, the Simple tab href is "/resources/simple?page=2".
After clicking it: {"url":"http://localhost:8200/resources/simple?page=2","n":0,"main":"Thumbnails|Details|Simple|Timeline|Filter these resources with an MRQL expression|Filter|Edit in MRQL editor"}
On /resources/simple (page 1): {"links":111,"unique":111,"footer":"1"}
Reproduced twice (from Details p2 and from Thumbnails p2).

**Verified by me (re-run against the live server):** `/resources/simple?page=1 renders 112 resource links; ?page=2 renders 4, while /resources?page=2 has 37 items`

**Screenshot:** `reslist-v2-simple-page2-blank.png`

### 71. Tag chips on Similar Resources cards link back to the current resource page instead of the tag's resource list

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Open a resource page that shows a 'Similar Resources' card (e.g. /resource?id=88, whose similar card is resource 63, tagged favorite + landscape).
2. Click the 'favorite' or 'landscape' tag chip inside the similar-resource card.

**Expected:** The chip navigates to the resources filtered by that tag, as it does everywhere else (/resources?tags=79).

**Actual:** The chip href is built against the current page, so it navigates to /resource?id=88&tags=79 - the same resource page plus a query parameter that does nothing. Title and content are unchanged; the click is a dead end.

**Evidence:** #81 on /resource?id=88: '[["favorite","/resource?id=88&tags=79"],["landscape","/resource?id=88&tags=82"]]'. Re-verified after a fresh navigation, #89: identical output. #83 after clicking one: 'Page URL: http://localhost:8200/resource?id=88&tags=79 / Page Title: Resource: hunt-resdetail test image A - mahresources'. The correct form exists on the list page, #85: '[["favorite (21)","/resources?tags=79"],["landscape (17)","/resources?tags=82"], ...]'. #95 confirms the chips belong to the similar card's resource, not the current one: 'res 63 tags: ['favorite', 'landscape']' / 'res 88 tags: []'. #203 lists the card's hrefs: '/v1/resource/view?id=63...', '/resource?id=63', '/group?id=71', '/resource?id=88&tags=79', '/resource?id=88&tags=82' - only the tag chips are wrong.

**Screenshot:** `similar-edit-tags.png`

### 72. SVG resources get a 0x0 JPEG preview, so their thumbnail renders blank ✅ **VERIFIED**

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=78`

**Steps**

1. Open the SVG resource page /resource?id=78 ('Logo Mark (SVG)').
2. Inspect the preview <img> naturalWidth/naturalHeight, or fetch /v1/resource/preview?id=78&height=300 and run `file` on the output.

**Expected:** The SVG is rasterised into a real thumbnail, or the app falls back to a type icon (as it does for text/plain, which 307-redirects to a static asset).

**Actual:** The preview endpoint returns HTTP 200 with a 591-byte JPEG whose dimensions are 0x0. In the page the <img> reports naturalWidth 0 / naturalHeight 0 with complete=true - a blank box that looks like a broken image.

**Evidence:** #171: '[{"src":"/v1/resource/preview?id=78&height=300&v=2844aaf657c452941d048fbc5ab74ae6baeba1bb","cw":0,"ch":0,"complete":true}]'. #173 headers: 'HTTP/1.1 200 OK / Cache-Control: max-age=2592000 / Content-Length: 591 / Content-Type: image/jpeg'. #199: 'res 78 preview: 200 591 JPEG image data, baseline, precision 8, 0x0, components 3'. #175 shows the record itself is '0 0 image/svg+xml 120' and that text/plain resource 87 gets a proper fallback instead: 'HTTP/1.1 307 Temporary Redirect ... Location: /publi[c]/...'.

**Verified by me (re-run against the live server):** `GET /v1/resource/preview?id=97 -> 200, image/jpeg, 591 bytes, JPEG SOF dimensions 0 x 0`

**Screenshot:** `res-78.png`

### 73. A failed rotate leaves the resource's cached preview degraded to a 0x0 image

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=97`

**Steps**

1. Upload an SVG resource and confirm its preview works (/v1/resource/preview?id=97&height=300 returns a real JPEG, 600px wide in the page).
2. Click 'Rotate' on that resource - it fails with HTTP 500 'image: unknown format'.
3. Fetch the preview endpoint again.

**Expected:** A failed transform leaves the resource and its cached preview exactly as they were.

**Actual:** Before the failed rotate the preview was a 5532-byte JPEG rendering 600px wide; afterwards the same endpoint returns a 591-byte 0x0 JPEG - the same degenerate output SVG resource 78 has.

**Evidence:** Before - #179: '[{"src":"/v1/resource/preview?id=97&height=300&v=372cf6232abefa97b7c68e9b477b87e2cd9e4887","nw":600}]' and #181: '=== my svg preview / HTTP/1.1 200 OK / Cache-Control: max-age=2592000 / Content-Length: 5532 / Content-Type: image/jpeg'. The failed rotate is #195. After - #199: 'res 97 preview: 200 591 JPEG image data, baseline, precision 8, 0x0, components 3'.

### 74. Lightbox does not restore focus when it closes - focus is dumped on <body>

- **Area:** Resources — list & detail · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Open /resource?id=88 and click the image inside a 'Similar Resources' card to open the media-viewer lightbox.
2. Press Escape (or click the lightbox close button).
3. Inspect document.activeElement.
4. Repeat, but first open the 'Resource info' panel, then press Escape once (closes the panel) and inspect focus again.

**Expected:** Closing a modal returns focus to the control that opened it (the card thumbnail link); closing the nested info panel returns focus to the 'Resource info' toggle.

**Actual:** After every close path focus is on BODY, so keyboard and screen-reader users are returned to the top of the document and lose their place in the card list. The same happens for the nested info panel while the dialog is still open.

**Evidence:** #227 Escape with no info panel: 'after 1 Esc (no info): {"vis":false,"active":"BODY null"}'. #229 via the close button: 'after close button: {"vis":false,"active":"BODY site bg-stone-50"}'. #213 Escape while the info panel is open: '{"dialogOpen":true,"info":true,"active":"BODY.site bg-stone-50"}'. #223: 'after 2nd Esc: {"dlg":true,"info":false,"active":"BODY"}' and 'after 3rd Esc: ... "active":"BODY"'. #225 confirms the dialog really is closed at that point (display:none), so this is focus loss, not a stuck dialog: '[{"label":"Market Vendor, Kyoto","vis":false,"disp":"none"}, ...]'.

**Screenshot:** `lightbox-after-3esc.png`

### 75. One extreme-aspect-ratio image blows its grid card to 1831px tall and drags its row neighbour with it

- **Area:** Resources — list & detail · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources`

**Steps**

1. Open /resources in the thumbnails (grid) view at 1280px wide.
2. Scroll to the card named 'Tall: Seljalandsfoss' (the 400x1600 image).
3. Measure the heights of all article elements.

**Expected:** Cards keep a bounded, roughly uniform height; the thumbnail is fitted or cropped into the card's media box.

**Actual:** The tall image's card is 1831px high - 3.5x the 524px of every normal card - and because the grid row is height-matched, the unrelated 'Tiny 32x32 Icon' card next to it is stretched to 1831px too. The user has to scroll two full screens past one image.

**Evidence:** #29 normal cards: '[{"name":"Plain Text Notes","natural":"960x678","rendered":"402x284","cardH":524},{"name":"Unicode Name ...","natural":"400x300","rendered":"402x302","cardH":524}, ...]'. #31 measuring the tall card: '{"top":747.015625,"h":1830.5}'. #43 across the whole page: '{"count":50,"tall":[{"n":"Tiny 32x32 Icon","h":1831},{"n":"Tall: Seljalandsfoss","h":1831}]}'. #41 confirms no images are actually broken, so this is layout, not loading: '[]'.

**Screenshot:** `reslist-tall-card-blowup.png`

### 76. Details-view row checkboxes are 14x14px with no padded hit area

- **Area:** Resources — list & detail · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open /resources/details.
2. Measure .detail-table-checkbox and its containing cell.

**Expected:** At least a 24x24 CSS-pixel target (WCAG 2.5.8 Target Size Minimum), as the grid cards already provide.

**Actual:** The checkbox is 14x14 with 0 padding and no wrapping label, so the clickable target is 14x14 inside a 30x45 cell - well under the 24px minimum, and 40% smaller than the same control in the grid view.

**Evidence:** #261: '[{"w":14,"h":14,"cls":"detail-table-checkbox focus:ring-amber-600 text-amber-700 border-stone-300 rounded"},{"w":14,"h":14, ...}]'. #263: '{"parentTag":"TD","parentCls":"","parentBox":{"w":30,"h":45},"hasLabel":false,"cs":"0px"}' - no padding, no label to enlarge the target. Grid-view equivalent at #19: '{"w":24,"h":24,"cls":"card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded"}'.

**Screenshot:** `reslist-details-tiny-checkbox.png`

### 77. No empty state when a filter matches nothing - the results area just goes blank

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources?mrql=name%20~%20%22zzzz-no-such-resource*%22`

**Steps**

1. Open /resources and type a name filter that matches nothing (e.g. 'zzzz-no-such-resource') in the sidebar, then Apply Filters.
2. Repeat by putting an MRQL expression that matches nothing straight in the URL.
3. Repeat in the details view: /resources/details?mrql=name+%7E+%22*zzzz-no-such-resource*%22.

**Expected:** A 'no resources match these filters' message with a way to clear the filter.

**Actual:** Zero cards render and the main region contains only the view switcher and filter chrome - no message at all. In the details view an empty table with all its headers is rendered instead.

**Evidence:** MRQL path, #169: '{"cards":0,"mainText":"Thumbnails|Details|Simple|Timeline|Select All|The MRQL editor and sidebar form cannot represent the same filters. The form is disabled.|Use form values|Using the form values will remove the M..."}'. Sidebar-filter path, #173: 'Page URL: http://localhost:8200/resources?ResourceCategoryId=&mrql=name+%7E+%22*zzzz-no-such-resource*%22' then '{"cards":0,"main":"Thumbnails|Details|Simple|Timeline|Select All|Filter these resources with an MRQL expre..."}'. Details view, #175: '{"rows":0,"hasTable":true,"main":"Thumbnails|Details|Simple|Timeline|Select All|Filter these resources with an MRQL expression|Filter|Edit in MRQL editor|SELECT|\tID\tNAME\tPREV..."}'.

**Screenshot:** `reslist-no-results-sidebar-filter.png`

### 78. Bulk delete confirmation is a bare 'Are you sure you want to delete?' with no count and no names

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources?mrql=name%20~%20%22hunt-reslist%20throwaway*%22`

**Steps**

1. On /resources, select one resource with its card checkbox.
2. Open the Delete editor from the bulk toolbar and click 'Delete'. Note the confirm text.
3. Cancel, then click 'Select All' with four resources in view and repeat.

**Expected:** A destructive bulk action states the scope - 'Delete 4 resources?' - so a mis-click on Select All is not irreversible on confirm.

**Actual:** A native confirm reading exactly 'Are you sure you want to delete?' appears regardless of whether 1 or 4 items are selected; nothing in the dialog reveals the blast radius, and the toolbar/editor text is only 'Delete Selected|Delete'.

**Evidence:** 1 selected - #237: '{"checked":1,"card":"hunt-reslist throwaway B"}' and '{"txt":"Delete Selected|Delete","action":"/v1/resources/delete?redirect=%2Fresources%3Fmrql%3D..."}'; #239: '- ["confirm" dialog with message "Are you sure you want to delete?"]'. 4 selected - #245: '- ["confirm" dialog with message "Are you sure you want to delete?"]' with #247 confirming '{"checked":4,"cards":4}'. Identical string both times.

**Screenshot:** `reslist-delete-no-count.png`

### 79. Details view: row checkboxes do not reflect the live selection that the bulk toolbar is acting on

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources/details?mrql=name%20~%20%22hunt-reslist%20throwaway*%22`

**Steps**

1. On /resources (grid), select some resources.
2. Switch to the details view for the same filter (/resources/details?mrql=...).
3. Click 'Select All' and inspect which checkboxes are actually checked.

**Expected:** The row checkboxes render the current selection, and Select All checks every visible row.

**Actual:** Right after switching views, two checked checkboxes exist that are invisible and unnamed while every visible row checkbox is unchecked; the first Select All click left zero tbody checkboxes checked even though the bulk toolbar reacted. Re-loading the page and clicking Select All again did work (4 checked), so the state is desynchronised rather than permanently broken.

**Evidence:** #251: '{"selHeader":"<span class=\"sr-only\">Select</span>","rowCbs":4}' then '{"checked":0,"toolbar":true}'. #253: '{"allCbs":9,"checked":["",""],"toolbarVisible":false}' - the two checked boxes have no accessible name. #255: '[{"cls":"","aria":null,"vis":false,"checked":true},{"cls":"","aria":null,"vis":false,"checked":true},{"cls":"focus:ring-1 focus:ring-amber-600 h-3.5 ","aria":null,"vis":true,"checked":false} ...]'. After a reload, #259: '{"checked":4,"toolbarVisible":true}'.

### 80. Mobile: the filter sidebar pushes the first result 2155px down the page (2.5 screens of scrolling)

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources`

**Steps**

1. Resize the viewport to 390x844.
2. Open /resources.
3. Measure the offset of the first article and of the sidebar.

**Expected:** On a phone the filter panel is collapsed behind a toggle (or below the results) so results are visible without scrolling.

**Actual:** The sidebar is stacked above the results at full height (1865px), so main starts at y=2009 and the first card at y=2155 - about 2.5 viewport heights of filter controls before a single result. The same pattern affects /notes and /groups.

**Evidence:** #185: '{"docHeight":15129,"asideTop":120,"asideH":1865,"mainTop":2009,"firstArticleTop":2155,"viewport":844}'. #187 for the other list pages: '== notes / {"asideTop":120,"mainTop":1428}' and '== groups / {"asideTop":120,"mainTop":1599}'. There is no horizontal overflow, so this is purely the stacking order (#179: '{"docSW":390,"bodySW":390,"iw":390,"overflowers":[]}').

**Screenshot:** `reslist-mobile-grid.png`

### 81. Mobile: the MRQL filter input is only 149px wide

- **Area:** Resources — list & detail · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources`

**Steps**

1. Resize the viewport to 390x844.
2. Open /resources (or /resources/details) and scroll to the 'Filter these resources with an MRQL expression' bar.
3. Measure the visible input[name=mrql].

**Expected:** The expression field takes the available width; MRQL expressions like 'name ~ "*guide*"' are long and need to be readable while typing.

**Actual:** The visible MRQL input renders at 149px on a 390px viewport - roughly a third of the screen, sharing the row with the Filter button and the 'Edit in MRQL editor' link, so only a few characters of an expression are visible.

**Evidence:** #193: '[{"w":0,"vis":false},{"w":149,"vis":true}]' (the first is the hidden desktop copy). Re-measured after scrolling it into view, #197: '{"w":149}'. For scale, the expressions the UI itself generates are like 'name ~ "*guide*"' (#83) and 'contentType ~ "*text/plain*"' (#201).

**Screenshot:** `reslist-mobile-mrql-bar-cramped.png`

### 82. /queries list applies smart-typography to SQL source: '' becomes ", -- becomes –, ... becomes … ✅ **VERIFIED**

- **Area:** SQL saved queries (/queries list) · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/queries`

**Steps**

1. Open http://localhost:8200/queries
2. Scroll to the seeded cards "Empty Description Groups", "Empty Description Notes", "Empty Description Resources".
3. Read the SQL preview text on each card.
4. To see the full effect, click "Add", set Name = "typography test", set Query = SELECT * FROM tags WHERE name != 'x' AND id > 1 -- range 1--5 and dots...  then Save, and go back to /queries.
5. Compare the card preview against the SQL on the detail page /query?id=<id> (which renders correctly).

**Expected:** SQL source text is shown verbatim, e.g. SELECT * FROM groups WHERE description IS NULL OR description = '' and SELECT * FROM tags WHERE name != 'x' AND id > 1 -- range 1--5 and dots...

**Actual:** The list card renders SELECT * FROM groups WHERE description IS NULL OR description = " (the two ASCII apostrophes are collapsed into a single U+201C LEFT DOUBLE QUOTATION MARK) and SELECT * FROM tags WHERE name != 'x' AND id > 1 – range 1–5 and dots… — straight quotes become curly quotes, -- becomes an en dash, ... becomes an ellipsis. Anyone reading or copying the SQL from the list gets invalid SQL. The detail page /query?id=55 shows the correct '' so only the list preview is affected.

**Evidence:** playwright eval on the <p> preview returned charCode 8220 for the final character; raw HTML from `curl -s http://localhost:8200/queries | grep 'SELECT \* FROM groups WHERE description'` contains `description = &ldquo;` while `curl -s 'http://localhost:8200/v1/query?id=55'` returns Text: "SELECT * FROM groups WHERE description IS NULL OR description = ''". Reproduced on 3 seeded cards plus a freshly created query.

**Verified by me (re-run against the live server):** `created a query with SQL `name = 'abc' -- dash test ... dots`; /queries renders `&lsquo;abc&rsquo; &ndash; dash test &hellip; dots` — copied SQL would be invalid`

**Screenshot:** `queries-sql-typography-mangled.png`

### 83. Floating "Open jobs panel" button covers the right half of the "Next" pagination link on /queries

- **Area:** SQL saved queries (/queries) — pagination · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/queries`

**Steps**

1. Set the browser window to 1280x900 (also reproduces at 1024x768 and 1440x900).
2. Open http://localhost:8200/queries (60 seeded queries → 2 pages).
3. Scroll to the bottom of the page.
4. Click the centre of the "Next →" link in the bottom-right pagination row.

**Expected:** Clicking "Next" navigates to /queries?page=2.

**Actual:** The fixed, circular "Open jobs panel" button (class `fixed bottom-4 right-4 z-40`) sits on top of the right half of the "Next →" link, including its arrow. Clicking the centre of the link opens the jobs panel instead of paginating; the URL stays on /queries. Only the leftmost ~27px of the 70px-wide link (the word "Next") is still clickable. Navigating directly to /queries?page=2 works, so it is purely the overlap.

**Evidence:** Hit test at 1280x900: document.elementFromPoint at the centre of the Next link returns BUTTON[Open jobs panel]. Sweeping across the link: x=1197/1205/1213 → A[Next page]; x=1221..1261 (upper two thirds) → BUTTON[Open jobs panel] / its svg+path. Same result at 1024x768, 1280x800 and 1440x900. A real playwright click on the link left location.href at http://localhost:8200/queries.

**Screenshot:** `queries-next-covered-by-fab.png`

### 84. "Copy Name" / "Copy Original Name" corrupt emoji and other astral characters (🎨 becomes "Ἲ8")

- **Area:** Single resource page — Metadata card copy buttons · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=86`

**Steps**

1. Open http://localhost:8200/resource?id=86 (name: "Ünïcødé Ñame 测试 🎨").
2. Hover the "Name" card in the METADATA section and click the ⧉ button (aria-label "Copy Name").
3. Paste the clipboard anywhere (or read navigator.clipboard.readText()).
4. Repeat with the ⧉ on the "Original Name" card.

**Expected:** The clipboard should contain exactly "Ünïcødé Ñame 测试 🎨" (and "ünïcødé-ñame-测试-🎨.png").

**Actual:** The clipboard contains "Ünïcødé Ñame 测试 Ἲ8" and "ünïcødé-ñame-测试-Ἲ8.png". The 🎨 (U+1F3A8) is replaced by U+1F3A followed by the digit 8. Root cause is visible in the rendered attribute: the template emits a 5-hex-digit escape `Ἲ8` instead of the required surrogate pair `🎨`, so JS parses it as `Ἲ` + "8". Affects any resource whose name contains a character outside the BMP. The page itself renders the emoji correctly, so the corruption is invisible until you paste.

**Evidence:** navigator.clipboard.readText() after clicking Copy Name: {"copied":"Ünïcødé Ñame 测试 Ἲ8","codepoints":["dc","6e","ef","63","f8","64","e9","20","d1","61","6d","65","20","6d4b","8bd5","20","1f3a","38"]}. Button markup: @click="updateClipboard('Ünïcødé Ñame 测试 Ἲ8'); ...". Copy Original Name gives "ünïcødé-ñame-测试-Ἲ8.png".

**Screenshot:** `unicode-copy-emoji-corrupt.png`

### 85. Version "Download" links serve files with no extension (filename="v3_a3bf2ae2") ⚠️ **DISPUTED**

- **Area:** Single resource page — Versions section · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<resource with versions>`

**Steps**

1. Open a resource detail page and expand the "Versions (N)" disclosure.
2. Click any version's "Download" link (href /v1/resource/version/file?versionId=X).
3. Inspect the saved file name / the Content-Disposition response header.

**Expected:** A downloadable filename with the correct extension, e.g. "MyImage_v3.png", so the OS can open it.

**Actual:** Content-Disposition is `attachment; filename="v3_a3bf2ae2"` — version number plus a hash prefix, with no extension at all, even though Content-Type is correct (image/png, image/jpeg). Downloaded files will not open by double-click on macOS/Windows and give no clue which resource they came from. Reproduced on four different versions across two resources, including the seeded resource 59.

**Evidence:** curl -D - '/v1/resource/version/file?versionId=115' -> Content-Disposition: attachment; filename="v3_a3bf2ae2" (content-type image/png). versionId=116 -> filename="v4_c8dccf0d". versionId=117 -> filename="v5_2fcfffce" (image/jpeg). Seeded resource 59, versionId=59 -> filename="v1_888c9c92".

**My re-check:** UNDECIDED — my version-file request returned JSON rather than the file; Content-Disposition not observed.

### 86. SVG resources are offered Rotate / Recalculate Dimensions, which return HTTP 500 and a bare dead-end error page

- **Area:** Single resource page — image tools on non-raster resources · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<id of an SVG resource> (seeded example: /resource?id=78)`

**Steps**

1. Go to http://localhost:8200/resource/new and upload any .svg file (I used a 120x60 svg with a rect and a circle). Save.
2. On the resulting /resource?id=N page, look at the right sidebar: it renders an "UPDATE DIMENSIONS" section and a "ROTATE 90 DEGREES" section (the same sections appear on the seeded SVG /resource?id=78).
3. Click "Rotate".
4. Go back and click "Recalculate Dimensions".

**Expected:** Either the tools are not offered for formats the server cannot decode (the app already knows this — the lightbox crop panel says "This image could not be decoded in the browser; cropping is unavailable. Formats like SVG, ICO, AVIF, and HEIC need to be re-uploaded as PNG or JPEG before they can be cropped." and the Crop section is correctly hidden for SVG), or the failure is handled in-page with a friendly message.

**Actual:** Rotate navigates to http://localhost:8200/v1/resources/rotate and renders a full-page "Error 500 / An error has occurred / image: unknown format" with zero site navigation (document.querySelectorAll('nav a').length === 0) and only a "Go back" link. Recalculate Dimensions does the same with "encountered errors during dimension calculation". Reproduced twice on my own SVG (id 115); the same two buttons are present on the seeded SVG resource 78.

**Evidence:** After clicking Rotate: {"url":"http://localhost:8200/v1/resources/rotate","title":"Error 500","text":"An error has occurred | image: unknown format | Go back","navLinks":0}. After Recalculate Dimensions: {"url":"http://localhost:8200/v1/resource/recalculateDimensions?redirect=%2Fresource%3Fid%3D115","title":"Error 500","text":"An error has occurred | encountered errors during dimension calculation | Go back"}. curl -X POST /v1/resource/recalculateDimensions -d ID=115 -> 500.

**Screenshot:** `svg-rotate-500.png`

### 87. Description inline editor has no keyboard save path and no Save/Cancel controls; tabbing out strands unsaved text

- **Area:** Single resource page — inline description editing · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<any resource>`

**Steps**

1. Open any resource detail page, e.g. http://localhost:8200/resource?id=88.
2. Double-click the description block (title="Double-click to edit").
3. Type some text into the textarea.
4. Press Tab twice to move focus out of the textarea (focus lands on the "Copy Original Name" ⧉ button).
5. Check GET /v1/resource?id=N — the description is unchanged and the textarea is still open.
6. Separately: type text and press Escape.

**Expected:** An explicit Save/Cancel pair (or Ctrl+Enter to save and a confirmation before discarding). Blurring the field should either save or warn.

**Actual:** The editor's only save trigger is Alpine's @click.away, i.e. a mouse click elsewhere on the page. Tabbing out does nothing: the textarea stays open with unsaved text while focus sits on an unrelated button — a keyboard-only user has no way to commit the edit. Escape (@keydown.escape="editing = false") discards the typed text instantly with no confirmation and no undo. There are no Save or Cancel buttons rendered at all. Verified: typed "KB description attempt", pressed Tab twice -> stillEditing true, value retained, GET description still ''. A mouse click elsewhere then saved it. Escape test: typed "This description text will be lost", pressed Escape -> textarea gone, block back to "No description".

**Evidence:** After two Tabs: {"stillEditing":true,"val":"KB description attempt","active":"BUTTON/Copy Original Name"} and GET /v1/resource?id=101 -> DESC= ''. Then a mouse click elsewhere -> DESC= 'KB description attempt'. Escape test: {"textareaGone":true,"shown":"No description"}. Element markup shows only @click.away=... and @keydown.escape="editing = false"; no submit control.

**Screenshot:** `desc-editor-no-save-button.png`

### 88. Inline rename failure is invisible to sighted users — server sends a clear message, UI shows nothing

- **Area:** Single resource page — inline name editing · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Open http://localhost:8200/resource?id=88.
2. Click the pencil (aria-label "Edit name").
3. Clear the input completely (or type only spaces).
4. Press Enter.
5. Watch the page — no toast, banner, field highlight, or inline message appears anywhere.

**Expected:** A visible error such as "Name must not be empty" next to the field. The server already returns exactly that text.

**Actual:** POST /v1/resource/editName returns 400 with body {"error":"name must not be empty"}. The UI silently reverts to the old name. The only feedback is (a) a console error and (b) a 1x1 visually-hidden role=alert live region containing "Could not save name", which sighted users never see. A user who wanted to clear a name is left guessing whether anything happened. Reproduced 3 times (empty string, whitespace-only, and on a second resource).

**Evidence:** curl -X POST '/v1/resource/editName?id=101' -d '{"name":""}' -> HTTP 400, {"error":"name must not be empty"}. Console: [ERROR] Failed to load resource: the server responded with a status of 400 ... /v1/resource/editName?id=88; [ERROR] Error posting data: Error: Server responded with 400. Live-region scan: the only element containing "Could not save name" has role=alert with computed size 1x1 and position:absolute (screen-reader only). No visible [role=alert] with non-empty text exists on the page.

**Screenshot:** `inline-name-empty-no-visible-error.png`

### 89. At 390x844 the resource header never stacks: a long title is crushed into a 166px column 500px tall with the edit pencil floating in empty space

- **Area:** Single resource page — page header, mobile · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=74`

**Steps**

1. Resize the browser to 390x844.
2. Open http://localhost:8200/resource?id=74 ("A resource with a really extremely long descriptive name that ought to test how the interface truncates things in cards, tables, breadcrumbs and the browser title bar").
3. Look at the header area. Also compare with /resource?id=59 (an ordinary-length title).

**Expected:** On a narrow viewport the title should occupy the full width and the Edit/Delete buttons should wrap to their own row.

**Actual:** The header row stays horizontal, so the H1 is boxed into 166px of a 358px container and grows to 500px tall (about 15 one-or-two-word lines) with a large empty gap to its right; the "Edit name" pencil floats alone in the middle of that empty space and Edit/Delete are pushed to the bottom of the block, ~450px down the page. The same squeeze happens with the ordinary title on /resource?id=59 (3 wrapped lines). No horizontal page overflow (scrollWidth === clientWidth === 390), so it is purely wasted/mis-allocated width.

**Evidence:** Measured at 390x844: {"h1w":166,"h1h":500,"parentW":358,"parentCls":"flex items-end flex-1 min-w-0 gap-3 mt-3"}; {"sw":390,"cw":390}. See also /private/tmp/claude-501/-Users-egecan-Code-mahresources/fd4d26f7-b96a-4d41-9ee7-95edc5a0ba1d/scratchpad/shots/mobile-resource-59.png for the normal-title case.

**Screenshot:** `mobile-resource-74-longname.png`

### 90. Fullscreen "Expand" metadata overlay is not a dialog: Escape does not close it and Tab moves focus to controls hidden behind it

- **Area:** Single resource page — sidebar META DATA "Expand" view · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<resource with meta>`

**Steps**

1. Open a resource that has meta data (I used a resource with meta key hunt_key=hunt_value; any resource with a non-empty META DATA table works).
2. In the right sidebar, click the "Expand" button next to the META DATA heading (aria-label "Expand metadata to fullscreen").
3. Press Escape.
4. Press Tab a few times and inspect document.activeElement.

**Expected:** A full-viewport overlay should be role="dialog" aria-modal="true", close on Escape, and trap focus inside itself.

**Actual:** The overlay element is <div class="tableContainer flex gap-3 flex-col expanded"> with role=null, aria-modal=null and no keydown handler. Escape does nothing (overlay still present). Tab immediately moves focus to page controls that are completely covered by the overlay — "GUID: 019fad1b-...", "Copy Name", "Copy Original Name" — so a keyboard user is focusing invisible elements with no way out except finding the "Minimize" button by tabbing backwards. Reproduced twice.

**Evidence:** Overlay attrs: {"cls":"tableContainer flex gap-3 flex-col expanded","role":null,"modal":null,"keydown":null}. After Escape: {"stillExpanded":true}. Successive Tab presses: {"label":"GUID: 019fad1b-7668-...","inOverlay":false,"visible":true}, {"label":"Copy Name","inOverlay":false}, {"label":"Copy Original Name","inOverlay":false}.

**Screenshot:** `meta-expand.png`

### 91. Clicking "Add Tags" with no tag chosen ejects the user onto a bare HTTP 400 page with no navigation

- **Area:** Single resource page — sidebar Tags widget · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=59`

**Steps**

1. Open http://localhost:8200/resource?id=59 (or any resource detail page).
2. In the right sidebar under TAGS, do NOT type or pick anything in the "Add Tag" box.
3. Click the "Add Tags" button.

**Expected:** Either the button is disabled until at least one tag is selected, or the page shows an inline validation message and stays put.

**Actual:** The browser navigates to /v1/resources/addTags?redirect=... and lands on a standalone "Error 400 / An error has occurred / at least one tag ID is required" page. That page has no site header, no nav (document.querySelectorAll('nav a').length === 0), and a single "Go back" link — the user is thrown out of the application for a one-click mistake, and the message is developer wording ("tag ID"). Reproduced on resource 101 and resource 59.

**Evidence:** Console: [ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/resources/addTags?redirect=%2Fresource%3Fid%3D101. Page after click: {"url":"http://localhost:8200/v1/resources/addTags?redirect=%2Fresource%3Fid%3D59","title":"Error 400","text":"An error has occurred | at least one tag ID is required | Go back","navLinks":0}

**Screenshot:** `addtags-empty-400-deadend.png`

### 92. Tag merge with nothing selected: destructive confirm fires anyway, then dumps the user on a bare 400 page with internal jargon ('one or more losers required')

- **Area:** Tags — merge UI · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/tag?id=84`

**Steps**

1. Open any tag detail page, e.g. http://localhost:8200/tag?id=84. 2. Leave the 'Tags To Merge' selector empty. 3. Click the 'Merge' button in the sidebar. 4. A native confirm appears: 'Selected tags will be deleted and merged to <name>. Are you sure?'. 5. Click OK. 6. You land on http://localhost:8200/v1/tags/merge?redirect=%2Ftag%3Fid%3D84.

**Expected:** With no tags selected the Merge button should be disabled, or clicking it should show an inline 'select at least one tag' message on the page. No destructive confirm should fire when there is nothing to merge.

**Actual:** The scary destructive confirm fires even with an empty selection, and accepting it navigates to a raw API URL that renders a chrome-less error page: heading 'An error has occurred', body 'one or more losers required', and a single unstyled blue 'Go back' link. The message exposes internal terminology ('losers') that means nothing to a user, and the page has none of the app's navigation, so the only way out is the browser back button or that one link.

**Evidence:** Reproduced twice. Console on the error page: '[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/tags/merge?redirect=...'. Page text: 'An error has occurred / one or more losers required / Go back'.

**Screenshot:** `tags-merge-empty-error.png`

### 93. Invalid Meta JSON Schema is saved silently — no validation, no warning, and the schema-driven meta form then degrades to freeform with no hint

- **Area:** Taxonomy / Category template authoring (also Note Type, Resource Category) · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/category/edit?id=74`

**Steps**

1. Go to http://localhost:8200/category/new. 2. Type a name (e.g. hunt-tax-cat). 3. Click into the 'Meta JSON Schema' CodeMirror editor and type `{invalid json here`. 4. Click Save. 5. Observe the category is created and you land on /category?id=<new>. 6. Confirm the garbage is persisted: fetch('/v1/categories?Name=hunt-tax-cat') -> MetaSchema is "{invalid json here}". 7. Repeat on the edit form: open /category/edit?id=<new>, select the Meta JSON Schema editor, Cmd+A, type `not json at all ###`, Save -> persisted verbatim. 8. Now go to /group/new and pick that category in the Category selector: the meta editor silently renders the plain '+ Add Field' freeform block. 9. Compare with the Media category (valid schema): the same spot renders a 'Meta Data (Schema Enforced)' section with Camera / ISO / Shot at fields.

**Expected:** Saving a category whose Meta JSON Schema is not parseable JSON should be blocked, or at minimum warned about, the same way template lint issues are (the form already shows `confirm("This template has 1 issue that may break rendering. Save anyway?")` for shortcode lint errors). A category whose schema is broken should also surface that on the entity form instead of silently falling back.

**Actual:** The invalid JSON is accepted and stored verbatim with zero feedback. The pre-save confirm covers shortcode lint issues only and says nothing about the unparseable schema — I reproduced this with `[property]` in Custom Header AND `not json at all ###` in the schema at the same time, and the confirm still only said 'This template has 1 issue that may break rendering.' Downstream, the group create form silently drops back to the freeform meta editor, so the author gets no signal anywhere that the schema is dead.

**Evidence:** fetch('/v1/categories?Name=hunt-tax-cat') -> {"MetaSchema":"{invalid json here}"} on the first attempt and "not json at all ###" on the second. Screenshot shows the edit form reloaded from the DB with the garbage schema and no error marker anywhere.

**Screenshot:** `category-invalid-metaschema-saved.png`

### 94. Merge pickers offer the tag itself, and self-merge fails with a raw 400 / 'Bulk operation failed: Server error: 400'

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/tags?Name=hunt-tax and http://localhost:8200/tag?id=85`

**Steps**

1. Open /tags?Name=<something matching exactly one tag>.
2. Tick that tag's checkbox, click 'Merge Winner', and pick the same tag as the winner.
3. Click 'Merge' and accept the confirm.
4. Variant: on /tag?id=85, type the tag's own name into the 'Tags To Merge' combobox - the tag itself is offered as an option.

**Expected:** The current/winning tag is excluded from the loser picker, so an impossible merge cannot be constructed.

**Actual:** The tag itself is selectable on both surfaces. Executing it produces an alert 'Bulk operation failed: Server error: 400'; the API rejects it with {"error":"winner cannot also be the loser"}.

**Evidence:** #304/#305 - searching 'hunt-tax' in the 'Tags To Merge' combobox on tag 85's own page returns `["hunt-tax-tag1"]` (that is tag 85). #296/#297 `["alert" dialog with message "Bulk operation failed: Server error: 400"]`. #298/#299 `{"error":"winner cannot also be the loser"}` / `HTTP 400`. Reproduced through the bulk toolbar at #300-#303 (same alert).

### 95. Category template preview iframe floods the console with CORS failures on /v1/account/settings

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. Open /category/edit?id=72 (any category with custom templates).
2. Open the browser console.
3. Click 'Refresh' on the preview panel a couple of times.

**Expected:** The sandboxed preview iframe does not attempt cross-origin app requests, or it is given the settings it needs by the host page.

**Actual:** Script running inside the srcdoc preview iframe (origin 'null') fetches /v1/account/settings and is blocked by CORS. Six errors per render; the count climbs on every preview refresh (6 -> 18 -> 30).

**Evidence:** #80/#81 `[ERROR] Access to fetch at 'http://localhost:8200/v1/account/settings' from origin 'null' has been blocked by CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource. @ about:srcdoc:0` x3 plus `[ERROR] Failed to load resource: net::ERR_FAILED` x3. Repeated verbatim after a reload at #82/#83. Console total grows to `- Console: 18 errors` (#97) and `- Console: 30 errors, 0 warnings` (#281) as the preview is refreshed. #86/#87 confirm the preview is an iframe srcdoc document.

### 96. 'Format JSON' does nothing at all when the JSON is invalid - no change, no error, no announcement

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=73`

**Steps**

1. Open /category/edit?id=73 with an invalid Meta JSON Schema (see the previous finding).
2. Click the 'Format JSON' button above the schema editor.
3. Observe the editor contents and any status/alert region.

**Expected:** The button reports why it cannot format ('Expected property name or '}' at position 2'), which is exactly the message the Visual Editor's Raw JSON tab already computes.

**Actual:** Nothing happens. The content is unchanged, no lint marker appears, and no live region or alert carries a message.

**Evidence:** #118/#119 the button is `<button type="button" @click="formatContent()" ...>`; #120/#121 the live-region sweep after the click returns only unrelated text (`"Rendering..."`, `"Loading media"`, `"0 items ready to upload"`, `"0 items found"` ...) - no error message. Reproduced at #140/#141 on a fresh page load: `{"schema":"{ not valid json ][]}","lint":0}` after clicking 'Format JSON content'.

### 97. Closing the Meta JSON Schema modal drops keyboard focus to <body> instead of returning it to the trigger

- **Area:** Taxonomy / templates / MRQL · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. Open /category/edit?id=72.
2. Tab to (or click) the 'Visual Editor' button and activate it. Focus correctly lands on 'Edit Schema' inside the dialog.
3. Press Escape (or activate 'Cancel').
4. Press Tab and observe where you are.

**Expected:** On close, focus returns to the 'Visual Editor' button that opened the dialog (WCAG 2.4.3 focus order).

**Actual:** Focus is reset to document.body, so the next Tab starts again from the 'Skip to main content' link at the very top of the page. Keyboard users lose their place in a long form.

**Evidence:** Focus is correctly moved *into* the dialog on open - #158/#159 `"Edit Schema | inDialog=true"`. After Escape, #174/#175 `"CLOSED | focus=Skip to main content\n    \n    "`; #176/#177 repeat: `"BODY|Skip to main content"`. Same via the Cancel button at #178/#179: `"BODY|Skip to main content\n    "`.

### 98. Deleting a category that is still in use is confirmed with a generic prompt and silently uncategorizes its groups

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/category?id=73`

**Steps**

1. Create a category and assign at least one group to it.
2. Open the category detail page and click Delete.
3. Accept the confirm.
4. Open the group that used the category.

**Expected:** The confirm states how many groups will be affected (and/or the delete is blocked while the category is in use).

**Actual:** The confirm is the generic 'Are you sure you want to delete?'; after deleting, the group silently falls back to 'Uncategorized' with no warning at any point.

**Evidence:** #258/#259 `["confirm" dialog with message "Are you sure you want to delete?"]`. #260/#261 redirect to `http://localhost:8200/categories`. #262/#263 the group page now reads `"...Groups\nhunt-tax-group1\nUncategorized\nEdit\nUpdated: 2026-07..."` with `Total messages: 0 (Errors: 0, Warnings: 0)`.

### 99. Saved-query Delete buttons are invisible until hover and are below the minimum target size

- **Area:** Taxonomy / templates / MRQL · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open /mrql and scroll to 'Saved Queries'.
2. Without moving the mouse over a row, look for a way to delete a saved query.
3. Repeat at 390x844 (touch viewport), where hover does not exist.

**Expected:** A persistently visible (or at least touch-reachable) delete affordance, at least 24x24 CSS px.

**Actual:** Every delete button is rendered at opacity 0 and only revealed by :hover or :focus, so on a touch device it is invisible with no way to hover. The buttons measure 35x16 px, under the WCAG 2.2 (2.5.8) 24x24 minimum.

**Evidence:** #72/#73 `{"cls":"ml-2 text-xs text-red-600 hover:text-red-800 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity ...","opacity":"0","vis":"visible"}`. #142/#143 at 390x844: three buttons, each `{"op":"0","x":326,...,"w":35,"h":16}`.

### 100. A failing SQL query shows 'Something went wrong.' plus the raw JSON error body

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/query?id=62`

**Steps**

1. Create a query with SQL 'SELECT * FROM nonexistent_table_xyz' at /query/new and save it.
2. Click Run on the resulting query page.

**Expected:** The database message rendered as a message: 'no such table: nonexistent_table_xyz'.

**Actual:** The page renders the heading 'Something went wrong.' followed by the literal response body `{"error":"no such table: nonexistent_table_xyz"}`, plus an empty results box above it.

**Evidence:** #174/#175 `- heading "Something went wrong." [level=2]` / `- paragraph [ref=e70]: "{\"error\":\"no such table: nonexistent_table_xyz\"}"`, with `[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/query/run?id=62:0` and `44. [POST] http://localhost:8200/v1/query/run?id=62 => [400] Bad Request`. The screenshot confirms the raw JSON is what the user sees.

**Screenshot:** `query-broken-sql.png`

### 101. At 390x844 the taxonomy template-authoring forms overflow horizontally and the clipped controls are unreachable (page cannot scroll sideways)

- **Area:** Taxonomy admin — create/edit forms (Category, Note Type, Resource Category) · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. playwright-cli resize 390 844. 2. Go to http://localhost:8200/category/new. 3. Scroll to the 'Reuse & Presets' fieldset. 4. Observe the fieldset extends past the right edge: the body text is cut mid-word ('or an ex…'), and the 'Apply' and 'Copy' buttons next to the two selects are entirely off-screen. 5. Try to scroll right — nothing moves. 6. Repeat on /category/edit?id=72 (worse): every per-slot 'Generate' and 'Format HTML' button plus most of every CodeMirror editor sits off-screen. 7. Same overflow on /noteType/new, /noteType/edit?id=1 and /resourceCategory/new.

**Expected:** On a 390px viewport the form should reflow into a single column so every control is visible and tappable, or the wide region should live in its own horizontally scrollable container.

**Actual:** document.body.scrollWidth = 483 on /category/new, /noteType/new, /noteType/edit?id=1 and /resourceCategory/new, and 1778 on /category/edit?id=72, against a 390px viewport. html and body both have overflow-x:hidden and window.scrollTo(600,0) leaves window.scrollX at 0, so the overflowing content is clipped and cannot be reached by touch or trackpad. Buttons measured completely outside the viewport: 'Apply' at x=398, 'Copy' at x=406, the per-slot 'Generate' buttons at x=880.

**Evidence:** eval on /category/new: {"bodySW":483,"offscreenBtns":["Apply","Copy"]}. eval on /category/edit?id=72: {"bodySW":1778,"offscreenBtns":["Apply","Copy","Generate","Format HTML","Generate","Generate","Format HTML",…]}. eval: {"html":"hidden","body":"hidden"} and window.scrollX stays 0 after window.scrollTo(600,0). No element on the page has overflow-x auto/scroll.

**Screenshot:** `mobile-category-new-presets-clipped.png`

---

## LOW (59)

### 102. Floating "Open jobs panel" button covers the /logs "After" date filter — clicking the date picker opens the Jobs panel

- **Area:** /logs filter sidebar + global floating jobs button · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/logs`

**Steps**

1. Resize the browser viewport to 1280x720 (Playwright's default; a common laptop viewport).
2. Open http://localhost:8200/logs. Do not scroll.
3. Look at the "After" date input at the bottom of the right-hand Filter sidebar — the orange circular "Open jobs panel" FAB sits on top of its right edge.
4. Click the native calendar-picker icon inside the "After" input (approx. x=1248, y=684).

**Expected:** Clicking the date input's calendar icon opens the native date picker.

**Actual:** The click is intercepted by the floating jobs FAB and the Jobs side panel slides open instead. The date picker is unreachable by mouse at this viewport height. document.elementFromPoint over the picker icon returns the FAB's svg. (At 1280x800 the FAB clears the input and the picker opens normally, so the bug is height-dependent.)

**Evidence:** At 1280x720, hit test on the "After" input's picker icon:
{"inputRect":[864,665,400,38],"hitTag":"svg","hitAria":"Open jobs panel"}
Actual click at (1248,684) → Jobs panel opened (the "Open jobs panel" button is gone from the DOM, replaced by "Close jobs panel"; screenshot shows the Jobs drawer).
Reproduced twice (hit-test + real click). Overlap screenshot with the input outlined in red: /private/tmp/claude-501/-Users-egecan-Code-mahresources/fd4d26f7-b96a-4d41-9ee7-95edc5a0ba1d/scratchpad/shots/nav-logs-fab-overlap-hl.png

**Screenshot:** `nav-logs-fab-hijack-720.png`

### 103. Duplicate-upload error is developer wording and does not link to the resource it collided with

- **Area:** /resource/new — upload validation · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource/new`

**Steps**

1. Go to http://localhost:8200/resource/new, pick a file and Save (creates resource N).
2. Go to http://localhost:8200/resource/new again, pick the exact same file, give it a different name, and Save.

**Expected:** Something like "A resource with identical content already exists: <link to that resource>. Open it, or upload anyway."

**Actual:** A red banner reads "Could not save: following errors were encountered: existing resource (114) with same parent". The grammar is broken ("following errors were encountered"), "same parent" is internal jargon, and the id 114 is plain text with no link, so the user cannot jump to the conflicting resource. Reproduced twice. (Positive note: name, description, tag and group selections are correctly restored from the query string; only the file input is cleared, which the banner does not mention.)

**Evidence:** Redirect URL: /resource/new?...&error=following+errors+were+encountered%3A+existing+resource+%28114%29+with+same+parent&... Banner DOM: <div class="... bg-red-50 ..." role="alert" data-testid="form-error-banner"><p><strong>Could not save:</strong> following errors were encountered: existing resource (114) with same parent</p></div>. State retained: {"name":"hunt-resdetail dup-error state test","desc":"Some long description the user typed","chips":["Remove landscape","Remove Landscapes (Media)"]}

**Screenshot:** `upload-in-flight-no-feedback.png`

### 104. Export page: raw Go duration "24h0m0s", off-palette blue "Download tar" link and unstyled green native progress bar

- **Area:** Admin / Export · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/admin/export`

**Steps**

1. Open http://localhost:8200/admin/export.
2. Type "Kyoto" in "Search to add groups...", click "Kyoto Guesthouse", click "Compute estimate", then "Start export".
3. Read the retention sentence under the "Start export" button.
4. Look at the completed-export block: the progress bar and the "Download tar" link.
5. Compare with the "Export/import guide" link at the top of the same page, and with the same retention value on /admin/settings.

**Expected:** Consistent presentation: the retention shown as "24h" (as /admin/settings renders it), links in the app's amber palette, and a progress bar in the app's design system.

**Actual:** (a) The sentence reads "Completed exports are available for download for 24h0m0s after completion" — Go's `time.Duration.String()` output, while /admin/settings shows the same setting as "24h". (b) "Download tar" is `text-blue-700 underline` (bright blue) while every other link on the page, e.g. "Export/import guide", is `text-amber-700`. (c) The progress bar is a bare native `<progress class="w-full">` that renders as a full-width bright green OS bar, which does not appear anywhere else in the stone/amber UI.

**Evidence:** Retention text eval: `"Completed exports are available for download for 24h0m0s after completion, then removed automatically. ..."`; /admin/settings shows the same setting as `24h` and `Boot default: 24h`.
Style eval: `{"dl":{"color":"oklch(0.488 0.243 264.376)","deco":"underline","cls":"text-blue-700 underline self-center"},"docs":{"color":"oklch(0.555 0.163 48.998)","cls":"text-amber-700 hover:text-amber-900 underline"},"progress":{"tag":"PROGRESS","cls":"w-full","accent":"auto"}}`

**Screenshot:** `hunt-admin-export-raw-duration.png`

### 105. Export/import group pickers are bare text inputs with no combobox semantics or result announcement, unlike the app's standard selector

- **Area:** Admin / Export + Import · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/admin/export`

**Steps**

1. Open http://localhost:8200/admin/export.
2. Focus "Search to add groups..." and type `Kyoto`. A `<ul>` of matching groups appears below.
3. Press ArrowDown.
4. Inspect the input's ARIA: `const i=document.querySelector('input[placeholder*=Search]'); ({role:i.getAttribute('role'), exp:i.getAttribute('aria-expanded'), ctrl:i.getAttribute('aria-controls'), ac:i.getAttribute('aria-autocomplete')})`.
5. Compare with the Tags/Groups/Notes pickers on http://localhost:8200/resource/new.

**Expected:** The same autocomplete semantics the app already ships elsewhere: `role=combobox`, `aria-expanded`, `aria-controls` pointing at the listbox, `aria-autocomplete=list`, arrow-key navigation, and a live region announcing how many results appeared.

**Actual:** The export page's group picker (and the import page's Parent Group picker) is a plain `<input>` with only a placeholder/aria-label plus an unlabelled `<ul>` of `<button>`s. `role`, `aria-expanded`, `aria-controls` and `aria-autocomplete` are all null; ArrowDown leaves focus in the input and does nothing; no live region announces that results appeared. A screen-reader user gets no signal that typing produced matches (they are only reachable by blind Tabbing). The app's own selector component on /resource/new does all of this correctly, so the two similar screens behave inconsistently.

**Evidence:** /admin/export eval: `{"ph":"Search to add groups...","role":null,"aria":{"expanded":null,"controls":null,"autocomplete":null}}`; `document.querySelectorAll('[role=combobox]').length` → 0 (the only comboboxes in the DOM belong to hidden global modals).
After typing "Kyoto" + ArrowDown: `document.activeElement` is still `INPUT|Kyoto` (Tab is required to reach `BUTTON|Kyoto Guesthouse`).
Results markup: `<ul x-show="groupResults.length > 0" ...><template x-for="g in groupResults"><li><button type="button" @click="addGroup(g)">…` — no listbox/option roles.
/resource/new eval for comparison: `[{role:"combobox", exp:"false", ctrl:"input_autocompleter_1-listbox", ac:"list"}, …]`.

**Screenshot:** `mobile-admin-export.png`

### 106. Import failure shows the same raw Go error message twice, and it is an internal error chain rather than a user-facing message

- **Area:** Admin / Import · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/admin/import`

**Steps**

1. Create a junk file: `printf 'this is not a tar archive at all, just nonsense text\n' > /tmp/hunt-admin-nonsense.tar`.
2. Open http://localhost:8200/admin/import, click the file chooser and select that file.
3. Click "Upload & Parse".
4. Read the "Parsing Archive" section.
5. Repeat with a second junk file (`printf 'garbage2' > /tmp/hunt-admin-nonsense2.tar`).

**Expected:** One clear, human-readable error, e.g. "This file is not a valid mahresources export archive (expected a .tar containing manifest.json)."

**Actual:** The identical raw Go error chain is printed twice on the same screen: once inline after the phase word ("parsing Error: read manifest: archive: read first entry: unexpected EOF") and again in the red box below ("read manifest: archive: read first entry: unexpected EOF"). The wording exposes internal call-stack vocabulary and gives no guidance. Reproduced identically with both junk files.

**Evidence:** main textContent after upload: `" ... Upload & Parse Uploading... Parsing Archive parsing Error: read manifest: archive: read first entry: unexpected EOF read manifest: archive: read first entry: unexpected EOF "` (message appears twice). Identical output on the second attempt with a different junk file.

**Screenshot:** `import-error-duplicate.png`

### 107. /admin/users can only create and delete users — there is no way to edit one

- **Area:** Admin / Users · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/admin/users`

**Steps**

1. Open http://localhost:8200/admin/users.
2. Create a user (Username `hunt-admin-testuser`, Password `longenough123`, Role `editor`).
3. Look at the new row's "Actions" column and at the row itself for any way to change the user's role, password, display name, disabled flag or scope group.
4. Inspect the row markup and the create form's hidden fields.

**Expected:** An admin screen that manages accounts lets you edit them — at minimum change role, reset password, and enable/disable — since all of these are things an operator routinely needs.

**Actual:** The only per-row action is "Delete". There are no links in the table at all, and the row contains nothing but text cells plus the delete form. Once a user exists, none of their attributes can be changed from the UI, even though the create form carries a hidden `id` input (implying the endpoint supports an update path) and the CLAUDE.md-documented context layer supports `UpdateUser` / `SetUserPassword`.

**Evidence:** Table eval: `{"links":[], "btns":["Delete","Delete"], "rowHTML":"<tr ...><td>3</td><td>hunt-admin-testuser</td><td>hunt-admin test</td><td>editor</td><td>—</td><td>no</td><td><form method=\"post\" action=\"/v1/user/delete\" onsubmit=\"return confirm('Delete user hunt-admin-testuser?');\"><input type=\"hidden\" name=\"id\" value=\"3\"><button type=\"submit\">Delete</button></form></td></tr>"}`
Create-form fields include `{"n":"id","t":"hidden"}`.
(Test users created for this check were deleted afterwards; only `root` remains.)

**Screenshot:** `mobile-users.png`

### 108. Heading order skips a level on the taxonomy create/edit forms — H1 goes straight to H3, no H2 anywhere

- **Area:** Category / Note Type / Resource Category create+edit forms · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. Open http://localhost:8200/category/edit?id=72 (or /category/new). 2. Run: [...document.querySelectorAll('h1,h2,h3,h4,h5,h6')].map(h => h.tagName + ':' + h.innerText.trim()). 3. Observe the sequence.

**Expected:** Headings should not skip levels (WCAG 1.3.1 / axe 'heading-order'). The major form regions ('Live preview', 'Section Visibility' sub-blocks, the Reference panel) should be H2 under the page H1, with H3 only nested beneath an H2.

**Actual:** The visible heading sequence is H1 'Edit Category' -> H3 'Slot Locations' -> H3 'Shortcodes' -> H3 'HTML & Styling' -> H3 'Alpine.js' -> H3 'Live preview' -> H3 'Main Content' -> H3 'Own Entities' -> H3 'Related Entities' -> H3 'Relations' -> H3 'Sidebar'. There is no H2 in the page content at all — the only H2s in the DOM ('Edit Tags', 'Info', 'Upload to Unknown', 'Select Items') belong to hidden global modals (offsetParent === null). Screen-reader users navigating by heading level see the whole form's structure collapse.

**Evidence:** eval on /category/new: ["H1:Create Category:vis=true","H3:Slot Locations:vis=false","H3:Shortcodes:vis=false","H3:HTML & Styling:vis=false","H3:Alpine.js:vis=false","H3:Live preview:vis=true","H3:Main Content:vis=true","H3:Own Entities:vis=true","H3:Related Entities:vis=true","H3:Relations:vis=true","H3:Sidebar:vis=true","H2:Edit Tags:vis=false",…]. Same on /category/edit?id=72.

### 109. User-create password field has no client-side minlength although the server enforces 8 characters

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/users`

**Steps**

1. Open /admin/users
2. Inspect the create-user form inputs (username / displayName / password / role)
3. Submit a 3-character password

**Expected:** The password input carries minlength=8 (and a hint) so the browser blocks the submit before a round trip.

**Actual:** password is required=true but minLength=-1, so a too-short password is only caught by the server, which then throws the admin onto the raw error page described above.

**Evidence:** #304/#305: `[{"n":"username","req":true,"minlen":-1,"type":"text"},{"n":"displayName","req":false,"minlen":-1,"type":"text"},{"n":"password","req":true,"minlen":-1,"type":"password"},...]`, together with the server response at #230/#231 `password must be at least 8 characters`.

### 110. Admin pages render two visible <h1> elements (duplicate on /admin/shares, two different ones on /admin/settings)

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/shares`

**Steps**

1. Open /admin/shares
2. List the visible headings, e.g. [...document.querySelectorAll('h1,h2,h3')].filter(h=>h.offsetParent!==null)
3. Repeat on /admin/settings

**Expected:** One <h1> per page; secondary titles demoted to <h2>.

**Actual:** /admin/shares has 'Shared Notes' twice as <h1>. /admin/settings has 'Settings' and 'Runtime Settings' both as <h1>. Heading-based navigation therefore reports two page titles.

**Evidence:** #174/#175 (visible-only heading dump): `== /admin/shares` -> `["H1:Shared Notes","H1:Shared Notes"]`; `== /admin/settings` -> `["H1:Settings","H1:Runtime Settings","H2:uploads","H2:queries",...]`. Other pages in the same sweep have exactly one H1 (`== /logs` -> `["H1:Logs","H2:Filter"]`, `== /admin/export` -> `["H1:Export Groups",...]`). The agent also captured admin-users-double-h1.png at #298.

**Screenshot:** `admin-shares-double-h1.png`

### 111. There is no Series list page, and /series without an id returns a 404 whose only detail is the raw ORM message 'record not found'

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/series`

**Steps**

1. Open http://localhost:8200/series (no query string)
2. Try /seriesList and /serieses
3. Look for a Series entry in the header navigation

**Expected:** Either a series index page, or a 404 that explains that /series needs an id. The user-facing error should not be a database-layer string.

**Actual:** /series, /seriesList and /serieses are all 404. The /series page renders 'Error 404' with the H3 'record not found'. Only the detail route exists (/series?id=1 works), and no header link points to series at all, so series are only reachable from a resource.

**Evidence:** #174/#175: `== /series` -> `["H1:Error 404","H3:record not found"]`. #176/#177: `== /series 404`, `== /seriesList 404`, `== /serieses 404`, and `server/routes.go:85: "/series": {adaptTemplate(template_context_providers.SeriesContextProvider), "displaySeries.tpl", http.MethodGet}` — a detail route only. #178/#179 confirms `/series?id=1` renders `heading "Series Iceland Ri..."`. Header link inventory (digest abe06580 #134/#135) contains no /series entry: `["/dashboard","/notes","/resources","/tags","/groups","/mrql","/admin/overview","/queries","/categories","/resourceCategories","/relations","/relationTypes","/noteTypes","/templatePartials",...]`.

### 112. Settings section headings show raw snake_case config group keys ('remote_downloads', 'mrql') instead of human labels

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/settings`

**Steps**

1. Open /admin/settings
2. Read the section headings above each group of settings

**Expected:** Human-readable section titles, e.g. 'Remote downloads'.

**Actual:** The raw config group keys are printed and merely CSS-capitalized, so the user sees 'Uploads', 'Queries', 'Remote_downloads', 'Deduplication' — the underscore is visible because text-transform: capitalize cannot fix it.

**Evidence:** #248/#249: `[{"txt":"uploads","tt":"capitalize"},{"txt":"queries","tt":"capitalize"},{"txt":"remote_downloads","tt":"capitalize"},{"txt":"sharing","tt":"capita...` and the heading dump at #28/#29: `["H1: Settings","H1: Runtime Settings","H2: uploads","H2: queries","H2: remote_downloads","H2: sharing","H2: docs","H2: deduplication","H2: exports",...]`.

**Screenshot:** `settings-raw-section-key.png`

### 113. Download progress bar has a truncated accessible name ('Download progress: ') and stays at aria-valuenow=0 for unknown-size downloads

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/dashboard (jobs panel with a download in flight)`

**Steps**

1. Start a remote download (POST /v1/download/submit, or /resource/new with 'Download in background')
2. Open the jobs panel while the job is running
3. Inspect the [role=progressbar] element's aria-label / aria-valuenow / aria-valuetext

**Expected:** The label names the job ('Download progress: 1Gb.dat') and, when the total size is unknown, the bar is indeterminate (no aria-valuenow) or carries an aria-valuetext describing bytes transferred.

**Actual:** aria-label is the bare prefix 'Download progress: ' with nothing after the colon, aria-valuenow is pinned at 0 and aria-valuetext is null, so a screen reader announces an unnamed 0% bar for the whole transfer.

**Evidence:** #112/#113: `"[{\"label\":\"Download progress: \",\"now\":\"0\",\"min\":\"0\",\"max\":\"100\",\"txt\":null}]"` while the job was in flight (the submit response at #109 reports `"totalSize":-1,"progressPercent":-1`).

**Screenshot:** `jobs-panel-names.png`

### 114. Unknown /v1/* API paths return the HTML 404 page instead of a JSON error, even with Accept: application/json ✅ **VERIFIED**

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/v1/resources/count, http://localhost:8200/v1/jobs`

**Steps**

1. curl -H 'Accept: application/json' http://localhost:8200/v1/resources/count
2. curl -H 'Accept: application/json' http://localhost:8200/v1/jobs
3. Try to parse the body as JSON

**Expected:** A JSON error body ({"error":"..."}) for a request under the /v1 JSON API prefix.

**Actual:** The full HTML error page is returned, so any API client's JSON parse blows up instead of surfacing a clean 404.

**Evidence:** Digest abe06580 #142/#143: `<!DOCTYPE html> ... <title>404 Not Found - mahresources</title>`. Digest a7e14caa #118/#119: `curl POST /v1/jobs/268cbe7f/cancel` and `curl -H 'Accept: application/json' /v1/jobs` both returned `<!DOCTYPE html><html lang="en"><head>...`; the JSON parse at #114/#115 failed with `Exit code 1`. Note the real endpoints do answer JSON (e.g. `/v1/jobs/cancel?id=` -> `{"error":"job id is required"}` at #122/#123), so the gap is specific to unmatched /v1 paths.

**Verified by me (re-run against the live server):** `GET /v1/nonexistent-endpoint with Accept: application/json -> HTTP 404, Content-Type text/html`

### 115. Numeric runtime settings are plain text inputs with no client-side validation; a non-numeric value is only rejected by the server with HTTP 400

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/admin/settings`

**Steps**

1. Open /admin/settings
2. Type 'abc' into 'MRQL default LIMIT' (an int64 setting)
3. Click its Save button
4. Watch the network log / console

**Expected:** The field is type=number (or validated client-side) so 'abc' cannot be submitted at all.

**Actual:** The field accepts arbitrary text (the a11y tree reports role textbox, not spinbutton), the PUT goes out and comes back 400, and the failure is visible in the console as a resource-load error.

**Evidence:** #48/#49 the fill was accepted: `await page.getByRole('textbox', { name: 'MRQL default LIMIT', exact: true }).fill('abc');`. #50/#51: `[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/admin/settings/mrql_default_limit:0` and `18. [PUT] http://localhost:8200/v1/admin/settings/mrql_default_limit => [400] Bad Request`. Other numeric settings are textboxes too: #257 `- textbox "Hash aHash threshold" [ref=e181]: "5"`, #263 `- textbox "MRQL page query budget" [ref=e85]: "200"`.

**Screenshot:** `settings-raw-error.png`

### 116. Entity detail pages lose the main-nav active state, and the active nav item is never marked with aria-current

- **Area:** Dashboard / nav / search / logs / admin · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=85, /note?id=61, /group?id=68`

**Steps**

1. Visit /resources — 'Resources' is highlighted in the header
2. Click into any resource (/resource?id=85) — the highlight disappears
3. Repeat with /note?id=61 and /group?id=68
4. On /resources, inspect the nav links' aria-current attribute

**Expected:** A detail page keeps its section highlighted, and the active link carries aria-current="page" so it is announced, not just coloured.

**Actual:** Detail pages highlight nothing, so there is no visual indication of where you are in the app. On list pages the state is conveyed only by the CSS class navbar-link--active; aria-current is null on every nav link, so assistive tech gets no signal at all.

**Evidence:** #166/#167: `/dashboard => ["Dashboard[]","Dashboard[]"]`, `/resources => ["Resources[]","Resources[]"]`, but `/resource?id=85 => []` and `/note?id=61 => []`. #168/#169: `[{"t":"Dashboard","cls":"navbar-link ","cur":null},{"t":"Notes","cls":"navbar-link ","cur":null},{"t":"Resources","cls":"navbar-link navbar-link--active","cur":null},...]`.

### 117. Dashboard "Recent Activity" is the only widget without a "View All →" link to its full view (/logs)

- **Area:** Dashboard widgets · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/dashboard`

**Steps**

1. Open http://localhost:8200/dashboard.
2. Compare the header row of each widget: Recent Resources, Recent Notes, Recent Groups, Recent Tags each show a "View All →" link at the right.
3. Scroll to "Recent Activity" and look for the same link.

**Expected:** Consistent with its four sibling widgets, Recent Activity should link to its full view (/logs), which is the same data set with filters and pagination.

**Actual:** Recent Activity has no "View All →" link. The full activity log lives at /logs, which is only reachable through the Admin dropdown, so the dashboard's most time-sensitive widget is the one with no path to its full view.

**Evidence:** eval over dashboard h2 sections:
"Recent Resources => viewAll=/resources"
"Recent Notes => viewAll=/notes"
"Recent Groups => viewAll=/groups"
"Recent Tags => viewAll=/tags"
"Recent Activity => viewAll=NONE"

**Screenshot:** `nav-dashboard-desktop.png`

### 118. Dashboard Recent Activity <time datetime> values are local times labelled as UTC (off by the UTC offset, into the future) ✅ **VERIFIED**

- **Area:** Dashboard — Recent Activity widget · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/dashboard`

**Steps**

1. Open http://localhost:8200/dashboard in a browser whose timezone is not UTC (this machine: Europe/Istanbul, UTC+3).
2. Inspect the Recent Activity list items: each has a <time datetime="..."> element.
3. Compare the datetime attribute against the real current time: eval `const t=document.querySelector('time'); ({label:t.textContent, dt:t.getAttribute('datetime'), nowUTC:new Date().toISOString()})`.

**Expected:** The machine-readable datetime attribute should be a correct instant — either a real UTC value with Z, or a local value with the proper offset (e.g. +03:00). It must not resolve to a moment in the future.

**Actual:** The attribute carries the server's local wall-clock time but appends a "Z" (UTC) suffix, so every timestamp parses ~3 hours in the future. The visible label ("just now", "2 minutes ago") is computed server-side and is correct, but the datetime attribute — which assistive tech, browser tooltips and scrapers read — is wrong by the full UTC offset. All 20 <time> elements in the widget are affected.

**Evidence:** {
  "browserNowUTC": "2026-07-29T09:25:23.483Z",
  "browserNowLocal": "Wed Jul 29 2026 12:25:23",
  "samples": [
    {"label":"just now","datetime":"2026-07-29T12:25:09Z","minutesInFuture":180},
    {"label":"2 minutes ago","datetime":"2026-07-29T12:23:02Z","minutesInFuture":178},
    {"label":"28 minutes ago","datetime":"2026-07-29T11:57:09Z","minutesInFuture":152}
  ]
}
Reproduced on three separate page loads. /logs, /notes, /resources and /groups contain no <time> elements, so the dashboard widget is the only affected surface.

**Verified by me (re-run against the live server):** `dashboard emits datetime="2026-07-29T12:51:56Z" while actual UTC was 10:07 and server local was 13:07+0300 — local time stamped Z`

**Screenshot:** `nav-dashboard-desktop.png`

### 119. Two inconsistent 404 pages; entity-not-found leaks the raw ORM string "record not found" and neither offers a way back

- **Area:** Error pages / 404 handling · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=999999`

**Steps**

1. Open http://localhost:8200/does-not-exist → h1 "404 Not Found", body "Page not found", document.title "404 Not Found - mahresources".
2. Open http://localhost:8200/resource?id=999999 → h1 "Error 404", body "record not found", document.title "Error 404 - mahresources".
3. Repeat step 2 with /note?id=999999, /group?id=999999, /tag?id=999999, /log?id=999999 and /series — all show "Error 404 / record not found".
4. On either page, look inside <main> for any link.

**Expected:** One consistent 404 presentation, a human-readable message that says what was not found (e.g. "Resource #999999 does not exist"), and at least one recovery link ("Back to Resources" / "Go to dashboard").

**Actual:** Two different 404 templates with different h1 text and different document.title. The entity variant surfaces the raw GORM error string "record not found", which tells the user nothing about which entity or id, and mentions no next step. Neither page contains a single link inside <main> — the only way out is the top nav (which, per the sticky-header finding, is the sole affordance). HTTP status codes are correct (404) in all cases.

**Evidence:** fetch statuses/titles:
/does-not-exist => HTTP 404 | title=404 Not Found - mahresources
/series => HTTP 404 | title=Error 404 - mahresources
/resource?id=999999 => HTTP 404 | title=Error 404 - mahresources
/note?id=999999 => HTTP 404 | title=Error 404 - mahresources
/group?id=999999 => HTTP 404 | title=Error 404 - mahresources
/tag?id=999999 => HTTP 404 | title=Error 404 - mahresources
/log?id=999999 => HTTP 404 | title=Error 404 - mahresources
main innerText on /resource?id=999999: "record not found"
main innerText on /does-not-exist: "Page not found"; links in main: []
Generic-404 screenshot: /private/tmp/claude-501/-Users-egecan-Code-mahresources/fd4d26f7-b96a-4d41-9ee7-95edc5a0ba1d/scratchpad/shots/nav-404-generic.png

**Screenshot:** `nav-404-record-not-found.png`

### 120. Header declares position:sticky but never sticks — nav and search scroll out of reach on long pages

- **Area:** Global header / navigation · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources`

**Steps**

1. Open http://localhost:8200/resources at 1280x720.
2. Confirm the header's intent: eval `getComputedStyle(document.querySelector('header'))` → position "sticky", top "0px", z-index "40".
3. Scroll down 1500px.
4. Observe the header. Repeat on /logs (scroll to 2000).

**Expected:** With position:sticky; top:0 the header should pin to the top of the viewport so Dashboard/Notes/Resources/Groups/MRQL and the ⌘K search stay reachable while scrolling a long list.

**Actual:** The header scrolls completely off-screen (getBoundingClientRect().bottom = -1464 at scrollY 1500). The sticky declaration is inert because <body> is display:grid — the header is a grid item, so its containing block is its own ~36px-tall grid row and it has nowhere to stick. Users on any long list page (resources, notes, logs) must scroll all the way back to the top to reach navigation or global search.

**Evidence:** Header computed style: {"pos":"sticky","top":"0px","z":"40"}
After scrollTo(0,1500) on /resources: {"scrollY":1500,"headerBottom":-1464,"stickyDeclared":"sticky"}
After scrollTo(0,2000) on /logs: {"scrollY":1526,"headerTop":-1526,"headerVisible":false}
Ancestor chain: BODY overflow=hidden auto display=grid height=2246px; HTML overflow=hidden auto display=block height=720px

**Screenshot:** `nav-header-not-sticky.png`

### 121. Active main-nav link has no aria-current="page" — current location is conveyed by colour only

- **Area:** Global header / navigation · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/notes`

**Steps**

1. Open http://localhost:8200/notes.
2. Inspect the header links: eval `[...document.querySelectorAll('header a')].slice(0,3).map(a=>a.outerHTML)`.
3. Repeat on /resources, /tags, /groups, /mrql, /logs, /categories.

**Expected:** The nav link matching the current page should carry aria-current="page" so screen-reader and voice users can tell where they are. The app already does this correctly for pagination (aria-current="page" on the current page link in /logs pagination), so this is an internal inconsistency.

**Actual:** The active link only gets a visual CSS class (navbar-link--active, amber pill). No aria-current attribute is set on any header link, on any of the seven pages checked. The current location is communicated by colour/background alone.

**Evidence:** On /notes: `<a href="/notes" class="navbar-link navbar-link--active">Notes</a>` — no aria-current.
Sweep across pages, filtering for aria-current or an active class:
/notes => "Notes[cur=-],Notes[cur=-]"
/resources => "Resources[cur=-],Resources[cur=-]"
/tags => "Tags[cur=-],Tags[cur=-]"
/groups => "Groups[cur=-],Groups[cur=-]"
/mrql => "MRQL[cur=-],MRQL[cur=-]"
/logs => "Logs[cur=-],Logs[cur=-]"
/categories => "Categories[cur=-],Categories[cur=-]"
For contrast, /logs pagination: "A:1:cur=page | A:2:cur=- | ..."

**Screenshot:** `nav-dashboard-desktop.png`

### 122. Global search shows a completely blank body for a 1-character query — no results, no empty state, no hint

- **Area:** Global search (Cmd/Ctrl+K dialog) · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/dashboard`

**Steps**

1. Open http://localhost:8200/dashboard and press Cmd+K (or Ctrl+K).
2. Note the initial state: "Start typing to search / Search across all your resources, notes, groups, and more".
3. Type a single character, e.g. "a".
4. Wait 2 seconds.
5. Type a second character to make "ab" — results appear. Delete back to one character ("z") — blank again.

**Expected:** Either search on one character, or keep showing the "Start typing to search" placeholder / show an explicit hint such as "Type at least 2 characters". The panel should never go silently empty.

**Actual:** With exactly one character the dialog body collapses to nothing — the placeholder disappears, no results render, no "No results found" message, and no minimum-length hint. The user sees just the input and the keyboard-shortcut footer, with no explanation of why nothing happened. With 0 characters the placeholder is shown, with 2+ characters results or "No results found for …" are shown — so one character is the only unhandled state.

**Evidence:** query "a"  → dialog innerText: "↑\n↓\nnavigate\n↵\nselect\nesc\nclose"  (body empty)
query "ab" → "📝\nAB Test Results\nNote\n\nDetailed content for AB Test Results\n…"
query "z"  → "↑\n↓\nnavigate\n↵\nselect\nesc\nclose"  (body empty again)
query "zzzqqqnothinghere" → "No results found for \"zzzqqqnothinghere\"\n\nTry a different search term"

**Screenshot:** `nav-search-1char-blank.png`

### 123. Opening the lightbox "Info" panel autofocuses the Name field, which swallows Arrow keys and silently kills image navigation

- **Area:** Lightbox — Info / details panel · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources`

**Steps**

1. Open http://localhost:8200/resources and click any image thumbnail to open the lightbox.
2. Press ArrowRight a couple of times — the counter advances (e.g. 2/36 -> 3/36 -> 4/36) and the image changes. Arrow navigation works.
3. Click the "Info" button in the bottom toolbar.
4. Press ArrowRight twice more.

**Expected:** Arrow keys should keep navigating between images while the Info panel is open (the panel is meant for browsing details as you move), or the panel should not steal focus into an editable field.

**Actual:** Opening Info moves focus into the editable <input placeholder="Resource name">, so ArrowLeft/ArrowRight are consumed as caret movement and image navigation stops dead with no indication why. The counter stays frozen ("2 / 39") and the panel keeps showing the first item. Blurring the input (document.activeElement.blur()) immediately restores arrow navigation, confirming the cause. Reproduced twice.

**Evidence:** Run 1: after Info + 3x ArrowRight -> dialogLabel stayed "hunt-admin FG named download" and info name/size/dims unchanged across all three presses; document.activeElement = {"active":"INPUT/Resource name"}; after blur + ArrowRight -> {"dialogLabel":"300","infoName":"300"}. Run 2: after Info + 2x ArrowRight -> {"label":"hunt-resdetail alpha transparency test","counter":"2 / 39","active":"Resource name"}.

**Screenshot:** `lightbox-info-arrow-trap.png`

### 124. Focus is dropped to <body> after deleting a saved MRQL query

- **Area:** MRQL editor (/mrql) — Saved Queries panel · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open http://localhost:8200/mrql
2. Type any valid query (e.g. type = note LIMIT 2), click Save, name it "kbtest", click Save in the dialog.
3. Move keyboard focus to the "Delete saved query: kbtest" button (Tab through the Saved Queries list) and press Enter.
4. Accept the browser confirm.
5. Inspect document.activeElement.

**Expected:** After removing a list item, focus moves to a sensible neighbour — the next/previous saved query, or the "Saved Queries" heading/list container — so keyboard and screen-reader users keep their place.

**Actual:** document.activeElement is BODY. A keyboard user is dumped back to the top of the document and must Tab through the whole page (nav, search, NL generate box, editor, Run/Explain/Save) to get back to the Saved Queries list. Note the Save dialog itself handles focus correctly (traps Tab, Escape closes it, focus returns to the Save button), so the delete path is the outlier.

**Evidence:** After keyboard-triggered delete + dialog-accept, eval of document.activeElement.tagName returned "BODY". Reproduced twice — once via mouse click on the Delete button, once via keyboard focus + Enter.

### 125. MRQL results header says "1 items" and shows a truncation warning when nothing was truncated

- **Area:** MRQL editor (/mrql) — results header · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/mrql?q=type+%3D+resource+AND+name+~+%22sunset%22`

**Steps**

1. Open http://localhost:8200/mrql?q=type+%3D+resource+AND+name+~+%22sunset%22 (or type type = resource AND name ~ "sunset" and press Cmd+Enter).
2. Read the Results heading and the yellow banner beneath it.

**Expected:** "Results (1 item)", and no truncation warning when the result count is far below the applied limit (or a neutral, non-warning note).

**Actual:** The heading reads "Results (1 items)" — always the plural form regardless of count. Directly beneath it a full-width yellow warning banner reads "Default limit applied (500 rows) — add LIMIT / OFFSET to the query to paginate." even though only 1 row came back and nothing was cut off. The same banner appears on a GROUP BY returning 7 rows. Users reasonably read the warning as "your results were truncated".

**Evidence:** eval of [aria-label="Query results"] innerText: "Results (1 items)\nExport CSV\nExport JSON\nEntity: resource\nDefault limit applied (500 rows) — add LIMIT / OFFSET to the query to paginate.\n\nSunset at G...". Reproduced for 1-item, 7-row and 0-item result sets; the banner correctly disappears once an explicit LIMIT is in the query.

**Screenshot:** `mrql-1-items-grammar.png`

### 126. A todos block with no items renders as a completely blank card, while every other empty block type shows an empty-state message

- **Area:** Note blocks / rendering · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=67`

**Steps**

1. Create a note, click "Edit Blocks", add one block of each type (Todos, References, Gallery, Table), then click "Done".
2. Compare how the four empty blocks render in view mode.

**Expected:** Consistent empty states. The todos block should say something like "No items yet" (or the empty card should not be rendered at all), matching the other block types.

**Actual:** References renders "No groups selected", Gallery renders "No resources selected", Table renders "No table data" — but Todos renders nothing at all: a bordered white card with zero content and zero height inside. A reader has no idea what that empty box is or how to fill it.

**Evidence:** Per-block DOM read in view mode:
  blockTodos      -> text: "",  content height: 0
  blockReferences -> text: "No groups selected",  height 20
  blockGallery    -> text: "No resources selected", height 20
  blockTable      -> text: "No table data", height 20
Console: 0 errors.

**Screenshot:** `notes-empty-todos-block.png`

### 127. Note detail pages skip a heading level: H1 "Note" is followed by an H3 (the owner group card) before any H2

- **Area:** Note detail (a11y) · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=61`

**Steps**

1. Open http://localhost:8200/note?id=61 (any note that has an Owner group; also reproduces on ?id=62 and ?id=64).
2. Enumerate visible headings: [...document.querySelectorAll('h1,h2,h3')].filter(h=>h.offsetParent).map(h=>h.tagName+':'+h.textContent.trim())

**Expected:** Heading levels must not skip. The owner card's title inside the sidebar disclosure should be an H2 (or not a heading at all), so the outline reads H1 -> H2 -> ...

**Actual:** The document outline is H1 "Note" -> H3 <owner group name> -> H2 "Note Type" -> H2 "Tags" -> ... The H3 comes from the reusable group card (<h3 class="card-title">) embedded in the sidebar's "Owner:" disclosure, so every note that has an owner has a level skip immediately after the H1. Screen-reader users navigating by heading level get a broken outline.

**Evidence:** note 61: ["H1:Note","H3:Web App Redesign","H2:Note Type","H2:Tags","H2:Meta Data","H2:Sharing","H2:Attendees & agenda","H2:Groups","H2:Resources"]
note 62: ["H1:Note","H3:API Migration","H2:Note Type",...]
note 64: ["H1:Note","H3:Iceland March","H2:Note Type",...]
Offending element: <h3 class="card-title"><a href="/group?id=77" title="Iceland March">Iceland March</a></h3>, inside a <details>.

**Screenshot:** `notes-note61-initial.png`

### 128. "Unshare" permanently invalidates the public link on a single click with no confirmation, and re-sharing mints a different token

- **Area:** Note sharing · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=67`

**Steps**

1. Open a note and click "Share Note" in the sidebar. Record the token path shown (e.g. /s/747a77ebc33760be5bc43947ecf03ded).
2. Click "Unshare". Observe: it acts immediately, no confirm dialog, no undo.
3. Click "Share Note" again and record the new token path.
4. Repeat steps 2-3 once more.

**Expected:** Revoking a public link is destructive and irreversible for anyone holding the old URL, so it should ask for confirmation (the note "Delete" button and the block "Delete" button both do — "Are you sure you want to delete?" / "Delete this block?"). It is also inconsistent that the two other destructive actions on the same page confirm and this one does not.

**Actual:** One click revokes with no prompt. Re-sharing produces a brand-new token, so every previously distributed URL is dead with no way to restore it. Observed token sequence on the same note: /s/747a77ebc33760be5bc43947ecf03ded -> (unshare) -> /s/6ae34840d5cd6e8e84e07f5ac7625924 -> (unshare) -> /s/9d336dd34c015bcc3981ad081c378026.

**Evidence:** Clicking "Unshare" produced no modal state (playwright-cli reported no dialog; contrast with the note Delete button which reports `["confirm" dialog with message "Are you sure you want to delete?"]`).
Token after 1st share: /s/747a77ebc33760be5bc43947ecf03ded
After unshare + reshare: /s/6ae34840d5cd6e8e84e07f5ac7625924
After unshare + reshare again: /s/9d336dd34c015bcc3981ad081c378026

**Screenshot:** `notes-share-sidebar.png`

### 129. Edit forms have no Cancel or Back affordance

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/note/edit?id=66`

**Steps**

1. Open http://localhost:8200/note/edit?id=66 (also /group/edit?id=41 and /resource/edit?id=65).
2. Look for a Cancel button, a link back to the entity, or a breadcrumb.

**Expected:** A Cancel / back-to-entity link so an edit can be abandoned without the browser back button.

**Actual:** The only in-form controls are "+ Add Field" and "Save" (plus per-tag Remove buttons); every link on the page is global navigation, and there is no breadcrumb.

**Evidence:** #151 (notes digest) for all three forms, e.g. "{\"h1\":\"Edit Note\",\"links\":[\"Dashboard->/dashboard\",\"Notes->/notes\", ...],\"buttons\":[\"+ Add Field\",\"Save\"]}" and "{\"h1\":\"Edit Group\",...,\"buttons\":[\"Remove Dataset\",\"Remove finance\",\"Remove legal\",\"+ Add Field\",\"Save\"]}". Corroborated in the groups digest #83: "{\"links\":[],\"hasBreadcrumb\":false}" for /group/edit?id=88.

### 130. Destructive and structural actions use native browser confirm() dialogs

- **Area:** Notes / blocks / groups / relations · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/note?id=66 (Edit Blocks) and http://localhost:8200/group?id=90`

**Steps**

1. Open a note, click "Edit Blocks", then click the "Delete block 2" button.
2. Open a group and click "Clone".
3. Open a relation and click "Delete".

**Expected:** Confirmations use the app's own accessible modal (consistent styling, focus management, an explanation of what will be lost), as the app already does for its entity pickers.

**Actual:** All three fall back to the browser's native confirm() with terse text, inconsistent with the rest of the UI and unstyleable.

**Evidence:** #83 (notes): '### Modal state - ["confirm" dialog with message "Delete this block?"]'. #103 (groups): '### Modal state - ["confirm" dialog with message "Clone thi'. #207 (groups): '### Modal state - ["confirm" dialog with message "Are you sure you wa'.

### 131. "Compare" bulk action silently disappears when a third group is selected

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/groups`

**Steps**

1. Open http://localhost:8200/groups.
2. Tick two group cards - a "Compare" action appears in the bulk toolbar.
3. Tick a third group card and look at the toolbar again.

**Expected:** The action stays visible but disabled with a hint ("select exactly two groups to compare").

**Actual:** The Compare link collapses to zero width (disappears) with no explanation, while still carrying the stale href for the first two selections.

**Evidence:** #121 with 2 selected: "{\"tag\":\"A\",\"href\":\"/group/compare?g1=91&g2=90\",\"vis\":true,\"rect\":[341.375,189,92.8125,38]}". #123 with 3 selected: "{\"href\":\"/group/compare?g1=91&g2=90\",\"w\":0,...}".

**Screenshot:** `groups-bulk-toolbar.png`

### 132. Group compare with a nonexistent group id shows a bare "Error 404 / record not found" page

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/group/compare?g1=65&g2=99999`

**Steps**

1. Open http://localhost:8200/group/compare?g1=65&g2=99999.

**Expected:** A friendly message naming the missing group with a way back to the compare picker or the groups list.

**Actual:** A raw "Error 404" page whose only body text is the internal phrase "record not found".

**Evidence:** #113: "== g1=65&g2=99999 -> 404" (and "g1=abc&g2=70 -> 400"). #115: "...PLUGINS\nSearch\n⌘K\n⚙\nError 404\nrecord not found".

**Screenshot:** `compare-404.png`

### 133. Description textarea exposes role=combobox without aria-controls

- **Area:** Notes / blocks / groups / relations · **Kind:** a11y · **Provenance:** recovered
- **URL:** `http://localhost:8200/group/edit?id=88`

**Steps**

1. Open http://localhost:8200/group/edit?id=88 (or http://localhost:8200/note/edit?id=61).
2. Inspect the Description textarea's ARIA attributes.

**Expected:** A combobox declares the popup it controls (aria-controls / aria-owns) so assistive tech can follow the suggestion list.

**Actual:** role="combobox" with aria-expanded="false" and aria-autocomplete="list" but aria-controls=null (and no aria-label/aria-labelledby of its own).

**Evidence:** #87 (groups digest): "{\"role\":\"combobox\",\"exp\":\"false\",\"ac\":\"list\",\"controls\":null,\"labelledby\":null,\"al\":null,\"id\":\"Description\"}". Same pattern on the note edit page - notes digest #145: "[{\"name\":\"Description\",\"role\":\"combobox\",\"list\":null,\"aria\":null,\"id\":\"Description\",\"labelFor\":\"Text\",...}]".

### 134. Single-quoted string in the groups MRQL filter bar returns an empty list instead of a syntax error

- **Area:** Notes / blocks / groups / relations · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/groups?mrql=name+%7E+%27Photo%27`

**Steps**

1. Open http://localhost:8200/groups.
2. Type name ~ 'Photo' (single quotes) into the MRQL filter bar and submit.
3. Compare with name ~ "Photo" (double quotes).

**Expected:** Either single quotes are accepted, or the filter bar reports the syntax error.

**Actual:** The single-quoted query yields zero group cards with no visible error in the filter area, while the double-quoted equivalent returns 2 groups. A user who types the wrong quote style just sees "nothing matched".

**Evidence:** #25 the URL after submit: "http://localhost:8200/groups?mrql=name+%7E+%27Photo%27". #27: "[]" cards, with main text reading only "List\nText\nTree\nTimeline\nSelect All\nThe MRQL editor and sidebar form cannot represent the same filters. The form is disabled.\nUse form values\n...". #29 with double quotes: "[\"Street Photography\",\"Photography\"]". Note: only the first 400 characters of main were captured, so an error banner further down cannot be fully ruled out.

### 135. Missing note shows the raw ORM string "record not found" and offers no way back to the notes list

- **Area:** Notes error handling · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/note?id=99999`

**Steps**

1. Open http://localhost:8200/note?id=99999 (also /note/edit?id=99999, /note/text?id=99999, and /note with no id — all 404).
2. Read the error message and look for a recovery action.

**Expected:** A user-facing message such as "This note doesn't exist or has been deleted." plus a link back to /notes. Also a correct heading outline.

**Actual:** The page shows only H1 "Error 404" and H3 "record not found" — GORM's internal error string leaked to the UI. There is no "Back to notes" link or any recovery action; the only way out is the top nav. The heading level also skips from h1 to h3 (the page contains no visible h2), which axe-core's heading-order rule flags. Identical behaviour on /resource?id=99999 and /group?id=99999, so it is a shared error template.

**Evidence:** HTTP status codes: /note?id=99999 -> 404, /note?id=abc -> 400, /note/edit?id=99999 -> 404, /note/text?id=99999 -> 404, /note -> 404.
Visible headings on the 404 page: ["H1:Error 404","H3:record not found"] (remaining H2s belong to hidden dialogs).
/resource?id=99999 and /group?id=99999 emit the same ('1',''),('3','record not found') pair.

**Screenshot:** `notes-404.png`

### 136. Saving a plugin setting re-renders the page with the OLD value in the plugin's injected output, so the save looks like it failed

- **Area:** Plugins / Manage · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/plugins/manage`

**Steps**

1. Open http://localhost:8200/plugins/manage and click "Enable" on `example-plugin`. Its footer banner appears reading "Hello from Example Plugin!".
2. Change the "Greeting Message" setting to `hunt-admin greeting probe` and click "Save Settings".
3. Without reloading, read the footer banner on the page that comes back.
4. Reload the page and read the footer banner again.
5. Repeat steps 2–4 changing the value back to `Hello from Example Plugin!`.
6. Click "Disable" to restore the original state.

**Expected:** The page rendered as the response to the save shows the newly saved greeting.

**Actual:** The POST response re-renders /plugins/manage with the setting input correctly showing the NEW value but the plugin-injected footer still showing the PREVIOUS value; only after a manual reload does the footer update. Because the input and the live output disagree, it reads as "the save did not take". Reproduced in both directions (old→new and new→old).

**Evidence:** After save #1: `{"footer":" Hello from Example Plugin! ", "greeting":"hunt-admin greeting probe"}`; after reload: `{"footer":" hunt-admin greeting probe "}` and `curl -s http://localhost:8200/dashboard | grep -o 'hunt-admin greeting probe'` matches.
After save #2 (setting it back): `{"footerAfterSave":" hunt-admin greeting probe "}` then `{"footerAfterReload":" Hello from Example Plugin! "}`.
State restored: plugin disabled again, greeting back to "Hello from Example Plugin!".

### 137. Deleting a relation redirects to the Groups list instead of the Relations list

- **Area:** Relations · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/relation?id=8`

**Steps**

1. Create a relation, e.g. open /relation/new?FromGroupId=84&ToGroupId=85&GroupRelationTypeId=1&Name=test and click Save — you land on /relation?id=N.
2. Click Delete and accept the confirm dialog.
3. Note the landing page. Repeat once more with another relation.

**Expected:** Return to /relations (the list the item belongs to), or to one of the two groups involved.

**Actual:** Redirects to http://localhost:8200/groups — the Groups list — which is neither the relation's list nor either endpoint group. The user has to navigate to Admin > Relations to confirm the deletion.

**Evidence:** Reproduced twice: from /relation?id=7 -> "http://localhost:8200/groups"; from /relation?id=8 -> "http://localhost:8200/groups".

### 138. Relation cards mirror the badge/name order between the from-side and to-side, so the two halves don't line up

- **Area:** Relations · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/relations`

**Steps**

1. Go to http://localhost:8200/relations.
2. Compare the left (From) card and the right (To) card of any relation row.
3. Do the same on a relation detail page, e.g. /relation?id=1.

**Expected:** Both halves of the pair use the same internal order so the eye can compare them across the arrow.

**Actual:** The From card renders the relation-type badge above the group name ("Address" then "Anna Lindqvist") in the list, while the To card renders the group name above the badge ("Reykjavik Studio" then "Address"). On the detail page the order is flipped the other way round (From: name then badge; To: badge then name). The result is that the group names never sit on the same baseline across the arrow.

**Evidence:** Snapshot of /relations article 1: From article children order = [link "Address", heading "Anna Lindqvist"]; To article = [heading "Reykjavik Studio", link "Address"]. Visible in both screenshots below (relations-desktop.png and relation-empty-title.png).

**Screenshot:** `relations-desktop.png`

### 139. Details-table row checkboxes are 14x14 CSS px — below the WCAG 2.2 target-size minimum and inconsistent with the 24x24 grid checkboxes

- **Area:** Resource list — details/table view accessibility · **Kind:** a11y · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resources/details`

**Steps**

1. Open http://localhost:8200/resources/details.
2. Measure a row checkbox: document.querySelector('tbody input[type=checkbox]').getBoundingClientRect() → 14 x 14 (class 'detail-table-checkbox').
3. Open http://localhost:8200/resources and measure a card checkbox: 24 x 24 (class 'card-checkbox', Tailwind h-6 w-6).
4. Repeat at 390x844 — the table checkbox is still 14x14 on touch.

**Expected:** Selection checkboxes should be at least 24x24 CSS px (WCAG 2.2 SC 2.5.8 Target Size Minimum), and consistent between the grid and table views of the same list.

**Actual:** The details table uses 14x14 checkboxes with no enlarged hit area, half the size of the grid's 24x24 checkboxes. They are the only way to select rows for the bulk operations, and they are hard to hit on touch.

**Evidence:** /resources/details: {"tableCb":{"w":14,"h":14,"cls":"detail-table-checkbox focus:ring-amber-600 text-amber-700 bo"}}
/resources: {"w":24,"h":24,"bg":"rgb(255, 255, 255)","pos":"absolute"} (class card-checkbox … h-6 w-6)

**Screenshot:** `reslist-v2-tiny-checkbox.png`

### 140. Version delete uses a native browser confirm that does not say which version is being deleted

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Open a resource with several versions and expand the 'Versions (N)' disclosure.
2. Click the Delete button on any version row.

**Expected:** The confirmation names the version being destroyed (version number, comment, size) so you cannot delete the wrong one; ideally it is an in-app dialog rather than window.confirm.

**Actual:** An unstyled native confirm appears reading only 'Delete this version?'. Nothing identifies which of the rows it applies to, and the wording is identical for every row.

**Evidence:** #125: '### Modal state / - ["confirm" dialog with message "Delete this version?"]: can be handled by dialog-accept or dialog-dismiss'. Identical dialog on a second, different version at #131: '- ["confirm" dialog with message "Delete this version?"]'. The rows themselves come from per-version forms (#119: '<form action="/v1/resource/version/restore" method="post" class="inline"><input type="hidden" name="resourceId" value="88">...'), and versions 88/89/90/96 all render the same button text (#117).

### 141. Inline name editor is a 330px single-line input for a 166-character name (2215px of content)

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=85`

**Steps**

1. Open /resource?id=85 ('A resource with a really extremely long descriptive name that ought to ...').
2. Click the 'Edit name' button in the heading.
3. Measure the input inside the inline-edit shadow root.

**Expected:** The rename field grows to the available heading width (or wraps to a textarea) so a long name can be read and edited.

**Actual:** The input is 330px wide while its content is 2215px - roughly 15% of the name is visible at a time, and there is no wrapping, so editing a long name means scrolling blind inside a narrow box.

**Evidence:** #287: '{"tag":"INPUT","w":330,"valLen":166,"scrollW":2215,"shadow":true}'. The editor lives in a shadow root on the inline-edit element (#31: '<inline-edit post="/v1/resource/editName?id=59" name="name">Sunset at Golden Gate Bridge</inline-edit>'), and the heading it sits in is full-width (#277: '<h1 class="flex flex-col items-start gap-1 flex-1 min-w-0 text-2xl ...">').

**Screenshot:** `longname-inline-edit.png`

### 142. Submitting the upload form with no file round-trips through the server and dumps every field into the URL

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource/new`

**Steps**

1. Open /resource/new.
2. Without choosing a file, click 'Save'.

**Expected:** The file input is required, so the browser blocks submission and focuses the field; no navigation happens.

**Actual:** The form posts, the server rejects it, and the user is bounced back to /resource/new with every field serialised into the query string (including 'error=no+files+found+to+save' and 'Meta=%7B%7D'). The file input carries no required attribute, so there is no client-side guard at all.

**Evidence:** #35: '- Page URL: http://localhost:8200/resource/new?Description=&Meta=%7B%7D&Name=&PathName=&ResourceCategoryId=&SeriesId=&URL=&error=no+files+found+to+save&groups=&notes=&ownerId=&resource=&tags='. #39 shows the input markup with no required attribute: '<input id="resource" name="resource" multiple="" type="file">'. The same rejection at the API level is 'no files found to save' (digest a6c53d826c7ffa692 #107: '{"status":400,"body":"{\"error\":\"no files found to save\"}"}').

**Screenshot:** `res-new-form.png`

### 143. Meta Data sidebar shows the property key but not its value

- **Area:** Resources — list & detail · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=88`

**Steps**

1. Open /resource/edit?id=88, add a meta field with name 'hunt_key' and value 'hunt value 123', and save.
2. On the resource page, read the 'Meta Data' sidebar section.

**Expected:** The sidebar shows 'hunt_key' next to 'hunt value 123'.

**Actual:** The rendered text is the key followed by an empty cell - the value never appears in the section's text content, even though the server stored it.

**Evidence:** #251 confirms the write landed: 'meta: {'hunt_key': 'hunt valu[e 123]'...'. #253 reads the sidebar: '{"metaSection":["META DATA\nExpand\nhunt_key\t\nGUID: 019fad04-208e-7e67-9837-5d2d5c052d36"]}' - between the key and the next row (GUID) there is only a tab. Note the section also carries an 'Expand' control, so the value may be inside a collapsed element; either way it is not visible in the default state.

**Screenshot:** `meta-display.png`

### 144. Paste-upload dialog on the resource page is titled 'Upload to Unknown'

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=87`

**Steps**

1. Open a resource page (observed on /resource?id=87, the text/plain resource).
2. Inspect the headings in the document, including the hidden paste-upload dialog (#paste-upload-title).

**Expected:** The dialog names its target, e.g. 'Upload to Plain Text Notes'.

**Actual:** The heading text is the literal string 'Upload to Unknown' - the target name interpolation resolves to 'Unknown' on the resource page.

**Evidence:** #159 lists the page's h2s for resource 87: '["Custom Thumbnail","Tags","Resource Category","Meta Data","Metadata","Notes","Groups","Edit Tags","Info","Upload to Unknown", ...]'. #161 pins it to the paste-upload dialog and shows it is in the hidden (0-width) state: '[["Edit Tags",null,"block",0],["Info",null,"block",0],["Upload to Unknown","paste-upload-title","block",0],["Select Items","entity-picker-title","block",0]]'.

### 145. The main preview link leaves the app for the raw file URL while card thumbnails open the in-app lightbox

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resource?id=59`

**Steps**

1. Open /resource?id=59 and click the large preview image at the top of the page.
2. Go back, open /resource?id=88 and click the thumbnail inside a 'Similar Resources' card.

**Expected:** Consistent behaviour - both open the media viewer (or both open the file).

**Actual:** The main preview is a plain link that 302s to the file and hands the browser the raw image at /files/resources/.../<hash>.png#image/png, dropping the user out of the app onto the browser's native image view. The card thumbnail on the same kind of page instead opens the in-app lightbox dialog.

**Evidence:** #201 after clicking the main preview: '- Page URL: http://localhost:8200/files/resources/88/8c/9c/888c9c9202ef5db451a8a6cb7377c466590dc9a9.png#image/png / - Page Title: 888c9c9202ef5db451a8a6cb7377c466590dc9a9.png (1024x768)'. #207 after clicking a card thumbnail: '- dialog "Market Vendor, Kyoto" [ref=e233]: ... - button "Previous" [disabled] ...'. #131 shows the view endpoint is a redirect: 'before: 302 93'.

**Screenshot:** `lightbox-detail.png`

### 146. Out-of-range page number renders an empty grid with no explanation

- **Area:** Resources — list & detail · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/resources?page=99`

**Steps**

1. Open /resources?page=99 when only 2 pages exist.

**Expected:** Redirect to the last valid page, or say 'page 99 does not exist'.

**Actual:** HTTP 200 with zero cards and no message; the footer still shows 'Previous | 1 | 2', so the only clue that anything is wrong is the blank area.

**Evidence:** #163: '- Page URL: http://localhost:8200/resources?page=99 / - Page Title: Resources - mahresources' then '{"cards":0,"footer":"Previous|1|2","mainText":"Thumbnails\nDetails\nSimple\nTimeline\nSelect All\nFilter these res..."}'. Page 2 itself is valid and non-empty (#77: '{"cards":39,"footer":"Previous|1|2","first":"training slides"}').

**Screenshot:** `reslist-page99-empty.png`

### 147. Raw SQL query results reorder columns alphabetically, discarding the SELECT order

- **Area:** SQL saved queries (/query?id=) · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/query?id=<your id>`

**Steps**

1. Go to http://localhost:8200/query/new
2. Name = "column order test", Query = SELECT name AS zebra, id AS apple, description AS mango FROM tags ORDER BY id LIMIT 3
3. Save, then click Run on the detail page.
4. Read the table header row.
5. Repeat with SELECT id, name, created_at FROM tags ORDER BY id LIMIT 5.

**Expected:** Result columns appear in the order written in the SELECT list (zebra, apple, mango), the way every SQL client behaves — and the way the MRQL GROUP BY result table on /mrql already behaves (contentType, count, sum_fileSize is preserved there).

**Actual:** Columns are rendered alphabetically: apple, mango, zebra. The second query (SELECT id, name, created_at) renders created_at, id, name. The loss happens server-side: POST /v1/query/run?id=63 returns [{"apple":1,"mango":"...","zebra":"draft"}, ...] — rows are JSON objects, so Go map marshalling sorts the keys and the column order chosen by the query author is unrecoverable. This is also inconsistent with /mrql, which preserves order.

**Evidence:** playwright eval of table th text returned ["apple","mango","zebra"] for SELECT name AS zebra, id AS apple, description AS mango. curl -X POST http://localhost:8200/v1/query/run?id=63 returned [{"apple":1,"mango":"Tag for categorizing items as draft","zebra":"draft"},...]. Reproduced with two different queries.

**Screenshot:** `query-column-order-alphabetical.png`

### 148. word-break:break-all in metadata and compare cards splits timestamps and words mid-token ("Jul 29, 2026 1 / 2:01")

- **Area:** Single resource page metadata cards + /resource/compare metadata cards · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource/compare?r1=101&v1=2&v2=1 and http://localhost:8200/resource?id=74`

**Steps**

1. Open a resource with two versions and click Compare > Compare Selected (URL like http://localhost:8200/resource/compare?r1=N&v1=2&v2=1).
2. Look at the "CREATED" card in the Metadata grid at a 1280px viewport.
3. Separately open http://localhost:8200/resource?id=74 at 1280px and look at the METADATA "Name" and "Original Name" cards.

**Expected:** Text should wrap at word/space boundaries. Timestamps and words should never be split across lines.

**Actual:** Both card styles set word-break: break-all, so text breaks at arbitrary characters. The compare Created card renders "Jul 29, 2026 12:08 → Jul 29, 2026 1" / "2:01" — the clock time is split between "1" and "2:01", which reads as a wrong value. The resource Metadata name card renders "A resource with a really extremely l / ong descriptive name that ought to / test how the interface truncates thi / ngs in cards, tables, breadcrumbs / and the browser title bar".

**Evidence:** Computed style on the compare value node: {"cls":"compare-meta-card-value","wb":"break-all","ow":"normal","ws":"normal","txt":"Jul 29, 2026 12:08"}. Metadata card markup: <dd class="text-sm mt-0.5 break-all">. Second screenshot: /private/tmp/claude-501/-Users-egecan-Code-mahresources/fd4d26f7-b96a-4d41-9ee7-95edc5a0ba1d/scratchpad/shots/resource-74-desktop.png

**Screenshot:** `compare-created-word-break.png`

### 149. Similar-resource cards advertise "Double-click to edit" on the description but double-clicking does nothing

- **Area:** Single resource page — Similar Resources cards · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<resource with a SIMILAR RESOURCES section>`

**Steps**

1. Open a resource detail page that renders a "SIMILAR RESOURCES" section (e.g. an uploaded image similar to another; I used /resource?id=101).
2. Hover the description text inside a similar-resource card — the native tooltip "Double-click to edit" appears.
3. Double-click it.

**Expected:** Either an editor opens (matching the identical affordance on the main description block, which does work), or the tooltip/affordance is not offered.

**Actual:** Nothing happens — no textarea, no message, no console output. The card's Alpine scope is `() => ({ editing: false, descriptionEditUrl: '' })` and the handler is `@dblclick="editing = !!descriptionEditUrl"`, so with the empty URL the toggle can never fire, yet the element still carries title="Double-click to edit". The main resource description block on the same page has descriptionEditUrl: '/v1/resource/editDescription?id=101' and works.

**Evidence:** Element scopes on the page: ["() => ({ editing: false, descriptionEditUrl: '/v1/resource/editDescription?id=101' })", "() => ({ editing: false, descriptionEditUrl: '' })"]. Card markup: <div class="contents" @dblclick="editing = !!descriptionEditUrl" title="Double-click to edit">. After dblclick on "article .description": {"editing":false} (no textarea in the DOM).

### 150. Mobile breadcrumb chevrons detach into clipped diagonal marks when the trail wraps

- **Area:** Single resource page — breadcrumb, mobile · **Kind:** design · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=59`

**Steps**

1. Resize the browser to 390x844.
2. Open http://localhost:8200/resource?id=59 (breadcrumb: Home > Photography > Landscapes > Sunset at Golden Gate Bridge).
3. Look at the breadcrumb bar at the top of the page.

**Expected:** When the breadcrumb wraps to several lines the separators should either be dropped or stay attached to the item they follow, as they do at desktop width where they render as clean connected arrow segments.

**Actual:** Each crumb wraps onto its own line and the arrow-shaped separators are left stranded at the left margin as half-drawn diagonal chevrons that connect nothing, so the trail reads as broken graphics rather than a breadcrumb. Same page at 1280px renders the arrows correctly.

**Evidence:** Cropped screenshot of nav[aria-label=Breadcrumb] at 390x844 (attached). Compare the desktop rendering in /private/tmp/claude-501/-Users-egecan-Code-mahresources/fd4d26f7-b96a-4d41-9ee7-95edc5a0ba1d/scratchpad/shots/resdetail-59-image.png. No horizontal overflow (scrollWidth 390 === clientWidth 390), so it is a wrap-styling defect, not an overflow.

**Screenshot:** `mobile-breadcrumb-59.png`

### 151. Metadata card "Name" shows the stale old name after an inline rename (two conflicting names on one screen)

- **Area:** Single resource page — inline name editing · **Kind:** bug · **Provenance:** verified-run
- **URL:** `http://localhost:8200/resource?id=<any resource>`

**Steps**

1. Open any resource detail page, e.g. http://localhost:8200/resource?id=88.
2. Click the pencil button next to the title (aria-label "Edit name").
3. Type a new name and press Enter.
4. Without reloading, look at the METADATA card in the main column, field "Name".

**Expected:** Both the H1 and the Metadata card should show the new name (they are the same field), or the card should refresh.

**Actual:** The H1 and the browser title update to the new name, but the METADATA card's "Name" field keeps showing the old name until a manual page reload. The page therefore displays two different names for the same resource simultaneously. Reproduced twice: after renaming "hunt_rd_main.png" -> "hunt-resdetail sandbox image" the card still read "hunt_rd_main.png"; after a reload plus a second rename to "hunt-resdetail sandbox RENAMED" the card still read "hunt-resdetail sandbox image".

**Evidence:** Post-rename eval: {"h1":"hunt-resdetail sandbox RENAMED","metaAfter":"Name | hunt-resdetail sandbox image | ⧉"}. Screenshot shows H1 "hunt-resdetail sandbox image" while the Metadata Name card reads "hunt_rd_main.png".

**Screenshot:** `stale-metadata-name-after-rename.png`

### 152. Inline name edit swallows the server's error reason and shows a generic 'Could not save name'

- **Area:** Tags / Categories — inline name editor on detail pages · **Kind:** ux · **Provenance:** verified-run
- **URL:** `http://localhost:8200/tag?id=87`

**Steps**

1. Open any tag detail page and click the pencil 'Edit name' button next to the title. 2. Replace the name with the name of an existing tag, e.g. 'design'. 3. Press Enter. 4. Read the toast and the console.

**Expected:** The user should be told what went wrong — 'A tag named "design" already exists' — so they can correct it. (Escape-to-cancel and Enter-to-save both work correctly, so only the error text is at fault.)

**Actual:** A toast says only 'Could not save name'. The server actually returns a usable reason that the UI discards, and two red errors are logged to the console.

**Evidence:** Console: '[ERROR] Failed to load resource: the server responded with a status of 400 (Bad Request) @ http://localhost:8200/v1/tag/editName?id=87' and '[ERROR] Error posting data: Error: Server responded with 400'. Direct call: fetch('/v1/tag/editName?id=87', {method:'POST', body:JSON.stringify({name:'design'})}) -> 400 :: {"error":"UNIQUE constraint failed: tags.name"}. Page toast text: 'Could not save name'.

### 153. Bulk merge asks 'Are you sure you want to delete?' instead of describing the merge

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/tags?Name=hunt-tax`

**Steps**

1. Open /tags, tick one or more tags.
2. Open 'Merge Winner', choose a winner.
3. Click the 'Merge' button in the bulk toolbar and read the confirm dialog.

**Expected:** The confirm names the operation and its consequence, as the detail-page merge does ('Selected tags will be deleted and merged to X').

**Actual:** The bulk toolbar reuses the generic delete confirm: 'Are you sure you want to delete?'.

**Evidence:** #294/#295 `["confirm" dialog with message "Are you sure you want to delete?"]` after clicking Merge in the bulk toolbar; same text again at #300/#301. Contrast with the detail-page merge confirm quoted at #51.

### 154. Applying a template preset silently overwrites existing custom template content

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. Open a category that already has Custom Header content, e.g. /category/edit?id=72.
2. Choose 'Contact card (Category)' in the 'Start from preset' select.
3. Click 'Apply'.

**Expected:** A confirm ('this will replace the current template content') before clobbering authored templates.

**Actual:** The existing content is replaced immediately with no prompt. It is recoverable (Cmd+Z in the editor restores it, and nothing is persisted until Save), but nothing tells the user that.

**Evidence:** #264/#265 before: `"<div class='p-2 bg-blue-50 rounded'>[property name] - [property description default='no description']</div>"`. #266/#267 after Apply: `{"h":"<div class=\"pt-preset-contact\">...`. #268/#269 Cmd+Z restores the original text; #270/#271 confirms nothing was persisted.

### 155. A reference to a non-existent [partial] produces no diagnostic in the template editor

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/category/edit?id=72`

**Steps**

1. Open /category/edit?id=72 and select all in the Custom Header editor.
2. Type: OK:[partial name="hunt-tax-partial"] MISSING:[partial name="does-not-exist"]
3. Wait for lint, then click 'Refresh' on the preview.

**Expected:** An editor diagnostic (or at least a preview error box) naming the unknown partial, the way an invalid [mrql] query is flagged.

**Actual:** No diagnostic is attached to the unknown partial. The lint markers present on the page belong to other constructs.

**Evidence:** #274/#275 `{"content":"[partial name=\"does-not-exist\"] and [mrql query='SELECT bogus FROM nothing']","lintCount":2,"diagnostics":[]}`; #276/#277 the two lint ranges are `["query='SELECT bogus FROM nothing']","[property name]"]` - neither covers the `[partial name="does-not-exist"]` reference.

### 156. Template Partial detail page prints the entity type twice

- **Area:** Taxonomy / templates / MRQL · **Kind:** design · **Provenance:** recovered
- **URL:** `http://localhost:8200/templatePartial?id=1`

**Steps**

1. Create a template partial (e.g. name 'hunt-tax-partial').
2. Open its detail page and look at the page header.

**Expected:** Consistent with every other detail page: a small type pill plus the bare entity name.

**Actual:** The header shows the 'Template Partial' pill AND an h1 reading 'Template Partial: hunt-tax-partial'. Tag, Category and Query detail pages show only the name after the pill.

**Evidence:** #194 captured the header for this reason ('Screenshot duplicated heading'); the image shows the pill 'Template Partial' above the h1 'Template Partial: hunt-tax-partial'. Compare the tag header captured at #320 (pill 'Tag' + h1 'hunt-tax-tag1-renamed') and the query header in query-run-results.png (pill 'Query' + h1 'Categories Ordered By Name').

**Screenshot:** `partial-title-duplicated.png`

### 157. Template partial name rule is only enforced after submit, and the whole editor body is round-tripped through the URL

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/templatePartial/new`

**Steps**

1. Open /templatePartial/new.
2. Enter the name 'hunt tax partial with spaces!' and some template content.
3. Click Save and look at the address bar.

**Expected:** The kebab-case rule is stated next to the Name field (and checked client-side) before a round trip.

**Actual:** The form posts, comes back with 'Could not save: template partial name must be kebab-case: ...', and the entire template body plus the error text are carried in the URL query string. Field values are preserved, but every character of a large template ends up in the URL.

**Evidence:** #184/#185 the resulting URL: `http://localhost:8200/templatePartial/new?Content=%3Cb%3E%5Bproperty+path%3D%22Name%22%5D%3C%2Fb%3E&Description=&Name=hunt+tax+partial+with+spaces%21&error=template+partial+name+must+be+kebab-case%3A+...`. #186/#187 `{"err":["Could not save: template partial name must be kebab-case: lowercase letters, digits, and hyphens, starting with a letter", ...]}`. #188/#189 values are preserved: `{"n":"Name","v":"hunt tax partial with spaces!"}`.

**Screenshot:** `partial-name-error.png`

### 158. Result counts are not pluralized: 'Results (1 items)'

- **Area:** Taxonomy / templates / MRQL · **Kind:** ux · **Provenance:** recovered
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open /mrql.
2. Run a query that returns exactly one row, e.g. type = resource AND name ~ "sunset" LIMIT 5.
3. Read the results heading.

**Expected:** 'Results (1 item)'.

**Actual:** 'Results (1 items)'.

**Evidence:** Digest a951176 #64/#65: `- heading "Results (1 items)" [level=2] [ref=e1165]`. Independently in digest aca6d2c #92/#93: `"\n                        Results\n                        (1 items)\n                    "`.

### 159. MRQL API reports a different applied_limit for two identical calls (3 vs 500)

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/v1/mrql`

**Steps**

1. POST {"query":"type = group AND notes.count > 0"} to /v1/mrql.
2. Repeat the identical request.
3. Compare the default_limit_applied / applied_limit metadata in the two responses.

**Expected:** The same query yields the same applied limit, and it matches the configured MRQL default (500).

**Actual:** The first response reported applied_limit 3 with default_limit_applied true; a later identical call reported applied_limit 500. Both returned no results (the UI shows 'Results (0 items)' for this query).

**Evidence:** Digest aca6d2c #44/#45: `{"entityType":"group","default_limit_applied":true,"applied_limit":3}`. #48/#49 for the identical query: `== type = group AND notes.count > 0` -> `{'entityType': 'group', 'default_limit_applied': True, 'applied_limit': 500}` (the control `type = group` in the same loop returned `{'groups': 87}`). UI side: digest aca6d2c #42/#43 and digest a951176 #24/#25 both show `- heading "Results (0 items)"` with `"Entity: group"` for this query.

### 160. Autocomplete does not open for member completions after a dot, although the server returns a suggestion

- **Area:** Taxonomy / templates / MRQL · **Kind:** bug · **Provenance:** recovered
- **URL:** `http://localhost:8200/mrql`

**Steps**

1. Open /mrql and type: type = resource AND tags.c
2. Wait, then press Ctrl+Space.
3. Compare with: curl -s -X POST http://localhost:8200/v1/mrql/complete -H 'Content-Type: application/json' -d '{"query":"type = resource AND tags.c","cursor":25}'

**Expected:** The completion popup offers 'count' for the tags. prefix.

**Actual:** No popup appears either automatically or on Ctrl+Space, while the completion endpoint returns a suggestion for the same text and cursor.

**Evidence:** #114/#115 after the 'tags' prefix step: `=== now type tags.count prefix 'tag' ===` -> `"NO TOOLTIP"`. #116/#117 explicit Ctrl+Space -> `"NO TOOLTIP"`, immediately followed by the server response for the same query text: `{"suggestions":[{"value":"count","type":"field","label":"relation count"}]}`. Caveat: the digest records the shell wrapper but not the exact keystrokes typed for this sub-step, so the editor content at the moment of the check is not directly quoted - verify before filing.

---
