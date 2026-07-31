{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-8">
  <section>
    <h1 class="text-xl font-mono font-semibold mb-4">Users</h1>
    {# errorMessage is already rendered by layouts/base.tpl's alert region; a #}
    {# second copy here printed every failure twice. #}
    {% if queryValues.error.0 %}
    <div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
      <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
    </div>
    {% endif %}
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm border-collapse">
        <caption class="sr-only">User accounts</caption>
        <thead>
          <tr class="text-left border-b border-stone-300 font-mono">
            <th scope="col" class="py-2 pr-4">ID</th>
            <th scope="col" class="py-2 pr-4">Username</th>
            <th scope="col" class="py-2 pr-4">Display name</th>
            <th scope="col" class="py-2 pr-4">Role</th>
            <th scope="col" class="py-2 pr-4">Scope group</th>
            <th scope="col" class="py-2 pr-4">Disabled</th>
            <th scope="col" class="py-2 pr-4">Actions</th>
          </tr>
        </thead>
        <tbody>
          {% for u in users %}
          <tr class="border-b border-stone-100">
            <td class="py-2 pr-4">{{ u.ID }}</td>
            <td class="py-2 pr-4 font-medium">{{ u.Username }}</td>
            <td class="py-2 pr-4">{{ u.DisplayName }}</td>
            <td class="py-2 pr-4 font-mono">{{ u.Role }}</td>
            <td class="py-2 pr-4">{% if u.ScopeGroupId %}{{ u.ScopeGroupId }}{% else %}—{% endif %}</td>
            <td class="py-2 pr-4">{% if u.Disabled %}yes{% else %}no{% endif %}</td>
            <td class="py-2 pr-4">
              <form method="post" action="/v1/user/delete" onsubmit="return confirm('Delete user {{ u.Username }}?');" class="inline">
                <input type="hidden" name="id" value="{{ u.ID }}">
                <button type="submit" class="text-red-700 hover:underline">Delete</button>
              </form>
            </td>
          </tr>
          {% endfor %}
        </tbody>
      </table>
    </div>
  </section>

  <section class="max-w-xl">
    <h2 class="text-lg font-mono font-semibold mb-3">Create user</h2>
    <form method="post" action="/v1/users" class="space-y-4">
      {# Finding 34: a rejection used to land on a bare error page at /v1/users, so #}
      {# every value below was gone. The handler now redirects back here with them. #}
      <div>
        <label for="u-username" class="block text-sm font-mono mb-1">Username</label>
        <input id="u-username" name="username" type="text" required autocomplete="off"
               value="{{ queryValues.username.0 }}"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="u-display" class="block text-sm font-mono mb-1">Display name</label>
        <input id="u-display" name="displayName" type="text" value="{{ queryValues.displayName.0 }}"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="u-password" class="block text-sm font-mono mb-1">Password</label>
        <input id="u-password" name="password" type="password" required autocomplete="new-password"
               minlength="{{ minPasswordLength }}" aria-describedby="u-password-hint"
               class="w-full border border-stone-300 rounded px-3 py-2">
        <p id="u-password-hint" class="mt-1 text-xs text-stone-500">Must be at least {{ minPasswordLength }} characters. Never carried back in the URL, so retype it after a rejection.</p>
      </div>
      <div>
        <label for="u-role" class="block text-sm font-mono mb-1">Role</label>
        <select id="u-role" name="role" class="w-full border border-stone-300 rounded px-3 py-2">
          {% for role in roles %}<option value="{{ role }}"{% if queryValues.role.0 == role %} selected{% endif %}>{{ role }}</option>{% endfor %}
        </select>
      </div>
      <div>
        <label for="u-scope" class="block text-sm font-mono mb-1">Scope group ID <span class="text-stone-500">(required for guest; optional for user)</span></label>
        <input id="u-scope" name="scopeGroupId" type="number" min="1" value="{{ queryValues.scopeGroupId.0 }}"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div class="flex items-center gap-2">
        <input id="u-disabled" name="disabled" type="checkbox" value="true">
        <label for="u-disabled" class="text-sm font-mono">Disabled</label>
      </div>
      <button type="submit" class="bg-amber-700 hover:bg-amber-800 text-white font-mono py-2 px-4 rounded">Create user</button>
    </form>
  </section>
</div>
{% endblock %}
