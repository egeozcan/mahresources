import {test, expect} from '../../fixtures/base.fixture';

// A conditional whose branches set the same field to opposite consts made the
// form flip the field forever (also at base) once a wrapper echoed each
// emitted value back, as schema-editor does. Automatic assignments now stop
// once they keep recreating the same values, and an echo of the form's own
// value does not count as a new value from outside.
test('a contradictory conditional settles in the form', async ({page}) => {
  await page.goto('/notes');
  const result = await page.evaluate(async () => {
    await customElements.whenDefined('schema-editor');
    const element = document.createElement('schema-editor');
    let emits = 0;
    element.addEventListener('value-change', () => { emits++; });
    element.setAttribute('mode', 'form');
    element.setAttribute('schema', JSON.stringify({
      type: 'object',
      properties: {flag: {type: 'boolean'}},
      required: ['flag'],
      if: {properties: {flag: {const: true}}, required: ['flag']},
      then: {properties: {flag: {const: false}}},
      else: {properties: {flag: {const: true}}},
    }));
    element.setAttribute('value', '{}');
    document.body.append(element);
    await new Promise(resolve => setTimeout(resolve, 500));
    const afterHalfSecond = emits;
    await new Promise(resolve => setTimeout(resolve, 500));
    const afterSecond = emits;
    element.remove();
    return {afterHalfSecond, afterSecond};
  });
  expect(result.afterSecond).toBe(result.afterHalfSecond);
  expect(result.afterSecond).toBeLessThan(10);
});
