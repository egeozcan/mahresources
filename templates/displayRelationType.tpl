{% extends "/layouts/base.tpl" %}

{% block body %}

{% include "/partials/subtitle.tpl" with title=relationType.Name alternativeTitle="RelationType" %}
{% include "/partials/description.tpl" with description=relationType.Description descriptionEntity=relationType descriptionEditUrl="/v1/relationType/editDescription" descriptionEditId=relationType.ID %}

<div class="detail-panel">
    <div class="detail-panel-header">
        <h3 class="detail-panel-title">Category Flow</h3>
    </div>
    <div class="detail-panel-body">
        <div class="flex content-center gap-4 items-center">
            <a href="/category?id={{ relationType.FromCategory.ID }}">{{ relationType.FromCategory.Name }}</a>
            <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12l-18 12v-24z"/></svg>
            <a href="/category?id={{ relationType.ToCategory.ID }}">{{ relationType.ToCategory.Name }}</a>
        </div>

        {% if relationType.BackRelation %}
        <div class="mt-3">
            <strong>Reverse: </strong>
            <div class="inline-flex content-center gap-4 items-center">
                <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24" aria-hidden="true"><path d="M21 12l-18 12v-24z"/></svg>
                <a href="/relationType?id={{ relationType.BackRelation.ID }}">{{ relationType.BackRelation.Name }}</a>
            </div>
        </div>
        {% endif %}
    </div>
</div>

{% endblock %}

{% block sidebar %}
{% endblock %}
