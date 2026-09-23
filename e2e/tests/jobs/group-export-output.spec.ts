import { test, expect } from '../../fixtures/base.fixture';

type JobDetail = {
  id: string;
  title: string;
  state: string;
  outputs: Array<{ key: string; label?: string; availability: string; url: string }>;
};

async function readJob(request: import('@playwright/test').APIRequestContext, id: string): Promise<JobDetail | null> {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  if (!response.ok()) return null;
  return response.json();
}

test('a completed group export exposes its title and available output in Job Center detail', async ({ page, apiClient, request }) => {
  const suffix = Date.now();
  const category = await apiClient.createCategory(`job-output-cat-${suffix}`);
  const group = await apiClient.createGroup({ name: `job-output-group-${suffix}`, categoryId: category.ID });
  const submitted = await request.post('/v1/groups/export', {
    headers: { 'Content-Type': 'application/json' },
    data: { rootGroupIds: [group.ID] },
  });
  expect(submitted.ok(), await submitted.text()).toBe(true);
  const { canonicalJobId } = await submitted.json();
  expect(canonicalJobId).toEqual(expect.any(String));

  await expect.poll(async () => (await readJob(request, canonicalJobId))?.state ?? null, {
    timeout: 30_000,
    intervals: [500],
  }).toBe('succeeded');
  const detail = await readJob(request, canonicalJobId);
  expect(detail?.title).toBeTruthy();
  const output = detail?.outputs.find(record => record.availability === 'available');
  expect(output, 'completed export should advertise an available output').toBeTruthy();

  await page.goto(`/job?id=${encodeURIComponent(canonicalJobId)}`);
  const pageDetail = page.getByTestId('job-detail');
  await expect(pageDetail.getByRole('heading', { name: detail!.title, exact: true })).toBeVisible();
  const outputSection = pageDetail.locator('section[aria-labelledby="job-outputs-heading"]');
  const outputRow = outputSection.getByRole('listitem').filter({ hasText: output!.label || output!.key });
  await expect(outputRow.getByRole('link')).toHaveAttribute('href', output!.url);
});
