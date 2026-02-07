{% if entity %}
<article class="card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.GetName() }}" class="card-checkbox focus:ring-indigo-500 h-5 w-5 text-indigo-600 border-gray-300 rounded">
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
                    <a href="/group?id={{ entity.ID }}" title="{{ entity.GetName() }}">
                        <inline-edit post="/v1/group/editName?id={{ entity.ID }}" name="name">{{ entity.GetName() }}</inline-edit>
                    </a>
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
                    <a href="{{ withQuery("categories", stringId(entity.CategoryId), true) }}" class="card-badge card-badge--category{% if hasQuery("categories", stringId(entity.CategoryId)) %} card-badge--tag-active{% endif %}">
                        {{ entity.Category.Name }}
                    </a>
                </div>
                {% endif %}
            </div>
        </header>

        {% if entity.URL && entity.URL|printUrl && !fullText %}
            <a class="card-url" target="_blank" referrerpolicy="no-referrer" href="{{ entity.URL|printUrl }}" aria-label="External link: {{ entity.URL|printUrl }}">{{ entity.URL|printUrl }}</a>
        {% endif %}

        {% autoescape off %}
            {{ entity.Category.CustomSummary }}
        {% endautoescape %}

        {% if !reverse && relation && relation.Description && !noRelDescription %}
            <a target="_blank" href="/relation?id={{ relation.ID }}" referrerpolicy="no-referrer" class="card-description block">
                {% include "partials/description.tpl" with description=relation.Description preview=!fullText %}
            </a>
        {% endif %}

        {% if !noDescription %}
            <div class="card-description">
                {% include "partials/description.tpl" with description=entity.Description preview=!fullText %}
            </div>
        {% endif %}

        {% if !noTag && entity.Tags %}
            <div class="tags card-tags">
                {% for tag in entity.Tags %}
                <a class="card-badge{% if hasQuery("tags", stringId(tag.ID)) %} card-badge--tag-active{% else %} card-badge--tag{% endif %}" href='{{ withQuery("tags", stringId(tag.ID), true) }}'>
                    {{ tag.Name }}
                </a>
                {% endfor %}
            </div>
        {% endif %}
    </div>
</article>
{% endif %}
