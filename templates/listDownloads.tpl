{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/bulkEditorDownload.tpl" %}
{% endblock %}

{% block body %}
    {# partials/title.tpl already renders the page's <h1> from pageTitle, so this  #}
    {# page adds no heading of its own — a second one with the same words is       #}
    {# announced as two page titles.                                               #}
    <div x-data class="mb-3 space-y-1">
        <p class="text-sm text-stone-500">
            <span data-testid="downloads-count">{{ downloadsCount }} stored download{% if downloadsCount != 1 %}s{% endif %}</span>,
            plus any that are still running. Failed and cancelled downloads are kept longer than completed ones; both windows are set on
            <a href="/admin/settings" class="text-amber-700 underline decoration-amber-300 hover:decoration-amber-700">the settings page</a>.
        </p>
        <p class="text-sm text-red-700" x-show="$store.downloads.error" x-cloak x-text="$store.downloads.error" data-testid="downloads-error"></p>
        <p class="text-sm text-stone-700" x-show="$store.downloads.notice" x-cloak x-text="$store.downloads.notice" data-testid="downloads-notice"></p>
    </div>

    {# A single column rather than the .list-container grid: a download is a URL,   #}
    {# an error and a row of controls, none of which survive a 280px cell. Same     #}
    {# choice as /queries and /relations.                                           #}
    <section class="items-container" data-testid="downloads-page">
        {% for download in downloads %}
            {% include "/partials/download.tpl" %}
        {% empty %}
            {# There is no create page for a download: they arrive from the resource #}
            {# form's "Download in background", so createUrl is deliberately omitted. #}
            {% include "/partials/listEmpty.tpl" with label="downloads" %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter downloads">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
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
        </div>
    </form>
{% endblock %}
