<section class="rounded-lg bg-white border border-stone-200 p-5 space-y-5"
         aria-labelledby="ai-settings-heading" data-testid="ai-settings" x-data="aiSettings()" :aria-busy="loading || busy">
  <header class="space-y-1">
    <h2 id="ai-settings-heading" class="text-base font-semibold font-mono text-stone-800">AI generation</h2>
    <p class="text-sm text-stone-600">Choose providers and models for new generation requests. Each provider keeps its own saved models. Model IDs must support text generation; saving does not make a provider call.</p>
  </header>
  <p x-show="loading" class="text-sm text-stone-600" role="status">Loading AI settings…</p>
  <p x-show="error" x-text="error" class="text-sm text-red-700" role="alert"></p>
  <p x-show="effectiveError" x-text="effectiveError" class="text-sm text-amber-800" role="status"></p>

  <div x-show="loaded" x-cloak class="space-y-6">
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-5">
      <template x-for="feature in features" :key="feature.id">
        <form class="border border-stone-200 rounded-md p-4 min-w-0" novalidate
              :data-testid="'ai-' + feature.id + '-profile'" @submit.prevent="saveProfile(feature.id)">
          <fieldset class="min-w-0 space-y-3" :disabled="busy || loading">
            <legend class="font-semibold text-stone-800 mb-3" x-text="feature.label"></legend>
            <div>
              <label :for="'ai-' + feature.id + '-provider'" class="block text-sm font-medium text-stone-800">Provider</label>
              <select :id="'ai-' + feature.id + '-provider'" x-model="profiles[feature.id].provider" @change="selectProvider(feature.id)"
                      class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm focus:outline-hidden focus:ring-2 focus:ring-amber-500">
                <template x-for="provider in providers" :key="provider.id">
                  <option :value="provider.id" x-text="provider.label"></option>
                </template>
              </select>
            </div>
            <div>
              <label :for="'ai-' + feature.id + '-model'" class="block text-sm font-medium text-stone-800">Model ID</label>
              <input type="text" :id="'ai-' + feature.id + '-model'" x-model="profiles[feature.id].model" required autocomplete="off" spellcheck="false"
                     :aria-describedby="'ai-' + feature.id + '-model-hint'"
                     class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
              <p :id="'ai-' + feature.id + '-model-hint'" class="text-xs text-stone-500 mt-1">Enter the exact model ID from the selected provider.</p>
            </div>

            <details class="border-t border-stone-200 pt-3">
              <summary class="text-sm font-medium text-stone-700 cursor-pointer focus:outline-hidden focus:ring-2 focus:ring-amber-500">Advanced limits and template call profiles</summary>
              <div class="space-y-4 mt-3">
                <p class="text-xs text-stone-500">A limit of 0 or timeout of 0s uses the provider default. Timeouts use durations such as 60s or 2m. Output limits include reasoning where the provider counts it.</p>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label :for="'ai-' + feature.id + '-tokens'" class="block text-sm font-medium text-stone-800">Default output token limit</label>
                    <input type="number" min="0" max="65536" step="1" :id="'ai-' + feature.id + '-tokens'" x-model="profiles[feature.id].max_tokens"
                           class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
                  </div>
                  <div>
                    <label :for="'ai-' + feature.id + '-timeout'" class="block text-sm font-medium text-stone-800">Default call timeout</label>
                    <input type="text" :id="'ai-' + feature.id + '-timeout'" x-model="profiles[feature.id].timeout" spellcheck="false"
                           class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
                  </div>
                </div>
                <template x-if="feature.id === 'template'">
                  <div class="space-y-3">
                    <p class="text-xs text-stone-500">Leave a call's model empty to use the default model. Its 0 limits inherit the call profile shown below.</p>
                    <template x-for="kind in kinds" :key="kind.id">
                      <fieldset class="border border-stone-200 rounded p-3 min-w-0 space-y-2">
                        <legend class="text-sm font-medium text-stone-700 px-1" x-text="kind.label"></legend>
                        <div>
                          <label :for="'ai-template-' + kind.id + '-model'" class="block text-xs font-medium text-stone-700">Model override</label>
                          <input type="text" :id="'ai-template-' + kind.id + '-model'" x-model="profiles.template.kinds[kind.id].model" autocomplete="off" spellcheck="false"
                                 class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
                        </div>
                        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                          <div>
                            <label :for="'ai-template-' + kind.id + '-tokens'" class="block text-xs font-medium text-stone-700">Output token limit</label>
                            <input type="number" min="0" max="65536" step="1" :id="'ai-template-' + kind.id + '-tokens'" x-model="profiles.template.kinds[kind.id].max_tokens"
                                   class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
                          </div>
                          <div>
                            <label :for="'ai-template-' + kind.id + '-timeout'" class="block text-xs font-medium text-stone-700">Call timeout</label>
                            <input type="text" :id="'ai-template-' + kind.id + '-timeout'" x-model="profiles.template.kinds[kind.id].timeout" spellcheck="false"
                                   class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
                          </div>
                        </div>
                      </fieldset>
                    </template>
                  </div>
                </template>
              </div>
            </details>

            <template x-if="effective[feature.id]">
              <div class="text-xs text-stone-600 space-y-1" :data-testid="'ai-' + feature.id + '-effective'">
                <p><span class="font-medium">Currently saved:</span> <span x-text="providerLabel(effective[feature.id].provider)"></span> / <span class="font-mono" x-text="effective[feature.id].model || 'no model configured'"></span></p>
                <p x-text="effective[feature.id].configured ? 'Ready to generate.' : 'Unavailable until the selected provider has a key and model.'"></p>
                <template x-if="feature.id === 'mrql'">
                  <p x-text="effective.mrql.maxTokens + ' output tokens; ' + effective.mrql.timeout + ' timeout.'"></p>
                </template>
                <template x-if="feature.id === 'template'">
                  <div class="space-y-1">
                    <p x-text="'Whole-template deadline: ' + effective.template.bundleDeadline + '. Per-call timeouts do not extend this deadline.'"></p>
                    <ul class="list-disc pl-5 space-y-1">
                      <template x-for="profile in (effective.template.profiles || [])" :key="profile.kind">
                        <li><span x-text="kindLabel(profile.kind)"></span>: <span class="font-mono" x-text="profile.model"></span>, <span x-text="profile.maxTokens + ' tokens; ' + profile.timeout + '; ' + reasoningLabel(profile.reasoningMode)"></span></li>
                      </template>
                    </ul>
                  </div>
                </template>
              </div>
            </template>

            <div>
              <label :for="'ai-' + feature.id + '-reason'" class="block text-xs font-medium text-stone-700">Reason (optional)</label>
              <input type="text" :id="'ai-' + feature.id + '-reason'" x-model="profiles[feature.id].reason"
                     class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
            </div>
            <div class="flex flex-wrap gap-2">
              <button type="submit" class="inline-flex items-center px-3 py-1 text-sm font-medium text-white bg-amber-700 rounded hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600">Save profile</button>
              <button type="button" @click="resetProfile(feature.id)" class="inline-flex items-center px-3 py-1 text-sm font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-stone-400">Reset feature</button>
            </div>
          </fieldset>
          <div class="text-xs mt-2 min-h-[1.25rem]" role="status" aria-live="polite">
            <span x-show="profiles[feature.id].flash" x-text="profiles[feature.id].flash" class="text-green-700"></span>
            <span x-show="profiles[feature.id].error" x-text="profiles[feature.id].error" class="text-red-700"></span>
          </div>
        </form>
      </template>
    </div>

    <div class="space-y-3">
      <h3 class="text-sm font-semibold text-stone-800">Provider API keys</h3>
      <p class="text-xs text-stone-600">Saved keys are never sent back to this page. Replace changes a key; Clear disables its environment fallback; Reset restores that fallback. Keys persist unencrypted in the server database.</p>
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <template x-for="provider in providers" :key="provider.id">
          <form class="border border-stone-200 rounded-md p-4 min-w-0" novalidate
                :data-testid="'ai-key-' + provider.id" @submit.prevent="replaceCredential(provider.id)">
            <fieldset class="min-w-0 space-y-2" :disabled="busy || loading">
              <legend class="font-medium text-sm text-stone-800 mb-2" x-text="provider.label"></legend>
              <p class="text-xs text-stone-600" :data-testid="'ai-key-' + provider.id + '-status'" x-text="credentials[provider.id].configured ? 'Key configured' : 'No key configured'"></p>
              <p class="text-xs text-stone-500" x-text="credentials[provider.id].bootConfigured ? 'Environment fallback configured.' : 'No environment fallback configured.'"></p>
              <div>
                <label :for="'ai-key-' + provider.id + '-input'" class="block text-xs font-medium text-stone-700">New API key</label>
                <input type="password" :id="'ai-key-' + provider.id + '-input'" x-model="credentials[provider.id].value" autocomplete="new-password" spellcheck="false"
                       class="mt-1 w-full border border-stone-300 rounded px-2 py-1 text-sm font-mono focus:outline-hidden focus:ring-2 focus:ring-amber-500" />
              </div>
              <div class="flex flex-wrap gap-2">
                <button type="submit" class="inline-flex items-center px-3 py-1 text-sm font-medium text-white bg-amber-700 rounded hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-amber-600">Replace key</button>
                <button type="button" @click="clearCredential(provider.id)" class="inline-flex items-center px-3 py-1 text-sm font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-stone-400">Clear key</button>
                <button type="button" @click="resetCredential(provider.id)" class="inline-flex items-center px-3 py-1 text-sm font-medium text-stone-700 bg-stone-100 border border-stone-300 rounded hover:bg-stone-200 focus:outline-hidden focus:ring-2 focus:ring-offset-1 focus:ring-stone-400">Reset key</button>
              </div>
            </fieldset>
            <div class="text-xs mt-2 min-h-[1.25rem]" role="status" aria-live="polite">
              <span x-show="credentials[provider.id].flash" x-text="credentials[provider.id].flash" class="text-green-700"></span>
              <span x-show="credentials[provider.id].error" x-text="credentials[provider.id].error" class="text-red-700"></span>
            </div>
          </form>
        </template>
      </div>
    </div>
  </div>
</section>
