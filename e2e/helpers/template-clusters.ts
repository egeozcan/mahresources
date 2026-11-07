import type { Page } from '@playwright/test';

/**
 * The slot clusters on the category, resource category and note type editors
 * load collapsed (template-slot-clusters-collapse.spec.ts covers that). Specs
 * about what happens inside a cluster call this once per test, before
 * navigating, so every editor page they load opens all clusters at
 * DOMContentLoaded, as the page was before clusters could collapse.
 */
export async function openSlotClustersOnLoad(page: Page) {
  await page.addInitScript(() => {
    document.addEventListener('DOMContentLoaded', () => {
      document.querySelectorAll<HTMLDetailsElement>('details[data-template-cluster]').forEach((d) => {
        d.open = true;
      });
    });
  });
}
