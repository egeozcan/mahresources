import { LitElement, html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { sharedStyles } from '../../styles';
import type { JSONSchema } from '../../schema-core';

@customElement('schema-conditional-editor')
export class SchemaConditionalEditor extends LitElement {
  static override styles = [sharedStyles, css`
    .slot { padding: 12px; border: 1px solid #e5e7eb; border-radius: 4px; margin-bottom: 8px; }
    .slot-label { font-size: 11px; font-weight: 600; color: #6b7280; margin-bottom: 4px; }
    .slot-content { font-size: 12px; color: #374151; font-family: monospace; white-space: pre-wrap; max-height: 100px; overflow-y: auto; }
  `];

  @property({ type: Object }) schema: JSONSchema = {};

  override render() {
    return html`
      <div class="type-section">
        <h4>Conditional (if / then / else)</h4>
        <div class="slot">
          <div class="slot-label">if</div>
          <div class="slot-content">${JSON.stringify(this.schema.if, null, 2)}</div>
        </div>
        ${this.schema.then ? html`
          <div class="slot">
            <div class="slot-label">then</div>
            <div class="slot-content">${JSON.stringify(this.schema.then, null, 2)}</div>
          </div>
        ` : ''}
        ${this.schema.else ? html`
          <div class="slot">
            <div class="slot-label">else</div>
            <div class="slot-content">${JSON.stringify(this.schema.else, null, 2)}</div>
          </div>
        ` : ''}
        <p style="font-size:11px;color:#9ca3af;margin-top:8px;">Edit conditional schemas via the Raw JSON tab for full control.</p>
      </div>
    `;
  }
}
