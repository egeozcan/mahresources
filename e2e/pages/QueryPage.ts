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

  /** Set a CodeMirror editor's content by field name, updating both the editor and hidden input */
  private async fillCodeEditor(fieldName: string, text: string) {
    // Wait for the CodeMirror editor to initialize
    const hiddenInput = this.page.locator(`input[type="hidden"][name="${fieldName}"]`);
    await hiddenInput.waitFor({ state: 'attached' });
    const container = hiddenInput.locator('..');
    await container.locator('.cm-editor').first().waitFor({ state: 'visible' });

    // Set the value via the CodeMirror EditorView API and update the hidden input
    await this.page.evaluate(({ name, value }) => {
      const input = document.querySelector(`input[type="hidden"][name="${name}"]`) as HTMLInputElement;
      if (!input) return;
      input.value = value;
      // Access the EditorView via CodeMirror's DOM binding
      const cmEditor = input.parentElement?.querySelector('.cm-editor') as any;
      if (cmEditor?.cmView?.view) {
        const view = cmEditor.cmView.view;
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: value }
        });
      }
    }, { name: fieldName, value: text });
  }

  async create(data: {
    name: string;
    text: string;
    template?: string;
  }): Promise<number> {
    await this.gotoNew();

    await this.fillName(data.name);
    await this.fillCodeEditor('Text', data.text);

    if (data.template) {
      await this.fillCodeEditor('Template', data.template);
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
      await this.fillCodeEditor('Text', updates.text);
    }
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
    const runButton = this.page.locator('button:has-text("Run"), input[type="submit"][value="Run"]');
    if (await runButton.isVisible()) {
      await runButton.click();
      await this.page.waitForLoadState('load');
    }
  }
}
