import { Page, Locator } from '@playwright/test';

export class AdminExportPage {
  readonly page: Page;
  readonly groupSearchInput: Locator;
  readonly chips: Locator;
  readonly estimateButton: Locator;
  readonly estimateOutput: Locator;
  readonly submitButton: Locator;
  readonly progressPanel: Locator;
  readonly downloadLink: Locator;

  constructor(page: Page) {
    this.page = page;
    this.groupSearchInput = page.getByPlaceholder('Search to add groups...');
    this.chips = page.getByTestId('export-group-chips');
    this.estimateButton = page.getByTestId('export-estimate-button');
    this.estimateOutput = page.getByTestId('export-estimate-output');
    this.submitButton = page.getByTestId('export-submit-button');
    this.progressPanel = page.getByTestId('export-progress-panel');
    this.downloadLink = page.getByTestId('export-download-link');
  }

  async goto(preselect?: number[]) {
    const query = preselect ? '?groups=' + preselect.join(',') : '';
    await this.page.goto('/admin/export' + query);
    await this.page.waitForLoadState('load');
  }
}
