{% extends "/layouts/base.tpl" %}

{% block body %}

{# Name is shown once via the title-bar h1 (mainEntity/mainEntityType set by the provider), matching displayCategory.tpl #}
{% include "/partials/description.tpl" with description=noteType.Description descriptionEntity=noteType descriptionEditUrl="/v1/noteType/editDescription" descriptionEditId=noteType.ID %}

{% if notes %}
<div class="meta-strip">
    <div class="meta-strip-item">
        <span class="meta-strip-label">Notes</span>
        <span class="meta-strip-value">{{ notesTotal }}</span>
    </div>
</div>
{% endif %}

{% include "/partials/seeAll.tpl" with entities=notes subtitle="Notes" formAction="/notes" formID=noteType.ID formParamName="NoteTypeId" templateName="note" %}

{% endblock %}

{% block sidebar %}
{% endblock %}