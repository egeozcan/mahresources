{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
{% endblock %}

{% block body %}
    {% include "/partials/timeline.tpl" with entityApiUrl="/v1/groups" entityType="groups" entityDefaultView="/groups" %}
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col w-full" aria-label="Filter groups">
        {% if popularTags %}
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Tags" %}
            <div class="tags mb-2 gap-1 flex flex-wrap">
                {% for tag in popularTags %}
                <a class="no-underline" href='{{ withQuery("tags", stringId(tag.Id), true) }}'>
                    {% include "partials/tag.tpl" with name=tag.Name count=tag.Count active=hasQuery("tags", stringId(tag.Id)) %}
                </a>
                {% endfor %}
            </div>
        </div>
        {% endif %}
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Sort" %}
            {% include "/partials/form/multiSortInput.tpl" with name='SortBy' values=sortValues %}
        </div>
        <div class="sidebar-group">
            {% include "/partials/sideTitle.tpl" with title="Filter" %}
            {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
            {% include "/partials/form/checkboxInput.tpl" with name='SearchParentsForName' label='Search Parents For Name' value=queryValues.SearchParentsForName.0 id=getNextId("SearchParentsForName") %}
            {% include "/partials/form/checkboxInput.tpl" with name='SearchChildrenForName' label='Search Children For Name' value=queryValues.SearchChildrenForName.0 id=getNextId("SearchChildrenForName") %}
            {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
            {% include "/partials/form/textInput.tpl" with name='URL' label='URL' value=queryValues.URL.0 %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
            {% include "/partials/form/checkboxInput.tpl" with name='SearchParentsForTags' label='Search Parents For Tags' value=queryValues.SearchParentsForTags.0 id=getNextId("SearchParentsForTags") %}
            {% include "/partials/form/checkboxInput.tpl" with name='SearchChildrenForTags' label='Search Children For Tags' value=queryValues.SearchChildrenForTags.0 id=getNextId("SearchChildrenForTags") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/categories' elName='categories' title='Categories' selectedItems=categories id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/resources' elName='resources' title='Resources' selectedItems=resources id=getNextId("autocompleter") %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groupsSelection id=getNextId("autocompleter") extraInfo="Category" %}
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id=getNextId("autocompleter") extraInfo="Category" %}
            {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/groups/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
            {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
            {% include "/partials/form/searchButton.tpl" %}
        </div>
    </form>
{% endblock %}
