import { test } from '../fixtures/base.fixture';

test('measure relation type search params with a From Group selected', async ({ page, apiClient }) => {
  const runId = Date.now();
  const cat = await apiClient.createCategory(`ProofCat ${runId}`);
  const g = await apiClient.createGroup({ name: `ProofFrom ${runId}`, categoryId: cat.ID });

  const reqs: string[] = [];
  page.on('request', r => { if (r.url().includes('/v1/relationTypes')) reqs.push(new URL(r.url()).search); });

  await page.goto(`/relation/new?FromGroupId=${g.ID}`);
  await page.waitForLoadState('load');
  await page.getByRole('combobox', { name: 'Type' }).fill('Addr');
  await page.waitForTimeout(1200);

  const hidden = await page.evaluate(() =>
    [...document.querySelectorAll('input[type=hidden][name=FromGroupId]')]
      .map(i => { const e = i as HTMLInputElement; return `value="${e.value}" disabled=${e.disabled}`; }));

  console.log('>>> FromGroupId controls: ' + JSON.stringify(hidden));
  console.log('>>> relationTypes requests: ' + JSON.stringify(reqs));
});
