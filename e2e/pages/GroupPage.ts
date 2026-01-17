import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class GroupPage extends BasePage {
  readonly listUrl = '/groups';
  readonly newUrl = '/group/new';
  readonly displayUrlBase = '/group';
  readonly editUrlBase = '/group/edit';

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
    categoryName: string;
    url?: string;
    tags?: string[];
    ownerGroupName?: string;
    relatedGroupNames?: string[];
  }): Promise<number> {
    await this.gotoNew();

    // Category is required - select it first
    await this.selectFromAutocomplete('Category', data.categoryName);

    await this.fillName(data.name);

    if (data.description) {
      await this.fillDescription(data.description);
    }

    if (data.url) {
      await this.page.locator('input[name="URL"]').fill(data.url);
    }

    // Add tags
    if (data.tags) {
      for (const tag of data.tags) {
        await this.selectFromAutocomplete('Tags', tag);
      }
    }

    // Add owner
    if (data.ownerGroupName) {
      await this.selectFromAutocomplete('Owner', data.ownerGroupName);
    }

    // Add related groups
    if (data.relatedGroupNames) {
      for (const groupName of data.relatedGroupNames) {
        await this.selectFromAutocomplete('Groups', groupName);
      }
    }

    await this.save();

    await this.verifyRedirectContains(/\/group\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(id: number, updates: { name?: string; description?: string; url?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.nameInput.clear();
      await this.fillName(updates.name);
    }
    if (updates.description !== undefined) {
      await this.descriptionInput.clear();
      await this.fillDescription(updates.description);
    }
    if (updates.url !== undefined) {
      await this.page.locator('input[name="URL"]').clear();
      await this.page.locator('input[name="URL"]').fill(updates.url);
    }
    await this.save();
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyGroupInList(name: string) {
    await this.gotoList();
    // Use .first() to avoid strict mode violations when multiple elements match
    await expect(this.page.locator(`a:has-text("${name}")`).first()).toBeVisible();
  }

  async verifyGroupNotInList(name: string) {
    await this.gotoList();
    // Use toHaveCount(0) to check that no matching elements exist
    await expect(this.page.locator(`a:has-text("${name}")`)).toHaveCount(0);
  }

  async verifyHasTag(tagName: string) {
    await expect(this.page.locator(`a:has-text("${tagName}")`)).toBeVisible();
  }

  async verifyHasOwner(ownerName: string) {
    await expect(this.page.locator(`text=${ownerName}`).first()).toBeVisible();
  }

  // For selecting groups in bulk operations
  async selectGroupCheckbox(groupId: number) {
    // The checkbox is inside a div with x-data="selectableItem({ itemId: <id> })"
    await this.page.locator(`[x-data*="itemId: ${groupId}"] input[type="checkbox"]`).check();
  }

  async clickSelectAll() {
    await this.page.locator('button:has-text("Select All")').click();
  }

  async clickDeselectAll() {
    await this.page.locator('button:has-text("Deselect")').click();
  }
}
