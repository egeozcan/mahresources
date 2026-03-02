{% extends "/layouts/base.tpl" %}
{% block head %}
    <title>{{ pluginPageTitle }} - mahresources</title>
{% endblock %}
{% block body %}
    {% if pluginError %}
    <div class="bg-red-50 border border-red-200 rounded p-4 mb-4" role="alert">
        <h2 class="text-red-800 font-semibold mb-1">Plugin Error</h2>
        <p class="text-red-700 text-sm">{{ pluginError }}</p>
    </div>
    {% else %}
    {{ pluginContent | safe }}
    {% endif %}
{% endblock %}
