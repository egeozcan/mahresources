import {test, expect} from '../../fixtures/base.fixture';

// A nested schema the array editor shows (items, contains, a prefix item) is
// not typed by withTypedKeywords. Binding a non-string type to a select's
// value threw inside the child element, outside every schema boundary.
test('the array editor shows a nested non-string type as no choice', async ({page}) => {
  await page.goto('/notes');
  const result = await page.evaluate(async () => {
    await customElements.whenDefined('schema-array-editor');
    const errors: string[] = [];
    const onError = (e: ErrorEvent) => { errors.push(String(e.error ?? e.message)); };
    const onRejection = (e: PromiseRejectionEvent) => { errors.push(String(e.reason)); };
    window.addEventListener('error', onError);
    window.addEventListener('unhandledrejection', onRejection);
    const element = document.createElement('schema-array-editor') as any;
    element.schema = {type: 'array', items: {type: {toString: null}}, contains: {type: {toString: null}}};
    document.body.append(element);
    await element.updateComplete;
    await new Promise(resolve => setTimeout(resolve, 100));
    window.removeEventListener('error', onError);
    window.removeEventListener('unhandledrejection', onRejection);
    const values = ['arr-items-type', 'arr-contains-type'].map(id => (element.shadowRoot.querySelector(`#${id}`) as HTMLSelectElement | null)?.value);
    element.remove();
    return {errors, values};
  });
  expect(result.errors).toEqual([]);
  expect(result.values).toEqual(['', '']);
});
