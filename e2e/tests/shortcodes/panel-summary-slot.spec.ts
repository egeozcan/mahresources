/**
 * E2E tests for CustomPanelSummary: on a card shown inside a detail page's
 * entity panel (a group's Own / Related Entities and Relations, a note's
 * Groups / Resources, a resource's Notes / Groups / Series, a series' Resources,
 * and the tag / category / note type / resource category pages) it replaces
 * CustomSummary. Every other card surface keeps CustomSummary, and an empty
 * panel slot falls back to it.
 *
 * Each assertion is scoped to one entity's card inside one named panel: the
 * card also carries its entity as JSON in a data attribute, so the raw slot
 * text is in the HTML whichever slot rendered.
 */
import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';
import path from 'path';
import { MRQLPage } from '../../pages/MRQLPage';

const IMAGE = (n: number) => path.join(__dirname, `../../test-assets/sample-image-${n}.png`);

const SUMMARY = '<span data-probe="summary">S:[property path="Name"]</span>';
const PANEL = '<span data-probe="panel">P:[property path="Name"]</span>';

function panel(page: Page, title: string): Locator {
  return page.locator('.detail-panel').filter({
    // A title may end in a count badge: "Resources<span>2</span>".
    has: page.locator('.detail-panel-title', { hasText: new RegExp(`^\\s*${title}\\s*\\d*\\s*$`) }),
  });
}

function card(scope: Locator, name: string): Locator {
  return scope.locator('article.card').filter({
    has: scope.page().locator('.card-title a', { hasText: new RegExp(`^${name}$`) }),
  });
}

async function expectPanelSlot(c: Locator, name: string) {
  await expect(c).toHaveCount(1);
  await expect(c.locator('[data-probe="panel"]')).toHaveText(`P:${name}`);
  await expect(c.locator('[data-probe="summary"]')).toHaveCount(0);
}

async function expectSummarySlot(c: Locator, name: string) {
  await expect(c).toHaveCount(1);
  await expect(c.locator('[data-probe="summary"]')).toHaveText(`S:${name}`);
  await expect(c.locator('[data-probe="panel"]')).toHaveCount(0);
}

async function open(page: Page, url: string) {
  await page.goto(url);
  await page.waitForLoadState('load');
}

test.describe('Panel summary slot', () => {
  test('group cards: panels on detail pages use it, every other surface keeps the summary', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cssMarker = `pps-css-${stamp}`;
    const plain = await apiClient.createCategory(`PS Plain ${stamp}`);
    const cat = await apiClient.createCategory(`PS Cat ${stamp}`, 'panel summary', {
      CustomSummary: SUMMARY,
      CustomPanelSummary: PANEL,
      CustomPanelSummaryCSS: `.${cssMarker} { color: rgb(1, 2, 3); }`,
    });
    const fallbackCat = await apiClient.createCategory(`PS Fallback ${stamp}`, 'summary only', {
      CustomSummary: SUMMARY,
    });
    const tag = await apiClient.createTag(`PS Tag ${stamp}`);

    const relatedName = `PS Related ${stamp}`;
    const related = await apiClient.createGroup({ name: relatedName, categoryId: cat.ID });
    const parent = await apiClient.createGroup({ name: `PS Parent ${stamp}`, categoryId: plain.ID, groups: [related.ID] });
    const childName = `PS Child ${stamp}`;
    const child = await apiClient.createGroup({ name: childName, categoryId: cat.ID, ownerId: parent.ID, tags: [tag.ID] });
    const fallbackName = `PS FallbackGroup ${stamp}`;
    await apiClient.createGroup({ name: fallbackName, categoryId: fallbackCat.ID, ownerId: parent.ID });

    const outbound = await apiClient.createRelationType({ name: `PS Out ${stamp}`, fromCategoryId: plain.ID, toCategoryId: cat.ID });
    const inbound = await apiClient.createRelationType({ name: `PS In ${stamp}`, fromCategoryId: cat.ID, toCategoryId: plain.ID });
    const forward = await apiClient.createRelation({ fromGroupId: parent.ID, toGroupId: child.ID, relationTypeId: outbound.ID });
    await apiClient.createRelation({ fromGroupId: related.ID, toGroupId: parent.ID, relationTypeId: inbound.ID });

    const note = await apiClient.createNote({ name: `PS Note ${stamp}`, ownerId: child.ID, groups: [child.ID] });

    // The parent's detail page: every panel shows the panel slot, bound to the
    // card's own group, and the empty-slot category falls back to the summary.
    await open(page, `/group?id=${parent.ID}`);
    await expectPanelSlot(card(panel(page, 'Sub-Groups'), childName), childName);
    await expectPanelSlot(card(panel(page, 'Related Groups'), relatedName), relatedName);
    await expectPanelSlot(card(panel(page, 'Relations'), childName), childName);
    await expectPanelSlot(card(panel(page, 'Reverse Relations'), relatedName), relatedName);
    await expectSummarySlot(card(panel(page, 'Sub-Groups'), fallbackName), fallbackName);
    // The companion CSS reaches a page that shows the card only inside a panel.
    // Read textContent: Playwright's text matching skips <style> contents.
    const css = page.locator(`style[data-mr-custom-css="group:${cat.ID}"]`);
    await expect(css).toHaveCount(1);
    expect(await css.evaluate((el) => el.textContent)).toContain(cssMarker);

    // A note's detail page carries both kinds of card: the owner card in the
    // sidebar keeps the summary, the Groups panel uses the panel slot.
    await open(page, `/note?id=${note.ID}`);
    await expectPanelSlot(card(panel(page, 'Groups'), childName), childName);
    await expectSummarySlot(card(page.locator('details', { hasText: `Owner: ${childName}` }), childName), childName);

    await open(page, `/tag?id=${tag.ID}`);
    await expectPanelSlot(card(panel(page, 'Groups'), childName), childName);

    await open(page, `/category?id=${cat.ID}`);
    await expectPanelSlot(card(panel(page, 'Groups'), childName), childName);

    // Surfaces that are not detail-page panels.
    await open(page, `/groups?categories=${cat.ID}`);
    await expectSummarySlot(card(page.locator('main'), childName), childName);

    await open(page, `/relation?id=${forward.ID}`);
    await expectSummarySlot(card(page.locator('main'), childName), childName);

    // The dashboard holds only the few most recent groups, which parallel
    // workers churn, so assert the slot never renders there rather than
    // looking for this group's card.
    await open(page, '/dashboard');
    await expect(page.locator('[data-probe="panel"]')).toHaveCount(0);

    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(`type = group AND name = "${childName}"`);
    await mrql.executeQuery();
    await expectSummarySlot(card(mrql.resultsSection, childName), childName);
  });

  test('note cards: panels on detail pages use it, the list keeps the summary', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const plain = await apiClient.createCategory(`PSN Plain ${stamp}`);
    const noteType = await apiClient.createNoteType(`PSN Type ${stamp}`, 'panel summary', {
      CustomSummary: SUMMARY,
      CustomPanelSummary: PANEL,
    });
    const tag = await apiClient.createTag(`PSN Tag ${stamp}`);
    const owner = await apiClient.createGroup({ name: `PSN Owner ${stamp}`, categoryId: plain.ID });
    const relatedGroup = await apiClient.createGroup({ name: `PSN Related ${stamp}`, categoryId: plain.ID });
    const resource = await apiClient.createResource({ filePath: IMAGE(2), name: `PSN Resource ${stamp}` });
    const noteName = `PSN Note ${stamp}`;
    await apiClient.createNote({
      name: noteName,
      noteTypeId: noteType.ID,
      ownerId: owner.ID,
      groups: [relatedGroup.ID],
      resources: [resource.ID],
      tags: [tag.ID],
    });

    await open(page, `/group?id=${owner.ID}`);
    await expectPanelSlot(card(panel(page, 'Notes'), noteName), noteName);

    await open(page, `/group?id=${relatedGroup.ID}`);
    await expectPanelSlot(card(panel(page, 'Related Notes'), noteName), noteName);

    await open(page, `/resource?id=${resource.ID}`);
    await expectPanelSlot(card(panel(page, 'Notes'), noteName), noteName);

    await open(page, `/tag?id=${tag.ID}`);
    await expectPanelSlot(card(panel(page, 'Notes'), noteName), noteName);

    await open(page, `/noteType?id=${noteType.ID}`);
    await expectPanelSlot(card(panel(page, 'Notes'), noteName), noteName);

    await open(page, `/notes?NoteTypeId=${noteType.ID}`);
    await expectSummarySlot(card(page.locator('main'), noteName), noteName);
  });

  test('resource cards: panels on detail pages use it, the list keeps the summary', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const plain = await apiClient.createCategory(`PSR Plain ${stamp}`);
    const rc = await apiClient.createResourceCategory(`PSR Cat ${stamp}`, 'panel summary', {
      CustomSummary: SUMMARY,
      CustomPanelSummary: PANEL,
    });
    const tag = await apiClient.createTag(`PSR Tag ${stamp}`);
    const owner = await apiClient.createGroup({ name: `PSR Owner ${stamp}`, categoryId: plain.ID });
    const relatedGroup = await apiClient.createGroup({ name: `PSR Related ${stamp}`, categoryId: plain.ID });
    const slug = `psr-series-${stamp}`;
    const name = `PSR Resource ${stamp}`;
    const resource = await apiClient.createResource({
      filePath: IMAGE(3), name, ownerId: owner.ID, resourceCategoryId: rc.ID, seriesSlug: slug,
    });
    const sibling = await apiClient.createResource({
      filePath: IMAGE(4), name: `PSR Sibling ${stamp}`, ownerId: owner.ID, seriesSlug: slug,
    });
    await apiClient.addTagsToResources([resource.ID], [tag.ID]);
    const addGroups = await page.request.post('/v1/resources/addGroups', {
      form: { ID: String(resource.ID), EditedId: String(relatedGroup.ID) },
    });
    expect(addGroups.ok()).toBeTruthy();
    const note = await apiClient.createNote({ name: `PSR Note ${stamp}`, resources: [resource.ID] });
    const seriesId = (await apiClient.getResource(resource.ID) as any).seriesId as number;
    expect(seriesId).toBeTruthy();

    await open(page, `/group?id=${owner.ID}`);
    await expectPanelSlot(card(panel(page, 'Resources'), name), name);

    await open(page, `/group?id=${relatedGroup.ID}`);
    await expectPanelSlot(card(panel(page, 'Related Resources'), name), name);

    await open(page, `/note?id=${note.ID}`);
    await expectPanelSlot(card(panel(page, 'Resources'), name), name);

    await open(page, `/resource?id=${sibling.ID}`);
    await expectPanelSlot(card(panel(page, 'Series'), name), name);

    await open(page, `/series?id=${seriesId}`);
    await expectPanelSlot(card(panel(page, 'Resources'), name), name);

    await open(page, `/tag?id=${tag.ID}`);
    await expectPanelSlot(card(panel(page, 'Resources'), name), name);

    await open(page, `/resourceCategory?id=${rc.ID}`);
    await expectPanelSlot(card(panel(page, 'Resources'), name), name);

    await open(page, `/resources?ResourceCategoryId=${rc.ID}`);
    await expectSummarySlot(card(page.locator('main'), name), name);
  });

  test('a partial update that omits the field keeps it', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`PSU Cat ${stamp}`, 'update', {
      CustomPanelSummary: PANEL,
      CustomPanelSummaryCSS: '.psu {}',
    });
    // JSON update naming only the description.
    const res = await page.request.post('/v1/category', {
      data: { ID: cat.ID, Name: cat.Name, Description: 'changed' },
    });
    expect(res.ok()).toBeTruthy();
    const fetched = (await (await page.request.get(`/category.json?id=${cat.ID}`)).json()).category;
    expect(fetched.CustomPanelSummary).toBe(PANEL);
    expect(fetched.CustomPanelSummaryCSS).toBe('.psu {}');

    // The edit form offers the slot with its stored value.
    await open(page, `/category/edit?id=${cat.ID}`);
    await expect(page.locator('input[type="hidden"][name="CustomPanelSummary"]')).toHaveValue(PANEL);
    await expect(page.locator('input[type="hidden"][name="CustomPanelSummaryCSS"]')).toHaveValue('.psu {}');
  });
});
