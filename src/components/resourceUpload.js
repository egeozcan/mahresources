/**
 * Client-side bulk upload for the create-resource page.
 *
 * The native form posts every selected file in one multipart body, which the
 * server buffers with ParseMultipartForm. For a large batch that is minutes of a
 * page that looks hung, no per-file outcome, no cancel — and one `MaxBytesReader`
 * budget for the whole batch, so exceeding it wastes the entire transfer.
 *
 * Above a threshold this component intercepts the submit and posts one file per
 * request instead, through a bounded pool, reporting real byte progress from
 * XMLHttpRequest.upload. `POST /v1/resource` is unchanged and still accepts many
 * files per request: the split is purely a browser-side decision, and below the
 * threshold — or with JavaScript unavailable — the native post still happens.
 */

import { csrfToken } from '../utils/csrfToken.js';
import { parseUploadError } from '../utils/uploadError.js';
import { focusOn } from '../utils/focus.js';

// ---------------------------------------------------------------------------
// Pure helpers (exported for unit tests)
// ---------------------------------------------------------------------------

/**
 * Whether a selection should go through the client-side widget.
 *
 * Both thresholds are strictly-greater: selecting exactly the file-count
 * threshold, or exactly the size threshold, keeps the native post. A URL always
 * wins — that path is a server-side remote download and ignores the picker.
 *
 * @param {{fileCount: number, totalBytes: number, hasUrl: boolean, sizeThreshold: number, countThreshold: number}} input
 * @returns {boolean}
 */
export function shouldUseClientUpload({ fileCount, totalBytes, hasUrl, sizeThreshold, countThreshold }) {
  if (hasUrl) return false;
  if (!fileCount) return false;
  return totalBytes > sizeThreshold || fileCount > countThreshold;
}

/**
 * Aggregate byte progress across the batch.
 *
 * Bytes, not files-completed: a batch of one 4 GB file and nine small ones would
 * otherwise sit at 0% and then jump to 90%.
 *
 * @param {Array<{size: number, loaded: number, status: string}>} files
 * @returns {{loaded: number, total: number, percent: number}}
 */
export function aggregateProgress(files) {
  let loaded = 0;
  let total = 0;
  for (const f of files) {
    total += f.size;
    loaded += f.status === 'done' ? f.size : Math.min(f.loaded || 0, f.size);
  }
  const percent = total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : 0;
  return { loaded, total, percent };
}

/**
 * Human-readable byte count.
 *
 * Deliberately its own: downloadCockpit has a formatter of the same name that
 * trims a whole unit's decimal via parseFloat, so 1024 reads "1 KB" there and
 * "1.0 KB" here. This panel counts a file selection rather than a transfer, and
 * sharing one formatter would mean changing that surface's output to change
 * this one.
 * @param {number} bytes
 * @returns {string}
 */
export function formatBytes(bytes) {
  if (!bytes || bytes < 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

/**
 * The 4xx codes that explicitly mean "the same request may work later".
 * Everything else in the range is the server's settled answer about these bytes.
 */
const RETRYABLE_4XX = new Set([
  408, // Request Timeout
  423, // Locked
  425, // Too Early
  429, // Too Many Requests
]);

/**
 * A failure that a retry cannot change.
 *
 * Every deterministic 4xx qualifies, not only the duplicate 409: an oversized
 * file (refused in the browser, recorded as 413), an image the server cannot
 * decode (400) and a stale CSRF token (403) all answer identically however many
 * times the same bytes are sent. The exceptions are collected in RETRYABLE_4XX.
 *
 * @param {{httpStatus: number}} file
 * @returns {boolean}
 */
export function isPermanentFailure(file) {
  const s = file.httpStatus;
  if (RETRYABLE_4XX.has(s)) return false;
  return s >= 400 && s < 500;
}

// ---------------------------------------------------------------------------
// Alpine component
// ---------------------------------------------------------------------------

/**
 * Owns the whole create-resource form scope: `url` and `background` live here
 * too, because the form's :action / :enctype bindings read them and @submit has
 * to sit on the same element.
 */
export function resourceUpload() {
  return {
    // ----- form scope (was an inline x-data object) -----------------------
    url: '',
    background: false,

    // ----- widget state ---------------------------------------------------
    /** 'idle' | 'uploading' | 'partial' | 'done' */
    phase: 'idle',
    /** { file, name, size, loaded, status, error, errorResourceId, httpStatus } */
    files: [],
    doneCount: 0,
    selectionCount: 0,
    selectionBytes: 0,

    concurrency: 3,
    countThreshold: 10,
    sizeThreshold: 1 << 30,
    maxUploadSize: 0,

    cancelled: false,

    init() {
      // Captured here, not read from $el later. Alpine's $el resolves to the
      // element whose expression is being evaluated, so inside a method invoked
      // from @click it is the *button*, not this form — focusSummary() searching
      // it found nothing and focus fell to <body>. It only appeared to work from
      // finish(), which runs in a later macrotask where $el has fallen back to
      // the component root.
      this._root = this.$el;
      const d = this.$el.dataset;
      // The submit handler is registered on `document`, not as @submit on the
      // form, and that is load-bearing rather than stylistic.
      //
      // `schema-form-mode` registers its own submit listener on this same form
      // and calls preventDefault() + stopPropagation() when the Meta schema
      // fails validation. It is created inside an `x-if`, so it connects *after*
      // Alpine has wired the form — and listeners on one element fire in
      // registration order, so an @submit here would run first and start
      // uploading before validation had a chance to object. Listening on an
      // ancestor instead means stopPropagation() does exactly what it says: the
      // event never reaches this handler, and nothing is uploaded.
      this._submitHandler = (event) => {
        if (event.target !== this._root) return;
        this.onSubmit(event);
      };
      document.addEventListener('submit', this._submitHandler);
      // Read from data-* attributes rather than an interpolated x-data
      // expression: `url` is a reflected query parameter, and Pongo2 escapes a
      // quote to &#39;, which the HTML parser decodes back to a quote before
      // Alpine evaluates the attribute as JavaScript. A dataset value is only
      // ever read as a string.
      this.url = d.initialUrl || '';
      this.concurrency = toPositiveInt(d.uploadConcurrency, this.concurrency);
      this.countThreshold = toPositiveInt(d.uploadFileThreshold, this.countThreshold);
      this.sizeThreshold = toPositiveInt(d.uploadSizeThreshold, this.sizeThreshold);
      this.maxUploadSize = toPositiveInt(d.maxUploadSize, 0);

      this._xhrs = new Set();
      this._beforeUnload = (e) => {
        if (this.phase !== 'uploading') return;
        e.preventDefault();
        // Chrome ignores the string but requires returnValue to be set.
        e.returnValue = '';
      };
      window.addEventListener('beforeunload', this._beforeUnload);
    },

    destroy() {
      window.removeEventListener('beforeunload', this._beforeUnload);
      document.removeEventListener('submit', this._submitHandler);
      // Order matters. abortAll() on its own resolves the in-flight requests
      // without stopping the pool: the workers would read `cancelled` as false,
      // dequeue the next file and keep uploading from a component that is no
      // longer on the page — and finish() could then navigate the browser
      // somewhere the user never asked to go.
      this._destroyed = true;
      this.cancelled = true;
      this.abortAll();
    },

    // ----- selection ------------------------------------------------------

    /** Recompute the threshold decision whenever the picker changes. */
    onFilesChosen(event) {
      // A change during a run belongs to no batch: the running one was
      // snapshotted at submit and will not adopt it, and updating the hint from
      // it would describe a selection nothing is going to upload. The picker is
      // disabled while uploading and the paste handler skips a disabled input,
      // so this is the backstop rather than the mechanism.
      if (this.phase === 'uploading') return;

      const picked = [...(event?.target?.files || [])];
      // A fresh selection is a fresh batch. Without this the panel from a
      // previous partial run stays on screen describing files that are no longer
      // selected, and Save stays disabled with no way back to `idle`.
      this.phase = 'idle';
      this.files = [];
      this.doneCount = 0;
      this.cancelled = false;
      this.selectionCount = picked.length;
      this.selectionBytes = picked.reduce((sum, f) => sum + f.size, 0);
    },

    /**
     * Whether the current selection would go through the widget.
     *
     * A getter, not a field set in onFilesChosen: the answer also depends on the
     * URL field, which changes without the picker changing. Typing a URL used to
     * leave a stale "these will upload one at a time" promise on screen, and
     * clearing one left the hint hidden while the submit did use the widget.
     */
    get willUseWidget() {
      return shouldUseClientUpload({
        fileCount: this.selectionCount,
        totalBytes: this.selectionBytes,
        hasUrl: this.url.trim() !== '',
        sizeThreshold: this.sizeThreshold,
        countThreshold: this.countThreshold,
      });
    },

    /** The hint under the file picker, once the threshold is crossed. */
    get selectionSummary() {
      const n = this.selectionCount;
      return `${n} file${n === 1 ? '' : 's'}, ${formatBytes(this.selectionBytes)} — these will be uploaded one at a time, with progress.`;
    },

    // ----- submit ---------------------------------------------------------

    onSubmit(event) {
      // Not what stops schema-form-mode: init() is, by listening on `document`
      // so that handler's stopPropagation() keeps a failed Meta validation from
      // ever reaching here. This is the second line, and it covers the other
      // shape -- anything that calls preventDefault() without also stopping
      // propagation, whose event does arrive.
      if (event.defaultPrevented) return;

      const form = event.target;
      const input = form.querySelector('input[type="file"][name="resource"]');
      const picked = [...(input?.files || [])];

      if (!shouldUseClientUpload({
        fileCount: picked.length,
        totalBytes: picked.reduce((sum, f) => sum + f.size, 0),
        hasUrl: this.url.trim() !== '',
        sizeThreshold: this.sizeThreshold,
        countThreshold: this.countThreshold,
      })) {
        return; // native multipart post, unchanged
      }

      event.preventDefault();

      // Snapshot every other field ONCE. new FormData(form) reproduces native
      // submission exactly: it skips the autocompleters' disabled empty
      // sentinels and picks up the hidden input schema-form-mode appends for
      // Meta. Snapshotting means later DOM changes cannot alter the batch.
      const base = new FormData(form);
      base.delete('resource');
      this._baseEntries = [...base.entries()];
      this._action = form.getAttribute('action') || '/v1/resource';

      // The index is the key. A file input can hold two entries with the same
      // name and the same size — picked from different directories — so that
      // pair is not an identity, and Alpine reconciling two rows onto one key
      // reports the wrong progress against the wrong file.
      this.files = picked.map((file, index) => ({
        key: `${index}:${file.name}`,
        file,
        name: file.name,
        size: file.size,
        loaded: 0,
        status: 'queued',
        error: '',
        errorResourceId: null,
        httpStatus: 0,
      }));
      this.doneCount = 0;
      this.cancelled = false;

      this.run(this.files.map((_, i) => i));
    },

    // ----- the pool -------------------------------------------------------

    async run(indices) {
      this.phase = 'uploading';
      announce(`Uploading ${indices.length} file${indices.length === 1 ? '' : 's'}.`);

      let cursor = 0;
      const next = () => (cursor < indices.length ? indices[cursor++] : -1);

      const worker = async () => {
        for (let i = next(); i !== -1; i = next()) {
          if (this.cancelled) return;
          try {
            await this.uploadOne(i);
          } catch (err) {
            // uploadOne resolves on every path it knows about, so reaching here
            // means something unforeseen threw. Swallowing it into the file's
            // own row matters: an escaping rejection would reject Promise.all,
            // skip finish(), and leave the panel stuck in 'uploading' forever
            // with Save disabled and no cancel that ends anything.
            const entry = this.files[i];
            entry.status = 'failed';
            entry.error = err?.message || 'Upload failed unexpectedly.';
          }
        }
      };

      const width = Math.max(1, Math.min(this.concurrency, indices.length));
      await Promise.all(Array.from({ length: width }, worker));

      this.finish();
    },

    finish() {
      // A batch that ended because the component went away has no one left to
      // report to, and must not navigate.
      if (this._destroyed) return;

      const failed = this.files.filter((f) => f.status === 'failed');

      if (this.cancelled) {
        this.phase = 'partial';
        const inFlight = this.cancelledInFlight.length;
        announce(
          inFlight > 0
            ? `Upload cancelled. ${this.doneCount} of ${this.files.length} files were saved, and ${inFlight} were still in progress, which the server may have saved anyway.`
            : `Upload cancelled. ${this.doneCount} of ${this.files.length} files were saved.`
        );
        this.parkFocus();
        return;
      }

      if (failed.length > 0) {
        this.phase = 'partial';
        announce(`${this.doneCount} uploaded, ${failed.length} failed.`);
        this.parkFocus();
        return;
      }

      this.phase = 'done';
      announce(`All ${this.doneCount} files uploaded.`);
      this.navigateAfterSuccess();
    },

    /**
     * Where a fully successful batch lands.
     *
     * Mirrors the server's own redirect (resource_api_handlers.go): one file
     * goes to that resource, many go to the owner group. The server's fallback
     * for a batch with no owner is /group?id=0, which is a dead page — the list
     * is used instead.
     */
    navigateAfterSuccess() {
      window.removeEventListener('beforeunload', this._beforeUnload);

      if (this.files.length === 1 && this.files[0].resourceId) {
        window.location.assign(`/resource?id=${this.files[0].resourceId}`);
        return;
      }

      const ownerId = this._baseEntries
        .filter(([k, v]) => k === 'ownerId' && String(v).trim() !== '')
        .map(([, v]) => String(v))
        .pop();

      window.location.assign(ownerId ? `/group?id=${encodeURIComponent(ownerId)}` : '/resources');
    },

    // ----- one request ----------------------------------------------------

    uploadOne(index) {
      const entry = this.files[index];

      // Pre-check against the server's per-request limit. Its own answer is a
      // raw MaxBytesError string at HTTP 400, which is not worth transferring a
      // gigabyte to receive.
      if (this.maxUploadSize > 0 && entry.size > this.maxUploadSize) {
        entry.status = 'failed';
        entry.error = `File is larger than the ${formatBytes(this.maxUploadSize)} upload limit.`;
        entry.httpStatus = 413;
        return Promise.resolve();
      }

      const body = new FormData();
      for (const [key, value] of this._baseEntries) body.append(key, value);
      body.append('resource', entry.file, entry.name);

      entry.status = 'uploading';
      entry.loaded = 0;

      return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        this._xhrs.add(xhr);

        xhr.open('POST', this._action, true);
        // Without this the server answers a 302, which XHR follows silently.
        xhr.setRequestHeader('Accept', 'application/json');
        // The global CSRF wrapper only covers fetch().
        const token = csrfToken();
        if (token) xhr.setRequestHeader('X-CSRF-Token', token);

        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) entry.loaded = e.loaded;
        };

        const settle = () => {
          this._xhrs.delete(xhr);
          resolve();
        };

        xhr.onload = () => {
          entry.httpStatus = xhr.status;
          if (xhr.status >= 200 && xhr.status < 300) {
            entry.status = 'done';
            entry.loaded = entry.size;
            entry.resourceId = firstResourceId(xhr.responseText);
            this.doneCount++;
          } else {
            const parsed = parseUploadError(xhr.responseText, xhr.status);
            entry.status = 'failed';
            entry.error = parsed.message;
            entry.errorResourceId = parsed.resourceId;
          }
          settle();
        };

        xhr.onerror = () => {
          entry.status = 'failed';
          entry.error = 'Network error. Check your connection and retry.';
          settle();
        };

        xhr.onabort = () => {
          // Deliberately its own status, not 'queued'. Aborting an XHR stops the
          // browser reading the response; it does not stop the server. A request
          // whose body had already arrived may have been committed, so claiming
          // the file was not saved would be a guess presented as a fact. The
          // panel says so instead, and the row is neither counted as done nor
          // offered for retry.
          entry.status = 'cancelled';
          entry.loaded = 0;
          settle();
        };

        xhr.send(body);
      });
    },

    // ----- controls -------------------------------------------------------

    abortAll() {
      for (const xhr of this._xhrs || []) xhr.abort();
      this._xhrs?.clear();
    },

    cancel() {
      this.cancelled = true;
      // Cancel is about to be hidden by the phase change, and the browser drops
      // focus to <body> when the focused element disappears — a keyboard or
      // screen-reader user would have to navigate the whole page again. finish()
      // parks focus on the panel summary instead.
      this._returnFocusToPanel = true;
      this.abortAll();
    },

    /**
     * Move focus onto the panel summary when the control that had it is about to
     * disappear. Deferred by a macrotask: Alpine has not applied the x-show that
     * hides Cancel yet, and focusing before that runs lets the teardown take
     * focus straight back off again.
     */
    parkFocus() {
      if (!this._returnFocusToPanel) return;
      this._returnFocusToPanel = false;
      setTimeout(() => this.focusSummary(), 0);
    },

    /** Puts focus on the panel summary, which is present in every phase. */
    focusSummary() {
      // Optional chaining, not decoration: parkFocus reaches this a macrotask
      // later, by which time the component may have been torn down, and an
      // exception in a timer is unhandled — it reaches the page as an uncaught
      // error.
      const summary = this._root?.querySelector('[data-testid="bulk-upload-summary"]');
      if (summary) focusOn(summary);
    },

    /** Files worth sending again — a 409 would be refused identically forever. */
    get retryableIndices() {
      return this.files
        .map((f, i) => (f.status === 'failed' && !isPermanentFailure(f) ? i : -1))
        .filter((i) => i !== -1);
    },

    retryFailed() {
      const indices = this.retryableIndices;
      if (indices.length === 0) return;
      this.cancelled = false;
      for (const i of indices) {
        this.files[i].status = 'queued';
        this.files[i].error = '';
        this.files[i].loaded = 0;
      }
      // Same trap as Cancel: the phase change hides the button that was just
      // activated, and the browser drops focus to <body> when the focused
      // element disappears.
      //
      // Handled the other way round from Cancel, though — focus moves *before*
      // run() rather than after. Cancel's handoff happens from finish(), a
      // macrotask later, by which time the button is already gone. Here the
      // phase changes synchronously inside run(), so moving focus first means
      // the browser never has to relocate it at all, and there is no ordering
      // between Alpine's flush and a timer to get right.
      this.focusSummary();
      this.run(indices);
    },

    // ----- rendering ------------------------------------------------------

    get inFlight() {
      return this.files.filter((f) => f.status === 'uploading');
    },

    get failed() {
      return this.files.filter((f) => f.status === 'failed');
    },

    /**
     * Files whose request was aborted mid-flight. Their outcome is genuinely
     * unknown to the browser — see xhr.onabort.
     */
    get cancelledInFlight() {
      return this.files.filter((f) => f.status === 'cancelled');
    },

    get progress() {
      return aggregateProgress(this.files);
    },

    /** The batch summary line above the aggregate bar. */
    formatProgress() {
      const { loaded, total, percent } = this.progress;
      return `${this.doneCount} of ${this.files.length} files · ${formatBytes(loaded)} of ${formatBytes(total)} (${percent}%)`;
    },

    /**
     * aria-valuenow, or null for an indeterminate bar.
     *
     * ARIA says an indeterminate progressbar omits aria-valuenow; Alpine removes
     * an attribute bound to null. A bar pinned at 0 announces "0 percent" over
     * and over while bytes are arriving.
     */
    progressValueNow() {
      const { total, percent } = this.progress;
      return total > 0 ? percent : null;
    },

    /** Describes an indeterminate bar, where a percentage would be a lie. */
    progressValueText() {
      return this.progress.total > 0 ? null : 'Preparing upload';
    },

    /** Named after the batch, so it is never a bare prefix. */
    progressLabel() {
      return `Upload progress: ${this.formatProgress()}`;
    },

    fileValueNow(entry) {
      return entry.size > 0 ? Math.min(100, Math.round((entry.loaded / entry.size) * 100)) : null;
    },

    fileLabel(entry) {
      const pct = this.fileValueNow(entry);
      return pct === null
        ? `Upload progress: ${entry.name}, size unknown`
        : `Upload progress: ${entry.name}, ${pct}%`;
    },
  };
}

// ---------------------------------------------------------------------------
// Module-private
// ---------------------------------------------------------------------------

function toPositiveInt(raw, fallback) {
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

/** The endpoint answers with an array of the resources it created. */
function firstResourceId(responseText) {
  try {
    const parsed = JSON.parse(responseText);
    if (Array.isArray(parsed) && parsed[0]) return parsed[0].ID ?? null;
  } catch (_) {
    // Not JSON — the caller only uses this for the single-file redirect.
  }
  return null;
}

function announce(message) {
  // globalThis, not window: the pool is unit-tested outside a browser, and a
  // bare `window` reference would make an announcement throw mid-batch.
  globalThis.mahAnnounce?.(message);
}
