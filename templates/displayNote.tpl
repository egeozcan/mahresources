{% extends "layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=note.Description %}

    {% include "/partials/seeAll.tpl" with entities=note.Groups subtitle="Groups" formAction="/groups" formID=note.ID formParamName="notes" entityName="group" %}
    {% include "/partials/seeAll.tpl" with entities=note.Resources subtitle="Resources" formAction="/resources" formID=note.ID formParamName="notes" entityName="resource" %}

{% endblock %}

{% block sidebar %}
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in note.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}