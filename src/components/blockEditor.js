// src/components/blockEditor.js
const MIN_CHAR = 'a'.charCodeAt(0); // 97
const MAX_CHAR = 'z'.charCodeAt(0); // 122

// Simple debounce utility
function debounce(fn, delay) {
  let timeoutId;
  return function (...args) {
    clearTimeout(timeoutId);
    timeoutId = setTimeout(() => fn.apply(this, args), delay);
  };
}

export function blockEditor(noteId, initialBlocks = []) {
  // Create debounced update function outside the return object
  const debouncedUpdateFn = debounce(async function (blockId, content) {
    await this._doUpdateBlockContent(blockId, content);
  }, 500);

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
    _blockTypesLoaded: false,

    // Simple markdown-like rendering: escapes HTML, converts newlines to <br>, and handles basic formatting
    renderMarkdown(text) {
      if (!text) return '';
      // Escape HTML entities
      let escaped = text
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
      // Convert newlines to <br>
      escaped = escaped.replace(/\n/g, '<br>');

      // BH-021: Inline code: `text` -> <code>text</code>
      // Run BEFORE other inline tokens so inline-code content is immune to
      // subsequent bold/italic/strike passes (standard GFM-ish behavior).
      // Stash each <code> span behind a Unicode PUA sentinel so the raw
      // inner text never meets the bold/italic/strike regexes below.
      const codeSlots = [];
      escaped = escaped.replace(/`([^`]+)`/g, (_m, inner) => {
        const slot = 'CODE' + codeSlots.length + '';
        codeSlots.push('<code>' + inner + '</code>');
        return slot;
      });

      // Basic bold: **text** -> <strong>text</strong>
      escaped = escaped.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
      // Basic italic (asterisk form): *text* -> <em>text</em>
      escaped = escaped.replace(/\*([^*]+)\*/g, '<em>$1</em>');

      // BH-021: Italic (underscore form): _text_ -> <em>text</em>
      // Boundary rule: the underscore must not be adjacent to a word char
      // (A-Z a-z 0-9 _), so `snake_case_names` survive untouched. Inner
      // text must not contain underscores or newlines (keeps the regex
      // greedy-safe and avoids eating across lines).
      escaped = escaped.replace(
        /(^|[^A-Za-z0-9_])_([^_\n]+)_(?=$|[^A-Za-z0-9_])/g,
        '$1<em>$2</em>'
      );

      // BH-021: Strikethrough: ~~text~~ -> <s>text</s>
      escaped = escaped.replace(/~~([^~\n]+)~~/g, '<s>$1</s>');

      // Basic links: [text](url) -> <a href="url">text</a>
      escaped = escaped.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, text, href) => {
          // A denylist on the raw string (.trim().startsWith('javascript:')) is
          // bypassable: browsers strip whitespace/control chars embedded in a URL
          // scheme, so "java\tscript:" resolves to "javascript:". Strip all
          // whitespace, ASCII control chars, AND Unicode format/zero-width chars
          // (\p{Cf}: U+200B, U+00AD, ... which JS \s does NOT cover) first, then
          // ALLOWLIST safe schemes (or relative URLs) rather than denylisting.
          const STRIP = /[\s\x00-\x1f]|\p{Cf}/gu;
          const cleaned = href.replace(STRIP, '').toLowerCase();
          const hasScheme = /^[a-z][a-z0-9+.\-]*:/.test(cleaned);
          const safe = !hasScheme
              || cleaned.startsWith('http:')
              || cleaned.startsWith('https:')
              || cleaned.startsWith('mailto:')
              || cleaned.startsWith('tel:');
          if (!safe) {
              return text;
          }
          // Defang control chars that survived in the original href so they
          // cannot reconstitute a dangerous scheme in the attribute. Only ASCII
          // control chars are browser-stripped from schemes; we deliberately keep
          // regular whitespace here so legitimate URLs aren't mangled. (Quotes are
          // already HTML-escaped above, blocking attribute breakout.)
          const safeHref = href.replace(/[\x00-\x1f]/g, '');
          return `<a href="${safeHref}" class="text-blue-600 hover:underline" target="_blank" rel="noopener">${text}</a>`;
      });

      // Restore inline-code slots last so their contents never see earlier passes.
      escaped = escaped.replace(/CODE(\d+)/g, (_m, i) => codeSlots[Number(i)]);
      return escaped;
    },


    async init() {
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
          this.$nextTick(() => {
            const listbox = this.$el.querySelector('#add-block-listbox');
            if (listbox) {
              const first = listbox.querySelector('[role="option"][tabindex="0"]');
              if (first) first.focus();
            }
          });
        } else {
          // Restore focus to the trigger when the picker closes (Esc, click-away,
          // Tab, or after a selection) so keyboard users are not stranded at <body>.
          this.$nextTick(() => {
            const trigger = this.$el.querySelector('[data-testid="add-block-trigger"]');
            if (trigger && this.editMode) trigger.focus();
          });
        }
      });

      // Set up JS bridge for plugin blocks
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

      // Flush pending debounced edits before the page is hidden/unloaded so a
      // fast navigation does not drop the last few seconds of typing. pagehide +
      // visibilitychange(hidden) cover tab close, back/forward, and bfcache.
      this._flushPendingUpdates = () => this.flushPendingUpdates();
      window.addEventListener('pagehide', this._flushPendingUpdates);
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') this.flushPendingUpdates();
      });
    },

    // Best-effort synchronous flush of pending optimistic edits using fetch
    // keepalive so an in-flight navigation does not abort the save. No awaiting
    // and no UI update: this only runs as the page goes away.
    flushPendingUpdates() {
      const ids = Object.keys(this._pendingUpdates);
      for (const id of ids) {
        const content = this._pendingUpdates[id];
        delete this._pendingUpdates[id];
        try {
          // window.fetch is CSRF-wrapped and preserves keepalive.
          fetch(`/v1/note/block?id=${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ content }),
            keepalive: true
          });
        } catch (e) {
          // Best effort on unload; nothing useful to do with the error here.
        }
      }
    },

    // Move roving focus within the add-block picker listbox
    focusPickerItem(newIndex) {
      this.activePickerIndex = newIndex;
      this.$nextTick(() => {
        const listbox = this.$el.querySelector('#add-block-listbox');
        if (listbox) {
          const active = listbox.querySelector('[role="option"][tabindex="0"]');
          if (active) active.focus();
        }
      });
    },

    async loadBlockTypes() {
      try {
        const res = await fetch('/v1/note/block/types');
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
            filters: bt.filters || null
          }));
          this._blockTypesLoaded = true;
        }
      } catch (err) {
        console.warn('Failed to load block types from API, using defaults:', err);
      }
    },

    _formatLabel(type) {
      // Capitalize first letter
      return type.charAt(0).toUpperCase() + type.slice(1);
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
      this.loading = true;
      this.error = null;
      try {
        const res = await fetch(`/v1/note/blocks?noteId=${this.noteId}`);
        if (!res.ok) {
          throw new Error(`Failed to load blocks: ${res.status}`);
        }
        this.blocks = await res.json();
      } catch (err) {
        this.error = err.message;
        console.error('Error loading blocks:', err);
      } finally {
        this.loading = false;
      }
    },

    toggleEditMode() {
      this.editMode = !this.editMode;
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
        const card = this.$el.querySelector(`[data-block-id="${blockId}"]`);
        if (!card) return;
        const btn = card.querySelector('[data-block-control]:not([disabled])');
        if (btn) btn.focus();
      });
    },

    async addBlock(type, afterPosition = null) {
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
        await this.loadBlocks();
        this.announce(this._formatLabel(type) + ' block added');
      } catch (err) {
        this.error = err.message;
        console.error('Error adding block:', err);
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
      this._pendingUpdates[blockId] = content;
      debouncedUpdateFn.call(this, blockId, content);
    },

    // Immediate content update - use this for blur events or explicit saves
    async updateBlockContent(blockId, content) {
      // Cancel any pending debounced update for this block
      delete this._pendingUpdates[blockId];
      await this._doUpdateBlockContent(blockId, content);
    },

    // Internal method that performs the actual API call
    async _doUpdateBlockContent(blockId, content) {
      this.error = null;
      try {
        const res = await fetch(`/v1/note/block?id=${blockId}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content })
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to update block: ${res.status}`);
        }
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) {
          // Field-scoped merge: take the server's content but preserve any
          // locally-newer state so a concurrent state PATCH is not clobbered.
          this.blocks[idx] = { ...this.blocks[idx], content: updated.content, updatedAt: updated.updatedAt };
        }
        delete this._prevContent[blockId];
        // Clear the pending entry unless a newer edit superseded this save, so
        // the unload flush doesn't redundantly re-PUT already-persisted content.
        if (this._pendingUpdates[blockId] === content) delete this._pendingUpdates[blockId];
      } catch (err) {
        // Roll back the optimistic content so the UI reflects server truth and
        // the user is not misled into thinking an unsaved edit persisted.
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0 && Object.prototype.hasOwnProperty.call(this._prevContent, blockId)) {
          this.blocks[idx] = { ...this.blocks[idx], content: this._prevContent[blockId] };
        }
        delete this._prevContent[blockId];
        this.error = err.message;
        console.error('Error updating block content:', err);
      }
    },

    async updateBlockState(blockId, state) {
      this.error = null;
      try {
        const res = await fetch(`/v1/note/block/state?id=${blockId}`, {
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

    async deleteBlock(blockId) {
      this.error = null;
      const removedIdx = this.blocks.findIndex(b => b.id === blockId);
      try {
        const res = await fetch(`/v1/note/block?id=${blockId}`, {
          method: 'DELETE'
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to delete block: ${res.status}`);
        }
        this.blocks = this.blocks.filter(b => b.id !== blockId);
        // Drop any pending optimistic state so the unload flush can never re-PUT
        // a now-deleted block (which the server would reject).
        delete this._pendingUpdates[blockId];
        delete this._prevContent[blockId];
        this.announce('Block deleted');
        // Move focus to a sensible neighbor (the block now occupying the deleted
        // slot, else the add-block trigger) so keyboard users are not stranded.
        this.$nextTick(() => {
          const cards = this.$el.querySelectorAll('[data-block-id]');
          const target = cards[Math.min(removedIdx, cards.length - 1)];
          const btn = target && target.querySelector('[data-block-control]:not([disabled])');
          if (btn) { btn.focus(); return; }
          const trigger = this.$el.querySelector('[data-testid="add-block-trigger"]');
          if (trigger) trigger.focus();
        });
      } catch (err) {
        this.error = err.message;
        console.error('Error deleting block:', err);
      }
    },

    async moveBlock(blockId, direction) {
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
        await this.loadBlocks();
        // Announce the destination position (not the pre-move slot) and restore
        // focus to the moved block so repeated reordering stays keyboard-operable.
        this.announce(`Block moved to position ${newIdx + 1} of ${this.blocks.length}`);
        this.focusBlockControls(blockId);
      } catch (err) {
        this.error = err.message;
        console.error('Error moving block:', err);
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
