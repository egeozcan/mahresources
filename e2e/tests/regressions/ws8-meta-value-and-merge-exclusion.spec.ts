/**
 * UI bug hunt 2026-07-29, WS8's two remaining rows: findings 143 and 94.
 *
 * **143 is REJECTED**, and this file is the control that pins the rejection.
 *
 * The report says the Meta Data sidebar "shows the property key but not its value",
 * with `{"metaSection":["META DATA\nExpand\nhunt_key\t"]}` as evidence. Reproduced on
 * a fresh instance with the report's own steps, the value *is* rendered — measured
 * 109x17 px, and legible in a screenshot. What is empty is `innerText`, because the
 * value is inside an `<expandable-text>` custom element whose shadow root renders it:
 * `td.textContent` returns "hunt value 123" and `td.innerText` returns "". The report
 * self-caveated ("the value may be inside a collapsed element") — it is not
 * collapsed, it is in a shadow root, and no `innerText` read can see it.
 *
 * So the test asserts what a sighted reader gets: a painted box with the value in it.
 * If the meta table ever really did stop rendering values, this fails.
 *
 * 94 is confirmed and fixed; the unit-level coverage of the exclusion is in
 * src/selector/excludeValues.test.ts, and this is the end-to-end half.
 */
import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';

const IMAGE = path.join(__dirname, '../../test-assets/sample-image.png');

test('finding 143 (rejected) — a resource meta value is rendered, and innerText cannot see it', async ({ page, apiClient }) => {
  const resource = await apiClient.createResource({
    name: `ws8 meta ${Date.now()}`,
    filePath: IMAGE,
  });
  const res = await page.request.post('/v1/resources/addMeta', {
    data: { ID: [resource.ID], Meta: JSON.stringify({ hunt_key: 'hunt value 123' }) },
  });
  expect(res.ok(), await res.text()).toBe(true);

  await page.goto(`/resource?id=${resource.ID}`);

  const cell = page.locator('.sidebar td', { hasText: 'hunt value 123' }).first();
  await expect(cell).toBeVisible();

  const measured = await cell.evaluate((td) => {
    const host = td.querySelector('expandable-text');
    const box = (host ?? td).getBoundingClientRect();
    return {
      width: Math.round(box.width),
      height: Math.round(box.height),
      innerText: (td as HTMLElement).innerText,
      textContent: (td.textContent || '').trim(),
      hasShadowRoot: !!host?.shadowRoot,
      shadowText: host?.shadowRoot ? (host.shadowRoot.textContent || '').trim() : '',
    };
  });

  // The value occupies real space on the page — this is the assertion that would
  // fail if the finding were real.
  expect(measured.width).toBeGreaterThan(40);
  expect(measured.height).toBeGreaterThan(8);
  expect(measured.textContent).toContain('hunt value 123');
  expect(measured.hasShadowRoot).toBe(true);
  expect(measured.shadowText).toContain('hunt value 123');
  // And this documents *why* the report saw an empty cell, so the next reader of
  // that evidence does not re-open it.
  expect(measured.innerText).toBe('');
});

test('finding 94 — a tag is not offered in its own merge picker', async ({ page, apiClient }) => {
  const name = `ws8-merge-${Date.now()}`;
  const self = await apiClient.createTag(name);
  const other = await apiClient.createTag(`${name}-other`);

  await page.goto(`/tag?id=${self.ID}`);

  const form = page.locator('form[action^="/v1/tags/merge"]');
  await expect(form).toBeVisible();
  const picker = form.locator('input[role="combobox"]').first();
  await picker.fill(name);

  // The positive control: the *other* tag, whose name shares the prefix, must still
  // be offered — otherwise "the tag is absent" would be satisfied by a picker that
  // offers nothing at all, which is exactly how this test could pass for the wrong
  // reason.
  const options = form.locator('[role="option"]');
  await expect(options.filter({ hasText: `${name}-other` })).toHaveCount(1, { timeout: 10_000 });
  await expect(options.filter({ hasText: new RegExp(`^${name}$`) })).toHaveCount(0);

  await apiClient.deleteTag(other.ID).catch(() => {});
  await apiClient.deleteTag(self.ID).catch(() => {});
});
