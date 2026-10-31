import { afterEach, describe, expect, test, vi } from 'vitest';
import { aiSettings } from './aiSettings.js';

const providers = ['deepseek', 'openrouter', 'openai', 'anthropic'];
const kinds = ['slot', 'pair', 'plan', 'bundle', 'metaschema'];

function setting(key: string, current: string | number | null, type = 'string') {
  return { key, current, bootDefault: current, type, group: 'ai', overridden: false };
}

function settings() {
  const rows: any[] = [];
  for (const provider of providers) {
    rows.push({ ...setting(`ai_${provider}_api_key`, null), secret: true, configured: false, bootConfigured: false });
    for (const feature of ['mrql', 'template']) {
      rows.push(setting(`ai_${feature}_${provider}_model`, provider === 'deepseek' ? 'deepseek-flash' : ''));
      rows.push(setting(`ai_${feature}_${provider}_max_tokens`, 0, 'int'));
      rows.push(setting(`ai_${feature}_${provider}_timeout`, 0, 'duration'));
      if (feature === 'template') {
        for (const kind of kinds) {
          rows.push(setting(`ai_template_${provider}_${kind}_model`, ''));
          rows.push(setting(`ai_template_${provider}_${kind}_max_tokens`, 0, 'int'));
          rows.push(setting(`ai_template_${provider}_${kind}_timeout`, 0, 'duration'));
        }
      }
    }
  }
  rows.push(setting('ai_mrql_provider', 'deepseek'), setting('ai_template_provider', 'deepseek'));
  return rows;
}

function serve(rows = settings()) {
  const fetcher = vi.fn(async (url: string, options?: RequestInit) => {
    if (url.endsWith('/ai-profiles')) {
      return Response.json({
        mrql: { provider: 'deepseek', model: 'deepseek-flash', maxTokens: 800, timeout: '20s', configured: false },
        template: { provider: 'deepseek', model: 'deepseek-flash', configured: false, profiles: [] },
      });
    }
    return Response.json(rows);
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

function lastPayload(fetcher: ReturnType<typeof vi.fn>) {
  const calls = fetcher.mock.calls.filter(([, options]) => options?.method === 'PUT');
  return JSON.parse(calls.at(-1)![1].body);
}

afterEach(() => vi.unstubAllGlobals());

describe('AI settings profiles', () => {
  test('loads provider-specific selections and duration values for editing', async () => {
    const rows = settings();
    rows.find(row => row.key === 'ai_mrql_provider').current = 'openai';
    rows.find(row => row.key === 'ai_mrql_openai_model').current = 'my-model';
    rows.find(row => row.key === 'ai_mrql_openai_timeout').current = 90e9;
    serve(rows);
    const component = aiSettings();
    await component.init();
    expect(component.loading).toBe(false);
    expect(component.profiles.mrql.provider).toBe('openai');
    expect(component.profiles.mrql.model).toBe('my-model');
    expect(component.profiles.mrql.timeout).toBe('90s');
    expect(component.credentials.openai.value).toBe('');
  });

  test('switching provider restores its saved models without borrowing old ones', async () => {
    const rows = settings();
    rows.find(row => row.key === 'ai_template_deepseek_pair_model').current = 'deepseek-pair';
    rows.find(row => row.key === 'ai_template_openai_model').current = 'openai-saved';
    serve(rows);
    const component = aiSettings();
    await component.init();
    component.profiles.template.provider = 'openai';
    component.selectProvider('template');
    expect(component.profiles.template.model).toBe('openai-saved');
    expect(component.profiles.template.kinds.pair.model).toBe('');
    component.profiles.template.provider = 'deepseek';
    component.selectProvider('template');
    expect(component.profiles.template.kinds.pair.model).toBe('deepseek-pair');
  });

  test('saves a provider and all its profile fields atomically, without credentials', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    component.profiles.template.provider = 'anthropic';
    component.selectProvider('template');
    component.profiles.template.model = 'claude-custom';
    component.profiles.template.kinds.pair.model = 'claude-pair';
    component.credentials.anthropic.value = 'unsubmitted-secret';
    await component.saveProfile('template');
    const payload = lastPayload(fetcher);
    expect(payload.values.ai_template_provider).toBe('anthropic');
    expect(payload.values.ai_template_anthropic_model).toBe('claude-custom');
    expect(payload.values.ai_template_anthropic_pair_model).toBe('claude-pair');
    expect(Object.keys(payload.values)).toHaveLength(19);
    expect(JSON.stringify(payload)).not.toContain('api_key');
    expect(JSON.stringify(payload)).not.toContain('unsubmitted-secret');
    expect(component.credentials.anthropic.value).toBe('unsubmitted-secret');
  });

  test('does not submit a profile without a model', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    component.profiles.mrql.provider = 'openai';
    component.selectProvider('mrql');
    await component.saveProfile('mrql');
    expect(component.profiles.mrql.error).toContain('model');
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(0);
  });

  test('rejects a token budget outside the server bounds before submission', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    component.profiles.mrql.max_tokens = '65537';
    await component.saveProfile('mrql');
    expect(component.profiles.mrql.error).toContain('65536');
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(0);
  });

  test('resets all models and limits of one feature together without touching keys or the other feature', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    await component.resetProfile('mrql');
    const payload = lastPayload(fetcher);
    expect(payload.reset).toContain('ai_mrql_provider');
    expect(payload.reset).toContain('ai_mrql_openai_model');
    expect(payload.reset).toContain('ai_mrql_anthropic_timeout');
    expect(payload.reset).toHaveLength(13);
    expect(payload.reset.every((key: string) => key.startsWith('ai_mrql_'))).toBe(true);
    expect(payload.values).toEqual({});
  });

  test('saving a key preserves edits to a profile', async () => {
    serve();
    const component = aiSettings();
    await component.init();
    component.profiles.mrql.model = 'unsaved-draft-model';
    component.credentials.openai.value = 'new-key';
    await component.replaceCredential('openai');
    expect(component.profiles.mrql.model).toBe('unsaved-draft-model');
  });

  test('validation failures show the API errors message without discarding the draft', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    fetcher.mockImplementation(async () => Response.json({ errors: [{ message: 'Call timeout must not exceed 10m.' }] }, { status: 400 }));
    component.profiles.mrql.timeout = '11m';
    await component.saveProfile('mrql');
    expect(component.profiles.mrql.error).toBe('Call timeout must not exceed 10m.');
    expect(component.profiles.mrql.timeout).toBe('11m');
    expect(component.busy).toBe(false);
  });

  test('a successful save remains successful when refreshing effective values fails', async () => {
    const rows = settings();
    const fetcher = serve(rows);
    const component = aiSettings();
    await component.init();
    fetcher.mockImplementation(async (url: string) => url.endsWith('/ai-profiles')
      ? Response.json({ error: 'Unavailable' }, { status: 503 }) : Response.json(rows));
    await component.saveProfile('mrql');
    expect(component.profiles.mrql.flash).toContain('Saved.');
    expect(component.profiles.mrql.error).toBe('');
    expect(component.effectiveError).toContain('Could not refresh');
    expect(component.effective).toEqual({});
  });

  test('a pending mutation prevents duplicate submissions', async () => {
    const rows = settings();
    const fetcher = serve(rows);
    const component = aiSettings();
    await component.init();
    let finish!: (response: Response) => void;
    fetcher.mockImplementation(async (_url: string, options?: RequestInit) => options?.method === 'PUT'
      ? new Promise<Response>(resolve => { finish = resolve; }) : Response.json({}));
    const first = component.saveProfile('mrql');
    await component.saveProfile('mrql');
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(1);
    finish(Response.json(rows));
    await first;
    expect(component.busy).toBe(false);
  });

  for (const destination of ['nowhere', 'another control', 'disconnected input']) {
    test(`restores lost save focus after the DOM update, respecting ${destination}`, async () => {
      const rows = settings();
      const fetcher = serve(rows);
      const component = aiSettings();
      await component.init();
      const doc: any = { body: {}, documentElement: {} };
      const input = {
        isConnected: true,
        matches: () => true,
        focus: vi.fn(() => { doc.activeElement = input; }),
      };
      doc.activeElement = input;
      vi.stubGlobal('document', doc);
      const nextTicks: Array<() => void> = [];
      component.$nextTick = (callback: () => void) => { nextTicks.push(callback); };
      fetcher.mockImplementation(async (_url: string, options?: RequestInit) => {
        // Disabling the fieldset while saving drops browser focus to body.
        if (options?.method === 'PUT') doc.activeElement = doc.body;
        return Response.json(options?.method === 'PUT' ? rows : {});
      });
      await component.saveProfile('mrql');
      expect(component.busy).toBe(false);
      expect(input.focus).not.toHaveBeenCalled();
      if (destination === 'another control') doc.activeElement = {};
      if (destination === 'disconnected input') input.isConnected = false;
      for (const nextTick of nextTicks) nextTick();
      expect(input.focus).toHaveBeenCalledTimes(destination === 'nowhere' ? 1 : 0);
    });
  }
});

describe('write-only credential actions', () => {
  test('an empty Replace does not clear an existing key', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    await component.replaceCredential('openai');
    expect(component.credentials.openai.error).toContain('Enter');
    expect(fetcher.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(0);
  });

  test('Replace sends only that key and discards the input', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    component.credentials.openai.value = 'new-secret';
    await component.replaceCredential('openai');
    expect(lastPayload(fetcher).values).toEqual({ ai_openai_api_key: 'new-secret' });
    expect(component.credentials.openai.value).toBe('');
  });

  test('Clear saves an explicit empty override while Reset removes it', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    await component.clearCredential('openrouter');
    expect(lastPayload(fetcher)).toMatchObject({ values: { ai_openrouter_api_key: '' }, reset: [] });
    await component.resetCredential('openrouter');
    expect(lastPayload(fetcher)).toMatchObject({ values: {}, reset: ['ai_openrouter_api_key'] });
  });

  test('a failed replacement clears the submitted secret and reports a sanitized error', async () => {
    const fetcher = serve();
    const component = aiSettings();
    await component.init();
    fetcher.mockImplementation(async (_url: string, options?: RequestInit) => options?.method === 'PUT'
      ? Response.json({ error: 'Could not save settings.' }, { status: 500 }) : Response.json([]));
    component.credentials.anthropic.value = 'secret-that-must-be-cleared';
    await component.replaceCredential('anthropic');
    expect(component.credentials.anthropic.value).toBe('');
    expect(component.credentials.anthropic.error).toBe('Could not save settings.');
    expect(component.busy).toBe(false);
  });

  test('teardown discards every unsent credential', async () => {
    serve();
    const component = aiSettings();
    await component.init();
    component.credentials.deepseek.value = 'unsubmitted-key';
    component.destroy();
    expect(component.credentials.deepseek.value).toBe('');
  });
});
