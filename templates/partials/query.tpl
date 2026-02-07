{% if entity %}
<article class="card query-card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-indigo-500 h-5 w-5 text-indigo-600 border-gray-300 rounded">
    {% endif %}

    <div x-data='{ "entity": {{ entity|json }} }'>
        <header class="card-header card-header--compact">
            <div class="card-title-section">
                <h3 class="card-title">
                    <a href="/query?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
                </h3>
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
            </div>
        </header>

        {% if !noDescription && entity.Text %}
        <div class="card-description">
            {% include "partials/description.tpl" with description=entity.Text preview=true %}
        </div>
        {% endif %}
    </div>
</article>
{% endif %}
