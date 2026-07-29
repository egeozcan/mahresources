{% extends "/layouts/gallery.tpl" %}

{% block head %}{% custom_css resources %}{% endblock %}

{% block top %}
    <div class="my-4">{% include "/partials/boxSelect.tpl" with options=displayOptions %}</div>
    {% include "/partials/customListHeader.tpl" %}
    {% include "/partials/mrqlBar.tpl" with entity="resource" %}
{% endblock %}
{% block gallery %}
    {% for entity in resources %}
        {% include "/partials/resource.tpl" %}
    {% empty %}
        {% include "/partials/listEmpty.tpl" with label="resources" createUrl="/resource/new" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
<script>
    document.body.classList.add("simple")
</script>
{% endblock %}