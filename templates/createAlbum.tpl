{% extends "base.tpl" %}

{% block body %}
<form method="post" action="/v1/album">
    <input type="text" name="name" required>
    <input type="submit">
</form>
{% endblock %}