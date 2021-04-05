{% extends "layouts/base.tpl" %}

{% block body %}
<h1>Create a new album</h1>
<form method="post" action="/v1/album?redirect=%2Falbums" x-data="{ preview: '' }">
    <input type="hidden" :value="preview" name="Preview">
    <input type="hidden" :value="preview ? 'image/png' : ''" name="PreviewContentType">
    <p>
        Name
        <input type="text" name="Name" required>
    </p>
    <p>
        Preview
        <input @change="generatePreviewFromFile($event).then(val => preview = val).catch(console.error)" type="file">
    </p>
    <p>
        <input type="submit">
    </p>
</form>
{% endblock %}