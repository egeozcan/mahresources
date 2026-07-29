/**
 * Alpine.js data component for the schema editor modal.
 * Manages open/close state, tab switching, and sync between
 * the <schema-editor> component and the MetaSchema textarea.
 */

import { getPreviewDefaultValue } from '../schema-editor/schema-core';
import { captureTrigger, restoreFocus } from '../utils/focus.js';

// Alpine.js magic properties injected at runtime
interface AlpineMagics {
  $nextTick(callback: () => void): void;
  $refs: Record<string, HTMLElement>;
  $el: HTMLElement;
}

/**
 * Compute the correct preview default value for a given schema JSON string.
 * Returns a JSON-serialized string suitable for the form preview's `value` attribute.
 */
export function getPreviewValue(schemaStr: string): string {
  try {
    const schema = JSON.parse(schemaStr);
    // BH-010: use preview-specific defaults so numeric/string fields without
    // an explicit `default` render empty (not `0`/`""`) in the Preview Form.
    const defaultVal = getPreviewDefaultValue(schema, schema);
    // JSON.stringify(undefined) returns `undefined` (not a JSON string); normalize.
    const serialized = JSON.stringify(defaultVal);
    return serialized === undefined ? '{}' : serialized;
  } catch {
    return JSON.stringify({});
  }
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
    /**
     * The control that opened the modal (WS4 finding 97). Captured at open
     * rather than looked up at close: the previous code did
     * `$el.querySelector('.visual-editor-btn')` inside closeModal, and `$el` in
     * an Alpine method is the element whose directive is evaluating — the
     * modal's own close button — not the component root. It found nothing and
     * silently returned. `$root` is no better; it walks up from the same
     * element and is undefined once that subtree is torn down. Both halves of
     * that trap are already written down in docs/lessons.md.
     */
    _trigger: null as HTMLElement | null,

    openModal(textareaId: string, event?: Event) {
      this._trigger = captureTrigger(event as Event) as HTMLElement | null;
      this._textareaEl = document.getElementById(textareaId) as HTMLTextAreaElement;
      const raw = this._textareaEl?.value || '';

      try {
        const parsed = JSON.parse(raw || '{"type":"object","properties":{}}');
        // Reject non-object JSON (primitives, arrays, null) — the visual
        // editor only makes sense for object schemas.
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          this.currentSchema = raw;
          this.rawJson = raw;
          this.rawJsonValid = false;
          this.rawJsonError = 'Schema must be a JSON object';
          this.rawJsonDirty = true;
        } else {
          this.currentSchema = JSON.stringify(parsed);
          this.rawJson = JSON.stringify(parsed, null, 2);
          this.rawJsonValid = true;
          this.rawJsonError = '';
          this.rawJsonDirty = false;
        }
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
      // x-trap on the modal (schemaEditorModal.tpl) carries .noreturn, so this
      // is the only thing deciding where focus lands. Without .noreturn the
      // trap's own returnFocus fires on a setTimeout and overwrites this with
      // whatever had focus when it activated — which openModal's $nextTick had
      // already moved into the modal.
      //
      // It has to be deferred. `this.open = false` only *schedules* Alpine's
      // teardown, so a synchronous restore lands while the modal is still
      // mounted and its focus trap is still active — the trap's focusin guard
      // then pulls focus straight back in, and the reader ends on <body> when
      // the subtree finally goes. Measured settling on BODY before this defer.
      // The trigger itself is outside the x-if, so it stays connected and this
      // is not the $refs-after-teardown trap.
      const trigger = this._trigger;
      this._trigger = null;
      (this as unknown as AlpineMagics).$nextTick(() => {
        requestAnimationFrame(() => restoreFocus(trigger));
      });
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
        const parsed = JSON.parse(this.rawJson);
        // Reject non-object JSON (primitives, arrays, null)
        if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
          this.rawJsonValid = false;
          this.rawJsonError = 'Schema must be a JSON object';
          return;
        }
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

    getPreviewValue() {
      return getPreviewValue(this.currentSchema);
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
