{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for resource in resources %}
        {% include "./partials/resource.tpl" %}
    {% endfor %}
{% endblock %}