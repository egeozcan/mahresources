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

// Nested nullable variants with required children, through schema-editor's
// echo: base settles on the full nested default. A guard that counted
// repeated values stopped the last correction and lost lines.
test('nested nullable variants with required children settle on their full default', async ({page}) => {
  await page.goto('/notes');
  const value = await page.evaluate(async () => {
    await customElements.whenDefined('schema-editor');
    const element = document.createElement('schema-editor');
    element.setAttribute('mode', 'form');
    element.setAttribute('schema', JSON.stringify({
      type: 'object',
      properties: {
        shipping: {oneOf: [
          {type: 'object', required: ['address'], properties: {
            address: {oneOf: [
              {type: 'object', required: ['lines'], properties: {lines: {type: 'array', items: {type: 'string'}}}},
              {type: 'null'},
            ]},
          }},
          {type: 'null'},
        ]},
      },
    }));
    element.setAttribute('value', '{}');
    document.body.append(element);
    await new Promise(resolve => setTimeout(resolve, 500));
    const hidden = element.querySelector('schema-form-mode input[type="hidden"]') as HTMLInputElement | null;
    const result = hidden?.value ?? null;
    element.remove();
    return result;
  });
  expect(JSON.parse(value ?? 'null')).toEqual({shipping: {address: {lines: []}}});
});

async function settleThroughWrapper(page: any, schema: unknown, value: string) {
  return page.evaluate(async ({schema, value}: {schema: unknown; value: string}) => {
    await customElements.whenDefined('schema-editor');
    const element = document.createElement('schema-editor');
    let emits = 0;
    element.addEventListener('value-change', () => { emits++; });
    element.setAttribute('mode', 'form');
    element.setAttribute('schema', JSON.stringify(schema));
    element.setAttribute('value', value);
    document.body.append(element);
    await new Promise(resolve => setTimeout(resolve, 500));
    const afterHalfSecond = emits;
    await new Promise(resolve => setTimeout(resolve, 500));
    const hidden = element.querySelector('schema-form-mode input[type="hidden"]') as HTMLInputElement | null;
    const result = {afterHalfSecond, afterSecond: emits, value: hidden ? JSON.parse(hidden.value) : null};
    element.remove();
    return result;
  }, {schema, value});
}

// An initialisation the conditional's cleanup removes again looped forever
// (also at base); the cycle of whole values is now stopped.
test('an initialisation undone by conditional cleanup settles', async ({page}) => {
  await page.goto('/notes');
  const result = await settleThroughWrapper(page, {
    if: {required: ['enabled']},
    then: {properties: {name: {type: 'string'}}},
    else: {properties: {enabled: {type: 'boolean'}}, required: ['enabled']},
  }, '{}');
  expect(result.afterSecond).toBe(result.afterHalfSecond);
  expect(result.afterSecond).toBeLessThan(10);
});

// A const transition the active branch requires is made, as at base.
test('a conditional const settles on its active branch', async ({page}) => {
  await page.goto('/notes');
  const result = await settleThroughWrapper(page, {
    type: 'object',
    properties: {reviewed: {type: 'boolean'}, status: {type: 'string'}},
    required: ['reviewed'],
    if: {properties: {reviewed: {const: true}}, required: ['reviewed']},
    then: {properties: {status: {const: 'approved'}}},
    else: {properties: {status: {const: 'pending'}}},
  }, '{"status":"pending"}');
  expect(result.afterSecond).toBe(result.afterHalfSecond);
  expect(result.value).toEqual({status: 'pending', reviewed: false});
});
