{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=tag.Description preview=false %}
    {% include "/partials/seeAll.tpl" with entities=tag.Notes subtitle="Notes" formAction="/notes" formID=tag.ID formParamName="tags" templateName="note" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Groups subtitle="Groups" formAction="/groups" formID=tag.ID formParamName="tags" templateName="group" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Resources subtitle="Resources" formAction="/resources" formID=tag.ID formParamName="tags" templateName="resource" %}
{% endblock %}

{% block sidebar %}

{% endblock %}