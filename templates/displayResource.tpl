{% extends "/layouts/base.tpl" %}

{% block body %}
    <div x-data="{ entity: {{ resource|json }} }">
        {% autoescape off %}
            {{ resource.ResourceCategory.CustomHeader }}
        {% endautoescape %}
    </div>

    {% include "/partials/description.tpl" with description=resource.Description %}

    <section class="mb-6" aria-label="Resource metadata">
        <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
            {% if resource.Name %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Name</dt>
                <dd class="text-sm mt-0.5 break-all">{{ resource.Name }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Name"
                    @click="updateClipboard('{{ resource.Name|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}
            {% if resource.OriginalName %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Original Name</dt>
                <dd class="text-sm mt-0.5 break-all">{{ resource.OriginalName }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Original Name"
                    @click="updateClipboard('{{ resource.OriginalName|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}
            {% if resource.Width and resource.Height %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Dimensions</dt>
                <dd class="text-sm mt-0.5">{{ resource.Width }} × {{ resource.Height }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Dimensions"
                    @click="updateClipboard('{{ resource.Width }}x{{ resource.Height }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            {% endif %}
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Created</dt>
                <dd class="text-sm mt-0.5">{{ resource.CreatedAt|date:"Jan 02, 2006 15:04" }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Created"
                    @click="updateClipboard('{{ resource.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
            <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                <dt class="text-xs text-gray-500">Updated</dt>
                <dd class="text-sm mt-0.5">{{ resource.UpdatedAt|date:"Jan 02, 2006 15:04" }}</dd>
                <button
                    type="button"
                    class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                    aria-label="Copy Updated"
                    @click="updateClipboard('{{ resource.UpdatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                >⧉</button>
            </div>
        </dl>
        <details class="mt-3">
            <summary class="cursor-pointer text-sm text-gray-500 hover:text-gray-700 select-none py-1">Technical Details</summary>
            <dl class="grid grid-cols-2 md:grid-cols-3 gap-3 mt-3" x-data>
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">ID</dt>
                    <dd class="text-sm mt-0.5">{{ resource.ID }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy ID"
                        @click="updateClipboard('{{ resource.ID }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% if resource.Hash %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Hash{% if resource.HashType %} ({{ resource.HashType }}){% endif %}</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Hash }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Hash"
                        @click="updateClipboard('{{ resource.Hash|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
                {% if resource.Location %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Location</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Location }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Location"
                        @click="updateClipboard('{{ resource.Location|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
                {% if resource.OriginalLocation %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Original Location</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.OriginalLocation }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Original Location"
                        @click="updateClipboard('{{ resource.OriginalLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
                {% if resource.StorageLocation %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-gray-500">Storage Location</dt>
                    <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.StorageLocation }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Storage Location"
                        @click="updateClipboard('{{ resource.StorageLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
                {% if resource.Description %}
                <div class="group relative bg-gray-50 border border-gray-200 hover:border-gray-300 rounded-lg px-4 py-3 col-span-2 md:col-span-3">
                    <dt class="text-xs text-gray-500">Description</dt>
                    <dd class="text-sm mt-0.5">{{ resource.Description }}</dd>
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-gray-400 hover:text-gray-600 p-0.5"
                        aria-label="Copy Description"
                        @click="updateClipboard('{{ resource.Description|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button>
                </div>
                {% endif %}
            </dl>
        </details>
    </section>

    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/notes" formID=resource.ID formParamName="resources" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/groups" formID=resource.ID formParamName="resources" templateName="group" %}

    {% if resource.Series %}
    <section class="mb-6">
        <div class="flex gap-4 items-center mb-4">
            {% include "partials/subtitle.tpl" with small=true title="Series" %}
            <a href="/series?id={{ resource.Series.ID }}" class="text-blue-600 hover:text-blue-800 text-sm">{{ resource.Series.Name }}</a>
            <form method="POST" action="/v1/resource/removeSeries?redirect={{ url|urlencode }}"
                x-data="confirmAction({ message: 'Remove this resource from the series?' })"
                x-bind="events">
                <input type="hidden" name="Id" value="{{ resource.ID }}">
                <button type="submit" class="text-sm text-red-600 hover:text-red-800">Remove from series</button>
            </form>
        </div>
        {% if seriesSiblings %}
        <div class="list-container">
            {% for entity in seriesSiblings %}
                {% include partial("resource") %}
            {% endfor %}
        </div>
        {% endif %}
    </section>
    {% endif %}

    {% if similarResources %}
        {% include "/partials/seeAll.tpl" with entities=similarResources subtitle="Similar Resources" templateName="resource" %}
        <form
            x-data="confirmAction({ message: 'All the similar resources will be deleted. Are you sure?' })"
            action="/v1/resources/merge"
            method="post" :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location)"
            x-bind="events"
        >
            <input type="hidden" name="winner" value="{{ resource.ID }}">
            {% for entity in similarResources %}
                <input type="hidden" name="losers" value="{{ entity.ID }}">
            {% endfor %}
            <p>Merge others with this resource ({{ resource.FileSize | humanReadableSize }})?</p>
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge Others To This" %}</div>
        </form>
    {% endif %}

    {% include "/partials/versionPanel.tpl" with versions=versions currentVersionId=resource.CurrentVersionID resourceId=resource.ID %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div x-data="{ entity: {{ resource|json }} }">
        {% autoescape off %} {# KAN-6: by design — internal network app, all users trusted #}
            {{ resource.ResourceCategory.CustomSidebar }}
        {% endautoescape %}
    </div>

    {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
    <p>{{ resource.FileSize | humanReadableSize }}</p>
    <a href="/v1/resource/view?id={{ resource.ID }}&v={{ resource.Hash }}#{{ resource.ContentType }}">
        <img height="300" src="/v1/resource/preview?id={{ resource.ID }}&height=300&v={{ resource.Hash }}" alt="Preview of {{ resource.Name }}">
    </a>
    {% include "/partials/tagList.tpl" with tags=resource.Tags addTagUrl='/v1/resources/addTags' id=resource.ID %}

    {% if resource.ResourceCategory %}
    {% include "/partials/sideTitle.tpl" with title="Resource Category" %}
    <a href="/resourceCategory?id={{ resource.ResourceCategory.ID }}">{{ resource.ResourceCategory.Name }}</a>
    {% endif %}

    {% if isImage %}
        {% include "/partials/sideTitle.tpl" with title="Update Dimensions" %}
        <form action="/v1/resource/recalculateDimensions?redirect={{ url|urlencode }}" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">Recalculate Dimensions</button>
        </form>
        {% include "/partials/sideTitle.tpl" with title="Rotate 90 Degrees" %}
        <form action="/v1/resources/rotate" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <input type="hidden" name="degrees" value="90">
            <button type="submit" class="ml-3 inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500">Rotate</button>
        </form>
    {% endif %}

    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=resource.Meta %}
{% endblock %}