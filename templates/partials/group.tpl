{% if entity %}
<article class="card group-card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.GetName() }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
    {% endif %}

    <div x-data='{ "entity": {{ entity|json }} }'>
        <header class="card-header">
            {% if !fullText %}
            <div class="card-avatar">
                {% include "partials/avatar.tpl" with initials=entity.Initials() %}
            </div>
            {% endif %}

            <div class="card-title-section">
                {% if relation && reverse %}
                    <a href="/relation?id={{ relation.ID }}" class="card-badge card-badge--relation mb-1">
                        {{ relation.RelationType.Name }}
                    </a>
                {% endif %}

                <h3 class="card-title">
                    <a href="/group?id={{ entity.ID }}" title="{{ entity.GetName() }}">{{ entity.GetName() }}</a>
                </h3>

                {% if relation && !reverse %}
                    <a href="/relation?id={{ relation.ID }}" class="card-badge card-badge--relation mt-1">
                        {{ relation.RelationType.Name }}
                    </a>
                {% endif %}

                {% if !fullText %}
                <div class="card-meta">
                    <span class="card-meta-item">
                        <span class="card-meta-label">Updated:</span>
                        {{ entity.UpdatedAt|date:"2006-01-02 15:04" }}
                    </span>
                    <span class="card-meta-item">
                        <span class="card-meta-label">Created:</span>
                        {{ entity.CreatedAt|date:"2006-01-02 15:04" }}
                    </span>
                </div>
                {% endif %}

                {% if entity.Category && !fullText %}
                <div class="card-badges">
                    <a href="{% if tagBaseUrl %}{{ tagBaseUrl }}?categories={{ entity.CategoryId }}{% else %}{{ withQuery("categories", stringId(entity.CategoryId), true) }}{% endif %}" title="{{ entity.Category.Name }}" class="card-badge card-badge--category{% if !tagBaseUrl && hasQuery("categories", stringId(entity.CategoryId)) %} card-badge--tag-active{% endif %}">
                        {{ entity.Category.Name }}
                    </a>
                </div>
                {% endif %}
            </div>
        </header>

        {% if entity.URL && entity.URL|printUrl && !fullText %}
            <a class="card-url" target="_blank" referrerpolicy="no-referrer" href="{{ entity.URL|printUrl }}" aria-label="External link: {{ entity.URL|printUrl }}">{{ entity.URL|printUrl }}</a>
        {% endif %}

        {% process_shortcodes entity.Category.CustomSummary entity %}

        {% if !reverse && relation && relation.Description && !noRelDescription %}
            <a target="_blank" href="/relation?id={{ relation.ID }}" referrerpolicy="no-referrer" class="card-description block">
                {% include "partials/description.tpl" with description=relation.Description descriptionEntity=relation preview=!fullText %}
            </a>
        {% endif %}

        {% if !noDescription %}
            <div class="card-description">
                {% include "partials/description.tpl" with description=entity.Description descriptionEntity=entity preview=!fullText %}
            </div>
        {% endif %}

        {% if !noTag && entity.Tags %}
            <div class="tags card-tags">
                {% for tag in entity.Tags %}
                <a class="card-badge{% if !tagBaseUrl && hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{% if tagBaseUrl %}{{ tagBaseUrl }}?tags={{ tag.ID }}{% else %}{{ withQuery("tags", stringId(tag.ID), true) }}{% endif %}'>
                    {{ tag.Name }}
                </a>
                {% endfor %}
            </div>
        {% endif %}
        {% include "partials/pluginActionsCard.tpl" with entityType="group" %}
    </div>
</article>
{% endif %}
