{% extends "/layouts/base.tpl" %}

{% block body %}

    {% include "/partials/subtitle.tpl" with title=relation.Name alternativeTitle="Relation" %}
    {% include "/partials/description.tpl" with description=relation.Description descriptionEditUrl="/v1/relation/editDescription" descriptionEditId=relation.ID %}

    <div class="detail-panel">
        <div class="detail-panel-body">
            <div class="flex flex-wrap gap-4 items-start">
                <div class="flex-1 min-w-[200px]">
                    <h3 class="sidebar-group-title">From Group</h3>
                    {% include "partials/group.tpl" with entity=groupFrom relation=relation noDescription=true noRelDescription=true noTag=true tagBaseUrl="/groups" %}
                </div>
                <div class="flex items-center justify-center py-8 text-stone-400 flex-shrink-0">
                    <svg aria-hidden="true" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>
                </div>
                <div class="flex-1 min-w-[200px]">
                    <h3 class="sidebar-group-title">To Group</h3>
                    {% include "partials/group.tpl" with entity=groupTo relation=relation reverse=true noDescription=true noRelDescription=true noTag=true tagBaseUrl="/groups" %}
                </div>
            </div>
        </div>
    </div>

{% endblock %}

{% block sidebar %}
{% endblock %}
