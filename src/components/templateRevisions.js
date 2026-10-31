// templateRevisions powers the History page of a Category, Resource Category or
// Note Type. It lists the carrier's Template Revisions, diffs any two (or one
// against the current state) and edits labels. The server computes the diff;
// this only lays the hunks out. "Load into editor" is a plain link to the edit
// form, which reads ?loadRevision= (see templateBundle.js).

// Equal runs longer than this are folded to CONTEXT lines on each side of a change.
// Longest label the server accepts, in characters.
const LABEL_MAX = 200;
const FOLD_OVER = 8;
const CONTEXT = 3;

// The slots with no CSS companion: everything else named Custom* has one.
function slotHasCss(slot) {
  return slot.startsWith('Custom') && slot !== 'CustomCSS';
}

// foldEqualRuns replaces a long unchanged run with a gap row. The first and
// last runs keep context only on the side that touches a change.
function foldEqualRuns(rows) {
  const out = [];
  let i = 0;
  while (i < rows.length) {
    if (!rows[i].equal) {
      out.push(rows[i++]);
      continue;
    }
    let j = i;
    while (j < rows.length && rows[j].equal) j++;
    const run = rows.slice(i, j);
    const head = i === 0 ? 0 : CONTEXT;
    const tail = j === rows.length ? 0 : CONTEXT;
    if (run.length > FOLD_OVER && run.length > head + tail) {
      out.push(...run.slice(0, head), { gap: run.length - head - tail }, ...run.slice(run.length - tail));
    } else {
      out.push(...run);
    }
    i = j;
  }
  return out;
}

// unifiedRows is one row per line in the server's order.
export function unifiedRows(hunks) {
  const rows = [];
  for (const hunk of hunks) {
    for (const text of hunk.lines) rows.push({ op: hunk.op, text, equal: hunk.op === 'equal' });
  }
  return foldEqualRuns(rows);
}

// sideBySideRows pairs the deleted and inserted lines of each change, left
// against right, and shows an unchanged line on both sides.
export function sideBySideRows(hunks) {
  const rows = [];
  let removed = [];
  let added = [];
  const flush = () => {
    const n = Math.max(removed.length, added.length);
    for (let k = 0; k < n; k++) {
      rows.push({
        left: k < removed.length ? { op: 'delete', text: removed[k] } : null,
        right: k < added.length ? { op: 'insert', text: added[k] } : null,
      });
    }
    removed = [];
    added = [];
  };
  for (const hunk of hunks) {
    if (hunk.op === 'delete') removed.push(...hunk.lines);
    else if (hunk.op === 'insert') added.push(...hunk.lines);
    else {
      flush();
      for (const text of hunk.lines) rows.push({ left: { op: 'equal', text }, right: { op: 'equal', text }, equal: true });
    }
  }
  flush();
  return foldEqualRuns(rows);
}

export function templateRevisions({ carrierType, id, apiPrefix, editUrl } = {}) {
  return {
    carrierType,
    id,
    apiPrefix,
    editUrl,
    root: null,

    revisions: [],
    loading: true,
    error: '',
    status: '',

    // Diff selection, as <select> values: a revision number, or '' for none (from)
    // and for the current state (to).
    from: '',
    to: '',
    diff: null,
    diffLoading: false,
    diffError: '',
    layout: 'unified',
    cssChoice: {},
    _diffRequest: 0,

    editingLabel: null,
    labelDraft: '',
    savingLabel: false,
    _labelSession: 0,

    async init() {
      this.root = this.$el;
      await this.loadRevisions();
    },

    url(path, params = {}) {
      const query = new URLSearchParams({ id: String(this.id), ...params });
      return `${this.apiPrefix}/revisions${path}?${query}`;
    },

    async errorText(resp) {
      const body = await resp.json().catch(() => null);
      return (body && (body.error || body.Error)) || `Request failed (${resp.status})`;
    },

    async loadRevisions() {
      this.loading = true;
      this.error = '';
      try {
        const resp = await fetch(this.url(''), { headers: { Accept: 'application/json' } });
        if (!resp.ok) {
          this.error = await this.errorText(resp);
          return;
        }
        this.revisions = (await resp.json()) || [];
      } catch (err) {
        this.error = err.message || 'Network error';
      } finally {
        this.loading = false;
      }
    },

    // ---- Row display ---------------------------------------------------------

    when(rev) {
      const date = new Date(rev.createdAt);
      return Number.isNaN(date.getTime()) ? rev.createdAt : date.toLocaleString();
    },

    sourceText(rev) {
      return rev.source === 'restore' && rev.restoredFrom ? `restore (from #${rev.restoredFrom})` : rev.source;
    },

    slotsText(rev) {
      return (rev.changedSlots || []).join(', ');
    },

    // ---- Diff ----------------------------------------------------------------

    isCompared(rev) {
      return this.diff !== null && (this.from === rev.number || this.to === rev.number);
    },

    async compareWithCurrent(number) {
      this.from = number;
      this.to = '';
      await this.loadDiff({ focusResult: true });
    },

    async onSelectionChange() {
      if (!this.from) {
        // Nothing selected: a response still on its way must not install later.
        this._diffRequest++;
        this.diff = null;
        this.diffLoading = false;
        this.diffError = '';
        this.status = '';
        return;
      }
      await this.loadDiff({ focusResult: false });
    },

    async loadDiff({ focusResult }) {
      if (!this.from) return;
      const requestId = ++this._diffRequest;
      this.diffLoading = true;
      this.diffError = '';
      const params = { from: String(this.from) };
      if (this.to) params.to = String(this.to);
      let prepared = null;
      let failure = '';
      try {
        const resp = await fetch(this.url('/diff', params), { headers: { Accept: 'application/json' } });
        // Decode before the freshness check: a stale body is read here, not committed.
        if (resp.ok) prepared = this.prepareDiff(await resp.json());
        else failure = await this.errorText(resp);
      } catch (err) {
        failure = err.message || 'Network error';
      }
      if (requestId !== this._diffRequest) return;
      this.diffLoading = false;
      this.diff = prepared;
      this.diffError = failure;
      if (prepared) {
        this.status = prepared.slots.length
          ? `Showing ${prepared.slots.length} changed ${prepared.slots.length === 1 ? 'slot' : 'slots'}.`
          : 'No differences.';
      }
      if (focusResult) this.focusDiffHeading();
    },

    prepareDiff(data) {
      const slots = (data.slots || []).map((slot) => ({
        slot: slot.slot,
        hasCss: slotHasCss(slot.slot),
        fields: (slot.fields || []).map((field) => ({
          field: field.field,
          unified: unifiedRows(field.hunks || []),
          sideBySide: sideBySideRows(field.hunks || []),
        })),
      }));
      return { from: data.from, to: data.to || null, slots };
    },

    focusDiffHeading() {
      // The heading is inside an x-if that has just rendered.
      setTimeout(() => {
        const heading = this.root && this.root.querySelector('[data-revision-diff-heading]');
        if (heading) heading.focus();
      }, 0);
    },

    rightLabel() {
      return this.diff && this.diff.to ? `Revision #${this.diff.to}` : 'Current state';
    },

    marker(op) {
      return op === 'insert' ? '+' : op === 'delete' ? '-' : ' ';
    },

    // Colour is never the only signal: screen readers get this text.
    srText(op) {
      return op === 'insert' ? 'Added: ' : op === 'delete' ? 'Removed: ' : '';
    },

    rowClass(op) {
      if (op === 'insert') return 'bg-green-50 text-green-900';
      if (op === 'delete') return 'bg-red-50 text-red-900';
      return 'text-stone-700';
    },

    gapText(row) {
      return `${row.gap} unchanged ${row.gap === 1 ? 'line' : 'lines'}`;
    },

    // ---- Load into editor ----------------------------------------------------

    cssIncluded(slot) {
      return this.cssChoice[slot] !== false;
    },

    setCssIncluded(slot, value) {
      this.cssChoice = { ...this.cssChoice, [slot]: value };
    },

    loadWholeUrl(number) {
      return `${this.editUrl}&loadRevision=${number}`;
    },

    loadSlotUrl(number, slot, hasCss) {
      const css = hasCss && !this.cssIncluded(slot) ? '&includeCss=0' : '';
      return `${this.editUrl}&loadRevision=${number}&slot=${encodeURIComponent(slot)}${css}`;
    },

    // ---- Labels --------------------------------------------------------------

    startLabelEdit(rev) {
      if (this.savingLabel) return;
      if (this.editingLabel === rev.number) {
        // Reopening the open row keeps its draft.
        this.root.querySelector(`[data-label-input="${rev.number}"]`)?.focus();
        return;
      }
      if (this.editingLabel !== null) return;
      this._labelSession++;
      this.editingLabel = rev.number;
      this.labelDraft = rev.label || '';
      setTimeout(() => {
        const input = this.root.querySelector(`[data-label-input="${rev.number}"]`);
        if (input) input.focus();
      }, 0);
    },

    cancelLabelEdit(number) {
      if (this.savingLabel) return;
      this._labelSession++;
      this.editingLabel = null;
      this.focusLabelButton(number);
    },

    // The server counts characters (code points), not UTF-16 units.
    get labelTooLong() {
      return [...this.labelDraft].length > LABEL_MAX;
    },

    async saveLabel(rev) {
      if (this.savingLabel || this.labelTooLong) return;
      this.savingLabel = true;
      this.status = `Saving the label of revision ${rev.number}...`;
      const session = this._labelSession;
      // Only the edit that submitted may close the form and move focus.
      const stillMine = () => session === this._labelSession && this.editingLabel === rev.number;
      try {
        const resp = await fetch(this.url(`/${rev.number}/label`), {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ label: this.labelDraft }),
        });
        if (!resp.ok) {
          this.status = `Could not save the label: ${await this.errorText(resp)}`;
          const input = this.root.querySelector(`[data-label-input="${rev.number}"]`);
          if (input && stillMine()) input.focus();
          return;
        }
        const saved = await resp.json();
        this.revisions = this.revisions.map((r) => (r.number === rev.number ? { ...r, label: saved.label } : r));
        this.status = saved.label ? `Saved the label of revision ${rev.number}.` : `Cleared the label of revision ${rev.number}.`;
        if (stillMine()) {
          this.editingLabel = null;
          this.focusLabelButton(rev.number);
        }
      } catch (err) {
        this.status = `Could not save the label: ${err.message || 'network error'}`;
      } finally {
        this.savingLabel = false;
      }
    },

    // The input is removed with the form, so focus goes back to the control that opened it.
    focusLabelButton(number) {
      setTimeout(() => {
        const button = this.root.querySelector(`[data-label-button="${number}"]`);
        if (button) button.focus();
      }, 0);
    },
  };
}
