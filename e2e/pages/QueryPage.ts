import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class QueryPage extends BasePage {
  readonly listUrl = '/queries';
  readonly newUrl = '/query/new';
  readonly displayUrlBase = '/query';
  readonly editUrlBase = '/query/edit';

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
    text: string;
    template?: string;
  }): Promise<number> {
    await this.gotoNew();

    await this.fillName(data.name);
    await this.page.locator('textarea[name="Text"]').fill(data.text);

    // Note: Query form does not have a description field

    if (data.template) {
      await this.page.locator('textarea[name="Template"]').fill(data.template);
    }

    await this.save();

    await this.verifyRedirectContains(/\/query\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(id: number, updates: { name?: string; text?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.nameInput.clear();
      await this.fillName(updates.name);
    }
    if (updates.text !== undefined) {
      await this.page.locator('textarea[name="Text"]').clear();
      await this.page.locator('textarea[name="Text"]').fill(updates.text);
    }
    // Note: Query form does not have a description field
    await this.save();
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyQueryInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyQueryNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }

  async runQuery(id: number): Promise<void> {
    await this.gotoDisplay(id);
    // Look for a run button if available
    const runButton = this.page.locator('button:has-text("Run"), input[type="submit"][value="Run"]');
    if (await runButton.isVisible()) {
      await runButton.click();
      await this.page.waitForLoadState('load');
    }
  }
}
