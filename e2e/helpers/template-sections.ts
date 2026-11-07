import type { Page } from '@playwright/test';

/**
 * The slot clusters and the Reuse & Presets panel on the category, resource
 * category and note type editors load collapsed
 * (template-slot-clusters-collapse.spec.ts covers that). Specs about what
 * happens inside them call this once per test, before navigating, so every
 * editor page they load opens all of them at DOMContentLoaded, as the page was
 * before they could collapse.
 */
export async function openTemplateSectionsOnLoad(page: Page) {
  await page.addInitScript(() => {
    document.addEventListener('DOMContentLoaded', () => {
      document
        .querySelectorAll<HTMLDetailsElement>('details[data-template-cluster], details[data-testid="template-bundle-tools"]')
        .forEach((d) => {
          d.open = true;
        });
    });
  });
}
