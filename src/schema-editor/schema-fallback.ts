import { html, type TemplateResult } from 'lit';
import { SchemaTooComplexError } from './schema-core';

/**
 * What a schema-driven element renders when processing its schema throws.
 * The server rejects malformed schemas on save, but rows stored before that
 * validation, or schemas a plugin supplies, can still reach the client. Each
 * entry point (display panel, form, search, tree editor, [meta]) catches at
 * its render and shows the raw value instead of a blank or broken element.
 */
export function schemaFallback(element: string, error: unknown, value?: unknown): TemplateResult {
  const tooComplex = error instanceof SchemaTooComplexError;
  console.error(`${element}: schema ${tooComplex ? 'too complex' : 'could not be used'}`, error);
  const raw = value === undefined ? '' : JSON.stringify(value, null, 2);
  return html`
    <div class="schema-fallback text-sm" role="note">
      <p class="text-xs text-amber-800">${tooComplex
        ? 'Schema too complex to display; showing the raw value.'
        : 'Schema could not be used; showing the raw value.'}</p>
      ${raw ? html`<pre class="whitespace-pre-wrap break-all text-xs">${raw}</pre>` : ''}
    </div>
  `;
}
