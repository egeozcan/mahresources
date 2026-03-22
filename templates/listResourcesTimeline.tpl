{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
{% endblock %}

{% block body %}
    {% include "/partials/timeline.tpl" with entityApiUrl="/v1/resources" entityType="resources" entityDefaultView="/resources" %}
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}
