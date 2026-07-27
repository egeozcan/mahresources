{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/relation">
    <input type="hidden" name="RelationSideFrom" value="0" >
    <input type="hidden" name="RelationSideTo" value="1" >
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
                    {% include "/partials/form/autocompleter.tpl" with profile='single' entity='relationType' elName='GroupRelationTypeId' title='Type' selectedItems=relationType min=1 max=1 parameters="selectorFormParameters({ ForFromGroup: 'FromGroupId', ForToGroup: 'ToGroupId' })" id=getNextId("autocompleter") %}
                </div>
                <div>
                    {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' categoryDecoration=true elName='FromGroupId' title='From Group' selectedItems=fromGroup min=1 max=1 parameters="selectorFormParameters({ RelationTypeId: 'GroupRelationTypeId', RelationSide: 'RelationSideFrom' })" id=getNextId("autocompleter") %}
                </div>
                <div>
                    {% include "/partials/form/autocompleter.tpl" with profile='single' entity='group' categoryDecoration=true elName='ToGroupId' title='To Group' selectedItems=toGroup min=1 max=1 parameters="selectorFormParameters({ RelationTypeId: 'GroupRelationTypeId', RelationSide: 'RelationSideTo' })" id=getNextId("autocompleter") %}
                </div>
            </div>
        </div>
        {% endif %}
    </div>

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}