import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class ResourceCategoryPage extends BasePage {
  readonly listUrl = '/resourceCategories';
  readonly newUrl = '/resourceCategory/new';
  readonly displayUrlBase = '/resourceCategory';
  readonly editUrlBase = '/resourceCategory/edit';

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
      customSummary?: string;
      customAvatar?: string;
      metaSchema?: string;
    }
  ): Promise<number> {
    await this.gotoNew();
    await this.fillName(name);
    if (description) {
      await this.fillDescription(description);
    }
    if (options?.customHeader) {
      await this.fillCodeEditor('CustomHeader', options.customHeader);
    }
    if (options?.customSidebar) {
      await this.fillCodeEditor('CustomSidebar', options.customSidebar);
    }
    if (options?.customSummary) {
      await this.fillCodeEditor('CustomSummary', options.customSummary);
    }
    if (options?.customAvatar) {
      await this.fillCodeEditor('CustomAvatar', options.customAvatar);
    }
    if (options?.metaSchema) {
      await this.fillCodeEditor('MetaSchema', options.metaSchema);
    }
    await this.save();

    await this.verifyRedirectContains(/\/resourceCategory\?id=\d+/);
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

  async verifyInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }
}
