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
               data-resource-height="{{ entity.Height }}"
               {% if entity.Owner %}data-owner-name="{{ entity.Owner.Name }}" data-owner-id="{{ entity.Owner.ID }}"{% endif %}>
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
            <a class="card-badge{% if !tagBaseUrl && hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{% if tagBaseUrl %}{{ tagBaseUrl }}?tags={{ tag.ID }}{% else %}{{ withQuery("tags", stringId(tag.ID), true) }}{% endif %}'>
                {{ tag.Name }}
            </a>
            {% endfor %}
            <button x-cloak data-entity-type="resource" class="edit-in-list card-badge card-badge--action">
                Edit Tags
            </button>
            {% if pluginCardActions %}
            <div x-data="cardActionMenu()" @click.outside="close()" class="inline-block relative">
                <button @click="toggle()" class="card-badge card-badge--action" aria-label="More actions" aria-haspopup="true" :aria-expanded="open">
                    &#x22EF;
                </button>
                <div x-show="open" x-cloak class="absolute right-0 mt-1 w-48 bg-white dark:bg-gray-800 shadow-lg rounded-md z-50 border border-gray-200 dark:border-gray-600" role="menu">
                    {% for action in pluginCardActions %}
                    <button @click="runAction({{ action|json }}, {{ entity.ID }}, 'resource')"
                            class="block w-full text-left px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-gray-700" role="menuitem">
                        {{ action.Label }}
                    </button>
                    {% endfor %}
                </div>
            </div>
            {% endif %}
        </div>
    </div>
</article>
