{% extends "/layouts/base.tpl" %}

{% block body %}
    {# partials/title.tpl already renders the page's <h1> from pageTitle. #}
    <div class="mb-3 space-y-1">
        <p class="text-sm text-stone-500">
            <span data-testid="reductions-count">{{ reductionsCount }} Resource Reduction{% if reductionsCount != 1 %}s{% endif %}</span>
        </p>
    </div>

    {# A single column rather than the .list-container grid: a Reduction is a name, #}
    {# a status and a row of dates, none of which survive a 280px cell. Same choice  #}
    {# as /queries and /downloads.                                                   #}
    <section class="items-container" data-testid="reductions-page">
        {% for entity in reductions %}
            {% include "/partials/resourceReduction.tpl" %}
        {% empty %}
            {% if hasActiveFilter %}
                {% include "/partials/listEmpty.tpl" with label="Resource Reductions" createUrl="/reduction/new" %}
            {% else %}
                <div class="detail-empty reduction-empty space-y-3" data-testid="reductions-empty" x-data="reductionLibraryStart">
                    <p class="text-stone-700 max-w-2xl">
                        A Resource Reduction finds copies of the same file and near-identical images, lets you choose which copy to keep, and merges the rest into it. Nothing is removed until you review and apply.
                    </p>
                    <div class="flex flex-wrap items-center gap-3">
                        <button type="button" @click="start()" :disabled="busy" data-testid="reductions-find-everything"
                                class="inline-flex items-center justify-center min-h-[2.75rem] py-1.5 px-3 border border-transparent shadow-sm text-sm font-mono font-medium rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600 disabled:opacity-50 disabled:cursor-not-allowed">
                            Find duplicates in everything
                        </button>
                        <a href="/reduction/new"
                           class="inline-flex items-center justify-center min-h-[2.75rem] py-1.5 px-3 border border-stone-300 shadow-sm text-sm font-mono font-medium rounded-md text-stone-700 bg-white hover:bg-stone-50 focus:outline-hidden focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">New</a>
                    </div>
                    <p x-show="error" x-cloak x-text="error" role="alert" class="text-sm text-red-700"></p>
                </div>
            {% endif %}
        {% endfor %}
    </section>

{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter Resource Reductions">
        <div class="filter-controls-scroll">
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
        </div>
        </div>
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
