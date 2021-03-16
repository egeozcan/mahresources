{% extends "base.tpl" %}

{% block body %}
<form method="post" action="/v1/resource">
    <input type="text" name="resource">
    <input type="submit">
</form>
{% endblock %}