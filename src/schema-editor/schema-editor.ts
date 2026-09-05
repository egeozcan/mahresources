import { LitElement, html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { sharedStyles } from './styles';
import type { JSONSchema } from './schema-core';
import './modes/edit-mode';
import './modes/form-mode';
import './modes/search-mode';
import './modes/display-mode';

@customElement('schema-editor')
export class SchemaEditor extends LitElement {
  static override styles = [
    sharedStyles,
    css`
      :host {
        display: block;
      }
    `,
  ];

  /**
   * Form and search modes need light DOM so that hidden inputs are visible
   * to the parent `<form>` element and Tailwind styles are inherited.
   * Edit mode uses shadow DOM for style isolation inside the modal.
   *
   * Note: `createRenderRoot` is called once during first render and cached.
   * In practice each `<schema-editor>` element has a fixed `mode` attribute
   * set in the template, so this works correctly.
   */
  override createRenderRoot() {
    if (this.mode === 'edit') {
      return super.createRenderRoot();
    }
    return this;
  }

  @property({ type: String }) mode: 'edit' | 'form' | 'search' | 'display' = 'edit';
  @property({ type: String }) schema = '';
  @property({ type: String }) value = '';
  @property({ type: String }) name = 'Meta';
  /**
   * What is being displayed. `mode="display"` is the metadata panel on every
   * detail page, so a plugin display renderer reached from here is rendering a
   * real, identified entity — it needs to know which one to link back to it or
   * fetch a related record. Absent (the schema editor's own preview) means
   * "bound to nothing", and the renderer is told so.
   */
  @property({ type: String, attribute: 'entity-type' }) entityType = '';
  @property({ type: Number, attribute: 'entity-id' }) entityId = 0;
  @property({ type: String, attribute: 'meta-query' }) metaQuery = '';
  @property({ type: String, attribute: 'field-name' }) fieldName = 'MetaQuery';

  @state() private _parsedSchema: JSONSchema | null = null;
  // Memoized for the same reason _parsedSchema is, and load-bearing now that
  // the display child invalidates on `value` identity: parsing in render()
  // produced a fresh object every render, so every parent re-render looked like
  // a new value and would drop the plugin node cache that fb2a6f19 added.
  @state() private _parsedValue: unknown = {};

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('schema')) {
      this._parseSchema();
    }
    if (changed.has('value')) {
      this._parseValue();
    }
  }

  private _parseValue() {
    if (!this.value) {
      this._parsedValue = {};
      return;
    }
    if (typeof this.value === 'object') {
      this._parsedValue = this.value;
      return;
    }
    try {
      this._parsedValue = JSON.parse(this.value);
    } catch {
      this._parsedValue = {};
    }
  }

  /**
   * Alpine morph patched this element's attributes. Its children were skipped
   * (SCHEMA-EDITOR is client-owned), so the rendered subtree survives -- but the
   * plugin markup inside it was fetched for whatever entity the old attributes
   * named, so the display child has to drop it.
   *
   * Awaited on updateComplete rather than deferred by a microtask. The attribute
   * patch may have queued a Lit update of our own, and the display child we want
   * is rendered by it; a single microtask is not a guarantee that update has
   * run, whereas updateComplete is exactly that promise.
   */
  refreshFromMorph(_toEl?: Element) {
    void this.updateComplete.then(() => {
      const display = this.querySelector('schema-display-mode') as
        | (Element & { refreshFromMorph?: () => void })
        | null;
      display?.refreshFromMorph?.();
    });
  }

  private _parseSchema() {
    if (!this.schema) {
      this._parsedSchema = null;
      return;
    }
    // When Alpine binds with :schema="currentSchema", the value may arrive
    // as an already-parsed object (not a JSON string).  Handle both cases.
    // Reject non-plain-objects (arrays, primitives, null) — the visual editor
    // only makes sense for object schemas.
    if (typeof this.schema === 'object') {
      if (Array.isArray(this.schema) || this.schema === null) {
        this._parsedSchema = null;
        return;
      }
      this._parsedSchema = this.schema as any;
      return;
    }
    try {
      const parsed = JSON.parse(this.schema);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        this._parsedSchema = null;
        return;
      }
      this._parsedSchema = parsed;
    } catch {
      this._parsedSchema = null;
    }
  }

  // ─── Public API ──────────────────────────────────────────────────────────

  getSchema(): string {
    return this.schema;
  }

  getValue(): object {
    if (!this.value) return {};
    try {
      return JSON.parse(this.value);
    } catch {
      return {};
    }
  }

  validate(): boolean {
    // Placeholder — will be implemented in form-mode
    return true;
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  override render() {
    if (!this._parsedSchema) {
      return html`<slot></slot>`;
    }

    switch (this.mode) {
      case 'edit':
        return html`<schema-edit-mode .schema=${this._parsedSchema} @schema-change=${(e: CustomEvent) => {
          this.schema = e.detail.schema;
          this.dispatchEvent(new CustomEvent('schema-change', { detail: e.detail, bubbles: true, composed: true }));
        }}></schema-edit-mode>`;
      case 'form': {
        // Form mode accepts serialized JSON (or an already-parsed object).
        // Let it decode once: pre-parsing here makes scalar strings such as
        // "high" look like malformed JSON and coerces them to an empty object.
        return html`<schema-form-mode
          .schema=${this._parsedSchema}
          .value=${this.value}
          .name=${this.name}
          @value-change=${(e: CustomEvent) => {
            this.value = JSON.stringify(e.detail.value);
            this.dispatchEvent(new CustomEvent('value-change', { detail: e.detail, bubbles: true, composed: true }));
          }}
        ></schema-form-mode>`;
      }
      case 'search':
        return html`<schema-search-mode
          .schema=${this.schema}
          .metaQuery=${this.metaQuery}
          .fieldName=${this.fieldName}
        ></schema-search-mode>`;
      case 'display': {
        return html`<schema-display-mode
          .schema=${this._parsedSchema}
          .value=${this._parsedValue}
          .name=${this.name}
          .entityType=${this.entityType}
          .entityId=${this.entityId}
        ></schema-display-mode>`;
      }
      default:
        return nothing;
    }
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'schema-editor': SchemaEditor;
  }
}
