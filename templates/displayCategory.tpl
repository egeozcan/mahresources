{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=category.Description descriptionEditUrl="/v1/category/editDescription" descriptionEditId=category.ID preview=false %}

    {% if category.Groups %}
    <div class="meta-strip">
        <div class="meta-strip-item">
            <span class="meta-strip-label">Groups</span>
            <span class="meta-strip-value">{{ category.Groups|length }}</span>
        </div>
    </div>
    {% endif %}

    {% include "/partials/seeAll.tpl" with entities=category.Groups subtitle="Groups" formAction="/groups" addAction="/group/new" formID=category.ID formParamName="categories" addFormParamName="categoryId" templateName="group" %}
{% endblock %}

{% block sidebar %}

{% endblock %}