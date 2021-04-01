{% extends "layouts/base.tpl" %}

{% block body %}
<form method="post" action="/v1/resource" enctype="multipart/form-data">
    <input type="file" name="resource">
    <input type="submit">
</form>
{% endblock %}