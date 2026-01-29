{% extends "/shared/base.tpl" %}

{% block content %}
<article class="bg-white rounded-lg shadow-sm p-6">
    <header class="mb-6">
        <h1 class="text-2xl font-bold text-gray-900">{{ note.Name }}</h1>
        {% if note.Description %}
        <div class="mt-4 prose prose-sm max-w-none text-gray-600">
            {{ note.Description|safe }}
        </div>
        {% endif %}
    </header>

    {% if blocks && blocks|length > 0 %}
    <div class="space-y-4" x-data="{ shareToken: '{{ shareToken }}' }">
        {% for block in blocks %}
            {% include "/partials/blocks/sharedBlock.tpl" %}
        {% endfor %}
    </div>
    {% endif %}
</article>

<footer class="mt-8 text-center text-sm text-gray-500">
    Shared via Mahresources
</footer>
{% endblock %}
