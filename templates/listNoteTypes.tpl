{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for noteType in noteTypes %}
            <a href="/noteType?id={{ noteType.ID }}">
                <div class="bg-gray-50 p-4">
                    {% include "/partials/subtitle.tpl" with title=noteType.Name %}
                    {% include "/partials/description.tpl" with description=noteType.Description %}
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
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}