{% extends "/layouts/base.tpl" %}

{% block prebody %}
    {% include "/partials/boxSelect.tpl" with options=displayOptions %}
    {% include "/partials/bulkEditorResource.tpl" %}
{% endblock %}

{% block body %}
    <section class="note-container">
        {% for entity in resources %}
            {% include "/partials/resource.tpl" with selectable=true %}
        {% endfor %}
    </section>
{% endblock %}

{% block sidebar %}
    {% include "/partials/form/searchFormResource.tpl" %}
{% endblock %}