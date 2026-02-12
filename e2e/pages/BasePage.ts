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
    // Strategy 1: Use getByRole('combobox') with name to find the input
    let input = this.page.getByRole('combobox', { name: labelText });
    let inputCount = await input.count();

    if (inputCount === 0) {
      // Strategy 2: Find the grid row containing the label and look for combobox within
      // This handles cases where the label is in a span/label and the input is in a sibling div
      const gridRow = this.page.locator(`div.sm\\:grid:has(span:has-text("${labelText}"), label:has-text("${labelText}"))`).first();
      if (await gridRow.count() > 0) {
        input = gridRow.locator('input[role="combobox"]').first();
        inputCount = await input.count();
      }
    }

    if (inputCount === 0) {
      // Strategy 3: Look for container with the label text and find combobox inside
      const container = this.page.locator(`div:has(> span:has-text("${labelText}"), > label:has-text("${labelText}"))`).first();
      if (await container.count() > 0) {
        // Look deeper for the combobox
        input = container.locator('input[role="combobox"]').first();
        inputCount = await input.count();
      }
    }

    if (inputCount === 0) {
      // Strategy 4: Find any combobox in a div that contains the label text
      input = this.page.locator(`div:has-text("${labelText}"):has(input[role="combobox"]) input[role="combobox"]`).first();
      inputCount = await input.count();
    }

    if (inputCount === 0) {
      throw new Error(`Could not find autocomplete input for label: ${labelText}`);
    }

    if (inputCount > 1) {
      input = input.first();
    }

    await input.click();
    // Clear any existing text first
    await input.fill('');
    // Small delay to let any filters apply
    await this.page.waitForTimeout(100);
    await input.fill(searchText);

    // Wait for dropdown to appear and click the matching option.
    // Use :visible to avoid picking hidden options from other autocompleter popovers on the page.
    const option = this.page.locator(`div[role="option"]:visible:has-text("${optionText || searchText}")`).first();
    await option.waitFor({ timeout: 10000 });
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
    // Set up dialog handler for the confirm() prompt
    this.page.once('dialog', async (dialog) => {
      await dialog.accept();
    });
    const deleteButton = this.page.locator('input[type="submit"][value="Delete"], button:has-text("Delete")').first();
    await deleteButton.click();
    await this.page.waitForLoadState('load');
  }
}
