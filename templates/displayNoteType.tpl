{% extends "/layouts/base.tpl" %}

{% block body %}

{# Name is shown once via the title-bar h1 (mainEntity/mainEntityType set by the provider), matching displayCategory.tpl #}
{% include "/partials/description.tpl" with description=noteType.Description descriptionEntity=noteType descriptionEditUrl="/v1/noteType/editDescription" descriptionEditId=noteType.ID %}

<div class="meta-strip">
    {% if notes %}
    <div class="meta-strip-item">
        <span class="meta-strip-label">Notes</span>
        <span class="meta-strip-value">{{ notesTotal }}</span>
    </div>
    {% endif %}
    <div class="meta-strip-item">
        <span class="meta-strip-label">Schema</span>
        <span class="meta-strip-value">{% if noteType.MetaSchema %}Defined{% else %}None{% endif %}</span>
    </div>
    <div class="meta-strip-item">
        <span class="meta-strip-label">Sections</span>
        <span class="meta-strip-value">{% if noteType.SectionConfig %}Custom{% else %}Default{% endif %}</span>
    </div>
    {% if noteType.CustomHeader or noteType.CustomSidebar or noteType.CustomSummary or noteType.CustomAvatar or noteType.CustomMRQLResult or noteType.CustomCSS %}
    <div class="meta-strip-item">
        <span class="meta-strip-label">Custom templates</span>
        <span class="meta-strip-value">{% if noteType.CustomHeader %}Header {% endif %}{% if noteType.CustomSidebar %}Sidebar {% endif %}{% if noteType.CustomSummary %}Summary {% endif %}{% if noteType.CustomAvatar %}Avatar {% endif %}{% if noteType.CustomMRQLResult %}MRQL {% endif %}{% if noteType.CustomCSS %}CSS{% endif %}</span>
    </div>
    {% endif %}
</div>

{% include "/partials/seeAll.tpl" with entities=notes subtitle="Notes" formAction="/notes" formID=noteType.ID formParamName="NoteTypeId" templateName="note" %}

{% endblock %}

{% block sidebar %}
{% endblock %}