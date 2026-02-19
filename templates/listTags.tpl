{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/bulkEditorTag.tpl" %}
{% endblock %}

{% block body %}
    <div class="list-container">
        {% for tag in tags %}
            <article class="card tag-card card--selectable" x-data="selectableItem({ itemId: {{ tag.ID }} })">
                <input type="checkbox" :checked="selected() ? 'checked' : null" x-bind="events" aria-label="Select {{ tag.Name }}" class="card-checkbox focus:ring-indigo-500 h-6 w-6 text-indigo-600 border-gray-300 rounded">
                <h3 class="card-title card-title--simple">
                    <a href="/tag?id={{ tag.ID }}">{{ tag.Name }}</a>
                </h3>
                {% if tag.Description %}
                <div class="card-description">
                    {% include "/partials/description.tpl" with description=tag.Description preview=true %}
                </div>
                {% endif %}
            </article>
        {% empty %}
            <p class="text-gray-500 text-sm py-4">No tags found. <a href="/createTag" class="text-indigo-600 hover:text-indigo-800 underline">Create one</a>.</p>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col" aria-label="Filter tags">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}
