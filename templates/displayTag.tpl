{% extends "layouts/base.tpl" %}

{% block body %}
    {% include "/partials/seeAll.tpl" with entities=tag.Notes subtitle="Notes" formAction="/notes" formID=tag.ID formParamName="tags" entityName="note" %}
    {% include "/partials/seeAll.tpl" with entities=tag.Groups subtitle="Groups" formAction="/groups" formID=tag.ID formParamName="tags" entityName="group" %}
{% endblock %}

{% block sidebar %}

{% endblock %}