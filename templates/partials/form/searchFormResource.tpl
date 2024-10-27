<form class="flex gap-2 items-start flex-col bg-white pl-4">
    <div class="tags mt-3 mb-2 gap-1 flex flex-wrap" style="margin-left: -0.5rem">
        {% for tag in popularTags %}
        <a class="no-underline" href='{{ withQuery("tags", stringId(tag.Id), true) }}'>
            {% include "partials/tag.tpl" with name=tag.Name active=hasQuery("tags", stringId(tag.Id)) %}
        </a>
        {% endfor %}
    </div>
    {% include "/partials/sideTitle.tpl" with title="Sort" %}
    {% include "/partials/form/selectInput.tpl" with name='SortBy' label='Sort' values=sortValues %}
    {% include "/partials/sideTitle.tpl" with title="Filter" %}
    {% include "/partials/form/textInput.tpl" with name='Name' label='Name' value=queryValues.Name.0 %}
    {% include "/partials/form/textInput.tpl" with name='Description' label='Description' value=queryValues.Description.0 %}
    {% include "/partials/form/textInput.tpl" with name='OriginalName' label='Original Name' value=queryValues.OriginalName.0 %}
    {% include "/partials/form/textInput.tpl" with name='Hash' label='Hash' value=queryValues.Hash.0 %}
    {% include "/partials/form/textInput.tpl" with name='ContentType' label='Content Type' value=queryValues.ContentType.0 %}
    {% include "/partials/form/textInput.tpl" with name='OriginalLocation' label='Original Location' value=queryValues.OriginalLocation.0 %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Tags' selectedItems=tags id=getNextId("autocompleter") %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/notes' elName='notes' title='Notes' selectedItems=notes id=getNextId("autocompleter") %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='groups' title='Groups' selectedItems=groups id=getNextId("autocompleter") extraInfo="Category" %}
    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' max=1 elName='ownerId' title='Owner' selectedItems=owner id=getNextId("autocompleter") extraInfo="Category" %}
    {% include "/partials/form/freeFields.tpl" with name="MetaQuery" url='/v1/resources/meta/keys' fields=parsedQuery.MetaQuery id=getNextId("freeField") %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedBefore' label='Created Before' value=queryValues.CreatedBefore.0 %}
    {% include "/partials/form/dateInput.tpl" with name='CreatedAfter' label='Created After' value=queryValues.CreatedAfter.0 %}
    {% include "/partials/form/textInput.tpl" with name='MinWidth' label='Min Width' value=queryValues.MinWidth.0 %}
    {% include "/partials/form/textInput.tpl" with name='MaxWidth' label='Max Width' value=queryValues.MaxWidth.0 %}
    {% include "/partials/form/textInput.tpl" with name='MinHeight' label='Min Height' value=queryValues.MinHeight.0 %}
    {% include "/partials/form/textInput.tpl" with name='MaxHeight' label='Max Height' value=queryValues.MaxHeight.0 %}
    {% include "/partials/form/checkboxInput.tpl" with name='ShowWithSimilar' label='Show Only With Existing Similar Images' value=queryValues.ShowWithSimilar.0 id=getNextId("ShowWithSimilar") %}
    {% include "/partials/form/searchButton.tpl" %}
</form>
