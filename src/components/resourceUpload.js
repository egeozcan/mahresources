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
 * Human-readable byte count. Mirrors downloadCockpit.formatBytes so the two
 * progress surfaces read the same.
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
 * A failure that a retry cannot change.
 *
 * Every deterministic 4xx qualifies, not only the duplicate 409: an oversized
 * file (refused in the browser, recorded as 413), an image the server cannot
 * decode (400) and a stale CSRF token (403) all answer identically however many
 * times the same bytes are sent. The exceptions are the two 4xx codes that mean
 * "try again" — 408 Request Timeout and 429 Too Many Requests.
 *
 * @param {{httpStatus: number}} file
 * @returns {boolean}
 */
export function isPermanentFailure(file) {
  const s = file.httpStatus;
  if (s === 408 || s === 429) return false;
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
    willUseWidget: false,
    selectionCount: 0,
    selectionBytes: 0,

    concurrency: 3,
    countThreshold: 10,
    sizeThreshold: 1 << 30,
    maxUploadSize: 0,

    cancelled: false,

    init() {
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
        if (event.target !== this.$el) return;
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
      this.abortAll();
    },

    // ----- selection ------------------------------------------------------

    /** Recompute the threshold decision whenever the picker changes. */
    onFilesChosen(event) {
      const picked = [...(event?.target?.files || [])];
      // A fresh selection is a fresh batch. Without this the panel from a
      // previous partial run stays on screen describing files that are no longer
      // selected, and Save stays disabled with no way back to `idle`.
      if (this.phase !== 'uploading') {
        this.phase = 'idle';
        this.files = [];
        this.doneCount = 0;
        this.cancelled = false;
      }
      this.selectionCount = picked.length;
      this.selectionBytes = picked.reduce((sum, f) => sum + f.size, 0);
      this.willUseWidget = shouldUseClientUpload({
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
      // schema-form-mode registers its own bubble-phase submit listener and
      // preventDefault()s when the Meta schema fails validation. stopPropagation
      // does not stop a sibling listener on the same element, so without this
      // check the batch would upload past a validation failure.
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

      this.files = picked.map((file) => ({
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
      setTimeout(() => {
        // Optional chaining, not decoration: this runs a macrotask later, by
        // which time the component may have been torn down. An exception in a
        // timer is unhandled — it reaches the page as an uncaught error.
        const summary = this.$el?.querySelector('[data-testid="bulk-upload-summary"]');
        if (summary) focusOn(summary);
      }, 0);
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
