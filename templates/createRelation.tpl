{% extends "/layouts/base.tpl" %}

{% block body %}
<input type="hidden" name="RelationSideFrom" value="0" >
<input type="hidden" name="RelationSideTo" value="1" >

<form class="space-y-8" method="post" action="/v1/relation">
    {% if relation.ID > 0 %}
    <input type="hidden" value="{{ relation.ID }}" name="ID">
    {% endif %}
    <div class="space-y-8 sm:space-y-5">
        {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=relation.Name %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=relation.Description %}
        {% if !relation.ID %}
        <div class="mt-6 sm:mt-5 space-y-6 sm:space-y-5">
            <div class="mt-1 sm:mt-0 sm:col-span-2">
                <div>
                    {% include "/partials/form/autocompleter.tpl" with url='/v1/relationTypes' elName='GroupRelationTypeId' title='Type' selectedItems=relationType min=1 max=1 filterEls="[{ \"nameInput\": \"FromGroupId\", \"nameGet\": \"ForFromGroup\" }, { \"nameInput\": \"ToGroupId\", \"nameGet\": \"ForToGroup\" }]" id="autocompleter"|nanoid %}
                </div>
                <div>
                    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='FromGroupId' title='From Group' selectedItems=fromGroup min=1 max=1 filterEls="[{ \"nameInput\": \"GroupRelationTypeId\", \"nameGet\": \"RelationTypeId\" }, { \"nameInput\": \"RelationSideFrom\", \"nameGet\": \"RelationSide\" }]" id="autocompleter"|nanoid %}
                </div>
                <div>
                    {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='ToGroupId' title='To Group' selectedItems=toGroup min=1 max=1 filterEls="[{ \"nameInput\": \"GroupRelationTypeId\", \"nameGet\": \"RelationTypeId\" }, { \"nameInput\": \"RelationSideTo\", \"nameGet\": \"RelationSide\" }]" id="autocompleter"|nanoid %}
                </div>
            </div>
        </div>
        {% endif %}
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}