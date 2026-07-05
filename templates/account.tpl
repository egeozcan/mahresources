{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-8 max-w-xl">
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
    <form method="post" action="/v1/account/password" class="space-y-4">
      <div>
        <label for="cur-pw" class="block text-sm font-mono mb-1">Current password</label>
        <input id="cur-pw" name="currentPassword" type="password" required autocomplete="current-password"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="new-pw" class="block text-sm font-mono mb-1">New password</label>
        <input id="new-pw" name="newPassword" type="password" required autocomplete="new-password"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <button type="submit" class="bg-amber-600 hover:bg-amber-700 text-white font-mono py-2 px-4 rounded">Update password</button>
    </form>
  </section>

  <section x-data="{ name: '', created: '' }">
    <h2 class="text-lg font-mono font-semibold mb-3">API tokens</h2>
    <p class="text-stone-600 text-sm mb-3">
      Use API tokens with the <code>mr</code> CLI or other clients via the <code>Authorization: Bearer</code> header.
      {% if docsLinksEnabled %}
      <a href="{{ docsURL("features/authentication") }}" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Authentication docs</a>
      {% endif %}
    </p>

    <div x-show="created" x-cloak class="mb-4 rounded bg-green-50 border border-green-200 p-3 text-sm">
      <p class="mb-1 font-medium text-green-800">New token (copy it now — it won't be shown again):</p>
      <code class="block break-all font-mono" x-text="created"></code>
    </div>

    <div class="flex items-end gap-2 mb-4">
      <div class="flex-1">
        <label for="tok-name" class="block text-sm font-mono mb-1">New token label</label>
        <input id="tok-name" type="text" x-model="name" placeholder="e.g. laptop"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <button type="button"
              @click="fetch('/v1/account/tokens', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({name: name || 'mr cli'})}).then(r => r.json()).then(d => { created = d.token || ''; name=''; }).catch(() => { created=''; })"
              class="bg-amber-600 hover:bg-amber-700 text-white font-mono py-2 px-4 rounded">Create</button>
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
          {% for t in tokens %}
          <tr class="border-b border-stone-100">
            <td class="py-2 pr-4">{{ t.ID }}</td>
            <td class="py-2 pr-4">{{ t.Name }}</td>
            <td class="py-2 pr-4 font-mono">{{ t.Prefix }}…</td>
            <td class="py-2 pr-4">{% if t.LastUsedAt %}{{ t.LastUsedAt|date:"2006-01-02 15:04" }}{% else %}never{% endif %}</td>
            <td class="py-2 pr-4">
              <form method="post" action="/v1/account/tokens/delete" class="inline">
                <input type="hidden" name="id" value="{{ t.ID }}">
                <button type="submit" class="text-red-700 hover:underline">Revoke</button>
              </form>
            </td>
          </tr>
          {% endfor %}
        </tbody>
      </table>
    </div>
  </section>
  {% endif %}
</div>
{% endblock %}
