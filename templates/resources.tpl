{% extends "layouts/gallery.tpl" %}

{% block gallery %}
    {% for resource in resources %}
        <a href="/resource/{{ resource.ID }}">
            <div class="album">
                <h3>{{ resource.Name }}</h3>
                {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
                <img src="data:{{ album.PreviewContentType }};base64,{{ album.Preview|base64 }}" alt="{{ resource.Name }} preview">
                {% endif %}
            </div>
        </a>
    {% endfor %}
{% endblock %}