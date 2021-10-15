{% if name %}
    {% if link %}<a href="{{ link }}">{% endif %}
        <div class="
            text-xs inline-flex items-center font-bold leading-sm uppercase px-3 py-1
                {% if active %}bg-green-200 text-green-700 rounded-full
                {% else %} rounded-full bg-white text-gray-700 border
                {% endif %}">
            {{ name }}
        </div>
    {% if link %}</a>{% endif %}
{% endif %}