{% extends "/layouts/base.tpl" %}

{% block body %}
<section class="space-y-4" data-testid="downloads-page" x-data="downloadsManager()">
  <header class="space-y-1">
    {# partials/title.tpl already renders the page's <h1> from pageTitle, so this #}
    {# is an h2 — a second h1 with the same words is announced as two page titles. #}
    <h2 class="text-lg font-semibold font-mono text-stone-800">Downloads</h2>
    <p class="text-sm text-stone-500">
      Background downloads that have finished, plus any that are still running. Failed and cancelled downloads are kept longer than completed ones; both windows are set on
      <a href="/admin/settings" class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700">the settings page</a>.
    </p>
  </header>

  <div class="flex items-center justify-between gap-3 flex-wrap">
    <span class="text-xs text-stone-500" data-testid="downloads-count">
      {{ downloadsCount }} stored download{% if downloadsCount != 1 %}s{% endif %}
    </span>

    <div class="flex items-center gap-2" x-show="selectedCount > 0" x-cloak>
      <span class="text-xs text-stone-600" data-testid="downloads-selected-count"
            x-text="selectedCount + ' selected'"></span>
      <button type="button"
              @click="retrySelected()"
              :disabled="busy"
              class="inline-flex items-center gap-1 px-3 py-1 text-xs font-medium font-mono text-amber-800 border border-amber-300 rounded hover:bg-amber-50 disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-amber-600"
              data-testid="downloads-bulk-retry">
        Retry selected
      </button>
      <button type="button"
              @click="deleteSelected()"
              :disabled="busy"
              class="inline-flex items-center gap-1 px-3 py-1 text-xs font-medium font-mono text-red-700 border border-red-300 rounded hover:bg-red-50 disabled:opacity-50 focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-red-600"
              data-testid="downloads-bulk-delete">
        Delete selected
      </button>
    </div>
  </div>

  <p class="text-sm text-red-700" x-show="error" x-cloak x-text="error" data-testid="downloads-error"></p>
  <p class="text-sm text-stone-700" x-show="notice" x-cloak x-text="notice" data-testid="downloads-notice"></p>

  <div class="overflow-x-auto border border-stone-200 rounded">
    <table class="w-full text-sm" data-testid="downloads-table">
      <caption class="sr-only">Stored downloads, newest first</caption>
      <thead class="bg-stone-50 text-stone-600">
        <tr class="text-left">
          <th scope="col" class="p-2 w-8">
            <input type="checkbox"
                   x-ref="selectAll"
                   @change="toggleAll($event.target.checked)"
                   :checked="allSelected"
                   aria-label="Select all downloads on this page"
                   data-testid="downloads-select-all"
                   class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
          </th>
          <th scope="col" class="p-2">Status</th>
          <th scope="col" class="p-2">Download</th>
          <th scope="col" class="p-2">Size</th>
          <th scope="col" class="p-2">When</th>
          <th scope="col" class="p-2 w-32">Actions</th>
        </tr>
      </thead>
      <tbody>
        {% for download in downloads %}
        <tr class="border-t border-stone-100 align-top" data-testid="downloads-row" data-job-id="{{ download.JobID }}">
          <td class="p-2">
            {% if download.ID %}
            <input type="checkbox"
                   value="{{ download.ID }}"
                   @change="toggle({{ download.ID }}, $event.target.checked)"
                   :checked="isSelected({{ download.ID }})"
                   aria-label="Select {{ download.Name|default:download.URL|escape }}"
                   data-testid="downloads-row-checkbox"
                   class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
            {% endif %}
          </td>
          <td class="p-2 whitespace-nowrap">
            {% if download.Live %}
              <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-sky-100 text-sky-800" data-testid="downloads-status">{{ download.Status }}</span>
            {% elif download.Status == "failed" %}
              <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-red-100 text-red-800" data-testid="downloads-status">failed</span>
            {% elif download.Status == "cancelled" %}
              <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-stone-200 text-stone-700" data-testid="downloads-status">cancelled</span>
            {% else %}
              <span class="px-2 inline-flex text-xs leading-5 font-semibold font-mono rounded-full bg-emerald-100 text-emerald-800" data-testid="downloads-status">completed</span>
            {% endif %}
            {% if download.Attempts > 1 %}
            <span class="block mt-1 text-xs text-stone-500">{{ download.Attempts }} attempts</span>
            {% endif %}
          </td>
          <td class="p-2">
            <div class="font-medium text-stone-800 break-all">{{ download.Name|default:download.URL }}</div>
            <div class="font-mono text-xs text-stone-500 break-all">{{ download.URL }}</div>
            {% if download.Error %}
            {# The shared component: it previews the first 30 characters, offers an  #}
            {# accessible "show more", and adds a copy button — which is what a       #}
            {# reader wants from a download error, since the useful thing to do with  #}
            {# one is paste it somewhere. It takes no length attribute; passing one   #}
            {# would look like configuration and do nothing.                          #}
            <expandable-text class="block mt-1 text-xs text-red-700" data-testid="downloads-error-text">{{ download.Error }}</expandable-text>
            {% endif %}
            {% if download.LastRetryJobID and not download.Live %}
            {# The row records how this download ended; a resubmission is a new job #}
            {# with a new id, so without this line a failed row that has already     #}
            {# been retried looks untouched and invites a second Retry.              #}
            <div class="mt-1 text-xs text-stone-500" data-testid="downloads-retry-link">
              retried as <span class="font-mono">{{ download.LastRetryJobID }}</span>
            </div>
            {% endif %}
            {% if download.ResourceID %}
            <a href="/resource?id={{ download.ResourceID }}" class="inline-block mt-1 text-xs text-amber-700 underline decoration-amber-300 hover:decoration-amber-700" data-testid="downloads-resource-link">View resource</a>
            {% endif %}
          </td>
          <td class="p-2 text-xs text-stone-600 whitespace-nowrap">
            {% if download.TotalSize > 0 %}{{ download.TotalSize|filesizeformat }}{% elif download.Progress > 0 %}{{ download.Progress|filesizeformat }}{% else %}<span class="text-stone-400">unknown</span>{% endif %}
          </td>
          <td class="p-2 text-xs text-stone-600 whitespace-nowrap">
            <time datetime="{{ download.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}">{{ download.CreatedAt|date:"2006-01-02 15:04" }}</time>
          </td>
          <td class="p-2">
            <div class="flex gap-2">
              {% if download.Retryable %}
              <button type="button"
                      @click="retryOne({{ download.ID }})"
                      :disabled="busy"
                      class="text-xs text-amber-800 underline decoration-dotted hover:text-amber-900 disabled:opacity-50"
                      data-testid="downloads-retry">Retry</button>
              {% endif %}
              {% if download.Deletable and download.ID %}
              <button type="button"
                      @click="deleteOne({{ download.ID }})"
                      :disabled="busy"
                      class="text-xs text-red-700 underline decoration-dotted hover:text-red-900 disabled:opacity-50"
                      data-testid="downloads-delete">Delete</button>
              {% endif %}
              {% if download.Live %}
              <span class="text-xs text-stone-500" data-testid="downloads-live-note">in progress</span>
              {% endif %}
            </div>
          </td>
        </tr>
        {% empty %}
        {# The shared zero-result line, which distinguishes "nothing yet" from "your #}
        {# filter matched nothing" — the whole point of partials/listEmpty.tpl. There #}
        {# is no create page for a download: they arrive from the resource form's     #}
        {# "Download in background", so createUrl is deliberately omitted.            #}
        <tr>
          <td colspan="6" class="p-4" data-testid="downloads-empty">
            {% include "/partials/listEmpty.tpl" with label="downloads" %}
          </td>
        </tr>
        {% endfor %}
      </tbody>
    </table>
  </div>

  {% include "/partials/pagination.tpl" %}
</section>
{% endblock %}

{% block sidebar %}
<div class="sidebar-group">
  {% include "/partials/sideTitle.tpl" with title="Filter" %}
  <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter downloads">
    <fieldset class="w-full mt-2">
      <legend class="block text-sm font-medium font-mono text-stone-700">Status</legend>
      {% for status in downloadStatuses %}
        {% if status.Link %}
        <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
          <input type="checkbox" name="Status" value="{{ status.Link }}" {% if status.Active %}checked{% endif %}
                 class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
          {{ status.Title }}
        </label>
        {% endif %}
      {% endfor %}
    </fieldset>

    {% include "/partials/form/textInput.tpl" with name='URL' label='URL or name contains' value=queryValues.URL.0 %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='After' value=queryValues.CreatedAfter.0 %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Before' value=queryValues.CreatedBefore.0 %}
    {% include "/partials/form/searchButton.tpl" %}
  </form>
</div>
{% endblock %}
