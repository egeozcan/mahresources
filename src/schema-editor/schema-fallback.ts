import { html, type TemplateResult } from 'lit';
import { SchemaTooComplexError, withSchemaBudget } from './schema-core';

// ─── Action boundary ─────────────────────────────────────────────────────────
// render() catches what rendering throws, but a user action (Add Item, Edit,
// a variant switch, blur validation, submit) runs later, from an event
// handler, outside that try. Every handler an entry point binds goes through
// guardSchemaAction: it runs under one work budget, and an error it throws
// (SchemaTooComplexError or any other) is logged and puts the element into
// its fallback, instead of escaping as an uncaught exception. The element
// shows the fallback until its schema or value changes
// (clearSchemaActionError in willUpdate).

interface ActionHost {
  requestUpdate(): void;
}

const actionErrors = new WeakMap<ActionHost, unknown>();

/** The error a user action on host threw, if its fallback is showing. */
export function schemaActionError(host: ActionHost): unknown {
  return actionErrors.get(host);
}

/** Leave the action fallback: host's schema or value changed. */
export function clearSchemaActionError(host: ActionHost): void {
  actionErrors.delete(host);
}

/** handler (called with host as this) under the action boundary described above. */
export function guardSchemaAction<A extends unknown[]>(
  host: ActionHost,
  element: string,
  handler: (...args: A) => unknown,
): (...args: A) => unknown {
  const fail = (error: unknown) => {
    console.error(`${element}: schema action failed`, error);
    actionErrors.set(host, error);
    host.requestUpdate();
  };
  return (...args: A) => {
    try {
      const result = withSchemaBudget(() => handler.apply(host, args));
      if (result instanceof Promise) return result.catch(fail);
      return result;
    } catch (error) {
      fail(error);
      return undefined;
    }
  };
}

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
  const raw = value === undefined ? '' : rawText(value);
  return html`
    <div class="schema-fallback text-sm" role="note">
      <p class="text-xs text-amber-800">${tooComplex
        ? 'Schema too complex to display; showing the raw value.'
        : 'Schema could not be used; showing the raw value.'}</p>
      ${raw ? html`<pre class="whitespace-pre-wrap break-all text-xs">${raw}</pre>` : ''}
    </div>
  `;
}

/** value as indented JSON, or a note when it cannot be serialised (too deep, a cycle). */
function rawText(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2) ?? String(value);
  } catch {
    return '(The raw value is too large or too deeply nested to show.)';
  }
}
