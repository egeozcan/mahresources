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

    // Select relation type (form label is just "Type")
    await this.selectFromAutocomplete('Type', data.relationTypeName);

    // Wait for the hidden input to be created by Alpine.js after selection
    // The autocomplete adds a hidden input with name="GroupRelationTypeId" when a type is selected
    await this.page.waitForSelector('input[name="GroupRelationTypeId"]', { state: 'attached', timeout: 5000 });
    // Give Alpine.js a moment to update the DOM
    await this.page.waitForTimeout(200);

    // Select From Group
    await this.selectFromAutocomplete('From Group', data.fromGroupName);

    // Wait for the hidden input for From Group
    await this.page.waitForSelector('input[name="FromGroupId"]', { state: 'attached', timeout: 5000 });
    await this.page.waitForTimeout(100);

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
    // The server redirects to /groups after deleting a relation
    await this.verifyRedirectContains('/groups');
  }

  async verifyRelationInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`).first()).toBeVisible();
  }

  async verifyRelationNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toHaveCount(0);
  }
}
