import { describe, expect, it } from 'vitest';
import { registerGenerationDisclosureStore } from './generationDisclosure.js';

function makeStore() {
  let state: any;
  registerGenerationDisclosureStore({ store: (_name: string, value: any) => { state = value; } });
  return state;
}

describe('generation provider disclosure', () => {
  it('describes provider-default reasoning without claiming it is disabled', () => {
    const state = makeStore();
    state.disclosure = { destination: { profiles: [{ kind: 'slot', model: 'model', reasoningMode: 'provider_default', thinking: false, maxTokens: 8192, maxCalls: 2, timeout: '1m0s' }] } };
    expect(state.profiles[0]).toContain('reasoning: provider default');
    expect(state.profiles[0]).not.toContain('thinking off');
    expect(state.profiles[0]).toContain('timeout 1m0s');
  });

  it('shows the pinned native API version and prompt-based JSON contract', () => {
    const state = makeStore();
    state.disclosure = { destination: { endpoint: 'https://api.anthropic.com/v1/messages' }, transport: { contentType: 'application/json', authorization: 'Bearer (key hidden)', stream: false, responseFormat: 'prompt_json', apiVersion: '2026-10-21' } };
    expect(state.transport).toContain('Anthropic-Version: 2026-10-21');
    expect(state.transport.join(' ')).toContain('JSON requested in prompt');
    expect(state.transport.join(' ')).not.toContain('response_format: prompt_json');
  });
});
