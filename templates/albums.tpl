{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    <form>
        {% for queryParam, values in queryValues %}
            {% if queryParam != "Name" && queryParam != "Page" %}
                {% for value in values %}
                    <input type="hidden" name="{{ queryParam }}" value="{{ value }}">
                {% endfor %}
            {% endif %}
        {% endfor %}
        <input type="text" name="Name" value="{{ queryValues.Name.0 }}">
        <input type="submit">
    </form>
    <p>{{ query }}</p>
    {% include "./partials/albumList.tpl" %}
{% endblock %}