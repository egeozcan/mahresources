{% extends "/layouts/base.tpl" %}

{% block body %}
<div class="max-w-xl space-y-4">
  <h1 class="text-xl font-mono font-semibold">Edit user</h1>

  {# No `{% if editUser %}` guard: when the provider cannot load the user it sets a #}
  {# non-200 `_statusCode`, and render_template renders error.tpl instead of this   #}
  {# file — precisely so a page template never has to defend against a nil entity.  #}
  {# Every other edit template relies on the same contract.                         #}
  {# Same banner markup as adminUsers.tpl and every create form: the rejection #}
  {# path redirects back here with the typed values and an `error` param. #}
  {% if queryValues.error.0 %}
  <div class="mb-4 rounded-md bg-red-50 border border-red-200 p-4" role="alert" data-testid="form-error-banner">
    <p class="text-sm font-medium text-red-800"><strong>Could not save:</strong> {{ queryValues.error.0 }}</p>
  </div>
  {% endif %}

  {# Every field is prefilled so this full edit form submits the intended account #}
  {# state. UpdateUser itself is presence-aware for partial API/form clients. The #}
  {# hidden disabledPresent marker below makes an unchecked box explicitly false; #}
  {# password remains the one field where blank means unchanged.                  #}
  {# `queryValues.X.0|default:editUser.X` so a rejected save comes back with     #}
  {# what the admin typed, not with what is still in the database.               #}
  <form method="post" action="/v1/user" class="space-y-4" data-testid="user-edit-form">
    <input type="hidden" name="id" value="{{ editUser.ID }}">
    {# Marks the checkbox as present even when it is unchecked. Partial form API #}
    {# clients that omit this marker preserve the stored disabled state.          #}
    <input type="hidden" name="disabledPresent" value="true">

    <div>
      <label for="ue-username" class="block text-sm font-mono mb-1">Username</label>
      <input id="ue-username" name="username" type="text" required autocomplete="off"
             value="{{ queryValues.username.0|default:editUser.Username }}"
             class="w-full border border-stone-300 rounded px-3 py-2">
    </div>

    <div>
      <label for="ue-display" class="block text-sm font-mono mb-1">Display name</label>
      <input id="ue-display" name="displayName" type="text"
             value="{{ queryValues.displayName.0|default:editUser.DisplayName }}"
             class="w-full border border-stone-300 rounded px-3 py-2">
    </div>

    <div>
      <label for="ue-password" class="block text-sm font-mono mb-1">New password</label>
      <input id="ue-password" name="password" type="password" autocomplete="new-password"
             minlength="{{ minPasswordLength }}" aria-describedby="ue-password-hint"
             class="w-full border border-stone-300 rounded px-3 py-2">
      <p id="ue-password-hint" class="mt-1 text-xs text-stone-500">Leave blank to keep the current password. A new one must be at least {{ minPasswordLength }} Unicode code points and at most {{ maxPasswordBytes }} UTF-8 bytes.</p>
    </div>

    <div>
      <label for="ue-role" class="block text-sm font-mono mb-1">Role</label>
      <select id="ue-role" name="role" class="w-full border border-stone-300 rounded px-3 py-2">
        {% for role in roles %}<option value="{{ role }}"{% if queryValues.role.0|default:editUser.Role == role %} selected{% endif %}>{{ role }}</option>{% endfor %}
      </select>
    </div>

    <div>
      <label for="ue-scope" class="block text-sm font-mono mb-1">Scope group ID <span class="text-stone-500">(required for guest; optional for user)</span></label>
      <input id="ue-scope" name="scopeGroupId" type="number" min="1"
             value="{{ queryValues.scopeGroupId.0|default:editUser.ScopeGroupId }}"
             class="w-full border border-stone-300 rounded px-3 py-2">
    </div>

    <div class="flex items-center gap-2">
      {# Checked from the stored value so saving an unrelated field does not      #}
      {# silently re-enable a disabled account. #}
      <input id="ue-disabled" name="disabled" type="checkbox" value="true"
             aria-describedby="ue-disabled-hint"
             {% if formSubmitted %}
               {% if queryValues.disabled.0 == "true" %}checked data-disabled-replayed="checked"{% else %}data-disabled-replayed="unchecked"{% endif %}
             {% elif editUser.Disabled %}checked{% endif %}>
      <label for="ue-disabled" class="text-sm font-mono">Disabled</label>
    </div>
    <p id="ue-disabled-hint" class="text-xs text-stone-500">Saving a disabled account signs it out everywhere: its sessions and API tokens are revoked immediately.</p>

    <p class="text-xs text-stone-500">The last enabled administrator cannot be demoted or disabled. Renaming them, or setting a new password, is allowed.</p>

    {% include "/partials/form/createFormSubmit.tpl" %}
  </form>
</div>
{% endblock %}
