{% extends "/layouts/gallery.tpl" %}

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