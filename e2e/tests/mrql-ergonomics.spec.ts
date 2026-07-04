import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import { ApiClient } from '../helpers/api-client';
import { MRQLPage } from '../pages/MRQLPage';
import { request as playwrightRequest } from '@playwright/test';
import * as path from 'path';

// Package 6 ergonomics: BETWEEN (filter bar), ORDER BY RANDOM(), ORDER BY RANK.
test.describe('MRQL ergonomics (package 6)', () => {
  let runId: string;
  let tagId: number;
  let tagName: string;
  let noteTypeId: number;
  let bestNoteName: string;
  let term: string;

  test.beforeAll(async () => {
    const baseUrl = getWorkerBaseUrl();
    const ctx = await playwrightRequest.newContext({ baseURL: baseUrl });
    const api = new ApiClient(ctx, baseUrl);
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    tagName = `erg-${runId}`;
    tagId = (await api.createTag(tagName)).ID;

    // Four distinct image files so content-hash dedup doesn't merge them.
    for (let i = 0; i < 4; i++) {
      const res = await api.createResource({
        filePath: path.join(__dirname, `../test-assets/sample-image-1${i}.png`),
        name: `erg-res-${runId}-${i}`,
      });
      await api.addTagsToResources([res.ID], [tagId]);
    }

    // Two notes with a unique term where one is clearly more relevant (shorter
    // matching text ranks higher under bm25/ts_rank).
    term = `kubernetesmigration${runId.replace(/[^a-z0-9]/gi, '')}`;
    noteTypeId = (await api.createNoteType(`erg-nt-${runId}`)).ID;
    bestNoteName = `erg-note-best-${runId}`;
    await api.createNote({
      name: bestNoteName,
      description: term,
      noteTypeId,
    });
    await api.createNote({
      name: `erg-note-filler-${runId}`,
      description: `${term} appears once among many unrelated filler words about databases networking storage caching layers and other topics entirely`,
      noteTypeId,
    });

    await ctx.dispose();
  });

  test('filter bar accepts BETWEEN on the resources list', async ({ page }) => {
    // fileSize BETWEEN a huge inclusive range matches every seeded resource;
    // AND the tag narrows to just this run's four.
    const expr = `tags = "${tagName}" AND fileSize BETWEEN 1 AND 999999999`;
    await page.goto('/resources?mrql=' + encodeURIComponent(expr));

    // At least one of the seeded resources is shown — the BETWEEN filter parsed
    // and matched (a rejected filter would fail-closed to zero results).
    await expect(page.locator(`a[title="erg-res-${runId}-0"]`)).toBeVisible();

    // The fail-closed banner is not surfaced for a valid expression.
    const banner = page.locator('.mrql-bar [role="alert"]');
    await expect(banner).not.toBeVisible();
  });

  test('ORDER BY RANDOM() with LIMIT caps the result set', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(`type = resource AND tags = "${tagName}" ORDER BY RANDOM() LIMIT 3`);
    await mrql.executeQuery();

    const count = await mrql.getResultCount();
    expect(count).toBeGreaterThan(0);
    expect(count).toBeLessThanOrEqual(3);
  });

  test('ORDER BY RANK ranks the most relevant note first', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(`type = note AND TEXT ~ "${term}" ORDER BY RANK`);
    await mrql.executeQuery();

    const count = await mrql.getResultCount();
    expect(count).toBe(2);

    const results = await mrql.getResults();
    await expect(results.first()).toContainText(bestNoteName);
  });

  // Regex (~*) is PostgreSQL-only, so this only runs in the Postgres e2e suite.
  test('regex match (~*) filters resources on Postgres', async ({ page }) => {
    test.skip(!process.env.PG_DSN, 'regex match is PostgreSQL-only');

    const mrql = new MRQLPage(page);
    await mrql.navigate();
    // Anchored regex matching exactly this run's resource-0 name.
    await mrql.enterQuery(`type = resource AND name ~* "^erg-res-${runId}-0$"`);
    await mrql.executeQuery();

    const count = await mrql.getResultCount();
    expect(count).toBe(1);
    const results = await mrql.getResults();
    await expect(results.first()).toContainText(`erg-res-${runId}-0`);
  });
});
