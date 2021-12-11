{% extends "/layouts/base.tpl" %}

{% block body %}

    {% include "/partials/subtitle.tpl" with title=relation.Name alternativeTitle="Relation" %}
    {% include "/partials/description.tpl" with description=relation.Description %}

    <hr class="my-4">

    {% include "partials/subtitle.tpl" with small=true title="From Group" %}
    {% include "partials/group.tpl" with entity=groupFrom relation=relation noDescription=true noRelDescription=true noTag=true %}

    <hr class="my-4">

    {% include "partials/subtitle.tpl" with small=true title="To Group" %}
    {% include "partials/group.tpl" with entity=groupTo relation=relation reverse=true noDescription=true noRelDescription=true noTag=true %}

{% endblock %}

{% block sidebar %}
{% endblock %}