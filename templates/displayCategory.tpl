{% extends "/layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=category.Description descriptionEntity=category descriptionEditUrl="/v1/category/editDescription" descriptionEditId=category.ID preview=false %}

    {% if groupsInUse %}
    {# groupsInUse is a count query. category.Groups is the preloaded            #}
    {# association, capped at 50, so this strip used to report "50" for any      #}
    {# category with more than fifty groups — and the delete confirm below reads #}
    {# the same number (finding 98).                                            #}
    <div class="meta-strip">
        <div class="meta-strip-item">
            <span class="meta-strip-label">Groups</span>
            <span class="meta-strip-value">{{ groupsInUse }}</span>
        </div>
    </div>
    {% endif %}

    {% include "/partials/seeAll.tpl" with entities=category.Groups subtitle="Groups" formAction="/groups" addAction="/group/new" formID=category.ID formParamName="categories" addFormParamName="categoryId" templateName="group" %}
{% endblock %}

{% block sidebar %}

{% endblock %}