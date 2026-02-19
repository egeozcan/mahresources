{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorResource.tpl" %}
{% endblock %}

{% block body %}
    <section class="list-container"{% if owner && owner|length == 1 %} data-paste-context='{"type":"group","id":{{ owner.0.ID }},"name":"{{ owner.0.Name|escapejs }}"}'{% endif %}>
        {% for entity in resources %}
            {% include "/partials/resource.tpl" with selectable=true %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}