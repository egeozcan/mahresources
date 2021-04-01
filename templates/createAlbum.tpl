{% extends "layouts/base.tpl" %}

{% block body %}
<form method="post" action="/v1/album" x-data="{ preview: '' }">
    <input type="text" :value="preview" name="Preview">
    <input type="hidden" value="image/png" name="PreviewContentType">
    <input type="text" name="Name" required>
    <input @change="generatePreviewFromFile($event).then(val => preview = val).catch(console.error)" type="file">
    <input type="submit">
</form>
{% endblock %}