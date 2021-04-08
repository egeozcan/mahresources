{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for resource in resources %}
        <a href="/resource/{{ resource.ID }}">
            <div class="resource">
                <h3>{{ resource.Name }}</h3>
                {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
                <img src="data:{{ resource.PreviewContentType }};base64,{{ resource.Preview|base64 }}" alt="{{ resource.Name }} preview">
                {% endif %}
            </div>
        </a>
    {% endfor %}
{% endblock %}