import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { freeFields } from './freeFields.js';

// The create forms render freeFields inside `x-if="!currentSchema"`, so switching the
// category destroys and rebuilds it. Whatever init() hangs on the window has to come off
// again, or every rebuild leaves the old component alive behind a listener and an effect.
describe('freeFields lifecycle', () => {
  const runner = () => {};
  let alpine: { effect: ReturnType<typeof vi.fn>; release: ReturnType<typeof vi.fn> };

  beforeEach(() => {
    alpine = { effect: vi.fn(() => runner), release: vi.fn() };
    vi.stubGlobal('window', {
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      Alpine: alpine,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function mount() {
    const field = freeFields({ name: 'Meta', url: '', jsonOutput: true, id: 'f1', title: 'Meta', fromJSON: { a: 1 } }) as any;
    field.$el = { closest: () => null, dispatchEvent: vi.fn() };
    return field;
  }

  it('init() assigns only fields the component declares', async () => {
    const field = mount();
    const declared = new Set(Object.keys(field));
    await field.init();
    expect(Object.keys(field).filter(key => !declared.has(key))).toEqual([]);
  });

  it('destroy() removes the window listener and releases the effect init() created', async () => {
    const field = mount();
    await field.init();
    const [[type, listener]] = vi.mocked(window.addEventListener).mock.calls;
    expect(type).toBe('schema-fields-claimed');
    expect(alpine.effect).toHaveBeenCalledOnce();

    field.destroy();
    expect(window.removeEventListener).toHaveBeenCalledWith('schema-fields-claimed', listener);
    expect(alpine.release).toHaveBeenCalledWith(runner);
  });
});
