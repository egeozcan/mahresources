import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class ResourcePage extends BasePage {
  readonly listUrl = '/resources';
  readonly newUrl = '/resource/new';
  readonly displayUrlBase = '/resource';
  readonly editUrlBase = '/resource/edit';

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

  // Override name input since Resource uses "Name" not "name"
  async fillName(name: string) {
    await this.page.locator('input[name="Name"]').fill(name);
  }

  async createFromFile(data: {
    filePath: string;
    name?: string;
    description?: string;
    ownerGroupName?: string;
    tags?: string[];
  }): Promise<number> {
    await this.gotoNew();

    // Set file input
    const fileInput = this.page.locator('input[type="file"]');
    await fileInput.setInputFiles(data.filePath);

    if (data.name) {
      await this.fillName(data.name);
    }

    if (data.description) {
      await this.fillDescription(data.description);
    }

    // Add owner
    if (data.ownerGroupName) {
      await this.selectFromAutocomplete('Owner', data.ownerGroupName);
    }

    // Add tags
    if (data.tags) {
      for (const tag of data.tags) {
        await this.selectFromAutocomplete('Tags', tag);
      }
    }

    await this.save();

    // Wait for redirect to complete
    await this.page.waitForLoadState('load');

    // Try to extract ID from URL if redirected to display page
    const url = this.page.url();
    if (url.includes('/resource?id=')) {
      return this.extractIdFromUrl();
    }

    // If redirected to list, wait for the list to load and find the resource
    if (url.includes('/resources')) {
      // Wait a bit for the list to render
      await this.page.waitForTimeout(500);

      // Try to find the resource by name first, then by any recent resource
      if (data.name) {
        const resourceLink = this.page.locator(`a:has-text("${data.name}")`).first();
        const isVisible = await resourceLink.isVisible().catch(() => false);

        if (isVisible) {
          await resourceLink.click();
          await this.page.waitForLoadState('load');
          return this.extractIdFromUrl();
        }
      }

      // If we can't find by name, try to get the first resource link
      const anyResourceLink = this.page.locator('a[href*="/resource?id="]').first();
      const isAnyVisible = await anyResourceLink.isVisible().catch(() => false);

      if (isAnyVisible) {
        await anyResourceLink.click();
        await this.page.waitForLoadState('load');
        return this.extractIdFromUrl();
      }
    }

    throw new Error(`Could not determine resource ID after creation. Current URL: ${this.page.url()}`);
  }

  async createFromUrl(data: {
    url: string;
    name?: string;
    description?: string;
    ownerGroupName?: string;
    tags?: string[];
  }): Promise<number> {
    await this.gotoNew();

    // Fill URL
    await this.page.locator('textarea[name="URL"]').fill(data.url);

    if (data.name) {
      await this.fillName(data.name);
    }

    if (data.description) {
      await this.fillDescription(data.description);
    }

    // Add owner
    if (data.ownerGroupName) {
      await this.selectFromAutocomplete('Owner', data.ownerGroupName);
    }

    // Add tags
    if (data.tags) {
      for (const tag of data.tags) {
        await this.selectFromAutocomplete('Tags', tag);
      }
    }

    await this.save();
    await this.page.waitForLoadState('load');

    const currentUrl = this.page.url();
    if (currentUrl.includes('/resource?id=')) {
      return this.extractIdFromUrl();
    }

    // If redirected to list, find the resource by name and extract ID
    if (data.name) {
      const resourceLink = this.page.locator(`a:has-text("${data.name}")`).first();
      await resourceLink.click();
      await this.page.waitForLoadState('load');
      return this.extractIdFromUrl();
    }

    throw new Error('Could not determine resource ID after creation');
  }

  async update(id: number, updates: { name?: string; description?: string }) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.page.locator('input[name="Name"]').clear();
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

  async verifyResourceInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`text=${name}`).first()).toBeVisible();
  }

  async verifyResourceNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`text=${name}`)).not.toBeVisible();
  }

  async verifyHasTag(tagName: string) {
    await expect(this.page.locator(`a:has-text("${tagName}")`)).toBeVisible();
  }

  // Bulk selection
  async selectResourceCheckbox(resourceId: number) {
    await this.page.locator(`input[type="checkbox"][value="${resourceId}"]`).check();
  }
}
