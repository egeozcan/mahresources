{% extends "/layouts/base.tpl" %}

{% block settings %}
<label class="flex justify-between items-center content-center">
    Show Descriptions
    <input type="checkbox" name="showDescriptions" x-data x-init="$store.savedSetting.registerEl($root)" />
</label>
{% endblock %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorResource.tpl" %}
{% endblock %}

{% block body %}
    <section class="list-container">
        {% for entity in resources %}
            {% include "/partials/resource.tpl" with selectable=true %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}