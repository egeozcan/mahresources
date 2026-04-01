/**
 * Alpine.js data component for the schema editor modal.
 * Manages open/close state, tab switching, and sync between
 * the <schema-editor> component and the MetaSchema textarea.
 */
export function schemaEditorModal() {
  return {
    open: false,
    tab: 'edit' as 'edit' | 'preview' | 'raw',
    rawJson: '',
    currentSchema: '',
    /** The textarea element this modal reads/writes to */
    _textareaEl: null as HTMLTextAreaElement | null,

    openModal(textareaId: string) {
      this._textareaEl = document.getElementById(textareaId) as HTMLTextAreaElement;
      this.currentSchema = this._textareaEl?.value || '{"type":"object","properties":{}}';
      this.rawJson = this.currentSchema;
      try {
        // Pretty-print for raw tab
        this.rawJson = JSON.stringify(JSON.parse(this.currentSchema), null, 2);
      } catch { /* keep as-is */ }
      this.tab = 'edit';
      this.open = true;
      // Trap focus after render
      this.$nextTick(() => {
        const modal = this.$refs.modalContent as HTMLElement;
        modal?.querySelector<HTMLElement>('[autofocus], button, input, select')?.focus();
      });
    },

    closeModal() {
      this.open = false;
      // Return focus to trigger button
      this._textareaEl?.closest('.meta-schema-field')?.querySelector<HTMLElement>('.visual-editor-btn')?.focus();
    },

    handleSchemaChange(e: CustomEvent) {
      this.currentSchema = e.detail.schema;
      try {
        this.rawJson = JSON.stringify(JSON.parse(this.currentSchema), null, 2);
      } catch {
        this.rawJson = this.currentSchema;
      }
    },

    handleRawChange() {
      try {
        JSON.parse(this.rawJson);
        this.currentSchema = this.rawJson;
      } catch { /* invalid JSON — don't sync */ }
    },

    applySchema() {
      if (this._textareaEl) {
        // Minify for storage
        try {
          this._textareaEl.value = JSON.stringify(JSON.parse(this.currentSchema));
        } catch {
          this._textareaEl.value = this.currentSchema;
        }
        // Trigger input event for any watchers
        this._textareaEl.dispatchEvent(new Event('input', { bubbles: true }));
      }
      this.closeModal();
    },

    handleKeydown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        this.closeModal();
      }
    },

    getPropertyCount() {
      try {
        const schema = JSON.parse(this.currentSchema);
        const props = schema.properties ? Object.keys(schema.properties).length : 0;
        const req = schema.required ? schema.required.length : 0;
        return `${props} propert${props !== 1 ? 'ies' : 'y'} · ${req} required`;
      } catch { return ''; }
    },
  };
}
