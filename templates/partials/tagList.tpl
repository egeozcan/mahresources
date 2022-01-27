{% include "/partials/sideTitle.tpl" with title="Tags" %}
{% for tag in tags %}
    {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
{% endfor %}

{% if addTagUrl %}
<form x-cloak x-data class="mb-6 px-4" method="post" :action="'{{ addTagUrl }}?redirect=' + encodeURIComponent(window.location)">
    <input type="hidden" name="id" value="{{ id }}">
    <div class="flex gap-2 items-start">
        {% include "/partials/form/autocompleter.tpl" with url='/v1/tags' addUrl='/v1/tag' elName='editedId' title='Add Tag' id=getNextId("tag_autocompleter") %}
        <div class="mt-7">{% include "/partials/form/searchButton.tpl" with text="Add&nbsp;Tags" %}</div>
    </div>
</form>
{% endif %}