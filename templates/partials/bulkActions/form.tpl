<div x-data="bulkSelectionForms">
    <form data-editor-label="{% if action.Input %}{{ action.Label }}{% else %}{{ action.SubmitLabel|default:action.Label }}{% endif %}" x-cloak x-show="$selection.isActiveEditor($el)" x-collapse :class="$selection.isActiveEditor($el) && 'active'" method="post" action="{{ action.Endpoint }}" :action="'{{ action.Endpoint }}?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
          {% if action.NoAjax %}class="no-ajax"{% endif %} data-testid="bulk-{{ action.ID }}-{{ bulkEntity }}s-form" data-confirm-message="{{ action.Confirm }}"
          {% if action.ConfirmComponent %}x-data="{{ action.ConfirmComponent }}" x-bind="events"{% elif action.Confirm %}x-data="confirmAction()" x-bind="events"{% endif %}>
        {% include "/partials/form/formParts/connected/selectedIds.tpl" %}
        {% if action.Input == 'entity' %}
            {% if action.InputEntity == 'tag' and not action.Multiple %}
                {% include "/partials/form/autocompleter.tpl" with profile='tag' usage=bulkEntity elName='editedId' title=action.InputLabel id=getNextId("bulk_input") %}
            {% elif action.Multiple and action.InputEntity == 'tag' %}
                {% include "/partials/form/autocompleter.tpl" with profile='multi' entity='tag' usage=bulkEntity elName='editedId' title=action.InputLabel id=getNextId("bulk_input") %}
            {% elif action.Multiple %}
                {% include "/partials/form/autocompleter.tpl" with profile='multi' entity=action.InputEntity categoryDecoration=true elName='editedId' title=action.InputLabel id=getNextId("bulk_input") %}
            {% else %}
                {% include "/partials/form/autocompleter.tpl" with profile='single' entity=action.InputEntity max=1 elName='editedId' title=action.InputLabel id=getNextId("bulk_input") %}
            {% endif %}
        {% elif action.Input == 'meta' %}
            {% include "/partials/form/freeFields.tpl" with name="Meta" url=bulkMetaURL(bulkEntity) jsonOutput=true id=getNextId("bulk_meta") %}
        {% endif %}
        {% include "/partials/form/searchButton.tpl" with text=action.SubmitLabel|default:action.Label danger=action.Danger %}
    </form>
</div>
