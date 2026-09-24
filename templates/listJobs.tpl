{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/bulkActions.tpl" with bulkEntity="job" %}
{% endblock %}

{% block body %}
<div x-data="jobList()" data-testid="job-center">
    <p class="mb-2 text-xs text-stone-600" role="status" aria-live="polite" data-testid="job-live-status">
        <span x-text="connectionText">Live updates need JavaScript; reload for new jobs.</span>
    </p>
    {# Visible only: the bulk bar already announced it, and a second live region would read it twice. #}
    <p class="mb-2 text-sm text-stone-800" x-show="notice" x-text="notice" x-cloak data-testid="job-list-notice"></p>
    <section class="list-container" aria-label="Jobs">
    {% for job in jobs %}
        {% include "/partials/job.tpl" %}
    {% empty %}
        {% if pagination %}
        {# A later keyset page whose Jobs were dismissed or left the filter: the #}
        {# shared empty state reads a page position as no filter and would say  #}
        {# there are no Jobs at all, while Previous still leads to some.        #}
        <div class="detail-empty">No jobs on this page any more. Use <strong>Previous</strong> to go back.</div>
        {% else %}
        {% include "/partials/listEmpty.tpl" with label="jobs" %}
        {% endif %}
    {% endfor %}
    </section>
</div>
{% endblock %}

{% block sidebar %}
    {% if jobQuickFilters %}
    <div class="sidebar-group" data-job-quick-filters>
        {% include "/partials/sideTitle.tpl" with title="Show" %}
        <ul class="flex flex-wrap gap-1 mb-2">
            {% for quick in jobQuickFilters %}
            <li>
                <a class="no-underline" href="{{ quick.Link }}" data-job-quick-filter="{{ quick.Key }}"{% if quick.Active %} aria-current="true"{% endif %}>
                    <span class="text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1 font-mono rounded-full {% if quick.Active %}bg-amber-100 text-amber-700{% else %}bg-yellow-100 text-stone-700 border{% endif %}">{{ quick.Label }} ({{ quick.Count }})</span>
                </a>
            </li>
            {% endfor %}
        </ul>
    </div>
    {% endif %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter jobs" action="/jobs" method="get"
          x-data="jobFilterTimes()" @submit="submit()">
        <div class="filter-controls-scroll">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            {% include "/partials/form/textInput.tpl" with name='search' label='Search' value=jobFilter.Search %}

            <fieldset class="mt-2">
                <legend class="block text-xs font-mono font-medium text-stone-600">State</legend>
                {% for state in jobStateOptions %}
                <label class="flex items-center gap-2 min-h-7 cursor-pointer">
                    <input type="checkbox" name="state" value="{{ state }}"{% if state in jobFilter.States %} checked{% endif %} class="focus:ring-1 focus:ring-amber-600 h-3.5 w-3.5 text-amber-700 border-stone-300 rounded">
                    <span class="text-xs font-mono font-medium text-stone-600">{{ state }}</span>
                </label>
                {% endfor %}
            </fieldset>

            {% if jobKindOptions %}
            <fieldset class="mt-2">
                <legend class="block text-xs font-mono font-medium text-stone-600">Kind</legend>
                {% for kind in jobKindOptions %}
                <label class="flex items-center gap-2 min-h-7 cursor-pointer">
                    <input type="checkbox" name="kind" value="{{ kind }}"{% if kind in jobFilter.Kinds %} checked{% endif %} class="focus:ring-1 focus:ring-amber-600 h-3.5 w-3.5 text-amber-700 border-stone-300 rounded">
                    <span class="text-xs font-mono font-medium text-stone-600">{{ kind }}</span>
                </label>
                {% endfor %}
            </fieldset>
            {% endif %}

            <label for="job-filter-command" class="block text-xs font-mono font-medium text-stone-600 mt-2">Available command</label>
            <select name="command" id="job-filter-command" aria-describedby="job-filter-command-help" class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
                <option value="">Any</option>
                {% for key in jobCommandOptions %}
                <option value="{{ key }}"{% if jobFilter.Command == key %} selected{% endif %}>{{ key }}</option>
                {% endfor %}
            </select>
            <p id="job-filter-command-help" class="mt-0.5 text-xs text-stone-500">Jobs currently offering this command.</p>

            {% include "/partials/form/textInput.tpl" with name='origin' label='Origin' value=jobFilter.OriginText %}
            {% include "/partials/form/textInput.tpl" with name='ownerId' label='Owner ID' value=jobFilter.OwnerID %}
            {% include "/partials/form/textInput.tpl" with name='actorId' label='Actor ID' value=jobFilter.ActorID %}
            {# datetime-local, not the shared date input: a bookmark or a legacy link can #}
            {# name an instant, and a date would widen it to a whole day on the next submit. #}
            {# A value with seconds needs step="1", or the browser refuses to submit it. #}
            {# Without JS these are server-local; jobFilterTimes shows and sends the     #}
            {# reader's own zone, and returns an untouched bound as its exact instant.  #}
            <label for="acceptedAfter" class="block text-xs font-mono font-medium text-stone-600 mt-2">Accepted after</label>
            <input type="datetime-local" name="acceptedAfter" id="acceptedAfter" value="{{ jobFilter.AcceptedAfter }}" data-bound="start" data-instant="{{ jobFilter.AcceptedAfterInstant }}"{% if jobFilter.AcceptedAfter|length > 16 %} step="1"{% endif %} autocomplete="off"
                   class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
            <label for="acceptedBefore" class="block text-xs font-mono font-medium text-stone-600 mt-2">Accepted before</label>
            <input type="datetime-local" name="acceptedBefore" id="acceptedBefore" value="{{ jobFilter.AcceptedBefore }}" data-bound="end" data-instant="{{ jobFilter.AcceptedBeforeInstant }}"{% if jobFilter.AcceptedBefore|length > 16 %} step="1"{% endif %} autocomplete="off"
                   class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">

            <label for="job-filter-relationship" class="block text-xs font-mono font-medium text-stone-600 mt-2">Relationship</label>
            <select name="relationship" id="job-filter-relationship" class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
                <option value="">Any relationship</option>
                <option value="retry-of"{% if jobFilter.Relationship == "retry-of" %} selected{% endif %}>Retry successor</option>
                <option value="repeat-of"{% if jobFilter.Relationship == "repeat-of" %} selected{% endif %}>Repeat successor</option>
                <option value="parent-child"{% if jobFilter.Relationship == "parent-child" %} selected{% endif %}>Parent stage</option>
            </select>

            <label for="job-filter-pinned" class="block text-xs font-mono font-medium text-stone-600 mt-2">Pinned</label>
            <select name="pinned" id="job-filter-pinned" class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
                <option value="">Any</option>
                <option value="true"{% if jobFilter.Pinned == "true" %} selected{% endif %}>Pinned</option>
                <option value="false"{% if jobFilter.Pinned == "false" %} selected{% endif %}>Not pinned</option>
            </select>

            <label for="job-filter-dismissed" class="block text-xs font-mono font-medium text-stone-600 mt-2">Dismissed</label>
            <select name="dismissed" id="job-filter-dismissed" class="mt-0.5 focus:ring-1 focus:ring-amber-600 focus:border-amber-600 block w-full text-sm border-stone-300 rounded">
                <option value="">Not dismissed</option>
                <option value="any"{% if jobFilter.Dismissed == "any" %} selected{% endif %}>Any</option>
                <option value="true"{% if jobFilter.Dismissed == "true" %} selected{% endif %}>Dismissed</option>
            </select>
        </div>
        </div>
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
