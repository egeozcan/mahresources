{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorResource.tpl" %}
{% endblock %}

{% block body %}
    <div class="flex flex-col">
        <div class="-my-2 overflow-x-auto sm:-mx-6 lg:-mx-8">
            <div class="py-2 align-middle inline-block min-w-full sm:px-6 lg:px-8">
                <div class="shadow overflow-hidden border-b border-stone-200 sm:rounded-lg">
                    <table class="gallery min-w-full divide-y divide-stone-200">
                        <thead class="bg-stone-50">
                            <tr>
                                <td></td>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">ID</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Name</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Preview</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Size</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Created</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Updated</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Original Name</th>
                                <th scope="col" class="px-6 py-3 text-left text-xs font-medium font-mono text-stone-500 uppercase tracking-wider">Original Location</th>
                            </tr>
                        </thead>
                        <tbody>
                            {% for entity in resources %}
                                <tr class="bg-white" x-data="selectableItem({ itemId: {{ entity.ID }} })">
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500">
                                        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="focus:ring-amber-600 h-8 w-8 text-amber-700 border-stone-300 rounded">
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono">
                                        <a class="max-w-lg overflow-ellipsis overflow-hidden block" href="/resource?id={{ entity.ID }}">{{ entity.ID }}</a>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm font-medium text-stone-900">
                                        <a class="max-w-lg overflow-ellipsis overflow-hidden block" href="/resource?id={{ entity.ID }}">{{ entity.Name }}</a>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500">
                                        <a class="max-w-lg overflow-ellipsis overflow-hidden block" href="/v1/resource/view?id={{ entity.ID }}&v={{ entity.Hash }}#{{ entity.ContentType }}"
                                           @click.prevent="$store.lightbox.openFromClick($event, {{ entity.ID }}, '{{ entity.ContentType }}')"
                                           data-lightbox-item
                                           data-resource-id="{{ entity.ID }}"
                                           data-content-type="{{ entity.ContentType }}"
                                           data-resource-name="{{ entity.Name }}"
                                           data-resource-hash="{{ entity.Hash }}">
                                            <img height="50" src="/v1/resource/preview?id={{ entity.ID }}&height=50&v={{ entity.Hash }}" alt="Preview">
                                        </a>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono">
                                        {{ entity.FileSize | humanReadableSize }}
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono">
                                        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-stone-600">Created: </span>{{ entity.CreatedAt|date:"2006-01-02 15:04" }}</small>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono">
                                        <small class="min-w-0 whitespace-nowrap overflow-hidden overflow-ellipsis text-sm"><span class="text-stone-600">Updated: </span>{{ entity.UpdatedAt|date:"2006-01-02 15:04" }}</small>
                                    </td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono"><span class="max-w-lg overflow-ellipsis overflow-hidden block">{{ entity.OriginalName }}</span></td>
                                    <td class="px-6 py-4 whitespace-nowrap text-sm text-stone-500 font-mono"><span class="max-w-lg overflow-ellipsis overflow-hidden block">{{ entity.OriginalLocation }}</span></td>
                                </tr>
                            {% endfor %}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}