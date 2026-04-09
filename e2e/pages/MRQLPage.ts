import { Page, Locator, expect } from '@playwright/test';

export class MRQLPage {
  readonly page: Page;
  readonly editorContainer: Locator;
  readonly runButton: Locator;
  readonly saveButton: Locator;
  readonly resultsSection: Locator;
  readonly savedQueriesSection: Locator;
  readonly errorAlert: Locator;
  readonly validationError: Locator;
  readonly docsButton: Locator;
  readonly docsPanel: Locator;

  constructor(page: Page) {
    this.page = page;
    this.editorContainer = page.locator('[x-ref="editorContainer"]');
    this.runButton = page.locator('button:has-text("Run")');
    this.saveButton = page.locator('button:has-text("Save")').first();
    this.resultsSection = page.locator('section[aria-label="Query results"]');
    this.savedQueriesSection = page.locator('section[aria-label="Saved queries"]');
    this.errorAlert = page.locator('[role="alert"]');
    this.validationError = page.locator('section[aria-label="MRQL query editor"] [role="alert"]');
    this.docsButton = page.locator('[aria-controls="mrql-docs-panel"]');
    this.docsPanel = page.locator('#mrql-docs-panel');
  }

  async navigate() {
    await this.page.goto('/mrql');
    await this.page.waitForLoadState('load');
    // Wait for CodeMirror to initialize
    await this.editorContainer.locator('.cm-editor').waitFor({ state: 'visible', timeout: 15000 });
  }

  /**
   * Open the docs panel (idempotent — does nothing if already open).
   */
  async openDocs() {
    const expanded = await this.docsButton.getAttribute('aria-expanded');
    if (expanded !== 'true') {
      await this.docsButton.click();
    }
    await this.docsPanel.waitFor({ state: 'visible', timeout: 5000 });
  }

  /**
   * Type a query into the CodeMirror editor by dispatching changes via the EditorView API.
   */
  async enterQuery(query: string) {
    await this.page.evaluate((text) => {
      const container = document.querySelector('[x-ref="editorContainer"]') as any;
      if (!container) throw new Error('Editor container not found');
      // Use the view reference exposed by the mrqlEditor Alpine component
      const view = container._cmView;
      if (!view) throw new Error('CodeMirror view not found');
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: text },
      });
    }, query);
  }

  /**
   * Click the Run button to execute the current query.
   */
  async executeQuery() {
    await this.runButton.click();
    // Wait for execution to complete (button text changes from "Running..." back to "Run")
    await expect(this.runButton).toContainText('Run', { timeout: 15000 });
    // Small extra wait for results to render via Alpine.js
    await this.page.waitForTimeout(300);
  }

  /**
   * Execute query using the Ctrl+Enter / Meta+Enter keyboard shortcut.
   */
  async executeQueryWithKeyboard() {
    // Focus the editor first
    await this.editorContainer.locator('.cm-content').click();
    // Use Control+Enter -- the editor binds both Mod-Enter and Ctrl-Enter
    // to ensure cross-platform compatibility (including headless Chromium on macOS)
    await this.page.keyboard.press('Control+Enter');
    // Wait for execution to complete (button text changes from "Running..." back to "Run")
    await expect(this.runButton).toContainText('Run', { timeout: 15000 });
    await this.page.waitForTimeout(300);
  }

  /**
   * Get result link elements from the results section.
   */
  async getResults(): Promise<Locator> {
    return this.resultsSection.locator('a[href*="?id="]');
  }

  /**
   * Get the total result count shown in the results heading.
   */
  async getResultCount(): Promise<number> {
    const countText = await this.resultsSection.locator('h2 span').textContent();
    if (!countText) return 0;
    const match = countText.match(/\((\d+)\s+items?\)/);
    return match ? parseInt(match[1], 10) : 0;
  }

  /**
   * Open the save dialog, fill in the name, and save the current query.
   */
  async saveQuery(name: string, description?: string) {
    await this.saveButton.click();

    // Wait for the dialog to appear
    const dialog = this.page.locator('[role="dialog"][aria-label="Save MRQL query"]');
    await dialog.waitFor({ state: 'visible' });

    // Fill in the name
    await this.page.locator('#mrql-save-name').fill(name);

    if (description) {
      await this.page.locator('#mrql-save-desc').fill(description);
    }

    // Click the Save button inside the dialog
    await dialog.locator('button:has-text("Save")').click();

    // Wait for the dialog to close
    await dialog.waitFor({ state: 'hidden', timeout: 10000 });

    // Wait for saved queries list to refresh
    await this.page.waitForTimeout(500);
  }

  /**
   * Click a saved query by name to load it into the editor.
   */
  async loadSavedQuery(name: string) {
    const queryButton = this.savedQueriesSection.locator(`button:has-text("${name}")`).first();
    await queryButton.click();
    // Small wait for the editor to update
    await this.page.waitForTimeout(200);
  }

  /**
   * Get the current query text from the CodeMirror editor.
   */
  async getEditorContent(): Promise<string> {
    return this.page.evaluate(() => {
      const container = document.querySelector('[x-ref="editorContainer"]') as any;
      if (!container) return '';
      // Use the view reference exposed by the mrqlEditor Alpine component
      const view = container._cmView;
      if (!view) return '';
      return view.state.doc.toString();
    });
  }

  /**
   * Get the error message displayed in the results section.
   */
  async getErrors(): Promise<string | null> {
    const alertLocator = this.resultsSection.locator('[role="alert"]');
    if (await alertLocator.count() === 0) return null;
    return alertLocator.textContent();
  }

  /**
   * Get the validation error text shown below the editor.
   */
  async getValidationError(): Promise<string | null> {
    if (await this.validationError.count() === 0) return null;
    return this.validationError.textContent();
  }

  /**
   * Get saved query names from the saved queries list.
   */
  async getSavedQueryNames(): Promise<string[]> {
    const items = this.savedQueriesSection.locator('li button span.text-sm.font-medium');
    const count = await items.count();
    const names: string[] = [];
    for (let i = 0; i < count; i++) {
      const text = await items.nth(i).textContent();
      if (text) names.push(text.trim());
    }
    return names;
  }
}
