import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class RelationTypePage extends BasePage {
  readonly listUrl = '/relationTypes';
  readonly newUrl = '/relationType/new';
  readonly displayUrlBase = '/relationType';
  readonly editUrlBase = '/relationType/edit';

  constructor(page: Page) {
    super(page);
  }

  async gotoList() {
    await this.page.goto(this.listUrl);
    await this.page.waitForLoadState('load');
  }

  async gotoNew() {
    await this.page.goto(this.newUrl);
    await this.page.waitForLoadState('load');
  }

  async gotoDisplay(id: number) {
    await this.page.goto(`${this.displayUrlBase}?id=${id}`);
    await this.page.waitForLoadState('load');
  }

  async gotoEdit(id: number) {
    await this.page.goto(`${this.editUrlBase}?id=${id}`);
    await this.page.waitForLoadState('load');
  }

  async create(data: {
    name: string;
    description?: string;
    fromCategoryName?: string;
    toCategoryName?: string;
  }): Promise<number> {
    await this.gotoNew();

    await this.fillName(data.name);

    if (data.description) {
      await this.fillDescription(data.description);
    }

    // Select From Category
    if (data.fromCategoryName) {
      await this.selectFromAutocomplete('From Category', data.fromCategoryName);
    }

    // Select To Category
    if (data.toCategoryName) {
      await this.selectFromAutocomplete('To Category', data.toCategoryName);
    }

    await this.save();

    await this.verifyRedirectContains(/\/relationType\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(id: number, updates: { name?: string; description?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      // Use fill() which clears and types in one action, more reliable than clear() + fill()
      await this.nameInput.fill(updates.name);
    }
    if (updates.description !== undefined) {
      await this.descriptionInput.fill(updates.description);
    }
    await this.save();
    // Wait for redirect to display page after save
    await this.verifyRedirectContains(/\/relationType\?id=\d+/);
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyRelationTypeInList(name: string) {
    await this.gotoList();
    // Use .first() to avoid strict mode violations when multiple elements match
    await expect(this.page.locator(`a:has-text("${name}")`).first()).toBeVisible();
  }

  async verifyRelationTypeNotInList(name: string) {
    await this.gotoList();
    // Check that no matching elements exist
    await expect(this.page.locator(`a:has-text("${name}")`)).toHaveCount(0);
  }
}
