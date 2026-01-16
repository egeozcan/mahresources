import { Page, Locator, expect } from '@playwright/test';

export class BasePage {
  readonly page: Page;
  readonly saveButton: Locator;
  readonly nameInput: Locator;
  readonly descriptionInput: Locator;

  constructor(page: Page) {
    this.page = page;
    this.saveButton = page.locator('input[type="submit"][value="Save"], button[type="submit"]:has-text("Save")');
    this.nameInput = page.locator('input[name="name"]');
    this.descriptionInput = page.locator('textarea[name="Description"]');
  }

  async fillName(name: string) {
    await this.nameInput.fill(name);
  }

  async fillDescription(description: string) {
    await this.descriptionInput.fill(description);
  }

  async save() {
    await this.saveButton.click();
    await this.page.waitForLoadState('load');
  }

  async selectFromAutocomplete(
    labelText: string,
    searchText: string,
    optionText?: string
  ) {
    // Find the autocomplete container by looking for the label
    const label = this.page.locator(`label:has-text("${labelText}")`);
    const container = label.locator('xpath=..');
    const input = container.locator('input[type="text"]').first();

    await input.click();
    await input.fill(searchText);

    // Wait for dropdown to appear and click the matching option
    const option = this.page.locator(`div[role="option"]:has-text("${optionText || searchText}")`).first();
    await option.waitFor({ state: 'visible', timeout: 5000 });
    await option.click();

    // Wait for dropdown to close (indicates selection was registered)
    await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {
      // Dropdown might already be hidden, which is fine
    });
  }

  async getPageTitle(): Promise<string> {
    const titleElement = this.page.locator('h1').first();
    return await titleElement.textContent() || '';
  }

  async verifyRedirectContains(pattern: string | RegExp) {
    await expect(this.page).toHaveURL(pattern);
  }

  extractIdFromUrl(): number {
    const url = this.page.url();
    const match = url.match(/[?&]id=(\d+)/);
    return match ? parseInt(match[1]) : 0;
  }

  async verifyElementVisible(text: string) {
    await expect(this.page.locator(`text=${text}`).first()).toBeVisible();
  }

  async verifyElementNotVisible(text: string) {
    await expect(this.page.locator(`text=${text}`)).toHaveCount(0);
  }

  async clickLink(text: string) {
    await this.page.locator(`a:has-text("${text}")`).first().click();
    await this.page.waitForLoadState('load');
  }

  async submitDelete() {
    const deleteButton = this.page.locator('input[type="submit"][value="Delete"], button:has-text("Delete")').first();
    await deleteButton.click();
    await this.page.waitForLoadState('load');
  }
}
