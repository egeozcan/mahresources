import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class RelationPage extends BasePage {
  readonly listUrl = '/relations';
  readonly newUrl = '/relation/new';
  readonly displayUrlBase = '/relation';
  readonly editUrlBase = '/relation/edit';

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
    relationTypeName: string;
    fromGroupName: string;
    toGroupName: string;
  }): Promise<number> {
    await this.gotoNew();

    await this.fillName(data.name);

    if (data.description) {
      await this.fillDescription(data.description);
    }

    // Select relation type
    await this.selectFromAutocomplete('Relation Type', data.relationTypeName);

    // Select From Group
    await this.selectFromAutocomplete('From Group', data.fromGroupName);

    // Select To Group
    await this.selectFromAutocomplete('To Group', data.toGroupName);

    await this.save();

    await this.verifyRedirectContains(/\/relation\?id=\d+/);
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

  async verifyRelationInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyRelationNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }
}
