/**
 * BH-002: renderJsonTable(null) throws on entities with no Meta.
 *
 * templates/partials/json.tpl calls appendChild(renderJsonTable(jsonData))
 * without guarding for null. tableMaker.js:3 (renderJsonTable) falls through
 * the Array/Date/object branches on null and returns a primitive string from
 * the final ternary, which makes appendChild throw TypeError.
 *
 * Fix: renderJsonTable returns an empty DocumentFragment for null/undefined.
 * appendChild(fragment) is a no-op — no throw, no DOM pollution.
 *
 * src/main.js already exposes renderJsonTable on window for template x-init,
 * so we reuse that for the pure-function tests.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-002: renderJsonTable handles null/undefined', () => {
  test('renderJsonTable(null) returns a Node that appendChild accepts', async ({ page }) => {
    // Load any page that bundles main.js so renderJsonTable is globally available.
    await page.goto('/tags');

    const result = await page.evaluate(() => {
      const fn = (window as unknown as { renderJsonTable?: (v: unknown) => unknown })
        .renderJsonTable;
      if (typeof fn !== 'function') {
        return { error: 'window.renderJsonTable not found' };
      }

      const host = document.createElement('div');
      try {
        const out = fn(null);
        host.appendChild(out as Node); // Must NOT throw
        return {
          ok: true,
          isNode: out instanceof Node,
          isFragment: out instanceof DocumentFragment,
          childCount: host.childNodes.length,
        };
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        return { error: msg };
      }
    });

    expect(result).toEqual({ ok: true, isNode: true, isFragment: true, childCount: 0 });
  });

  test('renderJsonTable(undefined) returns a Node that appendChild accepts', async ({ page }) => {
    await page.goto('/tags');

    const result = await page.evaluate(() => {
      const fn = (window as unknown as { renderJsonTable?: (v: unknown) => unknown })
        .renderJsonTable;
      if (typeof fn !== 'function') {
        return { error: 'window.renderJsonTable not found' };
      }
      const host = document.createElement('div');
      try {
        const out = fn(undefined);
        host.appendChild(out as Node);
        return { ok: true, isFragment: out instanceof DocumentFragment };
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        return { error: msg };
      }
    });

    expect(result).toEqual({ ok: true, isFragment: true });
  });

  test('tag detail page with no Meta produces no renderJsonTable errors', async ({
    page,
    apiClient,
  }) => {
    const tagName = `BH002-tag-${Date.now()}`;
    const tag = await apiClient.createTag(tagName); // No .Meta assigned

    const consoleErrors: string[] = [];
    page.on('pageerror', (err) => consoleErrors.push(String(err)));
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });

    await page.goto(`/tag?id=${tag.ID}`);
    // Wait for the Alpine x-init in json.tpl to have executed — the metaHeader
    // renders synchronously once Alpine processes the component, which happens
    // immediately after DOMContentLoaded. We just need to yield a microtask.
    await page.locator('.metaHeader').first().waitFor({ state: 'attached' });
    // A small settle to let any async x-init/appendChild run.
    await page.waitForTimeout(250);

    const offending = consoleErrors.filter((m) =>
      /renderJsonTable|appendChild|parameter 1 is not of type 'Node'/i.test(m),
    );
    expect(offending, `unexpected errors: ${offending.join('\n')}`).toEqual([]);
  });
});
