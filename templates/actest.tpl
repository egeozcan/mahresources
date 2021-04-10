{% extends "layouts/base.tpl" %}

{% block body %}
    <form>
        {% include "./partials/form/autocompleter.tpl" with url='/v1/tags' elName='tags' title='Select a tag' selectedTags=selectedTags id="autocompleter"|nanoid %}
        <input type="submit">
    </form>
{% endblock %}