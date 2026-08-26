{% extends "/layouts/base.tpl" %}

{% block body %}
    {# partials/title.tpl already renders the page's <h1> from pageTitle. #}
    <div class="mb-3 space-y-1">
        <p class="text-sm text-stone-500">
            <span data-testid="reductions-count">{{ reductionsCount }} Resource Reduction{% if reductionsCount != 1 %}s{% endif %}</span>.
            A Resource Reduction never expires — it stays until you delete it.
        </p>
    </div>

    {# A single column rather than the .list-container grid: a Reduction is a name, #}
    {# a status and a row of dates, none of which survive a 280px cell. Same choice  #}
    {# as /queries and /downloads.                                                   #}
    <section class="items-container" data-testid="reductions-page">
        {% for entity in reductions %}
            {% include "/partials/resourceReduction.tpl" %}
        {% empty %}
            {# There is no create page: a Reduction is made from a selection on the  #}
            {# resources or groups list, so createUrl is deliberately omitted.       #}
            {% include "/partials/listEmpty.tpl" with label="Resource Reductions" %}
        {% endfor %}
    </section>

{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter Resource Reductions">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            <fieldset class="w-full mt-2">
                <legend class="block text-sm font-medium font-mono text-stone-700">Status</legend>
                {% for status in reductionStatuses %}
                    {% if status.Link %}
                    <label class="flex items-center gap-2 mt-1 text-sm text-stone-700">
                        <input type="checkbox" name="Status" value="{{ status.Link }}" {% if status.Active %}checked{% endif %}
                               class="reduction-control rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                        {{ status.Title }}
                    </label>
                    {% endif %}
                {% endfor %}
            </fieldset>

            {% include "/partials/form/textInput.tpl" with name='Name' label='Name contains' value=queryValues.Name.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>
{% endblock %}
