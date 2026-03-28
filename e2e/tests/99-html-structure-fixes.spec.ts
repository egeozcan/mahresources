import { test, expect } from '../fixtures/base.fixture';

/**
 * Tests for HTML structure correctness in form templates.
 *
 * Bug 1: createRelation.tpl has hidden inputs (RelationSideFrom, RelationSideTo)
 *        outside the <form> element, so they are never submitted.
 * Bug 2: createResource.tpl has <p> inside <span> (invalid HTML nesting).
 * Bug 3: createResource.tpl has <p> inside <label> (invalid HTML nesting).
 * Bug 4: createResource.tpl has autocomplete="name" on the Name input,
 *        causing browsers to autofill with the user's personal name.
 */
test.describe('HTML structure: createRelation hidden inputs inside form', () => {
  test('RelationSideFrom and RelationSideTo inputs should be inside the form', async ({
    page,
  }) => {
    await page.goto('/relation/new');
    await page.waitForLoadState('load');

    // Both hidden inputs must be descendants of the <form> element
    const formEl = page.locator('form');
    await expect(formEl).toBeVisible();

    const sideFrom = formEl.locator('input[name="RelationSideFrom"]');
    await expect(sideFrom).toBeAttached();
    await expect(sideFrom).toHaveAttribute('type', 'hidden');

    const sideTo = formEl.locator('input[name="RelationSideTo"]');
    await expect(sideTo).toBeAttached();
    await expect(sideTo).toHaveAttribute('type', 'hidden');
  });
});

test.describe('HTML structure: createResource valid nesting', () => {
  test('Series label container should be a div, not a span (no <p> inside <span>)', async ({
    page,
  }) => {
    await page.goto('/resource/new');
    await page.waitForLoadState('load');

    // The Series section label used to be a <span> containing a <p>.
    // After fix, it should be a <div> (which can legally contain <p>).
    // The container has the "Series" text plus a helper paragraph about grouping.
    const seriesContainer = page.getByText('Series Optional. Resources in').first();
    const tagName = await seriesContainer.evaluate((el) => el.tagName.toLowerCase());
    expect(tagName).toBe('div');
  });

  test('URL label should not contain a <p> element', async ({
    page,
  }) => {
    await page.goto('/resource/new');
    await page.waitForLoadState('load');

    // The <label for="URL"> used to contain a <p> child.
    // After fix, the <p> should be replaced with a <span class="block">.
    const urlLabel = page.locator('label[for="URL"]');
    const pCount = await urlLabel.locator('p').count();
    expect(pCount).toBe(0);

    // The help text should now be in a <span> element
    const helpSpan = urlLabel.locator('span');
    await expect(helpSpan).toBeAttached();
    await expect(helpSpan).toContainText('file picker will be ignored');
  });

  test('resource Name input should have autocomplete="off"', async ({
    page,
  }) => {
    await page.goto('/resource/new');
    await page.waitForLoadState('load');

    const nameInput = page.locator('input[name="Name"]');
    await expect(nameInput).toHaveAttribute('autocomplete', 'off');
  });
});
