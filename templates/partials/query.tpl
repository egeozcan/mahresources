{% if entity %}
<article class="card query-card{% if selectable %} card--selectable{% endif %}" {% if selectable %}x-data="selectableItem({ itemId: {{ entity.ID }} })"{% endif %}>
    {% if selectable %}
        <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ entity.Name }}" class="card-checkbox focus:ring-amber-600 h-5 w-5 text-amber-700 border-stone-300 rounded">
    {% endif %}

    <div x-data='{ "entity": {{ entity|json }} }'>
        <header class="card-header card-header--compact">
            <div class="card-title-section">
                <h2 class="card-title">
                    <a href="/query?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
                </h2>
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
        {# Finding 82: this used to go through partials/description.tpl with        #}
        {# preview=true, whose preview branch runs the pongo2-addons `markdown`     #}
        {# filter — blackfriday with Smartypants — so '' became a curly quote, --   #}
        {# an en dash and ... an ellipsis. SQL copied off a card was invalid. SQL   #}
        {# is code, not prose: render it verbatim in a <code> block and never send  #}
        {# it through a markdown filter.                                            #}
        <div class="card-description">
            <pre class="query-card-sql"><code>{{ entity.Text|truncatechars:250 }}</code></pre>
        </div>
        {% endif %}
    </div>
</article>
{% endif %}
