{% extends "/layouts/base.tpl" %}

{% block body %}
<section class="space-y-5" data-testid="admin-shares">
  <header class="space-y-1">
    <h1 class="text-lg font-semibold font-mono text-stone-800">Shared Notes</h1>
    <p class="text-sm text-stone-500">Every note currently reachable via a public share token. Revoke a single share with the button on the row; bulk-revoke with the selection checkboxes.</p>
  </header>

  {% if not shareUrlConfigured %}
  {# BH-033: warn that absolute URLs cannot be shown until SHARE_PUBLIC_URL is set. #}
  <div class="rounded border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800" data-testid="admin-shares-public-url-warning">
    <p class="font-medium">Share URL base is not configured.</p>
    <p class="mt-1">Set <code class="font-mono">SHARE_PUBLIC_URL</code> to enable absolute shareable links. Until then, each row shows the relative <code>/s/&lt;token&gt;</code> path; prepend your server's public URL manually.</p>
  </div>
  {% endif %}

  {% if shares|length == 0 %}
  <p class="text-sm text-stone-500" data-testid="admin-shares-empty">No notes are currently shared.</p>
  {% else %}
  {# BH-035: per-row revoke forms live outside the bulk form and are targeted via the HTML5 form="..." button attribute (nested forms are invalid HTML). #}
  {% for note in shares %}
  <form id="admin-share-revoke-form-{{ note.ID }}" method="post" action="/v1/admin/shares/bulk-revoke" class="hidden"
        onsubmit="return confirm('Revoke share for &quot;{{ note.Name|escape }}&quot;?');">
    <input type="hidden" name="ids" value="{{ note.ID }}">
  </form>
  {% endfor %}

  <form method="post" action="/v1/admin/shares/bulk-revoke" data-testid="admin-shares-form"
        onsubmit="return confirm('Revoke share tokens for all selected notes?');">
    <div class="flex items-center justify-between mb-2">
      <span class="text-xs text-stone-500" data-testid="admin-shares-count">{{ shares|length }} shared note{% if shares|length != 1 %}s{% endif %}</span>
      <button type="submit"
              class="inline-flex items-center gap-1 px-3 py-1 text-xs font-medium font-mono text-red-700 border border-red-300 rounded hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-red-600"
              data-testid="admin-shares-bulk-revoke">
        Revoke Selected
      </button>
    </div>

    <div class="overflow-x-auto border border-stone-200 rounded">
      <table class="w-full text-sm" data-testid="admin-shares-table">
        <thead class="bg-stone-50 text-stone-600">
          <tr class="text-left">
            <th class="p-2 w-8">
              <input type="checkbox" aria-label="Select all shared notes"
                     onclick="document.querySelectorAll('[data-share-row-checkbox]').forEach(function(cb){cb.checked=this.checked;}.bind(this));"
                     class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
            </th>
            <th class="p-2">Name</th>
            <th class="p-2">Public URL</th>
            <th class="p-2">Created</th>
            <th class="p-2 w-20">Revoke</th>
          </tr>
        </thead>
        <tbody>
          {% for note in shares %}
          <tr data-share-note-id="{{ note.ID }}" class="border-t border-stone-100 align-top">
            <td class="p-2">
              <input type="checkbox" name="ids" value="{{ note.ID }}"
                     data-share-row-checkbox
                     aria-label="Select {{ note.Name|escape }}"
                     class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
            </td>
            <td class="p-2">
              <a href="/note?id={{ note.ID }}" class="text-amber-700 hover:underline">{{ note.Name }}</a>
            </td>
            <td class="p-2 font-mono text-xs break-all">
              {% if shareUrlConfigured %}
              <a href="{{ shareBaseUrl }}/s/{{ note.ShareToken }}" target="_blank" rel="noopener">{{ shareBaseUrl }}/s/{{ note.ShareToken }}</a>
              {% else %}
              <span class="text-amber-700">(base not configured)</span>
              <code>/s/{{ note.ShareToken }}</code>
              {% endif %}
            </td>
            <td class="p-2 text-stone-600 text-xs">
              {% if note.ShareCreatedAtFormatted %}{{ note.ShareCreatedAtFormatted }}{% else %}<span class="text-stone-400" data-testid="admin-share-created-unknown">(unknown)</span>{% endif %}
            </td>
            <td class="p-2">
              {# Button targets its hidden per-row form via the HTML5 form=\"...\" attribute. #}
              <button type="submit" form="admin-share-revoke-form-{{ note.ID }}"
                      class="text-xs text-red-700 hover:text-red-900 underline decoration-dotted"
                      data-testid="admin-share-revoke">
                Revoke
              </button>
            </td>
          </tr>
          {% endfor %}
        </tbody>
      </table>
    </div>
  </form>
  {% endif %}
</section>
{% endblock %}
