import { focusedElement, restoreFocus } from '../utils/focus.js';

const PROVIDERS = [
  { id: 'deepseek', label: 'DeepSeek' },
  { id: 'openrouter', label: 'OpenRouter' },
  { id: 'openai', label: 'OpenAI' },
  { id: 'anthropic', label: 'Anthropic' },
];

const KINDS = [
  { id: 'slot', label: 'CSS slot' },
  { id: 'pair', label: 'HTML/CSS pair' },
  { id: 'plan', label: 'Design plan' },
  { id: 'bundle', label: 'Single-call bundle' },
  { id: 'metaschema', label: 'MetaSchema' },
];

function emptyProfile(provider = 'deepseek') {
  return {
    provider, model: '', max_tokens: '0', timeout: '0s', reason: '', error: '', flash: '',
    kinds: Object.fromEntries(KINDS.map(kind => [kind.id, { model: '', max_tokens: '0', timeout: '0s' }])),
  };
}

function editValue(view) {
  if (!view || view.current == null) return '';
  if (view.type === 'duration') return `${Number(view.current) / 1e9}s`;
  return String(view.current);
}

async function responseData(response) {
  let data;
  try { data = await response.json(); } catch { data = null; }
  if (!response.ok) {
    const errors = Array.isArray(data?.errors)
      ? data.errors.map(error => error?.message).filter(message => typeof message === 'string').join(', ')
      : '';
    const message = errors || data?.error || data?.message || `Request failed (${response.status}).`;
    throw new Error(message);
  }
  return data;
}

export function aiSettings() {
  return {
    providers: PROVIDERS,
    kinds: KINDS,
    features: [{ id: 'mrql', label: 'MRQL generation' }, { id: 'template', label: 'Template generation' }],
    settings: {},
    profiles: { mrql: emptyProfile(), template: emptyProfile() },
    credentials: Object.fromEntries(PROVIDERS.map(provider => [provider.id, {
      value: '', configured: false, bootConfigured: false, overridden: false, error: '', flash: '',
    }])),
    effective: {},
    loading: true,
    loaded: false,
    busy: false,
    error: '',
    effectiveError: '',
    _controller: null,
    _disposed: false,

    async init() {
      this._controller = new AbortController();
      try {
        const response = await fetch('/v1/admin/settings', {
          headers: { Accept: 'application/json' }, signal: this._controller.signal,
        });
        const views = await responseData(response);
        if (this._disposed) return;
        this._adoptViews(views);
        this._loadProfile('mrql');
        this._loadProfile('template');
        this.loaded = true;
        await this._refreshEffective();
      } catch (error) {
        if (!this._disposed) this.error = error.message || 'Could not load AI settings.';
      } finally {
        this.loading = false;
      }
    },

    destroy() {
      this._disposed = true;
      this._controller?.abort();
      for (const credential of Object.values(this.credentials)) credential.value = '';
    },

    _adoptViews(views) {
      if (!Array.isArray(views)) throw new Error('The server returned invalid settings.');
      this.settings = Object.fromEntries(views.filter(view => view.group === 'ai').map(view => [view.key, view]));
      for (const provider of this.providers) {
        const view = this.settings[`ai_${provider.id}_api_key`];
        const credential = this.credentials[provider.id];
        credential.configured = !!view?.configured;
        credential.bootConfigured = !!view?.bootConfigured;
        credential.overridden = !!view?.overridden;
      }
    },

    _loadProfile(feature, provider = this.settings[`ai_${feature}_provider`]?.current || 'deepseek') {
      const previous = this.profiles[feature];
      const profile = emptyProfile(provider);
      profile.reason = previous.reason;
      for (const field of ['model', 'max_tokens', 'timeout']) {
        const value = editValue(this.settings[`ai_${feature}_${provider}_${field}`]);
        profile[field] = value || (field === 'model' ? '' : field === 'timeout' ? '0s' : '0');
      }
      if (feature === 'template') {
        for (const kind of this.kinds) {
          for (const field of ['model', 'max_tokens', 'timeout']) {
            const value = editValue(this.settings[`ai_template_${provider}_${kind.id}_${field}`]);
            profile.kinds[kind.id][field] = value || (field === 'model' ? '' : field === 'timeout' ? '0s' : '0');
          }
        }
      }
      this.profiles[feature] = profile;
    },

    selectProvider(feature) {
      this._loadProfile(feature, this.profiles[feature].provider);
    },

    providerLabel(id) {
      return this.providers.find(provider => provider.id === id)?.label || id;
    },

    kindLabel(id) {
      return this.kinds.find(kind => kind.id === id)?.label || id;
    },

    reasoningLabel(mode) {
      return mode === 'provider_default' ? 'model default reasoning' : `thinking ${mode}`;
    },

    async _refreshEffective() {
      try {
        const response = await fetch('/v1/admin/settings/ai-profiles', {
          headers: { Accept: 'application/json' }, signal: this._controller?.signal,
        });
        const effective = await responseData(response);
        if (!this._disposed) {
          this.effective = effective || {};
          this.effectiveError = '';
        }
      } catch {
        if (!this._disposed) {
          this.effective = {};
          this.effectiveError = 'Could not refresh effective profiles. Reload to check their current values.';
        }
      }
    },

    async _apply(values, reset, reason, target) {
      if (this.busy || !this.loaded || this._disposed) return false;
      const previousFocus = globalThis.document ? focusedElement() : null;
      this.busy = true;
      target.error = '';
      target.flash = '';
      try {
        const response = await fetch('/v1/admin/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
          body: JSON.stringify({ values, reset, reason }),
          signal: this._controller?.signal,
        });
        const views = await responseData(response);
        if (this._disposed) return false;
        this._adoptViews(views);
        await this._refreshEffective();
        return !this._disposed;
      } catch (error) {
        if (!this._disposed) target.error = error.message || 'Could not save AI settings.';
        return false;
      } finally {
        this.busy = false;
        if (previousFocus) this.$nextTick?.(() => {
          // Disabling the fieldset removes focus from its active control. Wait
          // until Alpine enables it again, without overriding a deliberate move.
          if (!this._disposed && !this.busy && previousFocus.isConnected && !focusedElement()) {
            restoreFocus(previousFocus);
          }
        });
      }
    },

    async saveProfile(feature) {
      const profile = this.profiles[feature];
      profile.error = '';
      profile.flash = '';
      if (!profile.model.trim()) {
        profile.error = 'Enter a model ID for the selected provider.';
        return;
      }
      const prefix = `ai_${feature}_${profile.provider}_`;
      const values = { [`ai_${feature}_provider`]: profile.provider };
      const addFields = (fields, keyPrefix) => {
        const tokens = fields.max_tokens === '' ? '0' : String(fields.max_tokens);
        if (!/^\d+$/.test(tokens) || Number(tokens) > 65536) {
          profile.error = 'Output token limits must be whole numbers from 0 to 65536.';
          return false;
        }
        values[keyPrefix + 'model'] = fields.model.trim();
        values[keyPrefix + 'max_tokens'] = tokens;
        values[keyPrefix + 'timeout'] = fields.timeout.trim() || '0s';
        return true;
      };
      if (!addFields(profile, prefix)) return;
      if (feature === 'template') {
        for (const kind of this.kinds) {
          if (!addFields(profile.kinds[kind.id], `${prefix}${kind.id}_`)) return;
        }
      }
      if (await this._apply(values, [], profile.reason, profile)) {
        this._loadProfile(feature);
        this.profiles[feature].flash = 'Saved. New generation requests use this profile.';
      }
    },

    async resetProfile(feature) {
      const profile = this.profiles[feature];
      const keys = Object.keys(this.settings).filter(key => key.startsWith(`ai_${feature}_`));
      if (await this._apply({}, keys, profile.reason, profile)) {
        this._loadProfile(feature);
        this.profiles[feature].flash = 'Reset this feature to boot defaults.';
      }
    },

    async replaceCredential(provider) {
      const credential = this.credentials[provider];
      credential.error = '';
      credential.flash = '';
      if (!credential.value) {
        credential.error = 'Enter a new API key to replace the saved key.';
        return;
      }
      if (this.busy || !this.loaded || this._disposed) return;
      const value = credential.value;
      credential.value = '';
      if (await this._apply({ [`ai_${provider}_api_key`]: value }, [], '', credential)) {
        credential.flash = 'Key replaced.';
      }
    },

    async clearCredential(provider) {
      const credential = this.credentials[provider];
      credential.value = '';
      if (await this._apply({ [`ai_${provider}_api_key`]: '' }, [], '', credential)) {
        credential.flash = 'Key cleared. Environment fallback is disabled until Reset.';
      }
    },

    async resetCredential(provider) {
      const credential = this.credentials[provider];
      credential.value = '';
      if (await this._apply({}, [`ai_${provider}_api_key`], '', credential)) {
        credential.flash = 'Key reset to the environment fallback.';
      }
    },
  };
}
