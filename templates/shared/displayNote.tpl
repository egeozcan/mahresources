{% extends "/shared/base.tpl" %}

{% block content %}
<article class="bg-white rounded-lg shadow-sm p-6">
    <header class="mb-6">
        <h1 class="text-2xl font-bold text-stone-900 font-mono">{{ note.Name }}</h1>
        {# Show the description unless a text block already mirrors it (keeps it visible for notes that have only non-text blocks). #}
        {% if note.Description && !note.HasTextBlock %}
        <div class="mt-4 prose prose-sm max-w-none text-stone-600 font-sans">
            {{ note.Description|markdown2|safe }}
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

<footer class="mt-8 text-center text-sm text-stone-500">
    Shared via Mahresources
</footer>
{% endblock %}
