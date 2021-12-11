{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for tag in tags %}
        <a href="/tag?id={{ tag.ID }}">
            <div class="bg-gray-50 p-4">
                {% include "/partials/subtitle.tpl" with title=tag.Name %}
                {% include "/partials/description.tpl" with description=tag.Description %}
            </div>
        </a>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}