import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import { MRQLPage } from '../../pages/MRQLPage';

// A selectable card's component is one scope on a stack with whatever
// component its list sits in (the Job Center's list, the MRQL editor), and a
// property it assigned without declaring was written to the outermost scope:
// every card then held the last card's checkbox, and a card that left the
// list unregistered another card's selection. Each card must hold its own.
async function everyCardHoldsItsOwnCheckbox(page: Page, cards: string) {
  return page.evaluate((selector) => {
    const found = [...document.querySelectorAll(selector)];
    return {
      cards: found.length,
      nested: found.some(card => !!card.parentElement?.closest('[x-data]')),
      own: found.every(card => (window as any).Alpine.$data(card)._checkbox === card.querySelector('input[type="checkbox"]')),
    };
  }, cards);
}

test('Job Center cards, inside the list\'s own component, each hold their own checkbox', async ({ page, request }) => {
  const stamp = Date.now();
  const group = await (await request.post('/v1/group', { data: { Name: `own-checkbox-${stamp}` } })).json();
  for (const suffix of ['a', 'b', 'c']) {
    const name = `own-checkbox-${stamp}-${suffix}.bin`;
    const response = await request.post('/v1/download/submit', { data: { URL: `http://127.0.0.1:9/${name}`, OwnerId: group.ID ?? group.id, FileName: name } });
    expect(response.status(), await response.text()).toBe(202);
  }
  await page.goto(`/jobs?search=${encodeURIComponent(`own-checkbox-${stamp}`)}&dismissed=false`);
  await expect(page.locator('[data-job-id]')).toHaveCount(3);
  expect(await everyCardHoldsItsOwnCheckbox(page, '[data-job-id]')).toEqual({ cards: 3, nested: true, own: true });
});

test('MRQL result cards, inside the editor\'s component, each hold their own checkbox', async ({ page, apiClient }) => {
  const prefix = `own-checkbox-mrql-${Date.now()}`;
  const noteType = await apiClient.createNoteType(prefix);
  for (let i = 0; i < 3; i++) await apiClient.createNote({ name: `${prefix} ${i}`, noteTypeId: noteType.ID });
  const mrql = new MRQLPage(page);
  await mrql.navigate();
  await mrql.enterQuery(`type = note AND name ~ "${prefix}" ORDER BY name ASC`);
  await mrql.executeQuery();
  const cards = '[data-selection-scope="mrql-note"] [x-data^="selectableItem"]';
  await expect(page.locator(cards)).toHaveCount(3);
  expect(await everyCardHoldsItsOwnCheckbox(page, cards)).toEqual({ cards: 3, nested: true, own: true });
});
