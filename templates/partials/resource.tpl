{% with cardSc=entity.ResourceCategory.SectionConfig|sectionConfig:"resource" %}
<article class="card resource-card{% if cardSc.PreviewImage %} card--with-image{% endif %}{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
    <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    {% endif %}

    <div data-entity='{{ entity|json }}' x-data="{ entity: JSON.parse($el.dataset.entity) }">
        {% if cardSc.PreviewImage %}
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
                <img height="300" src="/v1/resource/preview?id={{ entity.ID }}&height=300&v={{ entity.Hash }}" alt="Preview of {{ entity.Name }}" loading="lazy" decoding="async">
            </a>
        </div>
        {% endif %}

        <header class="card-header card-header--compact">
            <div class="card-title-section">
                <h{{ cardHeadingLevel|default:2 }} class="card-title">
                    <a href="/resource?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
                </h{{ cardHeadingLevel|default:2 }}>
                {% if cardSc.FileSize || (cardSc.Owner && entity.Owner) || (entity.ResourceCategory && (cardSc.CategoryLink || entity.ResourceCategory.CustomAvatar)) %}
                <div class="card-meta">
                    {% if cardSc.FileSize %}
                    <span class="card-meta-item">{{ entity.FileSize | humanReadableSize }}</span>
                    {% endif %}
                    {% if cardSc.Owner && entity.Owner %}
                    <span class="card-meta-item">
                        <span class="card-meta-label">Owner:</span>
                        <a href="/group?id={{ entity.Owner.ID }}" class="card-meta-link">{{ entity.Owner.Name }}</a>
                    </span>
                    {% endif %}
                    {% if entity.ResourceCategory && (cardSc.CategoryLink || entity.ResourceCategory.CustomAvatar) %}
                    <span class="card-meta-item">
                        {% process_shortcodes entity.ResourceCategory.CustomAvatar entity %}
                        {% if cardSc.CategoryLink %}
                        <a href="/resourceCategory?id={{ entity.ResourceCategory.ID }}" class="card-meta-link">{{ entity.ResourceCategory.Name }}</a>
                        {% endif %}
                    </span>
                    {% endif %}
                </div>
                {% endif %}
            </div>
        </header>

        {% process_shortcodes entity.ResourceCategory.CustomSummary entity %}

        {% if cardSc.Description && entity.Description %}
        <div class="card-description">
            {% include "partials/description.tpl" with description=entity.Description descriptionEntity=entity preview=true %}
        </div>
        {% endif %}

        {% if cardSc.Tags || pluginCardActions %}
        <div class="tags card-tags">
            {% if cardSc.Tags %}
            {% for tag in entity.Tags %}
            <a class="card-badge{% if !tagBaseUrl && hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{% if tagBaseUrl %}{{ tagBaseUrl }}?tags={{ tag.ID }}{% else %}{{ withQuery("tags", stringId(tag.ID), true) }}{% endif %}'>
                {{ tag.Name }}
            </a>
            {% endfor %}
            <button x-cloak data-entity-type="resource" class="edit-in-list card-badge card-badge--action">
                Edit Tags
            </button>
            {% endif %}
            {% include "partials/pluginActionsCard.tpl" with entityType="resource" %}
        </div>
        {% endif %}
    </div>
</article>
{% endwith %}
