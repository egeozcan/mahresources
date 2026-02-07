{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resourceCategory.Description preview=false %}

    {% include "/partials/seeAll.tpl" with entities=resources subtitle="Resources" formAction="/resources" formID=resourceCategory.ID formParamName="ResourceCategoryId" templateName="resource" %}
{% endblock %}

{% block sidebar %}

{% endblock %}
