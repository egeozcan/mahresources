{% extends "/layouts/base.tpl" %}

{# WS6, finding 32 — the destination for the Cmd+K dialog's "see all" row.     #}
{# /search answered 404 before this, so the dialog's fifteen rows were all      #}
{# there was. Deliberately unpaginated: the search service caps its result set  #}
{# at 50 rows, so a page 2 would be empty by construction. The cap is stated    #}
{# instead of being paged past.                                                 #}

{% block body %}
    <div class="flex flex-col gap-4 items-container">
        {% if searchQuery == "" %}
            <div class="detail-empty">Enter a search term to look across resources, notes, groups and tags.</div>
        {% else %}
            <p class="text-sm text-stone-600 font-mono mb-2">
                {% if total == 0 %}
                    No results for &ldquo;{{ searchQuery }}&rdquo;.
                {% else %}
                    {{ total }}{% if totalCapped %}+{% endif %} result{% if total != 1 %}s{% endif %} for &ldquo;{{ searchQuery }}&rdquo;{% if totalCapped %} (showing the first {{ total }}){% endif %}.
                {% endif %}
            </p>

            {% for group in groups %}
                <section class="detail-panel" aria-labelledby="search-section-{{ group.Type }}">
                    <h2 id="search-section-{{ group.Type }}" class="detail-panel-title">{{ group.Label }}</h2>
                    <ul class="flex flex-col gap-2">
                        {% for item in group.Items %}
                            <li>
                                <a href="{{ item.URL }}" class="text-amber-700 hover:text-amber-900 underline">{{ item.Name }}</a>
                                {% if item.Description %}
                                    <div class="text-sm text-stone-600">{{ item.Description|truncatechars:160 }}</div>
                                {% endif %}
                            </li>
                        {% endfor %}
                    </ul>
                </section>
            {% empty %}
                <div class="detail-empty">No results match &ldquo;{{ searchQuery }}&rdquo;. Try a different search term.</div>
            {% endfor %}
        {% endif %}
    </div>
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Search" method="get" action="/search">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Search" %}
            {% include "/partials/form/textInput.tpl" with name='q' label='Search term' value=searchQuery %}
            {% include "/partials/form/searchButton.tpl" with text="Search" %}
        </div>
    </form>
{% endblock %}
