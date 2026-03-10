{% if name %}
    <a href="{% if link %}{{ link }}{% elif ID %}/category?id={{ ID }}{% endif %}">
        <div class="
            text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1 font-mono
                {% if active %}bg-amber-100 text-amber-700 rounded-full
                {% else %} rounded-full bg-white text-stone-700 border
                {% endif %}">
            {{ name }}
        </div>
    {% if link %}</a>{% endif %}
{% endif %}