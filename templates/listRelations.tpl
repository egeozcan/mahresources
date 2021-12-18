{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="flex gap-4 flex-wrap">
        {% for relation in relations %}
            <div class="bg-gray-50 p-4">
                <a href="/relation?id={{ relation.ID }}" class="pb-3 block">
                    {% include "/partials/subtitle.tpl" with title=relation.Name alternativeTitle="Relation" %}
                </a>
                {% include "/partials/relation.tpl" with entity=relation %}
                {% include "/partials/relation_reverse.tpl" with entity=relation %}
            </div>
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    <form class="flex gap-2 items-start flex-col">
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/relationTypes' elName='GroupRelationTypeId' title='Type' max=1 selectedItems=fromTypes id="autocompleter"|nanoid %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='FromGroupId' title='From Group' max=1 selectedItems=fromGroups id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='ToGroupId' title='To Group' max=1 selectedItems=toGroups id="autocompleter"|nanoid extraInfo="Category" %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}