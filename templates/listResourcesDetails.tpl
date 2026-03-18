{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorResource.tpl" %}
{% endblock %}

{% block body %}
    <div class="detail-table-wrap">
        <table class="gallery detail-table">
            <thead>
                <tr>
                    <th scope="col"><span class="sr-only">Select</span></th>
                    <th scope="col">ID</th>
                    <th scope="col">Name</th>
                    <th scope="col">Preview</th>
                    <th scope="col">Size</th>
                    <th scope="col">Created</th>
                    <th scope="col">Updated</th>
                    <th scope="col">Original Name</th>
                    <th scope="col">Original Location</th>
                </tr>
            </thead>
            <tbody>
                {% for entity in resources %}
                    <tr x-data="selectableItem({ itemId: {{ entity.ID }} })">
                        <td>
                            <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="detail-table-checkbox focus:ring-amber-600 text-amber-700 border-stone-300 rounded">
                        </td>
                        <td class="detail-table-secondary">
                            <a href="/resource?id={{ entity.ID }}">{{ entity.ID }}</a>
                        </td>
                        <td class="detail-table-name">
                            <a href="/resource?id={{ entity.ID }}">{{ entity.Name }}</a>
                        </td>
                        <td class="detail-table-preview">
                            <a href="/v1/resource/view?id={{ entity.ID }}&v={{ entity.Hash }}#{{ entity.ContentType }}"
                               @click.prevent="$store.lightbox.openFromClick($event, {{ entity.ID }}, '{{ entity.ContentType }}')"
                               data-lightbox-item
                               data-resource-id="{{ entity.ID }}"
                               data-content-type="{{ entity.ContentType }}"
                               data-resource-name="{{ entity.Name }}"
                               data-resource-hash="{{ entity.Hash }}">
                                <img height="32" src="/v1/resource/preview?id={{ entity.ID }}&height=32&v={{ entity.Hash }}" alt="Preview of {{ entity.Name }}">
                            </a>
                        </td>
                        <td class="detail-table-secondary">{{ entity.FileSize | humanReadableSize }}</td>
                        <td class="detail-table-secondary">{{ entity.CreatedAt|date:"2006-01-02 15:04" }}</td>
                        <td class="detail-table-secondary">{{ entity.UpdatedAt|date:"2006-01-02 15:04" }}</td>
                        <td class="detail-table-secondary detail-table-truncate">{{ entity.OriginalName }}</td>
                        <td class="detail-table-secondary detail-table-truncate">{{ entity.OriginalLocation }}</td>
                    </tr>
                {% endfor %}
            </tbody>
        </table>
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}