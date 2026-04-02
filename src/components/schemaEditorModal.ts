/**
 * Alpine.js data component for the schema editor modal.
 * Manages open/close state, tab switching, and sync between
 * the <schema-editor> component and the MetaSchema textarea.
 */

// Alpine.js magic properties injected at runtime
interface AlpineMagics {
  $nextTick(callback: () => void): void;
  $refs: Record<string, HTMLElement>;
  $el: HTMLElement;
}

export function schemaEditorModal() {
  return {
    open: false,
    tab: 'edit' as 'edit' | 'preview' | 'raw',
    rawJson: '',
    rawJsonValid: true,
    rawJsonError: '',
    rawJsonDirty: false,
    currentSchema: '',
    /** The textarea element this modal reads/writes to */
    _textareaEl: null as HTMLTextAreaElement | null,

    openModal(textareaId: string) {
      this._textareaEl = document.getElementById(textareaId) as HTMLTextAreaElement;
      const raw = this._textareaEl?.value || '';

      try {
        const parsed = JSON.parse(raw || '{"type":"object","properties":{}}');
        this.currentSchema = JSON.stringify(parsed);
        this.rawJson = JSON.stringify(parsed, null, 2);
        this.rawJsonValid = true;
        this.rawJsonError = '';
        this.rawJsonDirty = false;
      } catch (e) {
        // Content is not valid JSON -- show it as-is but mark invalid
        this.currentSchema = raw;
        this.rawJson = raw;
        this.rawJsonValid = false;
        this.rawJsonError = e instanceof Error ? e.message : 'Invalid JSON';
        // Mark dirty so Apply is disabled (Apply requires rawJsonValid || !rawJsonDirty)
        this.rawJsonDirty = true;
      }

      this.tab = 'edit';
      this.open = true;
      // Trap focus after render
      (this as unknown as AlpineMagics).$nextTick(() => {
        const modal = (this as unknown as AlpineMagics).$refs.modalContent as HTMLElement;
        modal?.querySelector<HTMLElement>('[autofocus], button, input, select')?.focus();
      });
    },

    closeModal() {
      this.open = false;
      // Return focus to trigger button (it lives in the same x-data root element)
      (this as unknown as AlpineMagics).$el.querySelector<HTMLElement>('.visual-editor-btn')?.focus();
    },

    handleSchemaChange(e: CustomEvent) {
      this.currentSchema = e.detail.schema;
      try {
        this.rawJson = JSON.stringify(JSON.parse(this.currentSchema), null, 2);
      } catch {
        this.rawJson = this.currentSchema;
      }
      // Visual edit overrides raw — clear dirty state
      this.rawJsonDirty = false;
      this.rawJsonValid = true;
      this.rawJsonError = '';
    },

    handleRawChange() {
      this.rawJsonDirty = true;
      try {
        JSON.parse(this.rawJson);
        this.rawJsonValid = true;
        this.rawJsonError = '';
        this.currentSchema = this.rawJson;
        this.rawJsonDirty = false; // Successfully synced
      } catch (e: any) {
        this.rawJsonValid = false;
        this.rawJsonError = e instanceof Error ? e.message : 'Invalid JSON';
        // Don't update currentSchema — keep last valid
      }
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

    handleTabKeydown(e: KeyboardEvent) {
      const tabs = ['edit', 'preview', 'raw'];
      const idx = tabs.indexOf(this.tab);
      if (e.key === 'ArrowRight') {
        e.preventDefault();
        this.tab = tabs[(idx + 1) % tabs.length] as 'edit' | 'preview' | 'raw';
        (this as unknown as AlpineMagics).$nextTick(() => (e.target as HTMLElement).closest('[role="tablist"]')?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus());
      }
      if (e.key === 'ArrowLeft') {
        e.preventDefault();
        this.tab = tabs[(idx - 1 + tabs.length) % tabs.length] as 'edit' | 'preview' | 'raw';
        (this as unknown as AlpineMagics).$nextTick(() => (e.target as HTMLElement).closest('[role="tablist"]')?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus());
      }
      if (e.key === 'Home') {
        e.preventDefault();
        this.tab = 'edit';
        (this as unknown as AlpineMagics).$nextTick(() => (e.target as HTMLElement).closest('[role="tablist"]')?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus());
      }
      if (e.key === 'End') {
        e.preventDefault();
        this.tab = 'raw';
        (this as unknown as AlpineMagics).$nextTick(() => (e.target as HTMLElement).closest('[role="tablist"]')?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus());
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
