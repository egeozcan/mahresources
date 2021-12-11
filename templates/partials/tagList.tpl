{% include "/partials/sideTitle.tpl" with title="Tags" %}
{% for tag in tags %}
    {% include "/partials/tag.tpl" with name=tag.Name ID=tag.ID %}
{% endfor %}