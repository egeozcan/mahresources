---
sidebar_position: 2
---

# Managing Resources

Resources are files of any type: images, documents, videos, or anything else you want to store and organize.

![Resource detail view](/img/resource-detail-view.png)

## Uploading Resources

### File Upload

![Create resource upload form](/img/upload-form.png)

1. Navigate to **Resources** in the top menu
2. Click the **Create** button
3. Use the file picker to select one or more files
4. Fill in optional metadata:
   - **Name** - Display name (defaults to filename if left empty)
   - **Description** - Text description of the resource. Type `@` to mention and link to notes, groups, or tags (see [Mentions](../features/mentions.md))
   - **Tags** - Labels for organization
   - **Groups** - Associate with groups
   - **Notes** - Link to existing notes
   - **Owner** - The group that owns this resource
   - **Resource Category** - Classify the resource type
   - **Series** - Group related resources into a series
   - **Storage** - Which filesystem to save the file to. Shown only when alternative file systems are configured (`-alt-fs`)
   - **Meta** - Custom key-value metadata
5. Click **Save** to upload

You can upload multiple files at once by selecting them in the file picker.

### Bulk Uploads

A small selection is posted as one request, exactly as it always was. A large one is not: above `upload_widget_file_threshold` files (10 by default) or `upload_widget_size_threshold` bytes (1 GiB by default), the page sends one request per file, `upload_concurrency` at a time (3 by default). A line under the file picker tells you before you click **Save** that the selection has crossed the threshold. See [Runtime Settings](../configuration/runtime-settings.md#bulk-resource-uploads) to change the three values.

While the batch runs, a progress panel replaces the form:

- Progress is aggregated over **bytes**, not files, so one large file among many small ones does not sit at zero and then jump
- Only files that are in flight or have failed get a row of their own; completed files collapse to a count
- **Cancel** stops the batch, leaving the files already uploaded in place
- **Retry failed** re-sends the failures that are worth re-sending. A 409 duplicate, a file the browser refused on size, and any other deterministic 4xx are excluded, because the same bytes would fail the same way; a duplicate's row links to the resource it collided with instead

`max_upload_size` bounds one request, so under this widget it becomes a per-file limit rather than a limit on the batch. Each file is checked against it in the browser before anything is sent.

When every file succeeds, a single-file upload opens that resource and a batch opens the owning group, or the resource list when no owner was chosen. A batch that partially failed stays on the page so you can retry.

### URL Import

Import resources directly from web URLs:

1. Navigate to **Resources** > **Create**
2. Instead of using the file picker, paste a URL into the **URL** field
3. Optionally check **Download in background** for large files
4. Fill in metadata as desired
5. Click **Save**

The URL field accepts multiple URLs (one per line) for batch imports.

### Background Downloads

For large files or slow connections, enable **Download in background**:

- **Save** starts the download and keeps you on the form: the **Jobs panel** opens on the new download, and the form says how many downloads started and lists any URL the server refused with its reason. The URL field is cleared and the other fields are kept for the next download. **Show in the Jobs panel** opens the panel on them again
- You can navigate away; the download carries on
- Progress is tracked in the **Jobs panel**, opened from the header: a bar with the amount downloaded, the speed, the time left and a speed graph
- When the download finishes, **View created resource** in the Jobs panel or the [Job Center](#job-center) opens the new resource
- Failed downloads remain in the [Job Center](#job-center) after restart and can be retried when the Job advertises Retry

### Paste Upload

Paste images or files from the clipboard to create resources:

1. Copy an image or file to the clipboard (e.g., screenshot, copied image from a webpage)
2. Navigate to a group or note detail page, or a list view filtered by a single owner
3. Press **Ctrl+V** / **Cmd+V** -- a modal appears showing a preview of the pasted content
4. Set optional fields: tags, resource category, series
5. Click **Upload**

The paste upload modal supports:

- **Batch uploads** -- paste multiple items and upload them together
- **Duplicate detection** -- if a resource you can see already holds the same content, the modal shows its ID
- **Context awareness** -- when pasting on a group or note detail page, or a list view filtered by a single owner, the uploaded resource is associated with that entity automatically
- **Auto-close** -- the modal closes and the page refreshes after a successful upload

Pasted text is uploaded too: rich text becomes an `.html` resource and plain text a `.txt` resource, each named after the moment it was pasted and previewed in the modal as a text snippet rather than a thumbnail.

## Viewing Resources

### Resource List

Resources display as cards. Each card shows:

- Thumbnail preview (click to open in lightbox)
- Resource name (click for detail page)
- File size, owner, category with avatar
- Expandable description
- Tags with inline edit button
- Checkbox for bulk selection

### Resource Detail Page

![Resource detail page with tags, groups, and metadata](/img/resource-detail.png)

Click a resource name to view its detail page, showing:

**Main Content**
- Full description
- Metadata panel (name, original name, dimensions, timestamps) with collapsible technical details (ID, hash, location, storage location)
- Related notes
- Related groups
- Series siblings (if the resource belongs to a series)
- Similar resources (when the [hash worker](/features/image-similarity) is running)

**Sidebar**
- File size
- Preview thumbnail
- Tags (with inline editing)
- Image operations (for image files)
- Custom metadata

### Previewing Files

Click a resource thumbnail to open images in the lightbox, view PDFs in the browser, or download other file types. The lightbox supports arrow-key navigation across all visible resources.

### Lightbox Tag Editing

Press **T** to open the Edit Tags panel in the lightbox. This panel lets you add/remove tags quickly using two methods:

**Tag Search**: Type in the search field at the top (press **0** to focus it) to find and add tags by name.

**Quick Slots**: The 3x3 grid below provides instant keyboard-driven tag toggling:

- **Tabs**: Four customizable tabs (QUICK 1-4) and a RECENT tab. Switch with **Z/X/C/V/B** keys.
- **Assigning tags**: Click an empty slot, then search for a tag to assign it. Slots can hold one or multiple tags.
- **Toggling**: Press **1-9** (matching the numpad layout: 7-8-9 top row, 4-5-6 middle, 1-2-3 bottom) to toggle the tags in that slot on/off for the current resource.

**Color indicators** show each slot's state:
- **Green**: All tags in the slot are on the resource (click/press to remove)
- **Amber**: Some tags are on the resource (click/press to add the missing ones)
- **Gray**: No tags from the slot are on the resource (click/press to add all)

#### Expanding Multi-Tag Slots

When a slot contains multiple tags, you can drill into it to toggle tags individually:

1. **Keyboard**: Hold a number key (**1-9**) for 400ms on a multi-tag slot. A progress bar at the bottom of the slot shows the hold duration.
2. **Mouse**: Click and hold a multi-tag slot card for 400ms.
3. A short press (tap) still toggles all tags in the slot as a batch.

In expanded mode:
- The tab bar is replaced with a **Back** button and "Slot N tags" label
- Each tag from the slot appears as its own card in the 3x3 grid
- Press **1-9** to toggle individual tags
- Tags show **green** (on resource, press to remove) or **gray** (not on resource, press to add)

**Exiting expanded mode:**
- Press **Escape**, **0**, **Z**, **X**, **C**, **V**, or **B**
- Click the **Back** button
- Click outside the quick tag panel
- Click any tab button (also switches to that tab)

:::tip Keyboard shortcuts summary
| Key | Action |
|-----|--------|
| **T** | Toggle Edit Tags panel |
| **1-9** | Toggle slot (tap) or expand slot (hold) |
| **0** | Focus tag search (or exit expanded mode) |
| **Z/X/C/V** | Switch to QUICK 1-4 (or exit expanded mode) |
| **B** | Switch to RECENT tab (or exit expanded mode) |
| **Escape** | Exit expanded mode, or close lightbox |
| **Browser Back** | Close lightbox and stay on the page |
| **Browser Forward** | Reopen the lightbox on the image it was closed on |
:::

## Editing Resources

### Edit Page

1. Click **Edit** on any resource detail page
2. Modify fields as needed:
   - Name
   - Description
   - Tags
   - Groups
   - Notes
   - Owner
   - Resource Category
   - Series
   - Custom metadata
3. Click **Save** to apply changes

Note: You cannot replace the file itself when editing. To update a file, upload a new version and use the versioning system.

### Inline Name Editing

On the resource detail page, click the resource name in the header to edit it directly. Changes save automatically when you click away or press Enter.

### Tag Management

Manage tags directly from the resource detail page:
1. Click the **+** button in the Tags section
2. Search for and select tags
3. Tags are added immediately

To remove a tag, click the **x** on the tag label.

## Image Operations

Image resources have their operations in the sidebar under **Image Actions**. Clicking **Image Actions…** opens a dialog with **Recalculate Dimensions**, **Rotate 90°**, and **Crop…**; **Crop…** opens the crop dialog from there.

### Rotate

Rotate an image by a specified number of degrees. The UI provides a **Rotate 90°** button; the API accepts any integer angle:

1. Navigate to the image resource
2. In the sidebar, click **Image Actions…**
3. Click **Rotate 90°**

Rotation creates a new version with the rotated content and clears cached thumbnails.

### Crop

Crop an image to a rectangular region. Cropping is available for raster image files.

1. Navigate to the image resource
2. In the sidebar, click **Image Actions…**, then **Crop…**, to open the crop dialog
3. Under **Save as**, choose **New version** (the default) or **New resource**
4. Drag on the image to select the crop area, or type exact pixel values for **X**, **Y**, **Width**, and **Height**
5. Optionally pick an **Aspect ratio** (Free, 1:1, 16:9, 4:3, or Original) and add a **Comment**
6. Click the button in the dialog footer to apply

The crop is also available from the image viewer (lightbox) through its **Crop image** button, with the same **Save as** choice.

#### Save as new version

The default. The cropped image replaces the resource's current content as a new version and cached thumbnails are cleared. The **Comment** is stored on that version.

#### Save as new resource

The source resource is left completely untouched -- no new version, no dimension change, no thumbnail invalidation. The crop is saved as a separate resource that inherits the source's owner, groups, tags, and resource category, is named `<source name> (cropped)`, and records where it came from in its description along with the **Comment**. The crop dialog stays open with a link to the new resource, so several regions can be lifted out of one image in a row.

Identical content is deduplicated: cropping the exact same rectangle twice reports the resource that already holds it instead of creating a duplicate. The check covers what you can see, so a group-limited user is not told about a match outside their own group subtree.

Either way, JPEG and PNG images keep their format; GIF, WebP, BMP, and TIFF are re-encoded as PNG, and GIF animation is dropped. HEIC and AVIF images are decoded through the ImageMagick fallback (which must be installed) and re-encoded as PNG.

SVG and ICO files cannot be cropped -- re-upload them as PNG or JPEG first.

### Recalculate Dimensions

If image dimensions appear incorrect:

1. Navigate to the image resource
2. In the sidebar, click **Image Actions…**
3. Click **Recalculate Dimensions**

This re-reads the image file and updates the stored width/height values.

## Video Operations

Video resources have a trim operation in the sidebar under **Video Actions**. Clicking **Video Actions…** opens a dialog with the trimmer. Trimming requires FFmpeg.

### Trim Video

Cut a video down to a single time range.

1. Navigate to the video resource
2. In the sidebar, click **Video Actions…**
3. Watch the preview, and either drag the range slider handles to set the start and end or type exact **Start (s)** and **End (s)** values. Moving a handle moves the preview to the frame it would cut from
4. Alternatively, play or scrub the preview and press **Set Start** and **Set End** to mark the range off the playhead wherever it happens to be. **Preview Range** plays just the marked range, stopping where the trim will stop. The preview's own controls are there for scrubbing freely
5. Optionally add a **Comment**
6. Click **Trim Video**

Trimming creates a new version containing only the selected range and clears cached thumbnails. The output is always re-encoded as MP4 (H.264 video, AAC audio), regardless of the source format. Time values accept plain seconds, `MM:SS`, or `HH:MM:SS`.

Closing the dialog without trimming keeps whatever you had dialled in, so you can step away and come back to the same range.

:::note
The range slider needs to know how long the video is. The server probes it with `ffprobe` when the file is on a local filesystem; where it cannot — a network or alternative storage location, or no `ffprobe` on the host — the preview element in the dialog supplies the duration instead, so the slider appears either way. Only the *duration* needs the probe; the times are sent as text and are used exactly as typed.
:::

## Finding Similar Resources

Perceptual hashing finds visually similar images. On any image resource's detail page, the **Similar Resources** section shows matches as thumbnail cards sorted by similarity. Click **Merge Others To This** to combine duplicates into one resource.

This requires the background hash worker (enabled by default).

## Deleting Resources

### Single Resource

1. Navigate to the resource detail page
2. Click the **Delete** button in the header
3. Confirm the deletion

### Bulk Deletion

1. In the resource list, select multiple resources using checkboxes
2. Click the **Delete Selected** button in the bulk editor
3. Confirm the deletion

:::warning

Deleted files are backed up to the `/deleted/` directory before the database record is removed. Files are only physically deleted from primary storage if no other Resources or versions reference the same hash. The backup naming format is `{hash}__{id}__{ownerId}___{basename}`.

:::

## Resource Metadata

### Automatic Metadata

Automatically captured on upload:

- **Original Name** - The filename at upload
- **Original Location** - Source URL for imports
- **Content Type** - MIME type
- **File Size** - Size in bytes
- **Hash** - Content hash for deduplication
- **Dimensions** - Width/height for images
- **Created/Updated** - Timestamps

### Custom Metadata

Add custom key-value pairs using the **Meta Data** section:

1. In the create/edit form, find the **Meta Data** section
2. Enter a key name
3. Enter a value (supports text, numbers, JSON)
4. Click **+** to add more fields
5. Save the resource

Custom metadata is searchable and can be used in filters.

### Free-Form Metadata Fields

The metadata editor renders dynamic key-value input rows. Each row has a key name field and a value field. Values are automatically coerced to typed JSON values:

| Input | Stored As |
|-------|-----------|
| `true` / `false` | Boolean |
| `null` | Null |
| `42`, `3.14` | Number |
| `2026-03-05` | Date string |
| `{"key": "val"}` | JSON object |
| anything else | String |

Existing keys from other entities of the same type appear as autocomplete suggestions in the key name field, helping maintain consistent naming across resources.

## Thumbnails

Thumbnails are generated automatically for supported file types:

| File Type | Requirements |
|-----------|--------------|
| Images | Built-in (HEIC/AVIF require ImageMagick) |
| SVGs | Built-in (oksvg rasterizer) |
| Videos | Requires FFmpeg |
| Office documents | Requires LibreOffice |

Thumbnails are generated on demand when first requested and cached in the database. For video files, the background thumbnail worker can pre-generate thumbnails.

### Custom Thumbnails

You can replace the auto-generated thumbnail of any resource with your own image. The **Custom Thumbnail** section appears in the sidebar next to the preview, and opens a dialog:

- **Upload Image** -- pick an image file (PNG, JPEG, WebP, or GIF) to use as the thumbnail
- **Paste** -- with the dialog open, paste an image from the clipboard to upload it as the thumbnail. A paste anywhere else on the page is not accepted
- **Regenerate from Source** -- clear the custom and cached thumbnails so the next view regenerates from the original file

Uploading a custom thumbnail does not create a new version -- it only changes the stored preview. The image is resized so its longest edge is at most 1920px and stored as JPEG. For the full pipeline details, see [Thumbnail Generation](../features/thumbnail-generation.md).

## Job Center

Open the **Jobs** panel in the header, press **Cmd/Ctrl+Shift+D**, or visit
`/jobs` for the full Job Center. The panel is a drawer on the right that
groups work needing attention, active and scheduled work, and recently finished
work that needs nothing more (**Finished, no attention needed**: succeeded and
cancelled Jobs, since failed ones are under **Needs attention**). A scheduled Job says when it
starts on the Job Center card, and in the panel and on its page also how long
that is from now. Only running work shows a moving progress bar; a Job that is
waiting, paused or stopped shows only what it reported. A running
Job shows its progress bar, speed, time left, any figures it reports and a graph
of its speed; see [Progress, metrics and graphs](../features/job-system.md#progress-metrics-and-graphs). The Job Center lists every Job you can see: downloads, exports,
imports, Resource Reduction, maintenance, and plugin work.

The Job Center is a list page like Notes or Resources. It shows the Jobs you
have not dismissed, newest first, 50 per page, with **Previous** and **Next**
at the bottom. Its pages have no numbers, so a page does not shift while new
Jobs arrive. The list updates in place as Jobs change, without a reload. If the
server's database is restored from a backup or replaced while a page is open,
the Job Center and a Job's page reload themselves, and the Jobs panel on any
other page stops with a **Reload page** button, so you can save your work first.

The **Show** links in the sidebar select **Needs attention** (blocked, failed,
or interrupted), **Active** (scheduled, queued, running, or paused),
**Finished** (succeeded, failed, cancelled, or interrupted), and **Pinned by
me**. A failed or interrupted Job is both Finished and Needs attention. Each count is how many Jobs the list shows with that link selected,
under your other filters. Select an active link again to clear it. The **Filter** form narrows
the list by text, state, Kind, currently available command, origin, owner,
actor, and acceptance time. **Origin** lists where a Job came from: `api` (a
page or an API request), `cli`, `plugin`, `schedule` and `admin`. Times are in
your browser's time zone, and an
**Accepted before** time includes the whole minute it names. **Dismissed** starts
at **Not dismissed**; choose **Any** to include dismissed Jobs, or **Dismissed**
to see only those Jobs. The address always names this choice: opening `/jobs`
without it adds `dismissed=false`, so a copied Job Center address lists the same
Jobs through the API and the CLI, which include dismissed Jobs unless asked not
to. Dismissal does not delete Job history. **Saved searches** keep a filter for reuse.

Below the filters, **Summary of these jobs** reads the figures of the Jobs the
filters select, accepted in the last day, 7, 30 or 90 days: how many there are,
how many succeeded of those finished, how many failed, the median and 95th
percentile of the time they waited and ran, and their failures by class. It is
read only when you open it. **Export a summary** queues a CSV or JSON export of
the same filter for a range of whole days longer than 90 days, and links the
export's Job, whose page offers the file once it is ready. **Mine** is exported
as your account's own Jobs; a filter an export cannot take (the partially
completed state, **Has been**, **Has not been**, a deleted account as owner) is
named instead of the form. An account that cannot write is not offered the
export.

Each card's **Details** lists when the Job was accepted, started and finished,
and the fields of its summary, such as a download's host. Open a Job to see its
progress, when it ran and how long, its timeline, outputs, and related Jobs, each named with how it is related (such as **Retry of**), its
state and when it was accepted; see [The Job page](../features/job-system.md#the-job-page).
Use only the controls displayed on that Job; available commands are checked again when
submitted. Selecting several Jobs offers only commands they all advertise for
bulk use, with a separate result for each Job, named by its title and linked to
its page. A summary such as "Pinned 3 of 3 selected jobs." is shown on the page
and announced, and names the Jobs the command was not done for. Cancel and
Retry work on several Jobs at once: Cancel asks first and says how many Jobs it
stops, in the Kind's own words when every selected Job is of one Kind, and each
Job of a bulk Retry is checked as a Retry of that Job alone would be. A deferred
download's **Download now** is offered for one Job at a time.

A succeeded Job always shows complete progress, including a download whose
size the remote server never reported. Under each bar one line gives the amount,
then the speed and time left while the Job runs or its average speed once it
has ended. A blocked Job's row in the **Jobs** panel says why it is blocked. When a Job created a Resource, Note, or
Group, open it from the **Jobs** panel, the Job Center list, or the Job detail
page. A download shows **View created resource**; a plugin action shows a typed
entity link such as **View resource**. A finished export shows **Download
exported archive**, and the file saves under a name with its date, the end of
the Job's id and its extension. Older plugin Jobs with only a result
summary show **View result** for the entity; their Job detail page also shows
**View JSON result** for the stored summary.

A Retry creates a linked successor. In the Jobs panel, Retry stays on the page
you are on and offers **Open the new job**; the retried failure leaves **Needs
attention**. Dismiss hides a finished Job from your view: in the panel the
notice offers **Undo**, and a dismissed Job's page reads **Dismissed by you**
and offers **Undismiss**. **Dismiss finished** asks first, saying how many Jobs
it dismisses. **Forget saved input** removes the input a Job saved for Retry,
Continue and Repeat and cannot be undone, so it asks for confirmation. A pin keeps Job metadata and its event history from
ordinary retention, while linked Jobs and output artifacts keep their own
retention rules. Pinned Jobs show **Pinned by you** in the list, detail page,
and Jobs panel; **Unpin** removes your pin. Administrators can see Jobs across
accounts; their Jobs panel lists **My jobs** until they choose **Everyone's**,
and remembers the choice. Other accounts see only work allowed by current scope.

Old `/downloads` links redirect to `/jobs` with recognized filters translated,
listing downloads scheduled for later as well as immediate ones. A status
filter lists every state the old page showed under that status: `pending`
includes scheduled work, `paused` includes blocked work, and `failed` includes
interrupted work. A bare date in `CreatedAfter` or `CreatedBefore` names the whole
day in the server's time zone, as a date does on the Job Center, so a range from
one day to the same day lists that day's downloads.
Legacy `/v1/downloads` API routes remain available during the compatibility
window. Download history retention remains configurable on `/admin/settings`;
see [Job System](../features/job-system.md#retention) for canonical Job and
replay retention.
