{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=tag.Description preview=false %}

    <div class="meta-strip">
        <div class="meta-strip-item">
            <span class="meta-strip-label">Notes</span>
            <span class="meta-strip-value">{{ tag.Notes|length }}</span>
        </div>
        <div class="meta-strip-item">
            <span class="meta-strip-label">Groups</span>
            <span class="meta-strip-value">{{ tag.Groups|length }}</span>
        </div>
        <div class="meta-strip-item">
            <span class="meta-strip-label">Resources</span>
            <span class="meta-strip-value">{{ tag.Resources|length }}</span>
        </div>
    </div>

    {% include "/partials/seeAll.tpl" with entities=tag.Notes subtitle="Notes" formAction="/notes" formID=tag.ID formParamName="tags" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Groups subtitle="Groups" formAction="/groups" formID=tag.ID formParamName="tags" templateName="group" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Resources subtitle="Resources" formAction="/resources" formID=tag.ID formParamName="tags" templateName="resource" %}
{% endblock %}

{% block sidebar %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=tag.Meta %}
    </div>

    <div class="sidebar-group">
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
    </div>
{% endblock %}
