import { test, expect } from '../fixtures/base.fixture';

test.describe('Autocompleter behavior contract', () => {
  const runId = Date.now();
  const categoryIds: number[] = [];

  test.afterAll(async ({ apiClient }) => {
    for (const id of categoryIds) {
      try { await apiClient.deleteCategory(id); } catch { /* ignore cleanup failures */ }
    }
  });

  test('maximum-one selection atomically replaces the serialized value', async ({ page, apiClient }) => {
    const first = await apiClient.createCategory(`Single First ${runId}`);
    const replacement = await apiClient.createCategory(`Single Replacement ${runId}`);
    categoryIds.push(first.ID, replacement.ID);

    await page.goto('/group/new');

    const input = page.getByRole('combobox', { name: 'Category' });
    for (const category of [first, replacement]) {
      await input.fill(category.Name);
      const option = page.getByRole('option', { name: category.Name, exact: true });
      await expect(option).toBeVisible();
      await expect.poll(() => input.evaluate((element, query) => {
        const root = element.closest('[x-data]');
        const snapshot = (window as typeof window & { Alpine: { $data: (node: Element) => any } })
          .Alpine.$data(root!)._core.getSnapshot();
        return snapshot.query === query && snapshot.searchStatus === 'success';
      }, category.Name)).toBe(true);
      await option.click();
    }

    const serializedSelections = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');
    await expect(serializedSelections).toHaveCount(1);
    await expect(serializedSelections).toHaveValue(String(replacement.ID));
  });

  test('rapid queries render only the latest results and cancellation is not an error', async ({ page }) => {
    const firstQuery = `Delayed ${runId}`;
    const latestQuery = `Latest ${runId}`;
    let signalFirstRequest!: () => void;
    let releaseFirstResponse!: () => void;
    const firstRequestStarted = new Promise<void>((resolve) => { signalFirstRequest = resolve; });
    const firstResponseGate = new Promise<void>((resolve) => { releaseFirstResponse = resolve; });

    await page.goto('/group/new');
    await page.route('**/v1/categories?*', async (route) => {
      const query = new URL(route.request().url()).searchParams.get('name');
      if (query === firstQuery) {
        signalFirstRequest();
        await firstResponseGate;
        try {
          await route.fulfill({ json: [{ ID: 301, Name: firstQuery }] });
        } catch {
          // The browser is allowed to close the deliberately aborted request.
        }
        return;
      }
      if (query === latestQuery) {
        await route.fulfill({ json: [{ ID: 302, Name: latestQuery }] });
        return;
      }
      await route.continue();
    });

    const input = page.getByRole('combobox', { name: 'Category' });
    await input.fill(firstQuery);
    await firstRequestStarted;
    await input.fill(latestQuery);

    await expect(page.getByRole('option', { name: latestQuery, exact: true })).toBeVisible();
    releaseFirstResponse();

    await expect(page.getByRole('option', { name: firstQuery, exact: true })).toHaveCount(0);
    const selector = input.locator('xpath=../..');
    const errorAlert = selector.locator('[role="alert"]');
    await expect(errorAlert).toBeHidden();
  });

  test('Enter never commits stale or closed results while a search is pending', async ({ page }) => {
    const firstQuery = `Committed ${runId}`;
    const pendingQuery = `Pending ${runId}`;
    let pendingRequestStarted!: () => void;
    let releasePendingResponse!: () => void;
    const pendingRequest = new Promise<void>((resolve) => { pendingRequestStarted = resolve; });
    const pendingResponse = new Promise<void>((resolve) => { releasePendingResponse = resolve; });

    await page.goto('/group/new');
    await page.route('**/v1/categories?*', async (route) => {
      const query = new URL(route.request().url()).searchParams.get('name');
      if (query === firstQuery) {
        await route.fulfill({ json: [{ ID: 501, Name: firstQuery }] });
        return;
      }
      if (query === pendingQuery) {
        pendingRequestStarted();
        await pendingResponse;
        await route.fulfill({ json: [{ ID: 502, Name: pendingQuery }] });
        return;
      }
      await route.continue();
    });

    const input = page.getByRole('combobox', { name: 'Category' });
    const selected = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');
    await input.fill(firstQuery);
    await expect(page.getByRole('option', { name: firstQuery, exact: true })).toBeVisible();

    await input.fill(pendingQuery);
    await pendingRequest;
    await input.press('Enter');
    await expect(selected).toHaveCount(0);
    releasePendingResponse();

    await expect(page.getByRole('option', { name: pendingQuery, exact: true })).toBeVisible();
    await input.press('Escape');
    await expect(page.getByRole('listbox')).toBeHidden();
    await input.press('Enter');
    await expect(selected).toHaveCount(0);
  });

  test('clicking a rendered option commits it even while a newer search is pending', async ({ page }) => {
    // Enter is index-based and must not commit a stale roving option mid-search. A mouse
    // click names one specific rendered row, so it must always commit that row -- otherwise
    // the click is silently swallowed whenever the user out-types the debounce.
    const renderedQuery = `Rendered ${runId}`;
    const pendingQuery = `Rendered pending ${runId}`;
    let pendingRequestStarted!: () => void;
    let releasePendingResponse!: () => void;
    const pendingRequest = new Promise<void>((resolve) => { pendingRequestStarted = resolve; });
    const pendingResponse = new Promise<void>((resolve) => { releasePendingResponse = resolve; });

    await page.goto('/group/new');
    await page.route('**/v1/categories?*', async (route) => {
      const query = new URL(route.request().url()).searchParams.get('name');
      if (query === renderedQuery) {
        await route.fulfill({ json: [{ ID: 601, Name: renderedQuery }] });
        return;
      }
      if (query === pendingQuery) {
        pendingRequestStarted();
        await pendingResponse;
        await route.fulfill({ json: [{ ID: 602, Name: pendingQuery }] });
        return;
      }
      await route.continue();
    });

    const input = page.getByRole('combobox', { name: 'Category' });
    const selected = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');

    await input.fill(renderedQuery);
    const renderedOption = page.getByRole('option', { name: renderedQuery, exact: true });
    await expect(renderedOption).toBeVisible();

    // A newer search is now in flight while the previous query's row is still on screen.
    await input.fill(pendingQuery);
    await pendingRequest;
    await expect(renderedOption).toBeVisible();

    await renderedOption.click();

    await expect(selected).toHaveCount(1);
    await expect(selected).toHaveValue('601');
    releasePendingResponse();
  });

  test('a dropdown closes on blur after a mouse selection so it cannot cover the next field', async ({ page, apiClient }) => {
    // startSelecting() suppresses the blur-close so the mousedown can commit. Once it has
    // committed, the suppression must end -- otherwise the popover stays in the top layer and
    // swallows clicks aimed at the next selector on the same form.
    const category = await apiClient.createCategory(`Blur Close ${runId}`);
    categoryIds.push(category.ID);

    await page.goto('/group/new');

    const categoryInput = page.getByRole('combobox', { name: 'Category' });
    await categoryInput.fill(category.Name);
    const option = page.getByRole('option', { name: category.Name, exact: true });
    await expect(option).toBeVisible();

    const listbox = page.locator(
      `#${await option.evaluate(el => el.id.replace(/-result-\d+$/, ''))}-listbox`,
    );
    await expect(listbox).toBeVisible();
    await option.click();
    await expect(page.locator('input[type="hidden"][name="categoryId"]:not([value=""])')).toHaveCount(1);

    // Move focus to another selector on the same form.
    await page.getByRole('combobox', { name: 'Tags' }).click();

    await expect(listbox).toBeHidden({ timeout: 5000 });
  });

  test('selectors neither steal focus nor open a dropdown on page load', async ({ page }) => {
    // The shared field autofocuses only while the "Add X?" confirmation is showing, gated on
    // addModeForTag !== false. If the adapter downgrades that sentinel to '' during init, every
    // selector on the page grabs focus and runs an empty-query search, leaving an open popover
    // in the top layer that swallows clicks meant for the page underneath.
    await page.goto('/groups');
    await page.waitForLoadState('load');
    // Past the 1ms autofocus, the 200ms search debounce and its response.
    await page.waitForTimeout(1000);

    const focused = await page.evaluate(() => ({
      role: document.activeElement?.getAttribute('role') ?? null,
      id: document.activeElement?.id ?? null,
    }));
    expect(focused.role).not.toBe('combobox');
    await expect(page.locator('[role="option"]:visible')).toHaveCount(0);
  });

  test('a completed search stays closed when Escape wins the response race', async ({ page }) => {
    const query = `Close race ${runId}`;
    let requestStarted!: () => void;
    let releaseResponse!: () => void;
    const request = new Promise<void>((resolve) => { requestStarted = resolve; });
    const response = new Promise<void>((resolve) => { releaseResponse = resolve; });

    await page.goto('/group/new');
    await page.route('**/v1/categories?*', async (route) => {
      if (new URL(route.request().url()).searchParams.get('name') !== query) {
        await route.continue();
        return;
      }
      requestStarted();
      await response;
      await route.fulfill({ json: [{ ID: 503, Name: query }] });
    });

    const input = page.getByRole('combobox', { name: 'Category' });
    const listbox = page.getByRole('listbox');
    await input.fill(query);
    await request;
    await input.press('Escape');
    await expect(input).toBeFocused();
    releaseResponse();

    await expect(listbox).toBeHidden();
    await expect(input).toBeFocused();
  });

  test('legacy Add confirmation creates and selects the current query', async ({ page }) => {
    const newName = `Confirmed Category ${runId}`;
    await page.goto('/group/new');

    const input = page.getByRole('combobox', { name: 'Category' });
    await input.fill(newName);
    await expect(page.getByRole('option', { name: `Create "${newName}"`, exact: true })).toBeVisible();

    await input.press('Escape');
    await input.press('Enter');
    const confirmButton = page.getByRole('button', { name: `Add ${newName}?`, exact: true });
    await expect(confirmButton).toBeFocused();
    await confirmButton.press('Enter');

    const selected = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');
    await expect(selected).toHaveCount(1);
    categoryIds.push(Number(await selected.inputValue()));
  });

  test('clicking legacy Add confirmation creates and selects the current query', async ({ page, apiClient }) => {
    const newName = `Clicked Category ${runId}`;
    await page.goto('/group/new');

    const input = page.getByRole('combobox', { name: 'Category' });
    await input.fill(newName);
    await expect(page.getByRole('option', { name: `Create "${newName}"`, exact: true })).toBeVisible();

    await input.press('Escape');
    await input.press('Enter');
    const confirmButton = page.getByRole('button', { name: `Add ${newName}?`, exact: true });
    await confirmButton.click();

    const selected = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');
    await expect(selected).toHaveCount(1);
    await expect(confirmButton).toHaveCount(0);
    const selectedId = Number(await selected.inputValue());
    await expect.poll(async () => (await apiClient.getCategories()).some(
      (category) => category.ID === selectedId && category.Name === newName,
    )).toBe(true);
    categoryIds.push(selectedId);
  });

  test('create UI waits for the current query search and cancel restores the input focus path', async ({ page }) => {
    const newName = `Cancelled Category ${runId}`;
    let signalRequest!: () => void;
    let releaseResponse!: () => void;
    const requestStarted = new Promise<void>((resolve) => { signalRequest = resolve; });
    const responseGate = new Promise<void>((resolve) => { releaseResponse = resolve; });

    await page.goto('/group/new');
    await page.route('**/v1/categories?*', async (route) => {
      const query = new URL(route.request().url()).searchParams.get('name');
      if (query !== newName) {
        await route.continue();
        return;
      }
      signalRequest();
      await responseGate;
      await route.fulfill({ json: [] });
    });

    const input = page.getByRole('combobox', { name: 'Category' });
    await input.fill(newName);
    await requestStarted;

    await expect(page.getByRole('option', { name: `Create "${newName}"`, exact: true })).toHaveCount(0);
    await expect(page.getByRole('button', { name: `Add ${newName}?`, exact: true })).toHaveCount(0);

    releaseResponse();
    await expect(page.getByRole('option', { name: `Create "${newName}"`, exact: true })).toBeVisible();
    await input.press('Escape');
    await input.press('Enter');

    await page.getByRole('button', { name: 'Cancel', exact: true }).click();

    await expect(page.locator('input[type="hidden"][name="categoryId"]:not([value=""])')).toHaveCount(0);
    await expect(page.getByRole('combobox', { name: 'Category' })).toBeFocused();
  });

  test('relation group searches read the current relation type and side controls', async ({ page }) => {
    const typeA = { ID: 401, Name: `Dynamic Type A ${runId}` };
    const typeB = { ID: 402, Name: `Dynamic Type B ${runId}` };
    const groupRequests: Array<Record<string, string>> = [];

    await page.route('**/v1/relationTypes?*', async (route) => {
      const query = new URL(route.request().url()).searchParams.get('name');
      const result = query === typeA.Name ? typeA : query === typeB.Name ? typeB : null;
      await route.fulfill({ json: result ? [result] : [] });
    });
    await page.route('**/v1/groups?*', async (route) => {
      const params = Object.fromEntries(new URL(route.request().url()).searchParams.entries());
      if (params.name) groupRequests.push(params);
      await route.fulfill({ json: [] });
    });
    await page.goto('/relation/new');

    const typeInput = page.getByRole('combobox', { name: 'Type' });
    const fromGroupInput = page.getByRole('combobox', { name: 'From Group' });
    const selectType = async (type: { ID: number; Name: string }) => {
      await typeInput.fill(type.Name);
      const option = page.getByRole('option', { name: type.Name, exact: true });
      await expect(option).toBeVisible();
      await option.click();
      await expect(page.locator(`input[name="GroupRelationTypeId"][value="${type.ID}"]`)).toHaveCount(1);
    };
    const searchGroups = async (query: string) => {
      await fromGroupInput.fill(query);
      await expect.poll(() => groupRequests.some((request) => request.name === query)).toBe(true);
      return groupRequests.find((request) => request.name === query);
    };

    // Every request re-reads the live form controls. RelationSide is a plain hidden input, so
    // its current value is what gets sent.
    //
    // RelationTypeId comes from a selector field, which renders its value input followed by an
    // enabled-when-empty control of the same name. The lookup visits every matching control and
    // the last one wins, so a selected relation type is sent as an empty value and the dependent
    // group search is NOT actually narrowed. That is the long-standing behaviour this refactor
    // preserves; making these filters bite is a product change tracked separately in
    // tasks/todo.md, because it leaves uncategorized groups with no selectable relation type.
    await selectType(typeA);
    expect(await searchGroups('baseline-groups')).toMatchObject({
      RelationTypeId: '',
      RelationSide: '0',
    });

    await selectType(typeB);
    expect(await searchGroups('changed-type-groups')).toMatchObject({
      RelationTypeId: '',
      RelationSide: '0',
    });

    await page.locator('input[name="RelationSideFrom"]').evaluate((input: HTMLInputElement) => {
      input.value = '1';
    });
    expect(await searchGroups('changed-side-groups')).toMatchObject({
      RelationTypeId: '',
      RelationSide: '1',
    });

    // The guard Commit 42 needs: parameters are re-evaluated per request from the live DOM,
    // not captured once at initialization.
    await page.locator('input[name="RelationSideFrom"]').evaluate((input: HTMLInputElement) => {
      input.value = '0';
    });
    expect(await searchGroups('reverted-side-groups')).toMatchObject({
      RelationSide: '0',
    });
  });

  test('shared chip click and keyboard removal dispatch exactly once and restore focus', async ({ page }) => {
    await page.goto('/group/new');

    await page.evaluate(() => {
      const browserWindow = window as typeof window & {
        Alpine: { initTree: (element: Element) => void };
        __chipRemovalContract?: {
          removed: number[];
          aggregateEvents: Array<{ name: string; ids: number[] }>;
        };
      };
      browserWindow.__chipRemovalContract = { removed: [], aggregateEvents: [] };

      const harness = document.createElement('form');
      harness.id = 'chip-removal-contract';
      harness.innerHTML = `
        <div x-data="autocompleter({
               selectedResults: [
                 { ID: 101, Name: 'Click chip' },
                 { ID: 202, Name: 'Enter chip' },
                 { ID: 303, Name: 'Space chip' }
               ],
               max: 0,
               min: 0,
               ownerId: 0,
               url: '/v1/tags',
               elName: 'contractTags',
               onRemove: item => window.__chipRemovalContract.removed.push(item.ID)
             })">
          <input x-ref="autocompleter" role="combobox" aria-label="Contract tags">
          <div x-ref="dropdown" popover="manual"></div>
          <template x-for="(result, index) in selectedResults" :key="result.ID">
            <p>
              <button type="button"
                      :aria-label="'Remove ' + result.Name"
                      @click="removeItem(result)"
                      @keydown.enter.prevent="let root = $el.closest('[x-data]'); removeItem(result); $nextTick(() => root.querySelector('input[role=combobox]')?.focus())"
                      @keydown.space.prevent="let root = $el.closest('[x-data]'); removeItem(result); $nextTick(() => root.querySelector('input[role=combobox]')?.focus())">
                <span x-text="result.Name"></span>
              </button>
            </p>
          </template>
        </div>`;
      harness.addEventListener('multiple-input', ((event: CustomEvent) => {
        browserWindow.__chipRemovalContract?.aggregateEvents.push({
          name: event.detail.name,
          ids: event.detail.value.map((item: { ID: number }) => item.ID),
        });
      }) as EventListener);
      document.body.appendChild(harness);
      browserWindow.Alpine.initTree(harness);
    });

    const contractState = () => page.evaluate(() => (
      window as typeof window & {
        __chipRemovalContract?: {
          removed: number[];
          aggregateEvents: Array<{ name: string; ids: number[] }>;
        };
      }
    ).__chipRemovalContract);
    const input = page.getByRole('combobox', { name: 'Contract tags' });

    await page.getByRole('button', { name: 'Remove Click chip' }).click();
    await expect.poll(contractState).toEqual({
      removed: [101],
      aggregateEvents: [{ name: 'contractTags', ids: [202, 303] }],
    });

    const enterButton = page.getByRole('button', { name: 'Remove Enter chip' });
    await enterButton.focus();
    await enterButton.press('Enter');
    await expect(input).toBeFocused();
    await expect.poll(contractState).toEqual({
      removed: [101, 202],
      aggregateEvents: [
        { name: 'contractTags', ids: [202, 303] },
        { name: 'contractTags', ids: [303] },
      ],
    });

    const spaceButton = page.getByRole('button', { name: 'Remove Space chip' });
    await spaceButton.focus();
    await spaceButton.press('Space');
    await expect(input).toBeFocused();
    await expect.poll(contractState).toEqual({
      removed: [101, 202, 303],
      aggregateEvents: [
        { name: 'contractTags', ids: [202, 303] },
        { name: 'contractTags', ids: [303] },
        { name: 'contractTags', ids: [] },
      ],
    });
    await expect(page.locator('#chip-removal-contract button[aria-label^="Remove "]')).toHaveCount(0);
  });

  test('maximum-one replacement reports removal before the replacement selection', async ({ page }) => {
    await page.goto('/group/new');

    await page.evaluate(() => {
      const browserWindow = window as typeof window & {
        Alpine: {
          initTree: (element: Element) => void;
          $data: (element: Element) => Record<string, unknown>;
        };
        __selectorCallbacks?: { selected: number[]; removed: number[] };
      };
      browserWindow.__selectorCallbacks = { selected: [], removed: [] };

      const harness = document.createElement('div');
      harness.innerHTML = `
        <div id="single-callback-harness"
             x-data="autocompleter({
               selectedResults: [],
               max: 1,
               min: 0,
               ownerId: 0,
               url: '/v1/categories',
               elName: 'callbackCategoryId',
               onSelect: item => window.__selectorCallbacks.selected.push(item.ID),
               onRemove: item => window.__selectorCallbacks.removed.push(item.ID)
             })">
          <input x-ref="autocompleter" role="combobox">
          <div x-ref="dropdown" popover="manual"><div x-ref="list"></div></div>
          <template x-for="result in selectedResults" :key="result.ID">
            <input type="hidden" name="callbackCategoryId" :value="result.ID">
          </template>
        </div>`;
      document.body.appendChild(harness);
      browserWindow.Alpine.initTree(harness);
    });

    await page.evaluate(() => {
      const browserWindow = window as typeof window & {
        Alpine: { $data: (element: Element) => any };
      };
      const root = document.querySelector('#single-callback-harness');
      if (!root) throw new Error('callback harness was not initialized');
      const selector = browserWindow.Alpine.$data(root);

      for (const item of [{ ID: 101, Name: 'First' }, { ID: 202, Name: 'Replacement' }]) {
        selector.selectResult(item);
      }
    });

    await expect(page.locator('input[name="callbackCategoryId"]')).toHaveCount(1);
    await expect(page.locator('input[name="callbackCategoryId"]')).toHaveValue('202');
    await expect.poll(() => page.evaluate(() => (
      window as typeof window & { __selectorCallbacks?: { selected: number[]; removed: number[] } }
    ).__selectorCallbacks)).toEqual({ selected: [101, 202], removed: [101] });
  });
});
