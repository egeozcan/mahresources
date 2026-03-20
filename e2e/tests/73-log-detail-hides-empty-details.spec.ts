import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: The log detail page (/log?id=N) always renders the "Details" section
 * even when the log entry has no details (null in the database).
 *
 * Root cause: In displayLog.tpl, the condition `{% if log.Details %}` evaluates
 * a zero-value `types.JSON` as truthy, so the Details <dt>/<dd> block always
 * renders. Additionally, the x-init expression uses `try { ... } catch(e) {}`
 * which Alpine.js does not support in expressions (it wraps them in
 * `new AsyncFunction(...)` which only accepts expression syntax), causing a
 * SyntaxError on every log detail page visit.
 *
 * Expected: When a log entry has no details, the "Details" row should not
 * appear at all, and no Alpine.js errors should be logged to the console.
 */
test.describe('Log detail page should hide empty Details section', () => {
  test('log entry without details should not show Details row or trigger console errors', async ({
    page,
    apiClient,
  }) => {
    // 1. Create a tag to produce a log entry (tag creation logs have no details)
    const tag = await apiClient.createTag(`Log Detail Test Tag ${Date.now()}`, 'testing log detail page');

    // 2. Find the log entry for this tag creation
    const logsResponse = await page.request.get('/v1/logs', {
      headers: { Accept: 'application/json' },
    });
    const logsData = await logsResponse.json();
    const logs = logsData.logs || logsData;
    const logEntry = logs.find(
      (l: any) => l.entityType === 'tag' && l.entityId === tag.ID && l.action === 'create'
    );
    expect(logEntry, 'Should find the log entry for the created tag').toBeTruthy();

    // 3. Collect console errors
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error' || msg.type() === 'warning') {
        consoleErrors.push(msg.text());
      }
    });

    // 4. Navigate to the log detail page
    await page.goto(`/log?id=${logEntry.id}`);

    // 5. Verify the log entry info is displayed
    await expect(page.getByRole('heading', { name: `Log Entry #${logEntry.id}` })).toBeVisible();
    await expect(page.getByText('Created tag')).toBeVisible();

    // 6. The "Details" label should NOT be present since this log entry has no details
    const detailsDt = page.locator('dt', { hasText: 'Details' });
    await expect(
      detailsDt,
      'Details row should not be rendered for log entries without details'
    ).toHaveCount(0);

    // 7. No Alpine.js expression errors should appear in the console
    const alpineErrors = consoleErrors.filter(
      (err) => err.includes('Alpine') || err.includes('Unexpected token')
    );
    expect(
      alpineErrors,
      `No Alpine.js expression errors should occur. Errors:\n${alpineErrors.join('\n')}`
    ).toHaveLength(0);
  });
});
