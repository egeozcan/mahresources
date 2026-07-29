{% include "/partials/sideTitle.tpl" with title="Tags" %}
{% for tag in tags %}
    {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
{% endfor %}

{% if addTagUrl %}
{# Findings 56/91: this is a plain POST to /v1/…/addTags with a ?redirect=, so #}
{# submitting it empty navigated the whole page to the API URL and answered 400. #}
{% with hintId=getNextId("add_tags_hint") %}
<form x-cloak x-data="selectionRequired({ field: 'editedId' })" x-bind="events" class="mb-6 px-4" method="post" :action="'{{ addTagUrl }}?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)">
    <input type="hidden" name="id" value="{{ id }}">
    <div class="flex gap-2 items-start">
        {% include "/partials/form/autocompleter.tpl" with profile='tag' usage=usage elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
        <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add&nbsp;Tags" disabledWhen="!hasSelection" describedBy=hintId %}</div>
    </div>
    <p x-show="!hasSelection" id="{{ hintId }}" class="mt-1 text-xs text-stone-600">Choose at least one tag first.</p>
</form>
{% endwith %}
{% endif %}
