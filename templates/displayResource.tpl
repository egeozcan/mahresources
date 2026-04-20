{% extends "/layouts/base.tpl" %}

{% block body %}
    {% plugin_slot "resource_detail_before" %}
    <div x-data="{ entity: {{ resource|json }} }">
        {% process_shortcodes resource.ResourceCategory.CustomHeader resource %}
    </div>

    {% if sc.Description %}
    {% include "/partials/description.tpl" with description=resource.Description descriptionEntity=resource descriptionEditUrl="/v1/resource/editDescription" descriptionEditId=resource.ID %}
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if resource.ResourceCategory.MetaSchema && resource.Meta %}
    <schema-editor mode="display"
        schema='{{ resource.ResourceCategory.MetaSchema }}'
        value='{{ resource.Meta|json }}'
        name="{{ resource.ResourceCategory.Name }}">
    </schema-editor>
    {% endif %}
    {% endif %}

    {% if sc.MetadataGrid || sc.TechnicalDetails.State != "off" %}
    <div class="detail-panel" aria-label="Resource metadata">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Metadata</h2>
        </div>
        <div class="detail-panel-body">
            {% if sc.MetadataGrid %}
            <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
                {% if resource.Name %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Name</dt>
                    <dd class="text-sm mt-0.5 break-all">{{ resource.Name }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Name"
                        @click="updateClipboard('{{ resource.Name|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                {% if resource.OriginalName %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Original Name</dt>
                    <dd class="text-sm mt-0.5 break-all">{{ resource.OriginalName }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Original Name"
                        @click="updateClipboard('{{ resource.OriginalName|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                {% if resource.Width and resource.Height %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Dimensions</dt>
                    <dd class="text-sm mt-0.5">{{ resource.Width }} × {{ resource.Height }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Dimensions"
                        @click="updateClipboard('{{ resource.Width }}x{{ resource.Height }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                {% if sc.Timestamps %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Created</dt>
                    <dd class="text-sm mt-0.5">{{ resource.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Created"
                        @click="updateClipboard('{{ resource.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Updated</dt>
                    <dd class="text-sm mt-0.5">{{ resource.UpdatedAt|date:"Jan 02, 2006 15:04" }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Updated"
                        @click="updateClipboard('{{ resource.UpdatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
            </dl>
            {% endif %}
            {% if sc.TechnicalDetails.State != "off" %}
            <details class="detail-collapsible mt-3" {% if sc.TechnicalDetails.State == "open" %}open{% endif %}>
                <summary>Technical Details</summary>
                <div class="detail-panel-body">
                    <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">ID</dt>
                            <dd class="text-sm mt-0.5">{{ resource.ID }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy ID"
                                @click="updateClipboard('{{ resource.ID }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% if resource.Hash %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Hash{% if resource.HashType %} ({{ resource.HashType }}){% endif %}</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Hash }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Hash"
                                @click="updateClipboard('{{ resource.Hash|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.Location %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Location }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Location"
                                @click="updateClipboard('{{ resource.Location|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.OriginalLocation %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Original Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.OriginalLocation }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Original Location"
                                @click="updateClipboard('{{ resource.OriginalLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.StorageLocation %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Storage Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.StorageLocation }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Storage Location"
                                @click="updateClipboard('{{ resource.StorageLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if sc.Description && resource.Description %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3 col-span-2 md:col-span-3">
                            <dt class="text-xs text-stone-500 font-mono">Description</dt>
                            <dd class="text-sm mt-0.5 font-sans">{{ resource.Description }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Description"
                                @click="updateClipboard('{{ resource.Description|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                    </dl>
                </div>
            </details>
            {% endif %}
        </div>
    </div>
    {% endif %}

    {% if sc.Notes %}
    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/notes" formID=resource.ID formParamName="resources" templateName="note" %}
    {% endif %}
    {% if sc.Groups %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/groups" formID=resource.ID formParamName="resources" templateName="group" %}
    {% endif %}

    {% if sc.Series %}
    {% if resource.Series %}
    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Series</h2>
            <div class="detail-panel-actions">
                <a href="/series?id={{ resource.Series.ID }}" class="text-amber-700 hover:text-amber-800 text-sm">{{ resource.Series.Name }}</a>
                <form method="POST" action="/v1/resource/removeSeries?redirect={{ url|urlencode }}"
                    x-data="confirmAction({ message: 'Remove this resource from the series?' })"
                    x-bind="events">
                    <input type="hidden" name="Id" value="{{ resource.ID }}">
                    <button type="submit" class="text-sm text-red-700 hover:text-red-800">Remove from series</button>
                </form>
            </div>
        </div>
        {% if seriesSiblings %}
        <div class="detail-panel-body">
            <div class="list-container">
                {% for entity in seriesSiblings %}
                    {% include partial("resource") %}
                {% endfor %}
            </div>
        </div>
        {% endif %}
    </div>
    {% endif %}
    {% endif %}

    {% if sc.SimilarResources %}
    {% if similarResources %}
        <div class="detail-panel">
            <div class="detail-panel-header">
                <h2 class="detail-panel-title">Similar Resources</h2>
            </div>
            <div class="detail-panel-body">
                <div class="list-container">
                    {% for entity in similarResources %}
                        <div>
                            {% include partial("resource") %}
                            <a href="/resource/compare?r1={{ resource.ID }}&r2={{ entity.ID }}" class="btn btn-sm btn-outline mt-1 block text-center">Compare</a>
                        </div>
                    {% endfor %}
                </div>
            </div>
        </div>
        <form
            x-data="confirmAction({ message: 'All the similar resources will be deleted. Are you sure?' })"
            action="/v1/resources/merge"
            method="post" :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
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
    {% endif %}

    {% if sc.Versions %}
    {% include "/partials/versionPanel.tpl" with versions=versions currentVersionId=resource.CurrentVersionID resourceId=resource.ID %}
    {% endif %}
    {% plugin_slot "resource_detail_after" %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ resource|json }} }">
            {% process_shortcodes resource.ResourceCategory.CustomSidebar resource %}
        </div>
        {% if sc.Owner %}
        {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
        {% endif %}
        {% if sc.FileSize %}
        <p>{{ resource.FileSize | humanReadableSize }}</p>
        {% endif %}
    </div>

    {% if sc.PreviewImage %}
    <div class="sidebar-group">
        <a href="/v1/resource/view?id={{ resource.ID }}&v={{ resource.Hash }}#{{ resource.ContentType }}">
            <img height="300" src="/v1/resource/preview?id={{ resource.ID }}&height=300&v={{ resource.Hash }}" alt="Preview of {{ resource.Name }}">
        </a>
    </div>
    {% endif %}

    <div class="sidebar-group">
        {% if sc.Tags %}
        {% include "/partials/tagList.tpl" with tags=resource.Tags addTagUrl='/v1/resources/addTags' id=resource.ID %}
        {% endif %}
        {% if sc.CategoryLink %}
        {% if resource.ResourceCategory %}
            {% include "/partials/sideTitle.tpl" with title="Resource Category" %}
            <a href="/resourceCategory?id={{ resource.ResourceCategory.ID }}">{{ resource.ResourceCategory.Name }}</a>
        {% endif %}
        {% endif %}
    </div>

    {% if sc.ImageOperations %}
    {% if isImage %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Update Dimensions" %}
        <form action="/v1/resource/recalculateDimensions?redirect={{ url|urlencode }}" method="post" class="mb-3">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <button type="submit" class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Recalculate Dimensions</button>
        </form>
        {% include "/partials/sideTitle.tpl" with title="Rotate 90 Degrees" %}
        <form action="/v1/resources/rotate" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <input type="hidden" name="degrees" value="90">
            <button type="submit" class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Rotate</button>
        </form>
        {% if resource.ContentType in "image/jpeg,image/png,image/gif,image/webp,image/bmp,image/tiff,image/heic,image/heif,image/avif" %}
        {% include "/partials/sideTitle.tpl" with title="Crop" %}
        <button
            type="button"
            id="crop-open-{{ resource.ID }}"
            class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600"
            onclick="document.getElementById('crop-modal-{{ resource.ID }}').showModal()"
        >Crop…</button>
        {% endif %}
    </div>
    {% if resource.ContentType in "image/jpeg,image/png,image/gif,image/webp,image/bmp,image/tiff,image/heic,image/heif,image/avif" %}
    {% include "/partials/cropModal.tpl" with resource=resource %}
    {% endif %}
    {% endif %}
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=resource.Meta %}
    </div>
    {% endif %}

    <div class="sidebar-group">
        {% include "partials/pluginActionsSidebar.tpl" with entityId=resource.ID entityType="resource" %}
        {% plugin_slot "resource_detail_sidebar" %}
    </div>

    {% if resource.GUID %}
    <div class="sidebar-group">
        <button type="button" class="text-xs text-stone-600 break-all cursor-pointer text-left bg-transparent border-0 p-0 w-full" title="Click to copy GUID" onclick="navigator.clipboard.writeText('{{ resource.GUID }}')">
            GUID: {{ resource.GUID }}
        </button>
    </div>
    {% endif %}
{% endblock %}
