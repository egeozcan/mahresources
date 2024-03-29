{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=category.Description preview=false %}

    {% include "/partials/seeAll.tpl" with entities=category.Groups subtitle="Groups" formAction="/groups" addAction="/group/new" formID=category.ID formParamName="categories" addFormParamName="categoryId" templateName="group" %}
{% endblock %}

{% block sidebar %}

{% endblock %}