{% extends "/layouts/base.tpl" %}

{% block body %}

{% include "/partials/subtitle.tpl" with title=relationType.Name alternativeTitle="RelationType" %}
{% include "/partials/description.tpl" with description=relationType.Description %}

<hr class="my-4">

<div class="flex content-center gap-4 items-center">
    <a href="/category?id={{ relationType.FromCategory.ID }}">{{ relationType.FromCategory.Name }}</a>
    <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
    <a href="/category?id={{ relationType.ToCategory.ID }}">{{ relationType.ToCategory.Name }}</a>
</div>

{% if relationType.BackRelation %}
<div class="mt-3">
    <strong>Reverse: </strong>
    <div class="inline-flex content-center gap-4 items-center">
        <svg xmlns="http://www.w3.org/2000/svg" width="9" height="9" viewBox="0 0 24 24"><path d="M21 12l-18 12v-24z"/></svg>
        <a href="/relationType?id={{ relationType.BackRelation.ID }}">{{ relationType.BackRelation.Name }}</a>
    </div>
</div>
{% endif %}

{% endblock %}

{% block sidebar %}
{% endblock %}