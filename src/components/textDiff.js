import * as Diff from 'diff';

/** Unchanged lines kept either side of a change before the rest is folded away. */
const CONTEXT_LINES = 3;

/** A run of unchanged lines shorter than this is never worth a fold control. */
const MIN_FOLD_LINES = 6;

/**
 * Combined size above which the diff asks before running. Both bodies are
 * fetched whole and compared line by line, so a large log or dump would freeze
 * the tab with no warning and no way out. The prompt carries the real number, so
 * the person deciding can see what they are agreeing to.
 */
const CONFIRM_ABOVE_BYTES = 2 * 1024 * 1024;

function formatBytes(bytes) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${unit === 0 ? value : value.toFixed(1)} ${units[unit]}`;
}

/**
 * @param {object} options
 * @param {string|null} [options.leftUrl] fetched when the text is not supplied inline
 * @param {string|null} [options.rightUrl]
 * @param {string|null} [options.leftText] supplied inline by the group comparison
 * @param {string|null} [options.rightText]
 * @param {number} [options.totalBytes] combined size, used to decide whether to ask first
 */
export function textDiff({
  leftUrl = null,
  rightUrl = null,
  leftText = null,
  rightText = null,
  totalBytes = 0,
}) {
  return {
    mode: 'unified',
    loading: true,
    error: null,
    // Set when the pair is large enough to ask first. The fetch has not started.
    needsConfirmation: false,
    totalBytes,
    leftText: leftText ?? '',
    rightText: rightText ?? '',
    unifiedDiff: [],
    splitLeft: [],
    splitRight: [],
    // What the template loops over: hidden rows dropped, each collapsed run
    // replaced by a single fold row. Rebuilt when a fold opens.
    unifiedRows: [],
    splitLeftRows: [],
    splitRightRows: [],
    stats: { added: 0, removed: 0 },
    changeCount: 0,
    activeChange: -1,
    // Fold ids the reader has expanded. A fold hides unchanged lines only.
    expandedFolds: [],
    copied: false,
    _abort: null,

    get sizeLabel() {
      return formatBytes(this.totalBytes);
    },

    async init() {
      if (leftText !== null && rightText !== null) {
        this.computeDiff();
        this.loading = false;
        return;
      }

      if (this.totalBytes > CONFIRM_ABOVE_BYTES) {
        this.needsConfirmation = true;
        this.loading = false;
        return;
      }

      await this.load();
    },

    async load() {
      this.needsConfirmation = false;
      this.loading = true;
      this.error = null;

      // Leaving the page mid-fetch used to leave two full-size downloads running.
      this._abort = new AbortController();
      const signal = this._abort.signal;

      try {
        const [leftRes, rightRes] = await Promise.all([
          fetch(leftUrl, { signal }),
          fetch(rightUrl, { signal }),
        ]);

        if (!leftRes.ok || !rightRes.ok) {
          const failed = !leftRes.ok ? leftRes : rightRes;
          throw new Error(`Could not load the file to compare (HTTP ${failed.status}).`);
        }

        this.leftText = await leftRes.text();
        this.rightText = await rightRes.text();
        this.computeDiff();
      } catch (e) {
        if (e.name === 'AbortError') return;
        this.error = e.message;
      } finally {
        this.loading = false;
      }
    },

    destroy() {
      if (this._abort) this._abort.abort();
    },

    computeDiff() {
      const diff = Diff.diffLines(this.leftText, this.rightText);

      this.unifiedDiff = [];
      this.splitLeft = [];
      this.splitRight = [];
      this.expandedFolds = [];

      let leftNum = 0;
      let rightNum = 0;
      let added = 0;
      let removed = 0;
      let changeIndex = 0;

      // Pair each removal with the addition opposite it, so the split view puts
      // the two halves of one change on the same row and a single changed token
      // can be marked instead of the whole line being struck out and rewritten.
      const parts = [];
      for (const part of diff) {
        const lines = part.value.split('\n');
        if (lines[lines.length - 1] === '') lines.pop();
        parts.push({ added: part.added, removed: part.removed, lines });
      }

      for (let i = 0; i < parts.length; i++) {
        const part = parts[i];

        if (!part.added && !part.removed) {
          for (const line of part.lines) {
            leftNum++;
            rightNum++;
            this.unifiedDiff.push({
              type: 'context', prefix: ' ', content: line, segments: null,
              leftNum, rightNum, changeIndex: null,
            });
            this.splitLeft.push({ num: leftNum, content: line, segments: null, changed: false, blank: false, changeIndex: null });
            this.splitRight.push({ num: rightNum, content: line, segments: null, changed: false, blank: false, changeIndex: null });
          }
          continue;
        }

        const next = parts[i + 1];
        const removedLines = part.removed ? part.lines : [];
        const addedLines = part.removed && next && next.added ? next.lines : (part.added ? part.lines : []);
        if (part.removed && next && next.added) i++;

        const pairedCount = Math.min(removedLines.length, addedLines.length);
        const pairs = [];
        for (let p = 0; p < pairedCount; p++) {
          pairs.push(wordSegments(removedLines[p], addedLines[p]));
        }

        const unifiedStart = this.unifiedDiff.length;
        for (let p = 0; p < removedLines.length; p++) {
          leftNum++;
          removed++;
          this.unifiedDiff.push({
            type: 'removed', prefix: '-', content: removedLines[p],
            segments: p < pairedCount ? pairs[p][0] : null,
            leftNum, rightNum: null,
            changeIndex: this.unifiedDiff.length === unifiedStart ? changeIndex : null,
          });
        }
        for (let p = 0; p < addedLines.length; p++) {
          rightNum++;
          added++;
          this.unifiedDiff.push({
            type: 'added', prefix: '+', content: addedLines[p],
            segments: p < pairedCount ? pairs[p][1] : null,
            leftNum: null, rightNum,
            changeIndex: this.unifiedDiff.length === unifiedStart ? changeIndex : null,
          });
        }

        const splitStart = this.splitLeft.length;
        let pairedLeft = leftNum - removedLines.length;
        let pairedRight = rightNum - addedLines.length;
        const rowCount = Math.max(removedLines.length, addedLines.length);
        for (let p = 0; p < rowCount; p++) {
          const l = p < removedLines.length ? removedLines[p] : null;
          const r = p < addedLines.length ? addedLines[p] : null;
          const anchor = this.splitLeft.length === splitStart ? changeIndex : null;
          this.splitLeft.push(l === null
            ? { num: null, content: '', segments: null, changed: false, blank: true, changeIndex: anchor }
            : { num: ++pairedLeft, content: l, segments: p < pairedCount ? pairs[p][0] : null, changed: true, blank: false, changeIndex: anchor });
          this.splitRight.push(r === null
            ? { num: null, content: '', segments: null, changed: false, blank: true, changeIndex: anchor }
            : { num: ++pairedRight, content: r, segments: p < pairedCount ? pairs[p][1] : null, changed: true, blank: false, changeIndex: anchor });
        }

        changeIndex++;
      }

      // Each view gets its own fold pass because a change occupies one split row
      // and two unified ones, so a single index-based map would fold the two at
      // different places. Marking the rows themselves keeps that honest.
      const foldRanges = collapsibleRanges(this.splitLeft);
      applyFolds(this.splitLeft, foldRanges);
      applyFolds(this.splitRight, foldRanges);
      applyFolds(this.unifiedDiff, collapsibleRanges(
        this.unifiedDiff.map((row) => ({ changed: row.type !== 'context', blank: false })),
      ));

      this.changeCount = changeIndex;
      this.stats = { added, removed };
      this.activeChange = -1;
      this.rebuildDisplay();
    },

    rebuildDisplay() {
      this.unifiedRows = foldRows(this.unifiedDiff, this.expandedFolds);
      this.splitLeftRows = foldRows(this.splitLeft, this.expandedFolds);
      this.splitRightRows = foldRows(this.splitRight, this.expandedFolds);
    },

    expandFold(id) {
      if (id && !this.expandedFolds.includes(id)) {
        this.expandedFolds.push(id);
        this.rebuildDisplay();
      }
    },

    expandAll() {
      this.expandedFolds = [...this.allFoldIds()];
      this.rebuildDisplay();
    },

    allFoldIds() {
      const ids = new Set();
      for (const row of this.splitLeft) if (row.foldId) ids.add(row.foldId);
      for (const row of this.unifiedDiff) if (row.foldId) ids.add(row.foldId);
      return ids;
    },

    get hasFolds() {
      return this.expandedFolds.length < this.allFoldIds().size;
    },

    /** Moves to the next or previous change and scrolls it into view. */
    goToChange(delta) {
      if (this.changeCount === 0) return;
      let next = this.activeChange + delta;
      if (next < 0) next = this.changeCount - 1;
      if (next >= this.changeCount) next = 0;
      this.activeChange = next;
      this.$nextTick(() => {
        const target = this.$el.querySelector(`[data-change="${this.activeChange}"]`);
        if (target) target.scrollIntoView({ block: 'center', behavior: 'auto' });
      });
    },

    /**
     * The diff as a patch, so it can leave the page. Selecting thousands of
     * rendered table rows is the only alternative.
     */
    asUnifiedText() {
      return this.unifiedDiff.map((line) => line.prefix + line.content).join('\n');
    },

    async copyPatch() {
      try {
        await navigator.clipboard.writeText(this.asUnifiedText());
        this.copied = true;
        setTimeout(() => { this.copied = false; }, 1600);
      } catch (e) {
        this.copied = false;
      }
    },

    /**
     * WAI-ARIA radiogroup keyboard pattern.
     * See imageCompare.onRadiogroupKeydown — same contract.
     */
    onRadiogroupKeydown(e, stateKey, values) {
      if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft' && e.key !== 'Home' && e.key !== 'End') {
        return;
      }
      e.preventDefault();
      const currentIdx = values.indexOf(this[stateKey]);
      let nextIdx = currentIdx;
      if (e.key === 'ArrowRight') nextIdx = (currentIdx + 1) % values.length;
      else if (e.key === 'ArrowLeft') nextIdx = (currentIdx - 1 + values.length) % values.length;
      else if (e.key === 'Home') nextIdx = 0;
      else if (e.key === 'End') nextIdx = values.length - 1;
      this[stateKey] = values[nextIdx];
      const group = e.currentTarget;
      this.$nextTick(() => {
        const checked = group.querySelector('[role="radio"][aria-checked="true"]');
        if (checked instanceof HTMLElement) checked.focus();
      });
    }
  };
}

/**
 * Splits two paired lines into `{ text, changed }` runs so only the part that
 * actually differs is marked. Returns `[null, null]` when the lines share
 * nothing, where marking every token is noisier than marking the whole line.
 */
/**
 * Runs of unchanged rows long enough to be worth folding, as `{start, length}`
 * over the hidden portion only — the context window either side stays visible.
 */
function collapsibleRanges(rows) {
  const ranges = [];
  let runStart = -1;
  for (let i = 0; i <= rows.length; i++) {
    const unchanged = i < rows.length && !rows[i].changed && !rows[i].blank;
    if (unchanged && runStart === -1) runStart = i;
    if (!unchanged && runStart !== -1) {
      const leading = runStart === 0 ? 0 : CONTEXT_LINES;
      const trailing = i === rows.length ? 0 : CONTEXT_LINES;
      const hidden = i - runStart - leading - trailing;
      if (hidden >= MIN_FOLD_LINES) {
        ranges.push({ start: runStart + leading, length: hidden });
      }
      runStart = -1;
    }
  }
  return ranges;
}

/**
 * Drops rows inside a still-collapsed run and puts one fold row in their place.
 * Doing it here rather than with per-row conditions in the template is what keeps
 * a four-thousand-line diff to one element per visible line.
 */
function foldRows(rows, expanded) {
  const out = [];
  for (const row of rows) {
    if (!row.foldId || expanded.includes(row.foldId)) {
      out.push(row);
      continue;
    }
    if (row.foldHead) {
      out.push({ fold: true, foldId: row.foldId, foldLength: row.foldLength, changeIndex: null });
    }
  }
  return out;
}

function applyFolds(rows, ranges) {
  for (const row of rows) {
    row.foldId = null;
    row.foldHead = false;
    row.foldLength = 0;
  }
  ranges.forEach((range, index) => {
    const id = `fold-${index}`;
    for (let i = range.start; i < range.start + range.length; i++) {
      if (!rows[i]) continue;
      rows[i].foldId = id;
      rows[i].foldLength = range.length;
    }
    if (rows[range.start]) rows[range.start].foldHead = true;
  });
}

export function wordSegments(left, right) {
  const parts = Diff.diffWordsWithSpace(left, right);
  const leftSegments = [];
  const rightSegments = [];
  let common = 0;

  for (const part of parts) {
    if (part.added) {
      rightSegments.push({ text: part.value, changed: true });
    } else if (part.removed) {
      leftSegments.push({ text: part.value, changed: true });
    } else {
      common += part.value.trim().length;
      leftSegments.push({ text: part.value, changed: false });
      rightSegments.push({ text: part.value, changed: false });
    }
  }

  const shorter = Math.min(left.trim().length, right.trim().length);
  if (shorter === 0 || common / shorter < 0.3) return [null, null];
  return [leftSegments, rightSegments];
}
