{% extends "/layouts/gallery.tpl" %}

{% block head %}{% custom_css resources %}{% endblock %}

{% block top %}
    <div class="my-4">{% include "/partials/boxSelect.tpl" with options=displayOptions %}</div>
{% endblock %}
{% block gallery %}
    {% for entity in resources %}
        {% include "/partials/resource.tpl" %}
    {% endfor %}
{% endblock %}

{% block sidebar %}
<script>
    document.body.classList.add("simple")
</script>
{% endblock %}