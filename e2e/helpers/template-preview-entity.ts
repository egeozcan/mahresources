import type { Locator, Page } from '@playwright/test';

/**
 * The chosen entity in a category editor's "Preview against" field. The field
 * is the shared single-entity selector, so the choice is its one chip, whose
 * remove button is named "Remove <entity name>".
 */
export function previewEntityChip(page: Page): Locator {
  return page.locator('[data-tp-entity-field] button[aria-label^="Remove "]');
}
