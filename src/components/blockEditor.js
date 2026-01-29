// src/components/blockEditor.js
const MIN_CHAR = 'a'.charCodeAt(0); // 97
const MAX_CHAR = 'z'.charCodeAt(0); // 122

export function blockEditor(noteId, initialBlocks = []) {
  return {
    noteId,
    blocks: initialBlocks,
    editMode: false,
    loading: false,
    error: null,

    async init() {
      if (this.blocks.length === 0 && this.noteId) {
        await this.loadBlocks();
      }
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
      } catch (err) {
        this.error = err.message;
        console.error('Error adding block:', err);
      }
    },

    async updateBlockContent(blockId, content) {
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
        if (idx >= 0) this.blocks[idx] = updated;
      } catch (err) {
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
        if (idx >= 0) this.blocks[idx] = updated;
      } catch (err) {
        this.error = err.message;
        console.error('Error updating block state:', err);
      }
    },

    async deleteBlock(blockId) {
      this.error = null;
      try {
        const res = await fetch(`/v1/note/block?id=${blockId}`, {
          method: 'DELETE'
        });

        if (!res.ok) {
          const errorData = await res.json().catch(() => ({}));
          throw new Error(errorData.error || `Failed to delete block: ${res.status}`);
        }
        this.blocks = this.blocks.filter(b => b.id !== blockId);
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
      const defaults = {
        text: { text: '' },
        heading: { text: '', level: 2 },
        divider: {},
        gallery: { resourceIds: [] },
        references: { groupIds: [] },
        todos: { items: [] },
        table: { columns: [], rows: [] }
      };
      return defaults[type] || {};
    },

    blockTypes: [
      { type: 'text', label: 'Text', icon: '📝' },
      { type: 'heading', label: 'Heading', icon: '🔤' },
      { type: 'divider', label: 'Divider', icon: '──' },
      { type: 'gallery', label: 'Gallery', icon: '🖼️' },
      { type: 'references', label: 'References', icon: '📁' },
      { type: 'todos', label: 'Todos', icon: '☑️' },
      { type: 'table', label: 'Table', icon: '📊' }
    ]
  };
}
