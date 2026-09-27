import { test, expect } from '../../fixtures/base.fixture';

/**
 * A suggestion list that opens under a mouse pointer nobody is moving must leave
 * the keyboard's active option alone.
 *
 * When a list appears beneath a resting pointer the browser updates hover and
 * fires `mouseover` on the option now under it, with no movement at all. The
 * options set the active option on that event, so the highlight jumped to
 * whichever row happened to open under the cursor, sometimes after a keyboard
 * user had already read the first one, and Enter then committed a row they had
 * not chosen. That is how the inline tag editor's "Enter with empty input
 * selects the first tag" test kept adding the second tag. Only real pointer
 * movement may move the highlight.
 */
test('a list opening under a resting pointer keeps the first option active', async ({ page, apiClient }) => {
  const prefix = `rest-${Date.now()}`;
  const tags = [];
  for (const suffix of ['a', 'b', 'c']) {
    tags.push(await apiClient.createTag(`${prefix}-${suffix}`, 'resting pointer'));
  }

  try {
    await page.goto('/group/new');
    const input = page.getByRole('combobox', { name: 'Tags' });
    await input.fill(prefix);

    const options = page.getByRole('option', { name: new RegExp(`^${prefix}-`) });
    await expect(options).toHaveCount(3);
    await expect(options.first()).toHaveAttribute('aria-selected', 'true');

    // Park the pointer where the third option is, close the list, and bring it back up
    // from the keyboard: it reopens with the third row under a pointer that never moves.
    const third = (await options.nth(2).boundingBox())!;
    await page.mouse.move(third.x + third.width / 2, third.y + third.height / 2);
    await input.press('Escape');
    await expect(options).toHaveCount(0);
    await input.press('ArrowDown');
    await expect(options).toHaveCount(3);
    await expect(options.nth(2)).toBeInViewport();

    // Hover is updated on the frame after the list renders; give it several.
    await page.evaluate(() => new Promise((resolve) => {
      let frames = 5;
      const tick = () => (--frames ? requestAnimationFrame(tick) : resolve(null));
      requestAnimationFrame(tick);
    }));
    await expect(options.first()).toHaveAttribute('aria-selected', 'true');
    await expect(options.nth(2)).toHaveAttribute('aria-selected', 'false');

    // Moving the pointer is a choice, and it still moves the highlight.
    await page.mouse.move(third.x + third.width / 2 + 5, third.y + third.height / 2);
    await expect(options.nth(2)).toHaveAttribute('aria-selected', 'true');
  } finally {
    for (const tag of tags) await apiClient.deleteTag(tag.ID).catch(() => {});
  }
});
