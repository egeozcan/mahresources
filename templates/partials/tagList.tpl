{% include "/partials/sideTitle.tpl" with title="Tags" %}
{% if addTagUrl %}
{# The tag field persists each association as it is chosen (the `tagEditor` profile), #}
{# so there is no form, no submit button and no page reload. `selectedItems=tags` is #}
{# what makes the selector treat the entity's existing tags as already chosen: the  #}
{# core drops them from its results instead of offering a tag the entity already has. #}
<div class="mb-6 px-4">
    {% include "/partials/form/autocompleter.tpl" with profile='tagEditor' usage=usage elName='editedId' id=getNextId("tag_autocompleter") selectedItems=tags entityId=id addUrl=addTagUrl removeUrl=removeTagUrl title='Add Tag' standalone=true %}
</div>
{% else %}
{% for tag in tags %}
    {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
{% endfor %}
{% endif %}
