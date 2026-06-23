{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="space-y-8">
  <section>
    <h1 class="text-xl font-mono font-semibold mb-4">Users</h1>
    {% if errorMessage %}
    <div class="mb-4 rounded bg-red-50 border border-red-200 p-3 text-sm text-red-800" role="alert">{{ errorMessage }}</div>
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
      <div>
        <label for="u-username" class="block text-sm font-mono mb-1">Username</label>
        <input id="u-username" name="username" type="text" required autocomplete="off"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="u-display" class="block text-sm font-mono mb-1">Display name</label>
        <input id="u-display" name="displayName" type="text" class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="u-password" class="block text-sm font-mono mb-1">Password</label>
        <input id="u-password" name="password" type="password" required autocomplete="new-password"
               class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div>
        <label for="u-role" class="block text-sm font-mono mb-1">Role</label>
        <select id="u-role" name="role" class="w-full border border-stone-300 rounded px-3 py-2">
          {% for role in roles %}<option value="{{ role }}">{{ role }}</option>{% endfor %}
        </select>
      </div>
      <div>
        <label for="u-scope" class="block text-sm font-mono mb-1">Scope group ID <span class="text-stone-500">(required for guest; optional for user)</span></label>
        <input id="u-scope" name="scopeGroupId" type="number" min="1" class="w-full border border-stone-300 rounded px-3 py-2">
      </div>
      <div class="flex items-center gap-2">
        <input id="u-disabled" name="disabled" type="checkbox" value="true">
        <label for="u-disabled" class="text-sm font-mono">Disabled</label>
      </div>
      <button type="submit" class="bg-amber-600 hover:bg-amber-700 text-white font-mono py-2 px-4 rounded">Create user</button>
    </form>
  </section>
</div>
{% endblock %}
