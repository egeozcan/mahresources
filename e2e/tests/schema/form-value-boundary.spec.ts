import {test, expect} from '../../fixtures/base.fixture';

test('schema form preserves serialized scalar, array and object values', async ({page}) => {
  await page.goto('/notes');
  const cases = [
    {type:'string',value:'high'},
    {type:'string',value:'123'},
    {type:'string',value:'false'},
    {type:'string',value:'null'},
    {type:'string',value:'{"tag":"value"}'},
    {type:'number',value:42},
    {type:'boolean',value:false},
    {type:'null',value:null},
    {type:'array',value:['one']},
    {type:'object',value:{name:'Example'}},
  ];
  for (const entry of cases) {
    await page.evaluate(async entry => {
      document.querySelector('[data-testid="value-boundary"]')?.remove();
      await customElements.whenDefined('schema-editor');
      const element = document.createElement('schema-editor');
      element.setAttribute('data-testid','value-boundary');
      element.setAttribute('mode','form');
      element.setAttribute('schema',JSON.stringify({type:entry.type}));
      element.setAttribute('value',JSON.stringify(entry.value));
      document.body.append(element);
    },entry);
    const submitted = page.getByTestId('value-boundary').locator('schema-form-mode input[type="hidden"]');
    await expect(submitted,JSON.stringify(entry)).toHaveValue(JSON.stringify(entry.value),{timeout:1000});
  }
});
