{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-8 max-w-xl" x-data="accountSecurity()">
  <section>
    <h1 class="text-xl font-mono font-semibold mb-1">Account</h1>
    {% if account %}
    <p class="text-stone-600 text-sm">Signed in as <span class="font-mono">{{ account.Username }}</span> ({{ account.Role }}).</p>
    {% else %}
    <p class="text-stone-600 text-sm">Account management is only available when authentication is enabled.</p>
    {% endif %}
  </section>

  {% if account %}
  <section>
    <h2 class="text-lg font-mono font-semibold mb-3">Change password</h2>
    <form class="space-y-4" @submit.prevent="changePassword">
      <div>
        <label for="cur-pw" class="block text-sm font-mono mb-1">Current password</label>
        <input id="cur-pw" name="currentPassword" type="password" required autocomplete="current-password"
               x-model="currentPassword"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="new-pw" class="block text-sm font-mono mb-1">New password</label>
        <input id="new-pw" name="newPassword" type="password" required autocomplete="new-password"
               minlength="{{ minPasswordLength }}" aria-describedby="account-password-policy account-password-mismatch"
               x-model="newPassword" @input="passwordError = ''"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="confirm-pw" class="block text-sm font-mono mb-1">Confirm new password</label>
        <input id="confirm-pw" name="confirmPassword" type="password" required autocomplete="new-password"
               minlength="{{ minPasswordLength }}" aria-describedby="account-password-policy account-password-mismatch"
               x-model="confirmPassword" @input="passwordError = ''"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <p id="account-password-policy" class="text-xs text-stone-500">Use at least {{ minPasswordLength }} Unicode code points and at most {{ maxPasswordBytes }} UTF-8 bytes.</p>
      <p id="account-password-mismatch" x-show="passwordError" x-cloak role="alert" class="text-sm text-red-800" x-text="passwordError"></p>
      <p x-show="passwordSuccess" x-cloak role="status" aria-live="polite" class="text-sm text-green-800" x-text="passwordSuccess"></p>
      <button type="submit" :disabled="busy" class="bg-amber-700 hover:bg-amber-800 disabled:opacity-60 text-white font-mono py-2 px-4 rounded">
        <span x-text="busy ? 'Updating…' : 'Update password'">Update password</span>
      </button>
    </form>
  </section>

  <section>
    <div class="flex items-center justify-between gap-3 mb-3">
      <h2 class="text-lg font-mono font-semibold">API tokens</h2>
      <button type="button" @click="refreshTokens" :disabled="busy" class="text-sm text-amber-700 hover:underline disabled:opacity-60">Refresh tokens</button>
    </div>
    <p class="text-stone-600 text-sm mb-3">
      Use API tokens with the <code>mr</code> CLI or other clients via the <code>Authorization: Bearer</code> header.
      {% if docsLinksEnabled %}
      <a href="{{ docsURL("features/authentication") }}" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Authentication docs</a>
      {% endif %}
    </p>

    <div x-show="error" x-cloak class="mb-4 rounded-md bg-red-50 border border-red-200 p-3 text-sm text-red-800" role="alert">
      <span x-text="error"></span>
    </div>

    <div x-show="pendingRawToken" x-cloak class="mb-4 rounded bg-green-50 border border-green-200 p-3 text-sm" role="status" aria-live="polite">
      <p class="mb-1 font-medium text-green-800">New token (copy it now — it won't be shown again):</p>
      <code class="block break-all font-mono" x-text="pendingRawToken"></code>
      <button type="button" @click="dismissToken" class="mt-2 text-amber-800 hover:underline">I have saved this token</button>
    </div>

    <div class="flex items-end gap-2 mb-4">
      <div class="flex-1">
        <label for="tok-name" class="block text-sm font-mono mb-1">New token label</label>
        <input id="tok-name" type="text" x-model="name" placeholder="e.g. laptop"
               :disabled="busy || !!pendingRawToken"
               class="w-full border border-stone-300 rounded px-3 py-2 disabled:opacity-60">
      </div>
      <button type="button" @click="createToken" :disabled="busy || !!pendingRawToken"
              class="bg-amber-700 hover:bg-amber-800 disabled:opacity-60 text-white font-mono py-2 px-4 rounded">
        <span x-text="busy ? 'Creating…' : 'Create'">Create</span>
      </button>
    </div>

    <div class="overflow-x-auto">
      <table class="min-w-full text-sm border-collapse">
        <caption class="sr-only">Your API tokens</caption>
        <thead>
          <tr class="text-left border-b border-stone-300 font-mono">
            <th scope="col" class="py-2 pr-4">ID</th>
            <th scope="col" class="py-2 pr-4">Name</th>
            <th scope="col" class="py-2 pr-4">Prefix</th>
            <th scope="col" class="py-2 pr-4">Last used</th>
            <th scope="col" class="py-2 pr-4">Actions</th>
          </tr>
        </thead>
        <tbody>
          <template x-for="token in tokens" :key="token.id">
            <tr class="border-b border-stone-100">
              <td class="py-2 pr-4" x-text="token.id"></td>
              <td class="py-2 pr-4" x-text="token.name"></td>
              <td class="py-2 pr-4 font-mono"><span x-text="token.prefix"></span>…</td>
              <td class="py-2 pr-4" x-text="token.lastUsedAtDisplay"></td>
              <td class="py-2 pr-4">
                <form method="post" action="/v1/account/tokens/delete" class="inline"
                      x-data="confirmAction({ message: `Revoke API token ${token.name || token.prefix}?` })" x-bind="events">
                  <input type="hidden" name="id" :value="token.id">
                  <button type="submit" :aria-label="`Revoke API token ${token.name || token.prefix}`" class="text-red-700 hover:underline">Revoke</button>
                </form>
              </td>
            </tr>
          </template>
          <tr x-show="tokens.length === 0">
            <td colspan="5" class="py-3 text-stone-500">No API tokens.</td>
          </tr>
        </tbody>
      </table>
    </div>
    <script id="account-token-data" type="application/json">{{ tokensJSON|default:"[]"|safe }}</script>
  </section>
  {% endif %}
</div>
{% endblock %}
