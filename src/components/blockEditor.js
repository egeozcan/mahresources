// src/components/blockEditor.js
const MIN_CHAR = 'a'.charCodeAt(0); // 97
const MAX_CHAR = 'z'.charCodeAt(0); // 122

const SAVE_DEBOUNCE_MS = 500;

// A string as it reads back from an HTML attribute: the parser turns CR LF
// and lone CR into LF, and NUL into U+FFFD. renderedSource comes from one.
const asParsed = (str) => str.replace(/\r\n?/g, '\n').replace(/\0/g, '\uFFFD');

export function blockEditor(noteId, initialBlocks = []) {
  return {
    noteId,
    blocks: initialBlocks,
    editMode: false,
    addBlockPickerOpen: false, // State for add block dropdown
    activePickerIndex: 0, // Roving tabindex for add-block picker
    loading: false,
    error: null,
    _pendingUpdates: {}, // Track pending updates for optimistic UI
    _prevContent: {}, // Snapshot of pre-edit content per block for rollback on save failure
    // Per-block save queue: { timer, timerContent, chain, latest }. One PUT in
    // flight per block, and a queued save sends only the newest text. The old
    // single debounce was shared by every block, so typing in one block
    // cancelled another's pending save, and nothing stopped two PUTs for the
    // same block from landing out of order.
    _saves: {},
    // Per-block revision: bumped on every local edit and every completed save.
    // loadBlocks() compares it to the value when its GET started, so a list
    // snapshotted before a save cannot put the old text back.
    _revisions: {},
    // Bumped when an add, delete or reorder completes. A loadBlocks() GET that
    // started before one is discarded and reissued, so it cannot resurrect a
    // deleted block.
    _structuralEpoch: 0,
    _listGeneration: 0,
    // Saves and structural mutations still in flight; leaving edit mode waits
    // for all of them.
    _inFlight: new Set(),
    // Blocks whose most recent save failed; cleared by a later successful save.
    _failedSaves: new Set(),
    _leavingEditMode: false,
    // Text blocks as the server renders them for display (markdown, mentions,
    // shortcodes against the note), by block id. Read from the page on init,
    // refetched from GET /v1/note/renderedTextBlocks after the list reloads and
    // on leaving edit mode. Rendered HTML depends on the viewer, so it is never
    // stored on the block.
    renderedText: {},
    _renderGeneration: 0,
    // The message a failed render put in `error`, so a later successful render
    // clears it and nothing else.
    _renderError: null,
    // The block text each entry of renderedText was rendered from, by block
    // id. A block whose saved text differs is showing a rendering of older
    // text: it is shown inert until its new rendering arrives.
    renderedSource: {},
    _blockTypesLoaded: false,

    async init() {
      // The component root, captured once.
      //
      // UI bug hunt 2026-07-29, finding 6: `this.$el` inside an Alpine component
      // method is the element whose directive is evaluating — not the component
      // root. focusPickerItem() is invoked from @keydown on an <li role="option">,
      // so `this.$el.querySelector('#add-block-listbox')` looked for the listbox
      // *inside one of its own options*, got null, and skipped the .focus() call
      // without erroring. Measured: activePickerIndex walked 0→1→2→1→7→0 and the
      // roving tabindex/aria-selected followed it perfectly, while
      // document.activeElement never left option 0 — so a keyboard user could
      // only ever insert the first block type. The $watch below worked because a
      // watcher registered in init() evaluates with $el === the root.
      //
      // Same trap as descriptionEditor.js and adminExport.js in docs/lessons.md.
      this._root = this.$el;
      const onPage = this._root.querySelector('[data-rendered-text-blocks]');
      this.renderedText = this._readRenderedText(onPage);
      this.renderedSource = this._readRenderedSources(onPage);
      // A rendering the user has touched is theirs: a widget in it may hold a
      // draft or be saving one, by whatever means it saves (a click, a key, a
      // request still in flight). A background render never replaces it; see
      // _applyRenderedText. The mark lives on the element, which leaving edit
      // mode creates afresh.
      const touch = (event) => {
        const rendered = event.target.closest?.('[data-text-block-rendered]');
        if (rendered) rendered._touched = true;
      };
      for (const type of ['pointerdown', 'keydown', 'focusin', 'input', 'change', 'click']) {
        this._root.addEventListener(type, touch, true);
      }
      // Events inside an iframe never reach this document, and no browser
      // reliably says when focus enters or leaves one (Firefox fires no window
      // blur, frame to frame fires nothing). So a rendering that has ever held
      // a frame counts as touched, including one arriving later ([lazy]) or
      // one that has since removed itself. The rendering is found from where
      // the frame was added, not from the frame, which may already be gone.
      // _applyRenderedText also checks for a frame directly, for a render
      // landing before this observer runs.
      new MutationObserver((mutations) => {
        for (const m of mutations) {
          const added = [...m.addedNodes].some((node) => node.nodeType === 1 && (node.tagName === 'IFRAME' || node.querySelector('iframe')));
          if (!added) continue;
          const rendered = m.target.closest?.('[data-text-block-rendered]');
          if (rendered) rendered._touched = true;
        }
      }).observe(this._root, { childList: true, subtree: true });

      // Set up the JS bridge for plugin blocks BEFORE the awaits below.
      //
      // This used to sit after them, so window.mahBlock was undefined for the
      // two round-trips loadBlockTypes() and loadBlocks() take. A plugin block's
      // own script runs as soon as its markup is inserted, and the markup can be
      // on the page before either fetch resolves, so the bridge a plugin reaches
      // for first was reliably missing exactly when a plugin would use it.
      //
      // Nothing here touches the awaited state: every method closes over `self`
      // and reads this.blocks at call time.
      const self = this;
      window.mahBlock = {
        saveContent(blockId, content) {
          return self.updateBlockContent(blockId, content);
        },
        updateState(blockId, state) {
          return self.updateBlockState(blockId, state);
        },
        getBlock(blockId) {
          return self.blocks.find(b => b.id === blockId) || null;
        }
      };

      // Load block types from API if not already loaded
      if (!this._blockTypesLoaded) {
        await this.loadBlockTypes();
      }
      if (this.blocks.length === 0 && this.noteId) {
        await this.loadBlocks();
      }

      // Watch picker open state to reset index and focus first item
      this.$watch('addBlockPickerOpen', (open) => {
        if (open) {
          this.activePickerIndex = 0;
          this.$nextTick(() => this._focusActivePickerOption());
        } else {
          // Restore focus to the trigger when the picker closes (Esc, click-away
          // or after a selection) so keyboard users are not stranded at <body> —
          // but only when focus is still inside the listbox that is going away.
          // Tab no longer preventDefaults (finding 6), so the browser has already
          // moved focus onward by the time this runs, and pulling it back to the
          // trigger would make Tab appear not to work.
          this.$nextTick(() => {
            const root = this._root;
            if (!root || !this.editMode) return;
            const active = document.activeElement;
            const focusEscaped = active && active !== document.body && !root.contains(active);
            if (focusEscaped) return;
            const trigger = root.querySelector('[data-testid="add-block-trigger"]');
            if (trigger) trigger.focus();
          });
        }
      });

      // Flush pending debounced edits before the page is hidden/unloaded so a
      // fast navigation does not drop the last few seconds of typing. pagehide +
      // visibilitychange(hidden) cover tab close, back/forward, and bfcache.
      this._flushPendingUpdates = () => this.flushPendingUpdates();
      window.addEventListener('pagehide', this._flushPendingUpdates);
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') this.onPageHidden();
      });
    },

    // A hidden tab keeps running, so debounced saves go through the ordered
    // queue rather than as independent keepalive PUTs that could land after a
    // newer save.
    onPageHidden() {
      this._flushSaveTimers();
    },

    // Best-effort synchronous flush of pending optimistic edits using fetch
    // keepalive so an in-flight navigation does not abort the save. No awaiting
    // and no UI update: this only runs as the page goes away. Only an edit
    // that was never sent goes out, so this cannot resend older content; queued
    // saves for the block are superseded in case the page is restored.
    flushPendingUpdates() {
      const ids = Object.keys(this._pendingUpdates);
      for (const id of ids) {
        const content = this._pendingUpdates[id];
        const save = this._saves[id];
        if (save) {
          if (save.deleted || save.sentEditSeq >= save.editSeq) continue;
          clearTimeout(save.timer);
          save.timer = null;
          save.seq++;
          save.sentEditSeq = save.editSeq;
        }
        delete this._pendingUpdates[id];
        let sent = Promise.resolve();
        try {
          // window.fetch is CSRF-wrapped and preserves keepalive.
          sent = fetch(`/v1/note/block?id=${id}&noteId=${this.noteId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ content }),
            keepalive: true
          });
        } catch (e) {
          // Best effort on unload; nothing useful to do with the error here.
        }
        // If the page comes back from the bfcache, later saves for this block
        // queue behind this one instead of racing it, and a later failed save
        // rolls back to this content once it is known to be saved.
        if (save) {
          save.chain = Promise.resolve(sent).then((res) => {
            if (res && res.ok) this._prevContent[id] = content;
          }).catch(() => {});
        }
      }
    },

    // Move roving focus within the add-block picker listbox.
    focusPickerItem(newIndex) {
      this.activePickerIndex = newIndex;
      this.$nextTick(() => this._focusActivePickerOption());
    },

    /**
     * Put DOM focus on whichever option the roving tabindex currently marks.
     *
     * Reads `this._root` rather than `this.$el` — see the note in init(). Both the
     * open-watcher and focusPickerItem() go through here so the two paths cannot
     * drift apart again: it was the watcher working and focusPickerItem() not that
     * produced finding 6's signature, a listbox whose ARIA state was correct and
     * whose focus never moved.
     */
    _focusActivePickerOption() {
      const listbox = this._root?.querySelector('#add-block-listbox');
      if (!listbox) return;
      const option = listbox.querySelector('[role="option"][tabindex="0"]');
      if (option) option.focus();
    },

    async loadBlockTypes() {
      try {
        // Asked in the context of this note, so each entry comes back with an
        // `allowed` flag. The matching rules live on the server: a note's
        // owning-group category is not available in the browser, and
        // duplicating the rules here is what let the picker and the write path
        // disagree — a filtered type was offered on every note and errored
        // after the click.
        //
        // The full list is kept: this same list tells the editor how to render
        // the blocks the note already has, so dropping disallowed entries would
        // make an existing block vanish from a note that legitimately contains
        // it. Only the picker filters — see allowedBlockTypes.
        const url = this.noteId
          ? `/v1/note/block/types?noteId=${encodeURIComponent(this.noteId)}`
          : '/v1/note/block/types';
        const res = await fetch(url);
        if (res.ok) {
          const types = await res.json();
          // Update blockTypes with data from server
          this.blockTypes = types.map(bt => ({
            type: bt.type,
            label: bt.label || this._formatLabel(bt.type),
            icon: bt.icon || this._getIconForType(bt.type),
            description: bt.description || '',
            defaultContent: bt.defaultContent,
            plugin: bt.plugin || false,
            pluginName: bt.pluginName || null,
            filters: bt.filters || null,
            // Absent means the server was not asked about a note, which is
            // "no restriction", not "not allowed".
            allowed: bt.allowed !== false
          }));
          this._blockTypesLoaded = true;
        }
      } catch (err) {
        console.warn('Failed to load block types from API, using defaults:', err);
      }
    },

    /**
     * The block types the "+ Add Block" picker may offer for this note.
     *
     * Distinct from `blockTypes`, which stays complete because it also tells
     * the editor how to render the blocks the note already has. A type this
     * note may not *add* can still be one it legitimately *contains* — a block
     * created before its note moved to another category, say — and dropping it
     * from the list would make that block report its plugin as disabled and
     * disappear.
     */
    get allowedBlockTypes() {
      return this.blockTypes.filter(bt => bt.allowed !== false);
    },

    _formatLabel(type) {
      // Capitalize first letter
      return type.charAt(0).toUpperCase() + type.slice(1);
    },

    getBlockTypeLabel(type) {
      const blockType = this.blockTypes.find(bt => bt.type === type);
      return blockType?.label || this._formatLabel(type.split(':').pop() || type);
    },

    _getIconForType(type) {
      const icons = {
        text: '📝',
        heading: '🔤',
        divider: '──',
        gallery: '🖼️',
        references: '📁',
        todos: '☑️',
        table: '📊',
        calendar: '📅'
      };
      return icons[type] || '📦';
    },

    async loadBlocks() {
      const generation = ++this._listGeneration;
      const epoch = this._structuralEpoch;
      const revisionsAtStart = { ...this._revisions };
      this.loading = true;
      this.error = null;
      try {
        const res = await fetch(`/v1/note/blocks?noteId=${this.noteId}`);
        if (!res.ok) {
          throw new Error(`Failed to load blocks: ${res.status}`);
        }
        const loaded = await res.json();
        if (generation !== this._listGeneration) return;
        if (epoch !== this._structuralEpoch) {
          // An add, delete or reorder completed after this snapshot was taken.
          return this.loadBlocks();
        }
        this.blocks = loaded.map((block) => {
          const local = this.blocks.find((b) => b.id === block.id);
          const moved = (this._revisions[block.id] || 0) !== (revisionsAtStart[block.id] || 0);
          return local && moved ? { ...block, content: local.content } : block;
        });
        this.refreshRenderedText();
      } catch (err) {
        if (generation !== this._listGeneration) return;
        this.error = err.message;
        console.error('Error loading blocks:', err);
      } finally {
        if (generation === this._listGeneration) this.loading = false;
      }
    },

    async toggleEditMode() {
      if (!this.editMode) {
        this.editMode = true;
        return;
      }
      if (this._leavingEditMode) return;
      this._leavingEditMode = true;
      try {
        // Retry saves that failed earlier. Their error may already have been
        // cleared by another block's successful save, and refusing to leave
        // with nothing on screen would look like a dead button.
        for (const blockId of [...this._failedSaves]) {
          const content = this._pendingUpdates[blockId];
          if (content !== undefined) this.updateBlockContent(blockId, content);
        }
        // Typing can continue while this waits, so send debounced saves and
        // wait again until nothing is left either queued or in flight.
        let ok = true;
        for (;;) {
          this._flushSaveTimers();
          if (!this._inFlight.size) break;
          const results = await Promise.allSettled([...this._inFlight]);
          if (results.some((r) => r.status === 'rejected')) ok = false;
        }
        if (!ok || this._failedSaves.size) {
          // Normally the failed request's own message is showing; never
          // refuse silently.
          if (!this.error) this.error = 'Some changes could not be saved. Try again.';
          return;
        }
        this.editMode = false;
        // Every text block, not only the edited ones: editing the first one
        // changes the note's Description, which another block may display.
        this.refreshRenderedText();
      } finally {
        this._leavingEditMode = false;
      }
    },

    _readRenderedText(container) {
      const rendered = {};
      if (!container) return rendered;
      for (const tpl of container.querySelectorAll('template[data-rendered-text-block]')) {
        rendered[tpl.dataset.renderedTextBlock] = tpl.innerHTML;
      }
      return rendered;
    },

    // The block text each rendering was made from, as the server says: the
    // client's copy of a block can hold text not saved yet.
    _readRenderedSources(container) {
      const sources = {};
      if (!container) return sources;
      for (const tpl of container.querySelectorAll('template[data-rendered-text-block]')) {
        sources[tpl.dataset.renderedTextBlock] = tpl.dataset.source ?? '';
      }
      return sources;
    },

    // Until the response arrives the blocks keep their previous rendering; an
    // older response than the latest request is dropped. If rendering fails,
    // the blocks show their saved text rather than a stale rendering, and say so.
    async refreshRenderedText() {
      const generation = ++this._renderGeneration;
      try {
        const res = await fetch(`/v1/note/renderedTextBlocks?noteId=${this.noteId}`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const doc = new DOMParser().parseFromString(await res.text(), 'text/html');
        if (generation !== this._renderGeneration) return;
        this._applyRenderedText(this._readRenderedText(doc), this._readRenderedSources(doc));
        if (this.error === this._renderError) this.error = null;
        this._renderError = null;
      } catch (err) {
        if (generation !== this._renderGeneration) return;
        console.error('Error rendering text blocks:', err);
        // Every block falls back to its saved text: a stale rendering would
        // claim to show text it does not.
        this._applyRenderedText({}, {});
        this._renderError = `Could not render the text blocks (${err.message}); showing their text. Reload the page to try again.`;
        this.error = this._renderError;
      }
    },

    // Update the blocks whose rendering changed, except one the user has
    // touched. Replacing a rendering discards whatever a widget in it holds (a
    // draft, an inline [meta] editor mid-save), and comparing HTML cannot tell
    // when that is safe: [reload], [lazy] and [details] mint a fresh token on
    // every render. A touched block was painted from its current text (an
    // outdated one is inert, so it cannot be touched), so it keeps showing a
    // rendering of the right text; what it may miss is a value that changed
    // elsewhere, until its own [reload] or the next time the note leaves
    // edit mode.
    // `next` lacking a block means "no rendering": it shows its saved text.
    _applyRenderedText(next, sources) {
      const ids = new Set([...Object.keys(this.renderedText), ...Object.keys(next)]);
      for (const id of ids) {
        const html = next[id];
        if (this.renderedText[id] === html) {
          // Nothing to repaint, but it is now the rendering of this text (an
          // edit can render identically), so the block is no longer outdated.
          if (html !== undefined && id in sources) this.renderedSource[id] = sources[id];
          continue;
        }
        const el = this._renderedEl(id);
        // Focus inside or a frame counts as touched even if no event said so.
        // An outdated (inert) rendering is always replaced: nobody can be
        // using it.
        if (el && !el.inert && el.querySelector('iframe')) el._touched = true;
        if (el && !el.inert && (el._touched || el.contains(document.activeElement))) continue;
        if (html === undefined) {
          delete this.renderedText[id];
          delete this.renderedSource[id];
        } else {
          this.renderedText[id] = html;
          this.renderedSource[id] = sources[id];
        }
      }
    },

    /**
     * Show a text block's rendering in `el`: its server HTML, or its saved text
     * while it has none (not rendered yet, or the render failed). Writes only
     * when that changes, so replacing the block object (a list reload, a state
     * update) leaves a widget someone is typing in alone. Alpine markup in the
     * HTML is initialized the way x-html does it, and no <script> runs: a
     * block re-renders in place, and re-running scripts on each refresh
     * duplicated listeners and let document.write replace the page.
     */
    paintRenderedText(el, html, text, source) {
      // A rendering of text the block no longer has (edited, its new
      // rendering not here yet) is shown but cannot be used: a widget in it
      // would act on the old text, and its rendering is about to be replaced.
      const outdated = html !== undefined && source !== undefined && asParsed(source) !== asParsed(text || '');
      el.inert = outdated;
      if (outdated) el.setAttribute('aria-busy', 'true');
      else el.removeAttribute('aria-busy');
      el.classList.toggle('opacity-60', outdated);
      const painted = html === undefined ? { text: text || '' } : { html };
      if (el._painted && el._painted.html === painted.html && el._painted.text === painted.text) return;
      el._painted = painted;
      const Alpine = window.Alpine;
      Alpine.mutateDom(() => {
        for (const child of [...el.children]) Alpine.destroyTree(child);
        if (painted.html === undefined) {
          el.classList.add('whitespace-pre-wrap');
          el.textContent = painted.text;
          return;
        }
        el.classList.remove('whitespace-pre-wrap');
        el.innerHTML = painted.html;
        el._x_ignoreSelf = true;
        Alpine.initTree(el);
        delete el._x_ignoreSelf;
      });
    },

    _renderedEl(id) {
      return this._root?.querySelector(`[data-block-id="${id}"] [data-text-block-rendered]`) || null;
    },

    _track(promise) {
      this._inFlight.add(promise);
      const done = () => this._inFlight.delete(promise);
      promise.then(done, done);
      return promise;
    },

    _bumpRevision(blockId) {
      this._revisions[blockId] = (this._revisions[blockId] || 0) + 1;
    },

    // Per-block save state. `seq` counts queued saves (a queued save that is
    // no longer the newest skips sending); `editSeq` counts local edits (a
    // response is applied only if no edit was made after its content). Both
    // are counters, not `latest !== content`: this state sits behind Alpine's
    // reactive proxy, so a stored object reads back as a proxy and never
    // compares equal to the raw one.
    _saveState(blockId) {
      if (!this._saves[blockId]) {
        this._saves[blockId] = {
          timer: null, timerContent: null, timerEditSeq: 0,
          chain: Promise.resolve(), seq: 0, editSeq: 0, sentEditSeq: 0, deleted: false,
        };
      }
      return this._saves[blockId];
    },

    // Record a local edit and return its edit sequence number.
    _recordEdit(blockId, content) {
      const save = this._saveState(blockId);
      this._bumpRevision(blockId);
      this._pendingUpdates[blockId] = content;
      return ++save.editSeq;
    },

    _flushSaveTimers() {
      for (const [blockId, save] of Object.entries(this._saves)) {
        if (!save.timer || save.deleted) continue;
        clearTimeout(save.timer);
        save.timer = null;
        this._enqueueSave(Number(blockId), save.timerContent, save.timerEditSeq).catch(() => {});
      }
    },

    // Queue a PUT behind this block's in-flight one. A save superseded while
    // it waits resolves without sending; the newer one carries its text.
    _enqueueSave(blockId, content, editSeq) {
      const save = this._saveState(blockId);
      const seq = ++save.seq;
      const run = save.chain.catch(() => {}).then(() => {
        if (save.seq !== seq || save.deleted) return;
        save.sentEditSeq = Math.max(save.sentEditSeq, editSeq);
        return this._doUpdateBlockContent(blockId, content, save, editSeq);
      });
      save.chain = run;
      return this._track(run);
    },

    // Announce a status message to screen readers via the polite live region.
    // Clearing the text first guarantees that identical consecutive messages are
    // still re-announced (a polite region ignores no-op writes of the same text).
    announce(msg) {
      const liveRegion = this.$refs && this.$refs.liveRegion;
      if (!liveRegion) return;
      liveRegion.textContent = '';
      this.$nextTick(() => { liveRegion.textContent = msg; });
    },

    // Move keyboard focus to the first enabled control of a block after the list
    // is re-rendered, so keyboard users are not dropped to <body> on reorder/add.
    focusBlockControls(blockId) {
      this.$nextTick(() => {
        // this._root, not this.$el — see the note in init(). $el here is whichever
        // block control was clicked, and a control contains no [data-block-id].
        const card = this._root?.querySelector(`[data-block-id="${blockId}"]`);
        if (!card) return;
        const btn = card.querySelector('[data-block-control]:not([disabled])');
        if (btn) btn.focus();
      });
    },

    addBlock(type, afterPosition = null) {
      return this._track(this._addBlock(type, afterPosition)).catch(() => {});
    },

    async _addBlock(type, afterPosition) {
      this.error = null;
      try {
        const position = this.calculatePosition(afterPosition);
        const res = await fetch('/v1/note/block', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            noteId: this.noteId,
            type,
            position,
            content: this.getDefaultContent(type)
          })
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to add block: ${res.status}`);
        }
        this._structuralEpoch++;
        await this.loadBlocks();
        this.announce(this._formatLabel(type) + ' block added');
      } catch (err) {
        this.error = err.message;
        console.error('Error adding block:', err);
        throw err;
      }
    },

    // Debounced content update - use this for text inputs to avoid excessive API calls
    updateBlockContentDebounced(blockId, content) {
      // Optimistic update for immediate UI feedback
      const idx = this.blocks.findIndex(b => b.id === blockId);
      if (idx >= 0) {
        // Snapshot the pre-edit content once so a failed save can roll the
        // optimistic UI back to server truth instead of leaving it diverged.
        if (!Object.prototype.hasOwnProperty.call(this._prevContent, blockId)) {
          this._prevContent[blockId] = this.blocks[idx].content;
        }
        this.blocks[idx] = { ...this.blocks[idx], content };
      }
      if (this._saves[blockId]?.deleted) return;
      const editSeq = this._recordEdit(blockId, content);
      const save = this._saveState(blockId);
      clearTimeout(save.timer);
      save.timerContent = content;
      save.timerEditSeq = editSeq;
      save.timer = setTimeout(() => {
        save.timer = null;
        this._enqueueSave(blockId, content, editSeq).catch(() => {});
      }, SAVE_DEBOUNCE_MS);
    },

    // Immediate content update - use this for blur events or explicit saves.
    // Resolves true once saved, false if the save failed (the error is shown).
    updateBlockContent(blockId, content) {
      // A late caller (a calendar's resource fetch, say) for a deleted block.
      if (this._saves[blockId]?.deleted) return Promise.resolve(true);
      const editSeq = this._recordEdit(blockId, content);
      const save = this._saveState(blockId);
      clearTimeout(save.timer);
      save.timer = null;
      return this._enqueueSave(blockId, content, editSeq)
        .catch(() => {})
        .then(() => this._settled(save))
        .then(() => true, () => false);
    },

    // Wait until the newest save queued for this block has settled; reject if
    // it failed. A superseded save resolves here rather than as soon as it is
    // skipped, so a caller that reads the persisted content (a table's rows)
    // does not read it before the newest save lands.
    async _settled(save) {
      for (;;) {
        const chain = save.chain;
        try {
          await chain;
          if (chain === save.chain) return;
        } catch (err) {
          if (chain === save.chain) throw err;
        }
      }
    },

    // Internal method that performs the actual API call. Throws on failure so
    // the save queue and leaving edit mode can see it.
    async _doUpdateBlockContent(blockId, content, save, editSeq) {
      this.error = null;
      try {
        // No keepalive here: its quota (64 KiB of body in flight, shared by
        // every keepalive request on the page) rejects a large block outright.
        const res = await fetch(`/v1/note/block?id=${blockId}&noteId=${this.noteId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content })
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to update block: ${res.status}`);
        }
        const updated = await res.json();
        const newerLocalEdit = save.editSeq !== editSeq;
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0 && !newerLocalEdit) {
          // Field-scoped merge: take the server's content but preserve any
          // locally-newer state so a concurrent state PATCH is not clobbered.
          this.blocks[idx] = { ...this.blocks[idx], content: updated.content, updatedAt: updated.updatedAt };
        }
        // Clear the pending entry unless a newer edit superseded this save, so
        // the unload flush cannot re-PUT content older than what is saved.
        if (!newerLocalEdit) {
          delete this._prevContent[blockId];
          delete this._pendingUpdates[blockId];
        } else {
          // A newer edit is still unsaved; if its save fails, roll back to
          // what the server now holds, not to the content before this save.
          this._prevContent[blockId] = updated.content;
        }
        this._failedSaves.delete(blockId);
        this._bumpRevision(blockId);
        // A table or calendar whose own save failed refetches on this. Not
        // announced while a newer edit is unsaved: a refetch now would read
        // a configuration that is about to change.
        if (!newerLocalEdit) {
          window.dispatchEvent(new CustomEvent('mah:block-saved', { detail: { blockId } }));
        }
      } catch (err) {
        // The block was deleted meanwhile: there is nothing left to save to.
        if (save.deleted) return;
        // Roll back the optimistic content so the UI reflects server truth and
        // the user is not misled into thinking an unsaved edit persisted. A
        // newer local edit is left alone: its own save is still to come.
        const newerLocalEdit = save.editSeq !== editSeq;
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (!newerLocalEdit) {
          if (idx >= 0 && Object.prototype.hasOwnProperty.call(this._prevContent, blockId)) {
            this.blocks[idx] = { ...this.blocks[idx], content: this._prevContent[blockId] };
          }
          delete this._prevContent[blockId];
        }
        this._failedSaves.add(blockId);
        this.error = err.message;
        console.error('Error updating block content:', err);
        throw err;
      }
    },

    async updateBlockState(blockId, state) {
      this.error = null;
      try {
        const res = await fetch(`/v1/note/block/state?id=${blockId}&noteId=${this.noteId}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ state })
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to update block state: ${res.status}`);
        }
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) {
          // Field-scoped merge: take the server's state but preserve any
          // locally-newer content so a concurrent content PUT is not clobbered.
          this.blocks[idx] = { ...this.blocks[idx], state: updated.state, updatedAt: updated.updatedAt };
        }
      } catch (err) {
        this.error = err.message;
        console.error('Error updating block state:', err);
      }
    },

    deleteBlock(blockId) {
      return this._track(this._deleteBlock(blockId)).catch(() => {});
    },

    async _deleteBlock(blockId) {
      this.error = null;
      const removedIdx = this.blocks.findIndex(b => b.id === blockId);
      try {
        const res = await fetch(`/v1/note/block?id=${blockId}&noteId=${this.noteId}`, {
          method: 'DELETE'
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to delete block: ${res.status}`);
        }
        this._structuralEpoch++;
        this.blocks = this.blocks.filter(b => b.id !== blockId);
        // Deleting the first text block changes the note's Description, which
        // another block may display. In edit mode Done refreshes them all.
        if (!this.editMode) this.refreshRenderedText();
        // Drop any pending optimistic state so the unload flush can never re-PUT
        // a now-deleted block (which the server would reject).
        delete this._pendingUpdates[blockId];
        delete this._prevContent[blockId];
        // Kept as a tombstone rather than deleted, so a late save for this
        // block finds it and does nothing instead of recreating fresh state.
        const save = this._saveState(blockId);
        clearTimeout(save.timer);
        save.timer = null;
        save.deleted = true;
        this._failedSaves.delete(blockId);
        this.announce('Block deleted');
        // Move focus to a sensible neighbor (the block now occupying the deleted
        // slot, else the add-block trigger) so keyboard users are not stranded.
        this.$nextTick(() => {
          // this._root, not this.$el: this method is invoked from the deleted block's
          // own Delete button, so $el was that button — querySelectorAll returned an
          // empty list, the trigger lookup returned null, and focus was left on
          // <body> while this comment claimed otherwise. Measured after the finding-6
          // fix exposed the pattern.
          const root = this._root;
          if (!root) return;
          const cards = root.querySelectorAll('[data-block-id]');
          const target = cards[Math.min(removedIdx, cards.length - 1)];
          const btn = target && target.querySelector('[data-block-control]:not([disabled])');
          if (btn) { btn.focus(); return; }
          const trigger = root.querySelector('[data-testid="add-block-trigger"]');
          if (trigger) trigger.focus();
        });
      } catch (err) {
        this.error = err.message;
        console.error('Error deleting block:', err);
        throw err;
      }
    },

    moveBlock(blockId, direction) {
      return this._track(this._moveBlock(blockId, direction)).catch(() => {});
    },

    async _moveBlock(blockId, direction) {
      const idx = this.blocks.findIndex(b => b.id === blockId);
      if (idx < 0) return;

      const newIdx = direction === 'up' ? idx - 1 : idx + 1;
      if (newIdx < 0 || newIdx >= this.blocks.length) return;

      this.error = null;
      try {
        const positions = {};
        const movingBlock = this.blocks[idx];
        const targetBlock = this.blocks[newIdx];

        positions[movingBlock.id] = targetBlock.position;
        positions[targetBlock.id] = movingBlock.position;

        const res = await fetch('/v1/note/blocks/reorder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ noteId: this.noteId, positions })
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to reorder blocks: ${res.status}`);
        }
        this._structuralEpoch++;
        await this.loadBlocks();
        // Announce the destination position (not the pre-move slot) and restore
        // focus to the moved block so repeated reordering stays keyboard-operable.
        this.announce(`Block moved to position ${newIdx + 1} of ${this.blocks.length}`);
        this.focusBlockControls(blockId);
      } catch (err) {
        this.error = err.message;
        console.error('Error moving block:', err);
        throw err;
      }
    },

    calculatePosition(afterPosition) {
      if (!afterPosition) {
        if (this.blocks.length === 0) return 'n';
        const last = this.blocks[this.blocks.length - 1];
        return this.positionBetween(last.position, '');
      }

      const idx = this.blocks.findIndex(b => b.position === afterPosition);
      if (idx < 0 || idx === this.blocks.length - 1) {
        return this.positionBetween(afterPosition, '');
      }

      return this.positionBetween(afterPosition, this.blocks[idx + 1].position);
    },

    // Port of Go's lib/position.go algorithm for consistent lexicographic ordering
    positionBetween(before, after) {
      if (before === '' && after === '') {
        return 'n'; // middle of alphabet
      }
      if (before === '') {
        before = String.fromCharCode(MIN_CHAR);
      }
      if (after === '') {
        after = String.fromCharCode(MAX_CHAR + 1); // Conceptually past 'z'
      }
      return this._generateBetween(before, after);
    },

    _generateBetween(before, after) {
      const result = [];
      let i = 0;

      while (true) {
        // Get character at position i, or boundaries if past string length
        let prevChar, nextChar;
        if (i < before.length) {
          prevChar = before.charCodeAt(i);
        } else {
          prevChar = MIN_CHAR;
        }
        if (i < after.length) {
          nextChar = after.charCodeAt(i);
        } else {
          nextChar = MAX_CHAR + 1;
        }

        // Past the end of `before` but still inside `after`: we are already
        // greater than before, so we only need something < after[i:]. Pick a char
        // in [MIN_CHAR, after[i]) when there is room, else append MIN_CHAR and
        // descend. Ported from Go lib/position.go to keep JS/Go parity (without
        // this, e.g. positionBetween("a","aa") returned "aan" > "aa").
        if (i >= before.length && i < after.length) {
          const a = after.charCodeAt(i);
          if (a > MIN_CHAR) {
            let mid = Math.floor((MIN_CHAR + a) / 2);
            if (mid >= a) mid = a - 1;
            if (mid >= MIN_CHAR) {
              result.push(String.fromCharCode(mid));
              return result.join('');
            }
          }
          result.push(String.fromCharCode(MIN_CHAR));
          i++;
          continue;
        }

        // Past BOTH strings: the prefix built so far equals `after`; returning it
        // is the closest we can get for adjacent inputs (Go lib/position.go).
        if (i >= before.length && i >= after.length) {
          if (result.length === 0) return String.fromCharCode(MIN_CHAR);
          return result.join('');
        }

        if (prevChar === nextChar) {
          // Characters are equal, add to result and continue
          result.push(String.fromCharCode(prevChar));
          i++;
          continue;
        }

        // Try to find a character between prevChar and nextChar
        const midChar = Math.floor((prevChar + nextChar) / 2);
        if (midChar > prevChar && midChar < nextChar) {
          result.push(String.fromCharCode(midChar));
          return result.join('');
        }

        // No room between characters
        // Add prevChar and look for space in the next position
        result.push(String.fromCharCode(prevChar));
        i++;

        // Now find something > before[i:] and < after (conceptually 'z...')
        while (true) {
          if (i < before.length) {
            prevChar = before.charCodeAt(i);
          } else {
            prevChar = MIN_CHAR - 1; // Below 'a' conceptually
          }

          // We want something > prevChar
          if (prevChar < MAX_CHAR) {
            const midChar2 = Math.floor((prevChar + 1 + MAX_CHAR + 1) / 2);
            result.push(String.fromCharCode(midChar2));
            return result.join('');
          }

          // prevChar is 'z', we need to extend further
          result.push(String.fromCharCode(prevChar));
          i++;
        }
      }
    },

    getDefaultContent(type) {
      // First check if we have server-provided defaults
      const blockType = this.blockTypes.find(bt => bt.type === type);
      if (blockType && blockType.defaultContent) {
        return blockType.defaultContent;
      }
      // Fallback to hardcoded defaults if API hasn't loaded yet
      const fallbackDefaults = {
        text: { text: '' },
        heading: { text: '', level: 2 },
        divider: {},
        gallery: { resourceIds: [] },
        references: { groupIds: [] },
        todos: { items: [] },
        table: { columns: [], rows: [] }
      };
      return fallbackDefaults[type] || {};
    },

    // Default block types (will be replaced by API response)
    blockTypes: [
      { type: 'text', label: 'Text', icon: '📝' },
      { type: 'heading', label: 'Heading', icon: '🔤' },
      { type: 'divider', label: 'Divider', icon: '──' },
      { type: 'gallery', label: 'Gallery', icon: '🖼️' },
      { type: 'references', label: 'References', icon: '📁' },
      { type: 'todos', label: 'Todos', icon: '☑️' },
      { type: 'table', label: 'Table', icon: '📊' },
      { type: 'calendar', label: 'Calendar', icon: '📅' }
    ]
  };
}
