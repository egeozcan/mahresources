<article class="card resource-card card--with-image{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-indigo-500 h-6 w-6 text-indigo-600 border-gray-300 rounded">
    {% endif %}

    <div x-data="{ entity: {{ entity|json }} }">
        <div class="card-image">
            <a href="/v1/resource/view?id={{ entity.ID }}&v={{ entity.Hash }}#{{ entity.ContentType }}"
               @click.prevent="$store.lightbox.openFromClick($event, {{ entity.ID }}, '{{ entity.ContentType }}')"
               data-lightbox-item
               data-resource-id="{{ entity.ID }}"
               data-content-type="{{ entity.ContentType }}"
               data-resource-name="{{ entity.Name }}"
               data-resource-hash="{{ entity.Hash }}"
               data-resource-width="{{ entity.Width }}"
               data-resource-height="{{ entity.Height }}">
                <img height="300" src="/v1/resource/preview?id={{ entity.ID }}&height=300&v={{ entity.Hash }}" alt="Preview of {{ entity.Name }}">
            </a>
        </div>

        <header class="card-header card-header--compact">
            <div class="card-title-section">
                <h3 class="card-title">
                    <a href="/resource?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
                </h3>
                <div class="card-meta">
                    <span class="card-meta-item">{{ entity.FileSize | humanReadableSize }}</span>
                    {% if entity.Owner %}
                    <span class="card-meta-item">
                        <span class="card-meta-label">Owner:</span>
                        <a href="/group?id={{ entity.Owner.ID }}" class="card-meta-link">{{ entity.Owner.Name }}</a>
                    </span>
                    {% endif %}
                    {% if entity.ResourceCategory %}
                    <span class="card-meta-item">
                        {% autoescape off %}{{ entity.ResourceCategory.CustomAvatar }}{% endautoescape %}
                        <a href="/resourceCategory?id={{ entity.ResourceCategory.ID }}" class="card-meta-link">{{ entity.ResourceCategory.Name }}</a>
                    </span>
                    {% endif %}
                </div>
            </div>
        </header>

        {% autoescape off %}
            {{ entity.ResourceCategory.CustomSummary }}
        {% endautoescape %}

        {% if entity.Description %}
        <div class="card-description">
            {% include "partials/description.tpl" with description=entity.Description preview=true %}
        </div>
        {% endif %}

        <div class="tags card-tags">
            {% for tag in entity.Tags %}
            <a class="card-badge{% if hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                {{ tag.Name }}
            </a>
            {% endfor %}
            <button x-cloak data-entity-type="resource" class="edit-in-list card-badge card-badge--action">
                Edit Tags
            </button>
        </div>
    </div>
</article>
