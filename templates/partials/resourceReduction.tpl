{% if entity %}
<article class="card reduction-card" data-testid="reduction-card" data-reduction-id="{{ entity.ID }}">
    <header class="card-header card-header--compact">
        <div class="card-title-section">
            <h2 class="card-title">
                <a href="/reduction?id={{ entity.ID }}" title="{{ entity.Name }}">{{ entity.Name }}</a>
            </h2>
            <div class="card-meta">
                {# The creation date and time is what tells two similarly named    #}
                {# Reductions apart, so it leads rather than sitting in a tooltip. #}
                <span class="card-meta-item">
                    <span class="card-meta-label">Created:</span>
                    {{ entity.CreatedAt|date:"2006-01-02 15:04" }}
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Status:</span>
                    <span data-testid="reduction-status">{{ entity.StatusLabel }}</span>
                </span>
                <span class="card-meta-item">
                    <span class="card-meta-label">Matching:</span>
                    {{ entity.MatchingLabel }}
                </span>
                {% if entity.ComputedAt %}
                <span class="card-meta-item">
                    <span class="card-meta-label">Last computed:</span>
                    {{ entity.ComputedAt|date:"2006-01-02 15:04" }}
                </span>
                {% endif %}
            </div>
        </div>
    </header>
</article>
{% endif %}
