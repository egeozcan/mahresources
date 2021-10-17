{% extends "layouts/base.tpl" %}

{% block body %}
    {% include "/partials/description.tpl" with description=resource.Description %}

    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/resources" formID=resource.ID formParamName="resources" entityName="note" %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/resources" formID=resource.ID formParamName="resources" entityName="group" %}
{% endblock %}

{% block sidebar %}
    <a href="/files/{{ resource.Location }}">
        {% if resource.PreviewContentType != "" && len(resource.Preview) != 0 %}
            <img src="data:{{ resource.PreviewContentType }};base64,{{ resource.Preview|base64 }}" alt="">
        {% else %}
            <img src="/public/placeholders/file.jpg" alt="">
        {% endif %}
    </a>
    {% include "/partials/sideTitle.tpl" with title="Tags" %}
    <div>
        {% for tag in resource.Tags %}
            {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
        {% endfor %}
    </div>
{% endblock %}