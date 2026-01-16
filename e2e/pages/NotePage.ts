import { Page, expect } from '@playwright/test';
import { BasePage } from './BasePage';

export class NotePage extends BasePage {
  readonly listUrl = '/notes';
  readonly newUrl = '/note/new';
  readonly displayUrlBase = '/note';
  readonly editUrlBase = '/note/edit';

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

  // Override name input since Note uses "Name" not "name"
  async fillName(name: string) {
    await this.page.locator('input[name="Name"]').fill(name);
  }

  async create(data: {
    name: string;
    description?: string;
    noteTypeName?: string;
    ownerGroupName?: string;
    tags?: string[];
    groups?: string[];
    startDate?: string;
    endDate?: string;
  }): Promise<number> {
    await this.gotoNew();

    await this.fillName(data.name);

    if (data.description) {
      await this.fillDescription(data.description);
    }

    if (data.startDate) {
      await this.page.locator('input[name="startDate"]').fill(data.startDate);
    }

    if (data.endDate) {
      await this.page.locator('input[name="endDate"]').fill(data.endDate);
    }

    // Select note type
    if (data.noteTypeName) {
      await this.selectFromAutocomplete('Note Type', data.noteTypeName);
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
    if (data.groups) {
      for (const groupName of data.groups) {
        await this.selectFromAutocomplete('Groups', groupName);
      }
    }

    await this.save();

    await this.verifyRedirectContains(/\/note\?id=\d+/);
    return this.extractIdFromUrl();
  }

  async update(
    id: number,
    updates: { name?: string; description?: string; startDate?: string; endDate?: string }
  ) {
    await this.gotoEdit(id);
    if (updates.name !== undefined) {
      await this.page.locator('input[name="Name"]').clear();
      await this.fillName(updates.name);
    }
    if (updates.description !== undefined) {
      await this.descriptionInput.clear();
      await this.fillDescription(updates.description);
    }
    if (updates.startDate !== undefined) {
      await this.page.locator('input[name="startDate"]').fill(updates.startDate);
    }
    if (updates.endDate !== undefined) {
      await this.page.locator('input[name="endDate"]').fill(updates.endDate);
    }
    await this.save();
  }

  async delete(id: number) {
    await this.gotoDisplay(id);
    await this.submitDelete();
    await this.verifyRedirectContains(this.listUrl);
  }

  async verifyNoteInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).toBeVisible();
  }

  async verifyNoteNotInList(name: string) {
    await this.gotoList();
    await expect(this.page.locator(`a:has-text("${name}")`)).not.toBeVisible();
  }

  async verifyHasTag(tagName: string) {
    await expect(this.page.locator(`a:has-text("${tagName}")`)).toBeVisible();
  }

  async verifyHasOwner(ownerName: string) {
    await expect(this.page.locator(`text=${ownerName}`).first()).toBeVisible();
  }
}
