{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorTag.tpl" %}
{% endblock %}

{% block body %}
    <div class="list-container">
        {% for tag in tags %}
            <article class="card tag-card card--selectable" x-data="selectableItem({ itemId: {{ tag.ID }} })">
                <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ tag.Name }}" class="card-checkbox focus:ring-amber-600 h-6 w-6 text-amber-700 border-stone-300 rounded">
                <h2 class="card-title card-title--simple">
                    <a href="/tag?id={{ tag.ID }}">{{ tag.Name }}</a>
                </h2>
                {% if tag.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=tag.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            {% include "/partials/listEmpty.tpl" with label="tags" createUrl="/tag/new" %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter tags">
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Sort" %}
            {% include "/partials/form/multiSortInput.tpl" with name='SortBy' values=sortValues %}
        </div>
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>
{% endblock %}
