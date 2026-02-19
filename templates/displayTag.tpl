{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=tag.Description preview=false %}
    {% include "/partials/seeAll.tpl" with entities=tag.Notes subtitle="Notes" formAction="/notes" formID=tag.ID formParamName="tags" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Groups subtitle="Groups" formAction="/groups" formID=tag.ID formParamName="tags" templateName="group" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Resources subtitle="Resources" formAction="/resources" formID=tag.ID formParamName="tags" templateName="resource" %}
{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=tag.Meta %}

    <form
        x-data="confirmAction({ message: `Selected tags will be deleted and merged to {{ tag.Name|json }}. Are you sure?` })"
        action="/v1/tags/merge"
        :action="'/v1/tags/merge?redirect=' + encodeURIComponent(window.location)"
        method="post"
        x-bind="events"
    >
        <input type="hidden" name="winner" value="{{ tag.ID }}">
        <p>Merge others with this tag?</p>
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' elName='losers' title='Tags To Merge' id=getNextId("autocompleter") %}
        <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
    </form>
{% endblock %}