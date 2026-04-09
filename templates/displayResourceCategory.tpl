{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resourceCategory.Description descriptionEntity=resourceCategory descriptionEditUrl="/v1/resourceCategory/editDescription" descriptionEditId=resourceCategory.ID preview=false %}

    {% if resources %}
    <div class="meta-strip">
        <div class="meta-strip-item">
            <span class="meta-strip-label">Resources</span>
            <span class="meta-strip-value">{{ resources|length }}</span>
        </div>
    </div>
    {% endif %}

    {% include "/partials/seeAll.tpl" with entities=resources subtitle="Resources" formAction="/resources" formID=resourceCategory.ID formParamName="ResourceCategoryId" templateName="resource" %}
{% endblock %}

{% block sidebar %}

{% endblock %}
