// src/components/blockEditor.js
export function blockEditor(noteId, initialBlocks = []) {
  return {
    noteId,
    blocks: initialBlocks,
    editMode: false,
    loading: false,

    async init() {
      if (this.blocks.length === 0 && this.noteId) {
        await this.loadBlocks();
      }
    },

    async loadBlocks() {
      this.loading = true;
      try {
        const res = await fetch(`/v1/note/blocks?noteId=${this.noteId}`);
        this.blocks = await res.json();
      } finally {
        this.loading = false;
      }
    },

    toggleEditMode() {
      this.editMode = !this.editMode;
    },

    async addBlock(type, afterPosition = null) {
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

      if (res.ok) {
        await this.loadBlocks();
      }
    },

    async updateBlockContent(blockId, content) {
      const res = await fetch(`/v1/note/block?id=${blockId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content })
      });

      if (res.ok) {
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) this.blocks[idx] = updated;
      }
    },

    async updateBlockState(blockId, state) {
      const res = await fetch(`/v1/note/block/state?id=${blockId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ state })
      });

      if (res.ok) {
        const updated = await res.json();
        const idx = this.blocks.findIndex(b => b.id === blockId);
        if (idx >= 0) this.blocks[idx] = updated;
      }
    },

    async deleteBlock(blockId) {
      const res = await fetch(`/v1/note/block?id=${blockId}`, {
        method: 'DELETE'
      });

      if (res.ok) {
        this.blocks = this.blocks.filter(b => b.id !== blockId);
      }
    },

    async moveBlock(blockId, direction) {
      const idx = this.blocks.findIndex(b => b.id === blockId);
      if (idx < 0) return;

      const newIdx = direction === 'up' ? idx - 1 : idx + 1;
      if (newIdx < 0 || newIdx >= this.blocks.length) return;

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

      if (res.ok) {
        await this.loadBlocks();
      }
    },

    calculatePosition(afterPosition) {
      if (!afterPosition) {
        if (this.blocks.length === 0) return 'n';
        const last = this.blocks[this.blocks.length - 1];
        return this.positionAfter(last.position);
      }

      const idx = this.blocks.findIndex(b => b.position === afterPosition);
      if (idx < 0 || idx === this.blocks.length - 1) {
        return this.positionAfter(afterPosition);
      }

      return this.positionBetween(afterPosition, this.blocks[idx + 1].position);
    },

    positionAfter(pos) {
      const last = pos.charCodeAt(pos.length - 1);
      if (last < 122) {
        return pos.slice(0, -1) + String.fromCharCode(last + 1);
      }
      return pos + 'n';
    },

    positionBetween(a, b) {
      if (a.length === 1 && b.length === 1) {
        const mid = Math.floor((a.charCodeAt(0) + b.charCodeAt(0)) / 2);
        if (mid !== a.charCodeAt(0)) {
          return String.fromCharCode(mid);
        }
      }
      return a + 'n';
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
