import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class NoteTypePage extends BasePage {
  readonly listUrl = '/noteTypes';
  readonly newUrl = '/noteType/new';
  readonly displayUrlBase = '/noteType';
  readonly editUrlBase = '/noteType/edit';

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

  async create(
    name: string,
    description?: string,
    options?: {
      customHeader?: string;
      customSidebar?: string;
    }
  ): Promise<number> {
    await this.gotoNew();
    await this.fillName(name);
    if (description) {
      await this.fillDescription(description);
    }
    if (options?.customHeader) {
      await this.page.locator('textarea[name="CustomHeader"]').fill(options.customHeader);
    }
    if (options?.customSidebar) {
      await this.page.locator('textarea[name="CustomSidebar"]').fill(options.customSidebar);
    }
    await this.save();

    await this.verifyRedirectContains(/\/noteType\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(id: number, updates: { name?: string; description?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.nameInput.clear();
      await this.fillName(updates.name);
    }
    if (updates.description !== undefined) {
      await this.descriptionInput.clear();
      await this.fillDescription(updates.description);
    }
    await this.save();
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyNoteTypeInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyNoteTypeNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }
}
