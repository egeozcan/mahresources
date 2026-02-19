{% extends "/layouts/gallery.tpl" %}

{% block gallery %}
    <div{% if owners && owners|length == 1 %} data-paste-context='{"type":"group","id":{{ owners.0.ID }},"name":"{{ owners.0.Name|escapejs }}"}'{% endif %}>
    {% for entity in notes %}
        {% include "/partials/note.tpl" %}
    {% endfor %}
    </div>
{% endblock %}

{% block sidebar %}
    <form class="flex gap-2 items-start flex-col" aria-label="Filter notes">
        <div class="tags mt-3 mb-2 gap-1 flex flex-wrap" style="margin-left: -0.5rem">
            {% for tag in popularTags %}
            <a class="no-underline" href='{{ withQuery("tags", stringId(tag.Id), true) }}'>
                {% include "partials/tag.tpl" with name=tag.Name count=tag.Count active=hasQuery("tags", stringId(tag.Id)) %}
            </a>
            {% endfor %}
        </div>
        {% include "/partials/sideTitle.tpl" with title="Sort" %}
        {% include "/partials/form/multiSortInput.tpl" with name='SortBy' values=sortValues %}
        {% include "/partials/sideTitle.tpl" with title="Filter" %}
        {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
        {% include "/partials/form/textInput.tpl" with name='Description' label='Text' value=queryValues.Description.0 %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owners id=getNextId("autocompleter") extraInfo="Category" %}
        {% include "/partials/form/autocompleter.tpl" with url='/v1/note/noteTypes' elName='NoteTypeId' title='Note Type' selectedItems=noteTypes max=1 id=getNextId("autocompleter") %}
        {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/notes/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateBefore' label='Start Date Before' value=queryValues.StartDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='StartDateAfter' label='Start Date After' value=queryValues.StartDateAfter.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateBefore' label='End Date Before' value=queryValues.EndDateBefore.0 %}
        {% include "/partials/form/dateInput.tpl" with name='EndDateAfter' label='End Date After' value=queryValues.EndDateAfter.0 %}
        {% include "/partials/form/checkboxInput.tpl" with name='Shared' label='Shared Only' value=queryValues.Shared.0 id=getNextId("Shared") %}
        {% include "/partials/form/searchButton.tpl" %}
    </form>
{% endblock %}