{% extends "/layouts/base.tpl" %}

{% block body %}
    <div class="items-container">
        {% for relation in relations %}
            <article class="card relation-card card--relation-container">
                <header class="card-header">
                    <div class="card-title-section">
                        <a href="/relation?id={{ relation.ID }}">
                            <h2 class="card-title">{% if relation.Name %}{{ relation.Name }}{% else %}Relation{% endif %}</h2>
                        </a>
                    </div>
                </header>
                <div class="relation-groups">
                    {% include "/partials/relation_reverse.tpl" with entity=relation %}
                    <div class="relation-arrow">
                        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                            <line x1="5" y1="12" x2="19" y2="12"></line>
                            <polyline points="12 5 19 12 12 19"></polyline>
                        </svg>
                    </div>
                    {% include "/partials/relation.tpl" with entity=relation %}
                </div>
            </article>
        {% empty %}
            {% include "/partials/listEmpty.tpl" with label="relations" createUrl="/relation/new" %}
        {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter relations">
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='relationType' elName='GroupRelationTypeId' title='Type' max=1 selectedItems=fromTypes id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' categoryDecoration=true elName='FromGroupId' title='From Group' max=1 selectedItems=fromGroups id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' categoryDecoration=true elName='ToGroupId' title='To Group' max=1 selectedItems=toGroups id=getNextId("autocompleter") %}
            {% include "/partials/form/searchButton.tpl" %}
        </form>
    </div>
{% endblock %}
