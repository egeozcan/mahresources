{% extends "layouts/base.tpl" %}

{% block body %}

    {% include "/partials/description.tpl" with description=group.Description %}

    {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" formID=group.ID formParamName="groups" entityName="note" %}
    {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" formID=group.ID formParamName="groups" entityName="resource" %}
    {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" formID=group.ID formParamName="groups" entityName="note" %}
    {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" formID=group.ID formParamName="groups" entityName="resource" %}

{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in group.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}