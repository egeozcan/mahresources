/**
 * E2E tests for the [relations] shortcode: a group's typed relations in its
 * own template slots, a paired type listed once, incoming relations in a
 * block body that reads the far group's Meta, the [else] branch, list cards,
 * and the built-in panel hidden by SectionConfig.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('[relations] shortcode', () => {
  const stamp = Date.now();
  const ids: { categories: number[]; groups: number[]; types: number[] } = { categories: [], groups: [], types: [] };
  let svc: number;
  let api: { ID: number; Name: string };
  let auth: { ID: number; Name: string };
  let queue: { ID: number; Name: string };
  let lonely: { ID: number; Name: string };

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`Rel Svc ${stamp}`, 'services', {
      CustomSidebar: '<div data-testid="rel-out">[relations]</div>',
      CustomHeader:
        '<div data-testid="rel-in">[relations direction="in"]<span class="rel-row">[item path="GroupName"]: [item path="GroupMeta.status" default="no status"]</span>' +
        '[else]<p>Nothing points here.</p>[/relations]</div>',
      CustomSummary: '<div data-testid="rel-card">[relations type="Depends on" limit="3"]</div>',
      SectionConfig: JSON.stringify({ relations: { state: 'off' } }),
    });
    svc = category.ID;
    ids.categories.push(svc);
    const group = async (name: string, meta?: string) => {
      const g = await apiClient.createGroup({ name: `${name} ${stamp}`, categoryId: svc, meta });
      ids.groups.push(g.ID);
      return { ID: g.ID, Name: `${name} ${stamp}` };
    };
    api = await group('Rel API');
    auth = await group('Rel Auth', JSON.stringify({ status: 'green' }));
    queue = await group('Rel Queue', JSON.stringify({ status: 'amber' }));
    lonely = await group('Rel Lonely');

    const depends = await apiClient.createRelationType({
      name: `Depends on`, fromCategoryId: svc, toCategoryId: svc, reverseName: `Required by`,
    });
    const blocks = await apiClient.createRelationType({ name: `Blocks`, fromCategoryId: svc, toCategoryId: svc });
    ids.types.push(depends.ID, blocks.ID);
    await apiClient.createRelation({ fromGroupId: api.ID, toGroupId: auth.ID, relationTypeId: depends.ID });
    await apiClient.createRelation({ fromGroupId: api.ID, toGroupId: queue.ID, relationTypeId: depends.ID });
    await apiClient.createRelation({ fromGroupId: queue.ID, toGroupId: api.ID, relationTypeId: blocks.ID });
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of ids.groups) await apiClient.deleteGroup(id).catch(() => {});
    for (const id of ids.types) await apiClient.deleteRelationType(id).catch(() => {});
    for (const id of ids.categories) await apiClient.deleteCategory(id).catch(() => {});
  });

  test('the sidebar lists outgoing relations by type, and a link opens the far group', async ({ page }) => {
    await page.goto(`/group?id=${api.ID}`);
    const out = page.getByTestId('rel-out');
    const types = out.locator('.relations-shortcode__type');
    // Blocks reaches this group from Queue, so it is incoming and not listed here.
    await expect(types).toHaveCount(1);
    await expect(types.first().locator('.relations-shortcode__label')).toHaveText('Depends on');
    const list = types.first().getByRole('list');
    await expect(list.getByRole('listitem')).toHaveCount(2);
    const authLink = list.getByRole('link', { name: auth.Name });
    await expect(authLink).toHaveAttribute('href', `/group?id=${auth.ID}`);
    await expect(list.getByRole('listitem').first()).toContainText(`(Rel Svc ${stamp})`);

    // The category hid the built-in panels.
    await expect(page.locator('summary', { hasText: /^Relations$/ })).toHaveCount(0);

    await authLink.click();
    await expect(page).toHaveURL(new RegExp(`/group\\?id=${auth.ID}$`));
  });

  test('a paired type is listed once per side, as each group\'s outgoing edge', async ({ page }) => {
    await page.goto(`/group?id=${auth.ID}`);
    const out = page.getByTestId('rel-out');
    await expect(out.locator('.relations-shortcode__label')).toHaveText(['Required by']);
    await expect(out.getByRole('link', { name: api.Name })).toHaveCount(1);
  });

  test('incoming relations render the block body with the far group\'s Meta, and none renders [else]', async ({ page }) => {
    await page.goto(`/group?id=${api.ID}`);
    const incoming = page.getByTestId('rel-in');
    // "in" also lists the stored reverse half of the paired type, which is
    // why the docs point "in" at types without a back relation.
    await expect(incoming.locator('.relations-shortcode__label')).toHaveText(['Incoming: Blocks', 'Incoming: Required by']);
    await expect(incoming.locator('.relations-shortcode__entry .rel-row')).toHaveText([
      `${queue.Name}: amber`,
      `${auth.Name}: green`,
      `${queue.Name}: amber`,
    ]);

    await page.goto(`/group?id=${lonely.ID}`);
    await expect(page.getByTestId('rel-in')).toHaveText('Nothing points here.');
    await expect(page.getByTestId('rel-out').locator('.relations-shortcode')).toHaveCount(0);
  });

  test('list cards show each group\'s relations of one type', async ({ page }) => {
    await page.goto(`/groups?categories=${svc}`);
    const card = page.getByTestId('rel-card').filter({ has: page.getByRole('link', { name: auth.Name }) });
    await expect(card).toHaveCount(1);
    await expect(card.getByRole('link', { name: queue.Name })).toHaveAttribute('href', `/group?id=${queue.ID}`);
    // Auth's "Required by" edge is not shown on any card: the filter names one type.
    await expect(page.locator('.relations-shortcode__label', { hasText: 'Required by' })).toHaveCount(0);
  });
});
