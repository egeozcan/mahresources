import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class TagPage extends BasePage {
  readonly listUrl = '/tags';
  readonly newUrl = '/tag/new';
  readonly displayUrlBase = '/tag';
  readonly editUrlBase = '/tag/edit';

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

  async create(name: string, description?: string): Promise<number> {
    await this.gotoNew();
    await this.fillName(name);
    if (description) {
      await this.fillDescription(description);
    }
    await this.save();

    await this.verifyRedirectContains(/\/tag\?id=\d+/);
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

  async verifyTagInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyTagNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }

  async submitEmptyForm(): Promise<void> {
    await this.gotoNew();
    await this.save();
  }
}
